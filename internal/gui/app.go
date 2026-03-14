//go:build !nogui

package gui

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
)

// tabDef defines a tab in the right panel.
type tabDef struct {
	Title string
}

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

	// Right panel tabs
	tabs       []tabDef
	activeTab  int
	tabClicks  [8]widget.Clickable // max 8 tabs

	// Log tab
	logList    widget.List
	logEntries []LogEntry

	// Progress
	progress ProgressState

	// Running state
	isRunning bool
	cancelFn  context.CancelFunc

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
	a := &appState{
		events: make(chan UIEvent, 500),
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

	a.logList.List.Axis = layout.Vertical
	a.logList.List.ScrollToEnd = true

	a.statusMsg = "Ready"
	a.statusLevel = "info"
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
			}
		case eventComplete:
			a.isRunning = false
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
			a.isRunning = false
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

	// Sync button
	if a.syncBtn.Clicked(gtx) && !a.isRunning {
		a.startOperation(false)
	}

	// Compare button
	if a.compareBtn.Clicked(gtx) && !a.isRunning {
		a.startOperation(true)
	}

	// Cancel button
	if a.cancelBtn.Clicked(gtx) && a.isRunning && a.cancelFn != nil {
		a.cancelFn()
		a.statusMsg = "Cancelling..."
		a.statusLevel = "info"
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

func (a *appState) startOperation(dryRun bool) {
	workers, err := strconv.Atoi(a.workersEditor.Text())
	if err != nil || workers < 1 {
		workers = 5
	}

	source := a.sourceEditor.Text()
	dest := a.destEditor.Text()

	opts := RunOptions{
		Source:     source,
		Dest:       dest,
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

	// Validate paths
	if source == "" || dest == "" {
		a.statusMsg = "Source and destination paths are required"
		a.statusLevel = "error"
		return
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		a.statusMsg = fmt.Sprintf("Source path does not exist: %s", source)
		a.statusLevel = "error"
		return
	}
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		if opts.CreateDest {
			if mkErr := os.MkdirAll(dest, 0755); mkErr != nil {
				a.statusMsg = fmt.Sprintf("Failed to create destination: %v", mkErr)
				a.statusLevel = "error"
				return
			}
		} else {
			a.statusMsg = "Destination does not exist (enable 'Create destination')"
			a.statusLevel = "error"
			return
		}
	}

	// Add paths to history and save settings
	a.settings.AddSourcePath(source)
	a.settings.AddDestPath(dest)
	a.saveSettings()

	// Close history dropdowns
	a.sourceHistoryOpen = false
	a.destHistoryOpen = false

	// Reset state
	a.logEntries = nil
	a.progress = ProgressState{}
	a.report = nil
	a.isRunning = true
	a.statusLevel = "info"
	if dryRun {
		a.statusMsg = "Comparing..."
	} else {
		a.statusMsg = "Syncing..."
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel

	go runSync(ctx, opts, a.events)
}
