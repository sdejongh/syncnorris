package output

import (
	"io"

	"github.com/sdejongh/syncnorris/pkg/models"
)

// ProgressUpdate represents a progress notification during sync
type ProgressUpdate struct {
	Type         string // "file_start", "compare_start", "file_progress", "file_complete", "file_error", "summary"
	FilePath     string
	BytesWritten int64
	TotalBytes   int64
	CurrentFile  int
	TotalFiles   int
	Error        error
}

// Formatter defines the interface for output formatting
// Implementations include human-readable and JSON formatters
type Formatter interface {
	// Start initializes the formatter for a new sync operation
	// maxWorkers indicates the number of parallel workers for display purposes
	Start(writer io.Writer, totalFiles int, totalBytes int64, maxWorkers int) error

	// Progress reports progress during sync
	Progress(update ProgressUpdate) error

	// Complete finalizes output and displays summary
	Complete(report *models.SyncReport) error

	// Error reports an error during sync
	Error(err error) error

	// Name returns the formatter name
	Name() string
}
