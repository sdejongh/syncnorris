package gui

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
)

// uiEventType identifies the kind of UIEvent
type uiEventType int

const (
	eventLog      uiEventType = iota
	eventProgress
	eventComplete
	eventError
)

// LogEntry represents a single log line in the GUI
type LogEntry struct {
	Time    time.Time
	Level   string // "COPY", "UPDATE", "SKIP", "DELETE", "ERROR", "INFO"
	Message string
}

// RunningStats holds live counters updated during sync
type RunningStats struct {
	Copied  int
	Updated int
	Skipped int
	Errors  int
}

// ProgressState holds current progress for the UI
type ProgressState struct {
	Fraction    float32
	CurrentFile int
	TotalFiles  int
	CurrentPath string
	Stats       RunningStats
}

// UIEvent carries updates from the formatter to the GUI event loop
type UIEvent struct {
	Type     uiEventType
	Log      *LogEntry
	Progress *ProgressState
	Report   *models.SyncReport
	Error    error
}

// fileState tracks the event sequence for a single file.
// The pipeline sends events in a specific order that reveals the action:
//   COPY:   file_start → file_complete  (no compare phase)
//   UPDATE: compare_start → file_start → file_complete  (both phases)
//   SKIP:   compare_start → file_complete  (compare only, no copy)
type fileState struct {
	hadCompare bool
	hadCopy    bool
}

// GUIFormatter implements output.Formatter for the GUI.
// It tracks per-file state to infer the action (COPY/UPDATE/SKIP) from the
// event sequence the pipeline sends.
type GUIFormatter struct {
	events     chan<- UIEvent
	totalFiles int
	startTime  time.Time

	mu    sync.Mutex
	files map[string]*fileState
	stats RunningStats
}

// Compile-time check
var _ output.Formatter = (*GUIFormatter)(nil)

// NewGUIFormatter creates a formatter that sends events to the GUI
func NewGUIFormatter(events chan<- UIEvent) *GUIFormatter {
	return &GUIFormatter{
		events: events,
		files:  make(map[string]*fileState),
	}
}

func (f *GUIFormatter) Name() string { return "gui" }

func (f *GUIFormatter) Start(_ io.Writer, totalFiles int, totalBytes int64, maxWorkers int) error {
	f.totalFiles = totalFiles
	f.startTime = time.Now()
	f.sendLog("INFO", fmt.Sprintf("Starting with %d workers", maxWorkers))
	return nil
}

func (f *GUIFormatter) Progress(update output.ProgressUpdate) error {
	if update.TotalFiles > 0 && update.TotalFiles > f.totalFiles {
		f.totalFiles = update.TotalFiles
	}

	switch update.Type {
	case "scan_progress":
		f.sendProgressUpdate(0, 0, update.TotalFiles, "Scanning...")

	case "compare_start":
		f.mu.Lock()
		f.getOrCreateFile(update.FilePath).hadCompare = true
		f.mu.Unlock()
		f.sendProgressUpdate(f.fraction(update.CurrentFile), update.CurrentFile, f.totalFiles, update.FilePath)

	case "file_start":
		f.mu.Lock()
		f.getOrCreateFile(update.FilePath).hadCopy = true
		f.mu.Unlock()
		f.sendProgressUpdate(f.fraction(update.CurrentFile), update.CurrentFile, f.totalFiles, update.FilePath)

	case "file_progress":
		f.sendProgressUpdate(f.fraction(update.CurrentFile), update.CurrentFile, f.totalFiles, update.FilePath)

	case "file_complete":
		f.mu.Lock()
		state := f.files[update.FilePath]
		delete(f.files, update.FilePath)

		action := resolveAction(state)
		switch action {
		case "COPY":
			f.stats.Copied++
		case "UPDATE":
			f.stats.Updated++
		case "SKIP":
			f.stats.Skipped++
		}
		stats := f.stats
		f.mu.Unlock()

		f.sendLog(action, fmt.Sprintf("%s (%s)", update.FilePath, formatSize(update.TotalBytes)))
		f.sendProgressUpdateWithStats(f.fraction(update.CurrentFile), update.CurrentFile, f.totalFiles, update.FilePath, stats)

	case "file_error":
		f.mu.Lock()
		delete(f.files, update.FilePath)
		f.stats.Errors++
		stats := f.stats
		f.mu.Unlock()

		msg := update.FilePath
		if update.Error != nil {
			msg = fmt.Sprintf("%s: %v", update.FilePath, update.Error)
		}
		f.sendLog("ERROR", msg)
		f.sendProgressUpdateWithStats(f.fraction(update.CurrentFile), update.CurrentFile, f.totalFiles, "", stats)
	}
	return nil
}

