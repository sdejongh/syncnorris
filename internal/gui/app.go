//go:build !nogui

package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ncruces/zenity"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/sync/job"
)

// tabDef defines a tab in the right panel.
type tabDef struct {
	Title string
}

// runState represents the high-level state of the GUI.
type runState int

const (
	stateIdle    runState = iota
	stateRunning          // a sync/compare is in progress
	statePaused           // a job was paused or the app crashed with a running job
)

// appState holds all GUI state
type appState struct {
	window *app.Window
	theme  *material.Theme
	mu     sync.Mutex

	// Settings
	settings *Settings

	// Path inputs
	sourceEditor widget.Editor
	destEditor   widget.Editor
	sourceBrowse widget.Clickable
	destBrowse   widget.Clickable

	// Path history dropdowns
	sourceHistoryBtn    widget.Clickable
	destHistoryBtn      widget.Clickable
	sourceHistoryOpen   bool
	destHistoryOpen     bool
	sourceHistoryClicks [maxHistory]widget.Clickable
	destHistoryClicks   [maxHistory]widget.Clickable

	// Comparison method
	compEnum widget.Enum

	// Options
	dryRunCheck     widget.Bool
	deleteCheck     widget.Bool
	createDestCheck widget.Bool

	// Workers
	workersEditor widget.Editor

	// Excludes
	excludeEditor widget.Editor

	// Action buttons
	syncBtn    widget.Clickable
	compareBtn widget.Clickable
	cancelBtn  widget.Clickable

	// Pause/Resume/Abandon buttons (rendered by Task 13; declared now)
	pauseSoftBtn     widget.Clickable
	pauseHardBtn     widget.Clickable
	resumeBtn        widget.Clickable
	abandonBtn       widget.Clickable
	bannerResumeBtn  widget.Clickable
	bannerAbandonBtn widget.Clickable

	// Right panel tabs
	tabs       []tabDef
	activeTab  int
	tabClicks  [8]widget.Clickable // max 8 tabs

	// Log tab
	logList    widget.List
	logEntries []LogEntry

	// Progress
	progress  ProgressState
	bandwidth *BandwidthTracker

	// High-level state machine (replaces isRunning bool)
	runState runState
	cancelFn context.CancelFunc

	// Instance lock (nil until acquired in Task 14)
	instanceLock *InstanceLock

	// Job management
	jobStore  *job.JobStore
	activeJob *job.Job
	heartbeat *job.Heartbeat
	pauseSoft chan struct{}
	pauseHard chan struct{}

	// Events channel
	events chan UIEvent

	// Status message
	statusMsg   string
	statusLevel string // "info", "error", "success"

	// Report
	report *models.SyncReport
}

// Run starts the GUI window and event loop.
func Run() error {
	// Enforce single instance before creating any GUI state.
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = os.TempDir()
	}
	lockPath := filepath.Join(cfgDir, "syncnorris", "app.lock")
	lock, err := AcquireInstanceLock(lockPath)
	if err != nil {
		return fmt.Errorf("syncnorris is already running: %w", err)
	}
	defer lock.Release()

	a := &appState{
		events:       make(chan UIEvent, 500),
		instanceLock: lock,
	}
	a.init()

	go a.consumeEvents()

	var ops op.Ops
	for {
		event := a.window.Event()
		a.mu.Lock()
		switch e := event.(type) {
		case app.DestroyEvent:
			a.mu.Unlock()
			if a.cancelFn != nil {
				a.cancelFn()
			}
			a.saveSettings()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.handleEvents(gtx)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
		a.mu.Unlock()
	}
}

