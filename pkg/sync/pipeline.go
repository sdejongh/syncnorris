package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/ratelimit"
	"github.com/sdejongh/syncnorris/pkg/storage"
	"github.com/sdejongh/syncnorris/pkg/sync/job"
)

// errSoftPaused is a sentinel returned by the Walk callback when the soft-pause
// channel is closed. It signals a clean stop of the scanner (not a failure).
var errSoftPaused = errors.New("soft pause requested")

// Pipeline orchestrates the producer-consumer sync process
type Pipeline struct {
	source     storage.Backend
	dest       storage.Backend
	comparator compare.Comparator
	formatter  output.Formatter
	logger     logging.Logger
	operation  *models.SyncOperation

	// Task queue
	taskQueue chan *FileTask
	queueSize int

	// State tracking
	scanComplete   atomic.Bool
	totalFiles     atomic.Int32
	totalBytes     atomic.Int64
	processedFiles atomic.Int32
	processedBytes atomic.Int64

	// Destination file map for quick lookup (populated during scan)
	destFiles   map[string]*storage.FileInfo
	destDirs    map[string]*storage.FileInfo // Destination directories
	destFilesMu sync.RWMutex

	// Active files tracking for progress reporting
	activeFiles   map[string]int // path -> fileIndex
	activeFilesMu sync.RWMutex

	// Results collection
	results   []*FileTask
	resultsMu sync.Mutex

	// Rate limiter for bandwidth limiting (nil = unlimited)
	rateLimiter *ratelimit.Limiter

	// Optional job tracking for pause/resume support (nil = disabled)
	job           *job.Job
	jobStore      *job.JobStore
	pauseSoft     <-chan struct{}
	pauseHard     <-chan struct{}
	completionLog *job.CompletionLog
	doneSet       map[string]job.CompletionEntry
}

// PipelineConfig holds configuration for the pipeline
type PipelineConfig struct {
	MaxWorkers int
	QueueSize  int // Buffer size for the task queue
	// Job is optional. When set, the pipeline loads the completion log,
	// skips already-completed files, and appends to the log on success.
	Job      *job.Job
	JobStore *job.JobStore
	// PauseSoft, when closed, causes the scanner to stop queueing new
	// tasks while in-flight workers finish their current file.
	PauseSoft <-chan struct{}
	// PauseHard, when closed, cancels the context and rolls back in-flight
	// .partial files.
	PauseHard <-chan struct{}
}

// DefaultPipelineConfig returns sensible defaults
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		MaxWorkers: 5,
		QueueSize:  1000, // Buffer up to 1000 tasks
	}
}

// NewPipeline creates a new sync pipeline
func NewPipeline(
	source, dest storage.Backend,
	comparator compare.Comparator,
	formatter output.Formatter,
	logger logging.Logger,
	operation *models.SyncOperation,
	config PipelineConfig,
) *Pipeline {
	if config.MaxWorkers < 1 {
		config.MaxWorkers = 1
	}
	if config.QueueSize < 1 {
		config.QueueSize = 1
	}

	// Create rate limiter if bandwidth limit is set
	var rateLimiter *ratelimit.Limiter
	if operation.BandwidthLimit > 0 {
		rateLimiter = ratelimit.NewLimiter(operation.BandwidthLimit)
	}

	return &Pipeline{
		source:      source,
		dest:        dest,
		comparator:  comparator,
		formatter:   formatter,
		logger:      logger,
		operation:   operation,
		taskQueue:   make(chan *FileTask, config.QueueSize),
		queueSize:   config.QueueSize,
		destFiles:   make(map[string]*storage.FileInfo),
		destDirs:    make(map[string]*storage.FileInfo),
		activeFiles: make(map[string]int),
		results:     make([]*FileTask, 0),
		rateLimiter: rateLimiter,
		job:         config.Job,
		jobStore:    config.JobStore,
		pauseSoft:   config.PauseSoft,
		pauseHard:   config.PauseHard,
		doneSet:     make(map[string]job.CompletionEntry),
	}
}