// resolveAction determines the action from the tracked file state
func resolveAction(state *fileState) string {
	if state == nil {
		return "DONE"
	}
	switch {
	case state.hadCopy && !state.hadCompare:
		return "COPY" // new file — went straight to copy
	case state.hadCopy && state.hadCompare:
		return "UPDATE" // existing file — compared, found different, then copied
	case !state.hadCopy && state.hadCompare:
		return "SKIP" // existing file — compared, found identical
	default:
		return "DONE"
	}
}

func (f *GUIFormatter) Complete(report *models.SyncReport) error {
	copied := report.Stats.FilesCopied.Load()
	updated := report.Stats.FilesUpdated.Load()
	identical := report.Stats.FilesSynchronized.Load()
	errored := report.Stats.FilesErrored.Load()
	deleted := report.Stats.FilesDeleted.Load()
	transferred := report.Stats.BytesTransferred.Load()

	summary := fmt.Sprintf("Copied: %d  |  Updated: %d  |  Identical: %d",
		copied, updated, identical)
	if deleted > 0 {
		summary += fmt.Sprintf("  |  Deleted: %d", deleted)
	}
	if errored > 0 {
		summary += fmt.Sprintf("  |  Errors: %d", errored)
	}
	f.sendLog("INFO", summary)

	if transferred > 0 {
		f.sendLog("INFO", fmt.Sprintf("Transferred: %s in %s",
			formatSize(transferred),
			report.Duration.Round(time.Millisecond),
		))
	}

	f.send(UIEvent{Type: eventComplete, Report: report})
	return nil
}

func (f *GUIFormatter) Error(err error) error {
	f.send(UIEvent{Type: eventError, Error: err})
	return nil
}

func (f *GUIFormatter) getOrCreateFile(path string) *fileState {
	s := f.files[path]
	if s == nil {
		s = &fileState{}
		f.files[path] = s
	}
	return s
}

func (f *GUIFormatter) sendLog(level, message string) {
	f.send(UIEvent{
		Type: eventLog,
		Log:  &LogEntry{Time: time.Now(), Level: level, Message: message},
	})
}

func (f *GUIFormatter) sendProgressUpdate(fraction float32, current, total int, path string) {
	f.mu.Lock()
	stats := f.stats
	f.mu.Unlock()
	f.sendProgressUpdateWithStats(fraction, current, total, path, stats)
}

func (f *GUIFormatter) sendProgressUpdateWithStats(fraction float32, current, total int, path string, stats RunningStats) {
	f.send(UIEvent{
		Type: eventProgress,
		Progress: &ProgressState{
			Fraction:    fraction,
			CurrentFile: current,
			TotalFiles:  total,
			CurrentPath: path,
			Stats:       stats,
		},
	})
}

func (f *GUIFormatter) send(ev UIEvent) {
	select {
	case f.events <- ev:
	default:
	}
}

func (f *GUIFormatter) fraction(current int) float32 {
	if f.totalFiles <= 0 {
		return 0
	}
	frac := float32(current) / float32(f.totalFiles)
	if frac > 1 {
		frac = 1
	}
	return frac
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