func (a *appState) init() {
	a.window = new(app.Window)
	a.window.Option(
		app.Title("SyncNorris"),
		app.Size(fullWindowW, windowHeight),
		app.MinSize(fullWindowW, windowHeight),
		app.MaxSize(unit.Dp(1920), windowHeight),
	)
	a.tabs = []tabDef{{Title: "Logs"}}
	a.activeTab = 0
	a.theme = newTheme()

	// Load persisted settings
	a.settings = LoadSettings()
	s := a.settings

	// Apply settings to widgets
	a.compEnum.Value = s.Comparison

	a.workersEditor.SingleLine = true
	a.workersEditor.SetText(strconv.Itoa(s.Workers))

	a.sourceEditor.SingleLine = true
	a.destEditor.SingleLine = true

	a.excludeEditor.SingleLine = true
	a.excludeEditor.SetText(s.Excludes)

	a.dryRunCheck.Value = s.DryRun
	a.deleteCheck.Value = s.Delete
	a.createDestCheck.Value = s.CreateDest

	a.bandwidth = NewBandwidthTracker()

	a.logList.List.Axis = layout.Vertical
	a.logList.List.ScrollToEnd = true

	a.statusMsg = "Ready"
	a.statusLevel = "info"

	// Set up job store and detect interrupted jobs.
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = os.TempDir()
	}
	jobsDir := filepath.Join(cfgDir, "syncnorris", "jobs")
	a.jobStore = job.NewJobStore(jobsDir)

	if interrupted, _ := a.jobStore.Active(); interrupted != nil {
		a.activeJob = interrupted
		a.runState = statePaused
		// Restore widget fields from saved job options
		a.sourceEditor.SetText(interrupted.Source)
		a.destEditor.SetText(interrupted.Destination)
		a.compEnum.Value = interrupted.Options.Comparator
		a.workersEditor.SetText(strconv.Itoa(interrupted.Options.Workers))
		a.excludeEditor.SetText(strings.Join(interrupted.Options.ExcludePatterns, ","))
		a.dryRunCheck.Value = interrupted.Options.DryRun
		a.deleteCheck.Value = interrupted.Options.DeleteOrphans
		a.statusMsg = "Job interrupted — resume or abandon"
		a.statusLevel = "info"
	}
}

// collectSettings reads current widget state into a Settings struct.
func (a *appState) collectSettings() *Settings {
	workers, err := strconv.Atoi(a.workersEditor.Text())
	if err != nil || workers < 1 {
		workers = 5
	}
	return &Settings{
		Mode:          "oneway",
		Comparison:    a.compEnum.Value,
		Conflict:      "newer",
		Workers:       workers,
		Excludes:      a.excludeEditor.Text(),
		DryRun:        a.dryRunCheck.Value,
		Delete:        a.deleteCheck.Value,
		CreateDest:    a.createDestCheck.Value,
		SourceHistory: a.settings.SourceHistory,
		DestHistory:   a.settings.DestHistory,
	}
}

func (a *appState) saveSettings() {
	s := a.collectSettings()
	_ = s.Save() // best effort
}

// setStatus is a convenience helper for updating status in one call.
func (a *appState) setStatus(msg, level string) {
	a.statusMsg = msg
	a.statusLevel = level
}

// consumeEvents reads UIEvents from the channel and updates state.
func (a *appState) consumeEvents() {
	for ev := range a.events {
		a.mu.Lock()
		switch ev.Type {
		case eventLog:
			if ev.Log != nil {
				a.logEntries = append(a.logEntries, *ev.Log)
				if len(a.logEntries) > 5000 {
					a.logEntries = a.logEntries[len(a.logEntries)-4000:]
				}
			}
		case eventProgress:
			if ev.Progress != nil {
				a.progress = *ev.Progress
				a.bandwidth.Update(ev.Progress.BytesTransferred)
			}
		case eventComplete:
			a.cancelFn = nil
			a.report = ev.Report
			a.progress.Fraction = 1
			if ev.Report != nil {
				r := ev.Report
				a.statusMsg = fmt.Sprintf("Complete: %s  |  Copied: %d  Updated: %d  Identical: %d  Errors: %d  (%s)",
					r.Status,
					r.Stats.FilesCopied.Load(),
					r.Stats.FilesUpdated.Load(),
					r.Stats.FilesSynchronized.Load(),
					r.Stats.FilesErrored.Load(),
					r.Duration.Round(time.Millisecond),
				)
				if r.Status == models.StatusSuccess {
					a.statusLevel = "success"
				} else {
					a.statusLevel = "error"
				}
			}
			// Save settings after operation completes
			a.saveSettings()

		case eventError:
			a.cancelFn = nil
			if ev.Error != nil {
				a.statusMsg = fmt.Sprintf("Error: %v", ev.Error)
				a.statusLevel = "error"
				a.logEntries = append(a.logEntries, LogEntry{
					Time:    time.Now(),
					Level:   "ERROR",
					Message: ev.Error.Error(),
				})
			}

		case eventJobFinished:
			// Stop heartbeat first.
			if a.heartbeat != nil {
				a.heartbeat.Stop()
				a.heartbeat = nil
			}
			if a.activeJob != nil {
				if a.pauseWasRequested() {
					// User paused: persist state, stay in statePaused.
					a.runState = statePaused
					_ = a.jobStore.SetStatus(a.activeJob.ID, job.StatusPaused)
				} else {
					// Normal completion or cancellation: clean up the job.
					_ = a.jobStore.Delete(a.activeJob.ID)
					a.activeJob = nil
					a.runState = stateIdle
				}
			} else {
				a.runState = stateIdle
			}
		}
		a.mu.Unlock()
		a.window.Invalidate()
	}
}