// Run executes the pipeline and returns a sync report
func (p *Pipeline) Run(ctx context.Context) (*models.SyncReport, error) {
	startTime := time.Now()
	report := &models.SyncReport{
		OperationID: p.operation.ID,
		SourcePath:  p.operation.SourcePath,
		DestPath:    p.operation.DestPath,
		Mode:        p.operation.Mode,
		DryRun:      p.operation.DryRun,
		StartTime:   startTime,
		Status:      models.StatusSuccess,
	}

	if p.logger != nil {
		p.logger.Info(ctx, "Starting pipeline sync operation", logging.Fields{
			"operation_id": p.operation.ID,
			"source":       p.operation.SourcePath,
			"dest":         p.operation.DestPath,
			"max_workers":  p.operation.MaxWorkers,
		})
	}

	// Create a cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Hard pause: closing p.pauseHard cancels the context immediately, which
	// causes all in-flight I/O (including ctxReader) to abort and .partial
	// files to be cleaned up. The goroutine also exits on ctx.Done() to avoid
	// a goroutine leak when the pipeline finishes normally.
	if p.pauseHard != nil {
		go func() {
			select {
			case <-p.pauseHard:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	// Preload completion log if a job is attached
	if p.job != nil {
		loaded, err := job.LoadCompletionLog(p.job.CompletionLog)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn(ctx, "failed to load completion log, starting fresh", logging.Fields{"err": err.Error()})
			}
			loaded = make(map[string]job.CompletionEntry)
		}
		p.doneSet = loaded

		cl, err := job.NewCompletionLog(p.job.CompletionLog)
		if err != nil {
			return nil, fmt.Errorf("open completion log: %w", err)
		}
		p.completionLog = cl
		defer func() { _ = p.completionLog.Close() }()

		// Configure a job-scoped partial suffix on the local backend so that
		// abandonJob in the GUI can locate and remove residual .partial files
		// by walking the destination for the right suffix pattern.
		if ld, ok := p.dest.(*storage.Local); ok {
			ld.SetPartialSuffix(fmt.Sprintf(".syncnorris-%s.partial", p.job.ID[:8]))
		}
	}

	// Phase 1: Scan destination first (we need this for comparisons)
	if p.logger != nil {
		p.logger.Info(ctx, "Scanning destination directory", nil)
	}
	if err := p.scanDestination(ctx); err != nil {
		return nil, err
	}

	report.Stats.DestFilesScanned.Store(int32(len(p.destFiles)))

	// Phase 2: Setup progress callback for comparator
	if comp, ok := p.comparator.(interface {
		SetProgressCallback(func(path string, current, total int64))
	}); ok {
		comp.SetProgressCallback(func(path string, current, total int64) {
			if p.formatter != nil {
				p.activeFilesMu.RLock()
				fileIndex, exists := p.activeFiles[path]
				p.activeFilesMu.RUnlock()
				if exists {
					p.formatter.Progress(output.ProgressUpdate{
						Type:         "file_progress",
						FilePath:     path,
						BytesWritten: current,
						TotalBytes:   total,
						CurrentFile:  fileIndex,
					})
				}
			}
		})
	}

	// Setup rate limiting for comparator if bandwidth limiting is enabled
	if p.rateLimiter != nil {
		if comp, ok := p.comparator.(compare.RateLimitedComparator); ok {
			comp.SetReaderWrapper(func(rc io.ReadCloser) io.ReadCloser {
				return ratelimit.NewReadCloser(ctx, rc, p.rateLimiter)
			})
		}
	}

	// Phase 3: Start workers before scanning source
	var workersWg sync.WaitGroup
	workerCount := p.operation.MaxWorkers
	if workerCount < 1 {
		workerCount = 5
	}

	for i := 0; i < workerCount; i++ {
		workersWg.Add(1)
		go p.runWorker(ctx, i, report, &workersWg)
	}

	// Phase 4: Scan source and populate queue (producer)
	if p.logger != nil {
		p.logger.Info(ctx, "Scanning source directory and populating queue", nil)
	}

	// Initialize formatter with estimated values (will be updated as we scan)
	if p.formatter != nil {
		p.formatter.Start(nil, 0, 0, workerCount)
	}

	scanErr := p.scanSourceAndQueue(ctx, report)

	// Signal that scanning is complete
	p.scanComplete.Store(true)
	close(p.taskQueue)

	// Wait for all workers to finish
	workersWg.Wait()

	if scanErr != nil && !errors.Is(scanErr, errSoftPaused) {
		report.Status = models.StatusFailed
		return report, scanErr
	}

	// Phase 5: Delete orphan files if requested
	if p.operation.DeleteOrphans {
		p.deleteOrphanFiles(ctx, report)
	}

	// Phase 6: Collect results and build report
	p.buildReport(report)

	// Finalize report timing
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	// Complete formatter (after duration is calculated)
	if p.formatter != nil {
		p.formatter.Complete(report)
	}

	if len(report.Errors) > 0 {
		if int(report.Stats.FilesErrored.Load()) == int(p.totalFiles.Load()) {
			report.Status = models.StatusFailed
		} else {
			report.Status = models.StatusPartial
		}
	}

	if p.logger != nil {
		p.logger.Info(ctx, "Pipeline sync completed", logging.Fields{
			"duration":          report.Duration.String(),
			"status":            report.Status,
			"files_copied":      report.Stats.FilesCopied.Load(),
			"files_updated":     report.Stats.FilesUpdated.Load(),
			"files_synchronized": report.Stats.FilesSynchronized.Load(),
			"files_errored":     report.Stats.FilesErrored.Load(),
			"bytes_transferred": report.Stats.BytesTransferred.Load(),
		})
	}

	return report, nil
}

// scanDestination scans the destination and builds a lookup map
func (p *Pipeline) scanDestination(ctx context.Context) error {
	p.destFilesMu.Lock()
	defer p.destFilesMu.Unlock()

	return p.dest.Walk(ctx, "", func(fi storage.FileInfo) error {
		if shouldExclude(fi.RelativePath, p.operation.ExcludePatterns) {
			return nil
		}

		entry := fi // copy to avoid pointer aliasing
		if entry.IsDir {
			if entry.RelativePath != "." {
				p.destDirs[entry.RelativePath] = &entry
			}
		} else {
			p.destFiles[entry.RelativePath] = &entry
		}

		return nil
	})
}

// scanSourceAndQueue scans source files and adds them to the queue
func (p *Pipeline) scanSourceAndQueue(ctx context.Context, report *models.SyncReport) error {
	return p.source.Walk(ctx, "", func(f storage.FileInfo) error {
		// Skip directories
		if f.IsDir {
			return nil
		}

		// Apply exclude patterns
		if shouldExclude(f.RelativePath, p.operation.ExcludePatterns) {
			report.Stats.FilesSkipped.Add(1)

			if p.logger != nil {
				p.logger.Debug(ctx, "File skipped (excluded by pattern)", logging.Fields{
					"path": f.RelativePath,
					"size": f.Size,
				})
			}

			// Add to differences report
			p.resultsMu.Lock()
			report.Differences = append(report.Differences, models.FileDifference{
				RelativePath: f.RelativePath,
				Reason:       models.ReasonSkipped,
				Details:      "excluded by pattern",
				SourceInfo: &models.FileInfo{
					Size:    f.Size,
					ModTime: f.ModTime,
				},
			})
			p.resultsMu.Unlock()
			return nil
		}

		// Soft pause: if the pause channel is closed, stop producing new tasks.
		// Already-queued tasks will continue to drain through the workers.
		if p.pauseSoft != nil {
			select {
			case <-p.pauseSoft:
				return errSoftPaused
			default:
			}
		}

		// Skip files already completed in a previous run of this job
		if p.job != nil {
			if entry, done := p.doneSet[f.RelativePath]; done {
				srcInfo, err := p.source.Stat(ctx, f.RelativePath)
				if err == nil && srcInfo.Size == entry.Size && srcInfo.ModTime.UnixNano() == entry.MTime.UnixNano() {
					report.Stats.FilesSkipped.Add(1)
					if p.logger != nil {
						p.logger.Debug(ctx, "File skipped (previously completed)", logging.Fields{"path": f.RelativePath})
					}
					return nil
				}
				// Stale completion: fall through to normal flow
			}
		}

		// Create task and add to queue; only update totals once the task is
		// actually enqueued so counters never over-report on a soft-pause.
		task := NewFileTask(f.RelativePath, f.Size, f.ModTime)

		enqueueTask := func() {
			p.totalFiles.Add(1)
			p.totalBytes.Add(f.Size)
			if p.formatter != nil {
				p.formatter.Progress(output.ProgressUpdate{
					Type:       "scan_progress",
					TotalFiles: int(p.totalFiles.Load()),
					TotalBytes: p.totalBytes.Load(),
				})
			}
		}

		if p.pauseSoft != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-p.pauseSoft:
				// Soft pause fired while the scanner was blocked on the queue.
				// Stop producing; let already-queued tasks drain through workers.
				return errSoftPaused
			case p.taskQueue <- task:
				enqueueTask()
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.taskQueue <- task:
			enqueueTask()
			return nil
		}
	})
}

