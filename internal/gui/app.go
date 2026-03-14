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

// appState holds all GUI state
type appState struct {
	window *app.Window
	theme  *material.Theme
	mu     sync.Mutex

	// Path inputs
	sourceEditor widget.Editor
	destEditor   widget.Editor
	sourceBrowse widget.Clickable
	destBrowse   widget.Clickable

	// Mode
	modeEnum widget.Enum

	// Comparison method
	compEnum widget.Enum

	// Conflict resolution
	conflictEnum widget.Enum

	// Options
	dryRunCheck     widget.Bool
	deleteCheck     widget.Bool
	createDestCheck widget.Bool
	statefulCheck   widget.Bool

	// Workers
	workersEditor widget.Editor

	// Excludes
	excludeEditor widget.Editor

	// Action buttons
	syncBtn    widget.Clickable
	compareBtn widget.Clickable
	cancelBtn  widget.Clickable

	// Log panel
	logList    widget.List
	logEntries []LogEntry
	logVisible bool
	toggleLog  widget.Clickable

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
// This is the public entry point called from the CLI command.
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
		app.Size(unit.Dp(600), unit.Dp(700)),
		app.MinSize(unit.Dp(520), unit.Dp(500)),
	)
	a.theme = newTheme()

	// Set defaults
	a.modeEnum.Value = "oneway"
	a.compEnum.Value = "hash"
	a.conflictEnum.Value = "newer"

	a.workersEditor.SingleLine = true
	a.workersEditor.SetText("5")

	a.sourceEditor.SingleLine = true
	a.destEditor.SingleLine = true

	a.excludeEditor.SingleLine = true
	a.excludeEditor.SetText("*.tmp, .git/, node_modules/")

	a.logList.List.Axis = layout.Vertical
	a.logList.List.ScrollToEnd = true

	a.statusMsg = "Ready"
	a.statusLevel = "info"
}

// consumeEvents reads UIEvents from the channel and updates state.
// Runs in its own goroutine.
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

	// Toggle log panel
	if a.toggleLog.Clicked(gtx) {
		a.logVisible = !a.logVisible
		if a.logVisible {
			a.window.Option(app.Size(unit.Dp(1100), unit.Dp(700)))
		} else {
			a.window.Option(app.Size(unit.Dp(600), unit.Dp(700)))
		}
	}
}

func (a *appState) browseDirectory(editor *widget.Editor) {
	dir, err := zenity.SelectFile(
		zenity.Directory(),
		zenity.Title("Select directory"),
	)
	if err != nil {
		return // user cancelled or zenity not available
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

	opts := RunOptions{
		Source:     a.sourceEditor.Text(),
		Dest:       a.destEditor.Text(),
		Mode:       a.modeEnum.Value,
		Comparison: a.compEnum.Value,
		Conflict:   a.conflictEnum.Value,
		Workers:    workers,
		BufferSize: 65536,
		Excludes:   a.excludeEditor.Text(),
		DryRun:     dryRun || a.dryRunCheck.Value,
		Delete:     a.deleteCheck.Value,
		CreateDest: a.createDestCheck.Value,
		Stateful:   a.statefulCheck.Value,
	}

	// Validate paths
	if opts.Source == "" || opts.Dest == "" {
		a.statusMsg = "Source and destination paths are required"
		a.statusLevel = "error"
		return
	}
	if _, err := os.Stat(opts.Source); os.IsNotExist(err) {
		a.statusMsg = fmt.Sprintf("Source path does not exist: %s", opts.Source)
		a.statusLevel = "error"
		return
	}
	if _, err := os.Stat(opts.Dest); os.IsNotExist(err) {
		if opts.CreateDest {
			if mkErr := os.MkdirAll(opts.Dest, 0755); mkErr != nil {
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
