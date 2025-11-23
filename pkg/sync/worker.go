package sync

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// progressReader wraps an io.Reader to report progress
type progressReader struct {
	reader         io.Reader
	total          int64
	read           int64
	lastReported   int64
	lastReportTime time.Time
	onProgress     func(bytesRead int64)
}

// Progress reporting thresholds
const (
	progressReportInterval = 50 * time.Millisecond // Minimum time between progress reports
	progressReportBytes    = 64 * 1024             // Minimum bytes between reports (64KB)
)

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.read += int64(n)

		// Throttle progress callbacks: only report if either:
		// 1. Enough bytes have been read since last report (64KB threshold)
		// 2. Enough time has passed since last report (50ms threshold)
		// 3. This is the final read (err == io.EOF or err != nil)
		if pr.onProgress != nil {
			bytesSinceLastReport := pr.read - pr.lastReported
			timeSinceLastReport := time.Since(pr.lastReportTime)
			shouldReport := bytesSinceLastReport >= progressReportBytes ||
				timeSinceLastReport >= progressReportInterval ||
				err != nil // Always report on completion or error

			if shouldReport {
				pr.onProgress(pr.read)
				pr.lastReported = pr.read
				pr.lastReportTime = time.Now()
			}
		}
	}
	return n, err
}

// Worker manages parallel file transfer operations
type Worker struct {
	source      storage.Backend
	dest        storage.Backend
	maxWorkers  int
	semaphore   chan struct{}
}

// NewWorker creates a new worker pool
func NewWorker(source, dest storage.Backend, maxWorkers int) *Worker {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &Worker{
		source:     source,
		dest:       dest,
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
	}
}

// Execute processes file operations in parallel
// Continues processing all files even if some fail - errors are recorded in the report
func (w *Worker) Execute(ctx context.Context, operations []models.FileOperation, report *models.SyncReport, formatter output.Formatter) error {
	var wg sync.WaitGroup
	var errorsMu sync.Mutex // Only for appending to report.Errors slice

	currentFile := 0

	for i := range operations {
		op := &operations[i]

		// Skip operations that don't require action
		if op.Action == models.ActionSkip {
			// Distinguish between synchronized (identical) and skipped (other reasons)
			// Use atomic operations - no mutex needed
			if op.Reason == "files are identical" {
				report.Stats.FilesSynchronized.Add(1)
				// Note: formatter was already notified during comparison phase
			} else {
				report.Stats.FilesSkipped.Add(1)
			}
			continue
		}

		// Only copy and update actions are executed
		if op.Action != models.ActionCopy && op.Action != models.ActionUpdate {
			continue
		}

		// Acquire semaphore slot
		w.semaphore <- struct{}{}
		wg.Add(1)

		currentFile++
		fileNum := currentFile

		go func(operation *models.FileOperation, fileIndex int) {
			defer wg.Done()
			defer func() { <-w.semaphore }()

			startTime := time.Now()

			// Notify formatter of file start
			if formatter != nil {
				formatter.Progress(output.ProgressUpdate{
					Type:        "file_start",
					FilePath:    operation.Entry.RelativePath,
					TotalBytes:  operation.Entry.Size,
					CurrentFile: fileIndex,
				})
			}

			// Execute the copy/update
			err := w.copyFile(ctx, operation, formatter, fileIndex)
			operation.Duration = time.Since(startTime)

			if err != nil {
				operation.Error = err
				// Atomic increment for error counter
				report.Stats.FilesErrored.Add(1)

				// Mutex only needed for appending to slice (not thread-safe)
				errorsMu.Lock()
				report.Errors = append(report.Errors, models.SyncError{
					FilePath:  operation.Entry.RelativePath,
					Operation: operation.Action,
					Error:     err.Error(),
					Timestamp: time.Now(),
				})
				errorsMu.Unlock()

				// Notify formatter of error
				if formatter != nil {
					formatter.Progress(output.ProgressUpdate{
						Type:        "file_error",
						FilePath:    operation.Entry.RelativePath,
						CurrentFile: fileIndex,
						Error:       err,
					})
				}
			} else {
				operation.BytesCopied = operation.Entry.Size
				// Atomic add for bytes transferred
				report.Stats.BytesTransferred.Add(operation.Entry.Size)

				// Atomic increment for file counters
				if operation.Action == models.ActionCopy {
					report.Stats.FilesCopied.Add(1)
				} else if operation.Action == models.ActionUpdate {
					report.Stats.FilesUpdated.Add(1)
				}

				// Notify formatter of completion
				if formatter != nil {
					formatter.Progress(output.ProgressUpdate{
						Type:         "file_complete",
						FilePath:     operation.Entry.RelativePath,
						BytesWritten: operation.Entry.Size,
						TotalBytes:   operation.Entry.Size,
						CurrentFile:  fileIndex,
					})
				}
			}

			// Note: errors are already recorded in the report, no need to stop
			// Continue processing remaining files even if this one failed
		}(op, fileNum)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Don't return first error - let all files be processed
	// The report will contain all errors and the final status will be determined
	// based on whether any errors occurred
	return nil
}

// copyFile copies a single file from source to destination with progress reporting
func (w *Worker) copyFile(ctx context.Context, operation *models.FileOperation, formatter output.Formatter, fileIndex int) error {
	// Read from source
	reader, err := w.source.Read(ctx, operation.Entry.RelativePath)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}
	defer reader.Close()

	// Get source metadata to preserve timestamps and permissions
	sourceInfo, err := w.source.Stat(ctx, operation.Entry.RelativePath)
	if err != nil {
		return fmt.Errorf("failed to get source metadata: %w", err)
	}

	// Wrap reader with progress reporting
	progressReader := &progressReader{
		reader:         reader,
		total:          operation.Entry.Size,
		lastReportTime: time.Now(), // Initialize to enable throttling from start
		onProgress: func(bytesRead int64) {
			if formatter != nil {
				formatter.Progress(output.ProgressUpdate{
					Type:         "file_progress",
					FilePath:     operation.Entry.RelativePath,
					BytesWritten: bytesRead,
					TotalBytes:   operation.Entry.Size,
					CurrentFile:  fileIndex,
				})
			}
		},
	}

	// Write to destination with metadata preservation
	if err := w.dest.Write(ctx, operation.Entry.RelativePath, progressReader, operation.Entry.Size, sourceInfo); err != nil {
		return fmt.Errorf("failed to write destination: %w", err)
	}

	return nil
}