// runWorker is the worker goroutine that processes tasks
func (p *Pipeline) runWorker(ctx context.Context, workerID int, report *models.SyncReport, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		// Soft-pause check: between files, exit if pause was requested.
		// Buffered tasks still in the queue are abandoned; they were never
		// processed and will be re-scanned on resume (not in the completion
		// log), so no work is lost.
		if p.pauseSoft != nil {
			select {
			case <-p.pauseSoft:
				return
			default:
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-p.pauseSoft:
			return
		case task, ok := <-p.taskQueue:
			if !ok {
				// Queue is closed and empty, worker exits
				return
			}
			p.processTask(ctx, workerID, task, report)
		}
	}
}

// processTask handles a single file task with the complete workflow
func (p *Pipeline) processTask(ctx context.Context, workerID int, task *FileTask, report *models.SyncReport) {
	startTime := time.Now()
	task.MarkProcessing(workerID)

	// Notify formatter of file start
	fileIndex := int(p.processedFiles.Add(1))

	// Register this file as active for progress tracking
	p.activeFilesMu.Lock()
	p.activeFiles[task.RelativePath] = fileIndex
	p.activeFilesMu.Unlock()

	// Ensure we clean up when done
	defer func() {
		p.activeFilesMu.Lock()
		delete(p.activeFiles, task.RelativePath)
		p.activeFilesMu.Unlock()
	}()

	// Step 1: Check if destination file exists (from pre-scanned data)
	p.destFilesMu.RLock()
	destInfo, destExists := p.destFiles[task.RelativePath]
	p.destFilesMu.RUnlock()

	if p.logger != nil {
		p.logger.Debug(ctx, "Processing file", logging.Fields{
			"path":        task.RelativePath,
			"size":        task.Size,
			"worker":      workerID,
			"dest_exists": destExists,
		})
	}

	if !destExists {
		// File doesn't exist in destination - copy it
		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_start",
				FilePath:    task.RelativePath,
				TotalBytes:  task.Size,
				CurrentFile: fileIndex,
			})
		}
		p.copyFile(ctx, workerID, task, report, fileIndex, startTime)
		return
	}

	// Step 2: File exists in both - compare them
	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:        "compare_start",
			FilePath:    task.RelativePath,
			TotalBytes:  task.Size,
			CurrentFile: fileIndex,
		})
	}

	// Optimization: For namesize comparison, use already-scanned metadata
	// This avoids redundant Stat() calls which are slow on Windows
	var comparison *compare.Comparison
	var err error

	if p.comparator.Name() == "namesize" {
		// Fast path: use pre-scanned metadata
		if task.Size == destInfo.Size {
			comparison = &compare.Comparison{
				SourcePath: task.RelativePath,
				DestPath:   task.RelativePath,
				Result:     compare.Same,
				Reason:     "name and size match",
			}
		} else {
			comparison = &compare.Comparison{
				SourcePath: task.RelativePath,
				DestPath:   task.RelativePath,
				Result:     compare.Different,
				Reason:     "file sizes differ",
			}
		}
	} else {
		// Standard path: use comparator (for hash, md5, binary)
		comparison, err = p.comparator.Compare(ctx, p.source, p.dest, task.RelativePath, task.RelativePath)
	}
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "File comparison failed", err, logging.Fields{
				"path": task.RelativePath,
				"size": task.Size,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}

	if comparison.Result == compare.Same {
		// Files are identical - mark as synchronized
		task.MarkCompleted(ResultSynchronized, 0, time.Since(startTime))
		report.Stats.FilesSynchronized.Add(1)
		p.processedBytes.Add(task.Size)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Debug(ctx, "File synchronized (identical)", logging.Fields{
				"path":     task.RelativePath,
				"size":     task.Size,
				"duration": time.Since(startTime).String(),
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:         "file_complete",
				FilePath:     task.RelativePath,
				BytesWritten: task.Size,
				TotalBytes:   task.Size,
				CurrentFile:  fileIndex,
			})
		}
		return
	}

	// Files are different - update (copy with overwrite)
	_ = destInfo // Used for future metadata comparison if needed
	p.updateFile(ctx, workerID, task, report, fileIndex, startTime)
}