// handleEvents processes widget interactions each frame
func (a *appState) handleEvents(gtx layout.Context) {
	// Browse buttons
	if a.sourceBrowse.Clicked(gtx) {
		go a.browseDirectory(&a.sourceEditor)
	}
	if a.destBrowse.Clicked(gtx) {
		go a.browseDirectory(&a.destEditor)
	}

	// Source history toggle
	if a.sourceHistoryBtn.Clicked(gtx) {
		a.sourceHistoryOpen = !a.sourceHistoryOpen
		a.destHistoryOpen = false // close other
	}
	// Source history item clicks
	for i := range a.settings.SourceHistory {
		if i >= maxHistory {
			break
		}
		if a.sourceHistoryClicks[i].Clicked(gtx) {
			a.sourceEditor.SetText(a.settings.SourceHistory[i])
			a.sourceHistoryOpen = false
		}
	}

	// Dest history toggle
	if a.destHistoryBtn.Clicked(gtx) {
		a.destHistoryOpen = !a.destHistoryOpen
		a.sourceHistoryOpen = false // close other
	}
	// Dest history item clicks
	for i := range a.settings.DestHistory {
		if i >= maxHistory {
			break
		}
		if a.destHistoryClicks[i].Clicked(gtx) {
			a.destEditor.SetText(a.settings.DestHistory[i])
			a.destHistoryOpen = false
		}
	}

	// Sync/Compare buttons (only when Idle)
	if a.syncBtn.Clicked(gtx) && a.runState == stateIdle {
		a.startOperation(false)
	}
	if a.compareBtn.Clicked(gtx) && a.runState == stateIdle {
		a.startOperation(true)
	}

	// Cancel button (only when Running)
	if a.cancelBtn.Clicked(gtx) && a.runState == stateRunning && a.cancelFn != nil {
		a.cancelFn()
		a.statusMsg = "Cancelling..."
		a.statusLevel = "info"
	}

	// Pause buttons (only when Running)
	if a.pauseSoftBtn.Clicked(gtx) && a.runState == stateRunning {
		if a.pauseSoft != nil {
			close(a.pauseSoft)
			a.pauseSoft = nil
		}
	}
	if a.pauseHardBtn.Clicked(gtx) && a.runState == stateRunning {
		if a.pauseHard != nil {
			close(a.pauseHard)
			a.pauseHard = nil
		}
	}

	// Resume/Abandon buttons (only when Paused)
	if a.resumeBtn.Clicked(gtx) && a.runState == statePaused {
		a.resumeJob()
	}
	if a.abandonBtn.Clicked(gtx) && a.runState == statePaused {
		a.abandonJob()
	}

	// Banner buttons (available whenever activeJob is set)
	if a.bannerResumeBtn.Clicked(gtx) && a.activeJob != nil {
		a.resumeJob()
	}
	if a.bannerAbandonBtn.Clicked(gtx) && a.activeJob != nil {
		a.abandonJob()
	}

	// Tab clicks
	for i := range a.tabs {
		if i < len(a.tabClicks) && a.tabClicks[i].Clicked(gtx) {
			a.activeTab = i
		}
	}
}

func (a *appState) browseDirectory(editor *widget.Editor) {
	dir, err := zenity.SelectFile(
		zenity.Directory(),
		zenity.Title("Select directory"),
	)
	if err != nil {
		return
	}
	a.mu.Lock()
	editor.SetText(dir)
	a.mu.Unlock()
	a.window.Invalidate()
}

// buildRunOptions assembles a RunOptions from the current widget state.
func (a *appState) buildRunOptions(dryRun bool) RunOptions {
	workers, err := strconv.Atoi(a.workersEditor.Text())
	if err != nil || workers < 1 {
		workers = 5
	}
	return RunOptions{
		Source:     a.sourceEditor.Text(),
		Dest:       a.destEditor.Text(),
		Mode:       "oneway",
		Comparison: a.compEnum.Value,
		Conflict:   "newer",
		Workers:    workers,
		BufferSize: 65536,
		Excludes:   a.excludeEditor.Text(),
		DryRun:     dryRun || a.dryRunCheck.Value,
		Delete:     a.deleteCheck.Value,
		CreateDest: a.createDestCheck.Value,
	}
}

// optionsForJob converts RunOptions into the job.Options subset that is
// persisted to disk for pause/resume purposes.
func (a *appState) optionsForJob(opts RunOptions) job.Options {
	return job.Options{
		Mode:            opts.Mode,
		Comparator:      opts.Comparison,
		Workers:         opts.Workers,
		BufferSize:      opts.BufferSize,
		DeleteOrphans:   opts.Delete,
		DryRun:          opts.DryRun,
		ExcludePatterns: parseExcludes(opts.Excludes),
	}
}

