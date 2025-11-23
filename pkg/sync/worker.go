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
func (w *Worker) Execute(ctx context.Context, operations []models.FileOperation, report *models.SyncReport, formatter output.Formatter) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, len(operations))

	currentFile := 0

	for i := range operations {
		op := &operations[i]

		// Skip operations that don't require action
		if op.Action == models.ActionSkip {
			mu.Lock()
			// Distinguish between synchronized (identical) and skipped (other reasons)
			if op.Reason == "files are identical" {
				report.Stats.FilesSynchronized++
				// Note: formatter was already notified during comparison phase
			} else {
				report.Stats.FilesSkipped++
			}
			mu.Unlock()
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

			mu.Lock()
			if err != nil {
				operation.Error = err
				report.Stats.FilesErrored++
				report.Errors = append(report.Errors, models.SyncError{
					FilePath:  operation.Entry.RelativePath,
					Operation: operation.Action,
					Error:     err.Error(),
					Timestamp: time.Now(),
				})

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
				report.Stats.BytesTransferred += operation.Entry.Size

				if operation.Action == models.ActionCopy {
					report.Stats.FilesCopied++
				} else if operation.Action == models.ActionUpdate {
					report.Stats.FilesUpdated++
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
			mu.Unlock()

			if err != nil {
				errChan <- err
			}
		}(op, fileNum)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
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