// copyFile copies a file from source to destination
func (p *Pipeline) copyFile(ctx context.Context, workerID int, task *FileTask, report *models.SyncReport, fileIndex int, startTime time.Time) {
	if p.logger != nil {
		p.logger.Debug(ctx, "Copying file (new)", logging.Fields{
			"path":    task.RelativePath,
			"size":    task.Size,
			"dry_run": p.operation.DryRun,
		})
	}

	if p.operation.DryRun {
		task.MarkCompleted(ResultCopied, task.Size, time.Since(startTime))
		report.Stats.FilesCopied.Add(1)
		p.processedBytes.Add(task.Size)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Debug(ctx, "File would be copied (dry-run)", logging.Fields{
				"path": task.RelativePath,
				"size": task.Size,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:         "file_complete",
				FilePath:     task.RelativePath,
				BytesWritten: task.Size,
				TotalBytes:   task.Size,
				CurrentFile:  fileIndex,
			})
		}
		return
	}

	// Read from source
	reader, err := p.source.Read(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to read source file", err, logging.Fields{
				"path": task.RelativePath,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}
	defer reader.Close()

	// Wrap with rate limiter if bandwidth limiting is enabled
	if p.rateLimiter != nil {
		reader = ratelimit.NewReadCloser(ctx, reader, p.rateLimiter)
	}

	// Get source metadata
	sourceInfo, err := p.source.Stat(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to get source file metadata", err, logging.Fields{
				"path": task.RelativePath,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}

	// Wrap with progress reporting
	pr := &progressReader{
		reader:         reader,
		total:          task.Size,
		lastReportTime: time.Now(),
		onProgress: func(bytesRead int64) {
			if p.formatter != nil {
				p.formatter.Progress(output.ProgressUpdate{
					Type:         "file_progress",
					FilePath:     task.RelativePath,
					BytesWritten: bytesRead,
					TotalBytes:   task.Size,
					CurrentFile:  fileIndex,
				})
			}
		},
	}

	// Write to destination
	if err := p.dest.Write(ctx, task.RelativePath, pr, task.Size, sourceInfo); err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to write destination file", err, logging.Fields{
				"path": task.RelativePath,
				"size": task.Size,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}

	task.MarkCompleted(ResultCopied, task.Size, time.Since(startTime))
	report.Stats.FilesCopied.Add(1)
	report.Stats.BytesTransferred.Add(task.Size)
	p.processedBytes.Add(task.Size)
	p.addResult(task)
	p.recordCompletion(task, sourceInfo.ModTime)

	if p.logger != nil {
		p.logger.Debug(ctx, "File copied successfully", logging.Fields{
			"path":     task.RelativePath,
			"size":     task.Size,
			"duration": time.Since(startTime).String(),
		})
	}

	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:         "file_complete",
			FilePath:     task.RelativePath,
			BytesWritten: task.Size,
			TotalBytes:   task.Size,
			CurrentFile:  fileIndex,
		})
	}
}

