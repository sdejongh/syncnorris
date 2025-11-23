package output

import (
	"fmt"
	"io"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
)

// HumanFormatter formats output in human-readable format
type HumanFormatter struct {
	writer     io.Writer
	totalFiles int
	totalBytes int64
	startTime  time.Time
}

// NewHumanFormatter creates a new human-readable formatter
func NewHumanFormatter() *HumanFormatter {
	return &HumanFormatter{}
}

// Start initializes the formatter
func (f *HumanFormatter) Start(writer io.Writer, totalFiles int, totalBytes int64) error {
	f.writer = writer
	f.totalFiles = totalFiles
	f.totalBytes = totalBytes
	f.startTime = time.Now()

	if writer != nil {
		fmt.Fprintf(writer, "Starting sync: %d files, %s total\n",
			totalFiles, formatBytes(totalBytes))
	}

	return nil
}

// Progress reports progress during sync
func (f *HumanFormatter) Progress(update ProgressUpdate) error {
	if f.writer == nil {
		return nil
	}

	switch update.Type {
	case "file_start":
		fmt.Fprintf(f.writer, "[%d/%d] Copying %s (%s)...\n",
			update.CurrentFile, f.totalFiles,
			update.FilePath, formatBytes(update.TotalBytes))

	case "file_complete":
		fmt.Fprintf(f.writer, "[%d/%d] ✓ %s (%s)\n",
			update.CurrentFile, f.totalFiles,
			update.FilePath, formatBytes(update.BytesWritten))

	case "file_error":
		fmt.Fprintf(f.writer, "[%d/%d] ✗ %s: %v\n",
			update.CurrentFile, f.totalFiles,
			update.FilePath, update.Error)
	}

	return nil
}

// Complete finalizes output and displays summary
func (f *HumanFormatter) Complete(report *models.SyncReport) error {
	if f.writer == nil {
		f.writer = io.Discard
	}

	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Sync completed in %s\n", report.Duration.Round(time.Millisecond))
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Summary:\n")
	fmt.Fprintf(f.writer, "  Scanned:\n")
	fmt.Fprintf(f.writer, "    Source:         %d files, %d dirs\n", report.Stats.SourceFilesScanned.Load(), report.Stats.SourceDirsScanned.Load())
	fmt.Fprintf(f.writer, "    Destination:    %d files, %d dirs\n", report.Stats.DestFilesScanned.Load(), report.Stats.DestDirsScanned.Load())
	fmt.Fprintf(f.writer, "    Unique paths:   %d files, %d dirs\n", report.Stats.FilesScanned.Load(), report.Stats.DirsScanned.Load())
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "  Operations:\n")
	fmt.Fprintf(f.writer, "    Files copied:       %d\n", report.Stats.FilesCopied.Load())
	fmt.Fprintf(f.writer, "    Files updated:      %d\n", report.Stats.FilesUpdated.Load())
	fmt.Fprintf(f.writer, "    Files synchronized: %d\n", report.Stats.FilesSynchronized.Load())
	fmt.Fprintf(f.writer, "    Files skipped:      %d\n", report.Stats.FilesSkipped.Load())
	fmt.Fprintf(f.writer, "    Files errored:      %d\n", report.Stats.FilesErrored.Load())
	fmt.Fprintf(f.writer, "    Dirs created:       %d\n", report.Stats.DirsCreated.Load())
	fmt.Fprintf(f.writer, "    Dirs deleted:       %d\n", report.Stats.DirsDeleted.Load())
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "  Transfer:\n")
	fmt.Fprintf(f.writer, "    Data:           %s\n", formatBytes(report.Stats.BytesTransferred.Load()))

	if report.Duration.Seconds() > 0 {
		avgSpeed := float64(report.Stats.BytesTransferred.Load()) / report.Duration.Seconds()
		fmt.Fprintf(f.writer, "    Average speed:  %s/s\n", formatBytes(int64(avgSpeed)))
	}

	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Status: %s\n", report.Status)

	if len(report.Errors) > 0 {
		fmt.Fprintf(f.writer, "\nErrors:\n")
		for _, err := range report.Errors {
			fmt.Fprintf(f.writer, "  %s: %s\n", err.FilePath, err.Error)
		}
	}

	return nil
}

// Error reports an error
func (f *HumanFormatter) Error(err error) error {
	if f.writer != nil {
		fmt.Fprintf(f.writer, "Error: %v\n", err)
	}
	return nil
}

// Name returns the formatter name
func (f *HumanFormatter) Name() string {
	return "human"
}

// formatBytes formats bytes in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration formats duration in human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
