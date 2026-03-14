//go:build !nogui

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

// Total returns the total number of completed files.
func (s RunningStats) Total() int {
	return s.Copied + s.Updated + s.Skipped + s.Errors
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
type fileState struct {
	hadCompare bool
	hadCopy    bool
}

// GUIFormatter implements output.Formatter for the GUI.
// Progress fraction is based on completed files (stats totals) so it
// never regresses even when concurrent workers send events out of order.
type GUIFormatter struct {
	events     chan<- UIEvent
	totalFiles int
	startTime  time.Time

	mu    sync.Mutex
	files map[string]*fileState
	stats RunningStats
}

var _ output.Formatter = (*GUIFormatter)(nil)

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
		f.sendProgress(0, update.TotalFiles, "Scanning...")

	case "compare_start":
		f.mu.Lock()
		f.getOrCreateFile(update.FilePath).hadCompare = true
		f.mu.Unlock()
		// Progress fraction stays at current completed count — no change
		f.sendCurrentProgress(update.FilePath)

	case "file_start":
		f.mu.Lock()
		f.getOrCreateFile(update.FilePath).hadCopy = true
		f.mu.Unlock()
		f.sendCurrentProgress(update.FilePath)

	case "file_progress":
		// No fraction change for in-progress files, just update the path
		f.sendCurrentProgress(update.FilePath)

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
		f.sendProgressWithStats(stats, update.FilePath)

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
		f.sendProgressWithStats(stats, "")
	}
	return nil
}

func resolveAction(state *fileState) string {
	if state == nil {
		return "DONE"
	}
	switch {
	case state.hadCopy && !state.hadCompare:
		return "COPY"
	case state.hadCopy && state.hadCompare:
		return "UPDATE"
	case !state.hadCopy && state.hadCompare:
		return "SKIP"
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

// sendProgress sends a progress event with the given completed/total and path.
func (f *GUIFormatter) sendProgress(completed, total int, path string) {
	frac := float32(0)
	if total > 0 {
		frac = float32(completed) / float32(total)
		if frac > 1 {
			frac = 1
		}
	}
	f.mu.Lock()
	stats := f.stats
	f.mu.Unlock()
	f.send(UIEvent{
		Type: eventProgress,
		Progress: &ProgressState{
			Fraction:    frac,
			CurrentFile: completed,
			TotalFiles:  total,
			CurrentPath: path,
			Stats:       stats,
		},
	})
}

// sendCurrentProgress sends progress based on the current completed file count from stats.
func (f *GUIFormatter) sendCurrentProgress(path string) {
	f.mu.Lock()
	stats := f.stats
	f.mu.Unlock()
	f.sendProgressWithStats(stats, path)
}

// sendProgressWithStats computes fraction from stats.Total() (completed files).
// This ensures the progress bar never regresses.
func (f *GUIFormatter) sendProgressWithStats(stats RunningStats, path string) {
	completed := stats.Total()
	total := f.totalFiles
	frac := float32(0)
	if total > 0 {
		frac = float32(completed) / float32(total)
		if frac > 1 {
			frac = 1
		}
	}
	f.send(UIEvent{
		Type: eventProgress,
		Progress: &ProgressState{
			Fraction:    frac,
			CurrentFile: completed,
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