// updateFile updates an existing file in destination
func (p *Pipeline) updateFile(ctx context.Context, workerID int, task *FileTask, report *models.SyncReport, fileIndex int, startTime time.Time) {
	if p.logger != nil {
		p.logger.Debug(ctx, "Updating file (content differs)", logging.Fields{
			"path":    task.RelativePath,
			"size":    task.Size,
			"dry_run": p.operation.DryRun,
		})
	}

	// Notify formatter that we're switching from hashing to copying
	// This resets the progress and changes the icon from 🔍 to ⏳
	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:        "file_start",
			FilePath:    task.RelativePath,
			TotalBytes:  task.Size,
			CurrentFile: fileIndex,
		})
	}

	if p.operation.DryRun {
		task.MarkCompleted(ResultUpdated, task.Size, time.Since(startTime))
		report.Stats.FilesUpdated.Add(1)
		p.processedBytes.Add(task.Size)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Debug(ctx, "File would be updated (dry-run)", logging.Fields{
				"path": task.RelativePath,
				"size": task.Size,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:         "file_complete",
				FilePath:     task.RelativePath,
				BytesWritten: task.Size,
				TotalBytes:   task.Size,
				CurrentFile:  fileIndex,
			})
		}
		return
	}

	// Same as copy, but we record it as an update
	reader, err := p.source.Read(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to read source file for update", err, logging.Fields{
				"path": task.RelativePath,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}
	defer reader.Close()

	// Wrap with rate limiter if bandwidth limiting is enabled
	if p.rateLimiter != nil {
		reader = ratelimit.NewReadCloser(ctx, reader, p.rateLimiter)
	}

	sourceInfo, err := p.source.Stat(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to get source file metadata for update", err, logging.Fields{
				"path": task.RelativePath,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}

	pr := &progressReader{
		reader:         reader,
		total:          task.Size,
		lastReportTime: time.Now(),
		onProgress: func(bytesRead int64) {
			if p.formatter != nil {
				p.formatter.Progress(output.ProgressUpdate{
					Type:         "file_progress",
					FilePath:     task.RelativePath,
					BytesWritten: bytesRead,
					TotalBytes:   task.Size,
					CurrentFile:  fileIndex,
				})
			}
		},
	}

	if err := p.dest.Write(ctx, task.RelativePath, pr, task.Size, sourceInfo); err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

		if p.logger != nil {
			p.logger.Error(ctx, "Failed to write destination file for update", err, logging.Fields{
				"path": task.RelativePath,
				"size": task.Size,
			})
		}

		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:        "file_error",
				FilePath:    task.RelativePath,
				CurrentFile: fileIndex,
				Error:       err,
			})
		}
		return
	}

	task.MarkCompleted(ResultUpdated, task.Size, time.Since(startTime))
	report.Stats.FilesUpdated.Add(1)
	report.Stats.BytesTransferred.Add(task.Size)
	p.processedBytes.Add(task.Size)
	p.addResult(task)
	p.recordCompletion(task, sourceInfo.ModTime)

	if p.logger != nil {
		p.logger.Debug(ctx, "File updated successfully", logging.Fields{
			"path":     task.RelativePath,
			"size":     task.Size,
			"duration": time.Since(startTime).String(),
		})
	}

	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:         "file_complete",
			FilePath:     task.RelativePath,
			BytesWritten: task.Size,
			TotalBytes:   task.Size,
			CurrentFile:  fileIndex,
		})
	}
}