func (a *appState) startOperation(dryRun bool) {
	opts := a.buildRunOptions(dryRun)

	source := opts.Source
	dest := opts.Dest

	// Validate paths
	if source == "" || dest == "" {
		a.setStatus("Source and destination paths are required", "error")
		return
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		a.setStatus(fmt.Sprintf("Source path does not exist: %s", source), "error")
		return
	}
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		if opts.CreateDest {
			if mkErr := os.MkdirAll(dest, 0755); mkErr != nil {
				a.setStatus(fmt.Sprintf("Failed to create destination: %v", mkErr), "error")
				return
			}
		} else {
			a.setStatus("Destination does not exist (enable 'Create destination')", "error")
			return
		}
	}

	// Create persistent job record before launching
	j, err := a.jobStore.Create(source, dest, a.optionsForJob(opts))
	if err != nil {
		a.setStatus("failed to create job: "+err.Error(), "error")
		return
	}
	a.activeJob = j
	a.heartbeat = job.NewHeartbeat(a.jobStore, j.ID, 2*time.Second)
	a.heartbeat.Start()

	// Fresh pause channels for this run
	a.pauseSoft = make(chan struct{})
	a.pauseHard = make(chan struct{})
	opts.Job = j
	opts.JobStore = a.jobStore
	opts.PauseSoft = a.pauseSoft
	opts.PauseHard = a.pauseHard

	// Add paths to history and save settings
	a.settings.AddSourcePath(source)
	a.settings.AddDestPath(dest)
	a.saveSettings()

	// Close history dropdowns
	a.sourceHistoryOpen = false
	a.destHistoryOpen = false

	// Reset display state
	a.logEntries = nil
	a.progress = ProgressState{}
	a.report = nil
	a.bandwidth.Reset()
	a.runState = stateRunning
	a.statusLevel = "info"
	if dryRun {
		a.statusMsg = "Comparing..."
	} else {
		a.statusMsg = "Syncing..."
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel

	go func() {
		runSync(ctx, opts, a.events)
		a.events <- UIEvent{Type: eventJobFinished}
	}()
}

// pauseWasRequested does a non-blocking check on both pause channels to
// determine whether a pause was requested before the goroutine finished.
// The channels are already nil-guarded: if they were closed by the
// handleEvents handler they are set to nil after closing, so we compare
// against nil here to detect that case.
func (a *appState) pauseWasRequested() bool {
	// A closed channel (or nil pointer after close+nil assignment) signals pause.
	if a.pauseSoft == nil || a.pauseHard == nil {
		return true
	}
	select {
	case <-a.pauseSoft:
		return true
	default:
	}
	select {
	case <-a.pauseHard:
		return true
	default:
	}
	return false
}

// resumeJob relaunches the active job with fresh channels and a new heartbeat.
// Must be called from handleEvents (i.e. under a.mu).
func (a *appState) resumeJob() {
	if a.activeJob == nil {
		return
	}
	opts := a.buildRunOptions(a.activeJob.Options.DryRun)
	opts.Job = a.activeJob
	opts.JobStore = a.jobStore

	_ = a.jobStore.SetStatus(a.activeJob.ID, job.StatusRunning)

	a.heartbeat = job.NewHeartbeat(a.jobStore, a.activeJob.ID, 2*time.Second)
	a.heartbeat.Start()

	// Fresh pause channels for the resumed run
	a.pauseSoft = make(chan struct{})
	a.pauseHard = make(chan struct{})
	opts.PauseSoft = a.pauseSoft
	opts.PauseHard = a.pauseHard

	// Reset display
	a.logEntries = nil
	a.progress = ProgressState{}
	a.report = nil
	a.bandwidth.Reset()
	a.runState = stateRunning
	a.setStatus("Resuming...", "info")

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel

	go func() {
		runSync(ctx, opts, a.events)
		a.events <- UIEvent{Type: eventJobFinished}
	}()
}

// abandonJob cleans up partial files in the destination and deletes the job.
// Must be called from handleEvents (i.e. under a.mu).
func (a *appState) abandonJob() {
	if a.activeJob == nil {
		return
	}
	// Best-effort cleanup: remove any .partial files written by atomic copy.
	suffix := fmt.Sprintf(".syncnorris-%s.partial", a.activeJob.ID[:8])
	_ = filepath.Walk(a.activeJob.Destination, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), suffix) {
			_ = os.Remove(path)
		}
		return nil
	})
	_ = a.jobStore.Delete(a.activeJob.ID)
	a.activeJob = nil
	a.runState = stateIdle
	a.setStatus("Job abandoned", "info")
}