// recordCompletion appends an entry to the completion log after a successful
// copy or update. It is a no-op when no Job is attached (p.completionLog == nil).
func (p *Pipeline) recordCompletion(task *FileTask, sourceModTime time.Time) {
	if p.completionLog == nil {
		return
	}
	entry := job.CompletionEntry{
		Path:  task.RelativePath,
		Size:  task.Size,
		MTime: sourceModTime,
		Hash:  task.SourceHash, // empty when no hash computed; log uses "-" placeholder
	}
	if err := p.completionLog.Append(entry); err != nil && p.logger != nil {
		p.logger.Warn(context.Background(), "failed to append completion log", logging.Fields{
			"path": task.RelativePath,
			"err":  err.Error(),
		})
	}
}

// addResult safely adds a completed task to the results
func (p *Pipeline) addResult(task *FileTask) {
	p.resultsMu.Lock()
	p.results = append(p.results, task)
	p.resultsMu.Unlock()
}

// recordError adds an error to the report
func (p *Pipeline) recordError(report *models.SyncReport, task *FileTask) {
	var action models.Action
	switch task.Result {
	case ResultCopied:
		action = models.ActionCopy
	case ResultUpdated:
		action = models.ActionUpdate
	default:
		action = models.ActionSkip
	}

	p.resultsMu.Lock()
	report.Errors = append(report.Errors, models.SyncError{
		FilePath:  task.RelativePath,
		Operation: action,
		Error:     task.Error.Error(),
		Timestamp: time.Now(),
	})
	p.resultsMu.Unlock()
}

// buildReport converts results to the final report format
func (p *Pipeline) buildReport(report *models.SyncReport) {
	report.Stats.SourceFilesScanned.Store(p.totalFiles.Load())
	report.Stats.FilesScanned.Store(p.totalFiles.Load())

	// Build operations list from results
	p.resultsMu.Lock()
	defer p.resultsMu.Unlock()

	report.Operations = make([]models.FileOperation, 0, len(p.results))
	// Preserve existing differences (e.g., from deleteOrphanFiles)
	if report.Differences == nil {
		report.Differences = make([]models.FileDifference, 0)
	}

	for _, task := range p.results {
		var action models.Action
		var reason string

		switch task.Result {
		case ResultCopied:
			action = models.ActionCopy
			reason = "file copied from source"
		case ResultUpdated:
			action = models.ActionUpdate
			reason = "file updated from source"
		case ResultSynchronized:
			action = models.ActionSkip
			reason = "files are identical"
		case ResultSkipped:
			action = models.ActionSkip
			reason = "file skipped"
		case ResultFailed:
			action = models.ActionSkip
			reason = "processing failed"
		}

		op := models.FileOperation{
			Entry: &models.FileEntry{
				RelativePath: task.RelativePath,
				Size:         task.Size,
				ModTime:      task.ModTime,
			},
			Action:      action,
			Reason:      reason,
			Error:       task.Error,
			BytesCopied: task.BytesTransferred,
			Duration:    task.ProcessingDuration,
		}
		report.Operations = append(report.Operations, op)

		// Track differences
		switch task.Result {
		case ResultFailed:
			diff := models.FileDifference{
				RelativePath: task.RelativePath,
				Reason:       models.ReasonCopyError,
				Details:      task.Error.Error(),
				SourceInfo: &models.FileInfo{
					Size:    task.Size,
					ModTime: task.ModTime,
				},
			}
			report.Differences = append(report.Differences, diff)

		case ResultCopied:
			// File only exists in source (needs to be copied)
			diff := models.FileDifference{
				RelativePath: task.RelativePath,
				Reason:       models.ReasonOnlyInSource,
				Details:      "file exists only in source",
				SourceInfo: &models.FileInfo{
					Size:    task.Size,
					ModTime: task.ModTime,
				},
			}
			report.Differences = append(report.Differences, diff)

		case ResultUpdated:
			// File exists in both but differs (needs update)
			diff := models.FileDifference{
				RelativePath: task.RelativePath,
				Reason:       models.ReasonContentDiff,
				Details:      "file content differs",
				SourceInfo: &models.FileInfo{
					Size:    task.Size,
					ModTime: task.ModTime,
				},
			}
			// Add dest info if available
			p.destFilesMu.RLock()
			if destInfo, exists := p.destFiles[task.RelativePath]; exists {
				diff.DestInfo = &models.FileInfo{
					Size:    destInfo.Size,
					ModTime: destInfo.ModTime,
				}
			}
			p.destFilesMu.RUnlock()
			report.Differences = append(report.Differences, diff)
		}
	}

	// Note: Dest-only files are only reported when --delete is used
	// Without --delete, orphan files in destination are simply ignored
	// (not counted, not reported) as they are outside the scope of one-way sync
}

// deleteOrphanFiles deletes files and directories that exist in destination but not in source
func (p *Pipeline) deleteOrphanFiles(ctx context.Context, report *models.SyncReport) {
	// Build set of source files and directories from results
	sourceFiles := make(map[string]bool)
	sourceDirs := make(map[string]bool)
	p.resultsMu.Lock()
	for _, task := range p.results {
		sourceFiles[task.RelativePath] = true
		// Mark all parent directories as existing in source
		dir := filepath.Dir(task.RelativePath)
		for dir != "." && dir != "" {
			sourceDirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}
	p.resultsMu.Unlock()

	// Find orphan files
	p.destFilesMu.RLock()
	orphanFiles := make([]string, 0)
	for path := range p.destFiles {
		if !sourceFiles[path] {
			orphanFiles = append(orphanFiles, path)
		}
	}

	// Find orphan directories (not in source)
	orphanDirs := make([]string, 0)
	for path := range p.destDirs {
		if !sourceDirs[path] {
			orphanDirs = append(orphanDirs, path)
		}
	}
	p.destFilesMu.RUnlock()

	// Sort orphan directories by depth (deepest first) for proper deletion order
	sort.Slice(orphanDirs, func(i, j int) bool {
		return strings.Count(orphanDirs[i], string(filepath.Separator)) > strings.Count(orphanDirs[j], string(filepath.Separator))
	})

	// Delete orphan files first
	for _, path := range orphanFiles {
		// Get file info for the report
		p.destFilesMu.RLock()
		fileInfo := p.destFiles[path]
		p.destFilesMu.RUnlock()

		if p.operation.DryRun {
			report.Stats.FilesDeleted.Add(1)
			// Add to differences report
			p.resultsMu.Lock()
			diff := models.FileDifference{
				RelativePath: path,
				Reason:       models.ReasonDeleted,
				Details:      "file would be deleted (dry-run)",
			}
			if fileInfo != nil {
				diff.DestInfo = &models.FileInfo{
					Size:    fileInfo.Size,
					ModTime: fileInfo.ModTime,
				}
			}
			report.Differences = append(report.Differences, diff)
			p.resultsMu.Unlock()

			if p.logger != nil {
				p.logger.Info(ctx, "Would delete orphan file", logging.Fields{
					"path": path,
				})
			}
			continue
		}

		if err := p.dest.Delete(ctx, path); err != nil {
			report.Stats.FilesErrored.Add(1)
			p.resultsMu.Lock()
			report.Errors = append(report.Errors, models.SyncError{
				FilePath:  path,
				Operation: models.ActionDelete,
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
			p.resultsMu.Unlock()

			if p.logger != nil {
				p.logger.Error(ctx, "Failed to delete orphan file", err, logging.Fields{
					"path": path,
				})
			}
		} else {
			report.Stats.FilesDeleted.Add(1)
			// Add to differences report
			p.resultsMu.Lock()
			diff := models.FileDifference{
				RelativePath: path,
				Reason:       models.ReasonDeleted,
				Details:      "file deleted from destination",
			}
			if fileInfo != nil {
				diff.DestInfo = &models.FileInfo{
					Size:    fileInfo.Size,
					ModTime: fileInfo.ModTime,
				}
			}
			report.Differences = append(report.Differences, diff)
			p.resultsMu.Unlock()

			if p.logger != nil {
				p.logger.Info(ctx, "Deleted orphan file", logging.Fields{
					"path": path,
				})
			}
		}
	}

	// Delete orphan directories (deepest first)
	for _, path := range orphanDirs {
		if p.operation.DryRun {
			report.Stats.DirsDeleted.Add(1)
			if p.logger != nil {
				p.logger.Info(ctx, "Would delete orphan directory", logging.Fields{
					"path": path,
				})
			}
			continue
		}

		if err := p.dest.Delete(ctx, path); err != nil {
			// Ignore errors for non-empty directories (may contain files we didn't delete)
			if p.logger != nil {
				p.logger.Debug(ctx, "Could not delete directory (may not be empty)", logging.Fields{
					"path": path,
				})
			}
		} else {
			report.Stats.DirsDeleted.Add(1)
			if p.logger != nil {
				p.logger.Info(ctx, "Deleted orphan directory", logging.Fields{
					"path": path,
				})
			}
		}
	}

	// Remove deleted files from destFiles map so they don't appear in buildReport
	if !p.operation.DryRun {
		p.destFilesMu.Lock()
		for _, path := range orphanFiles {
			delete(p.destFiles, path)
		}
		p.destFilesMu.Unlock()
	}
}
