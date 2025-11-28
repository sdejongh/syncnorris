package sync

import (
	"context"
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
	"github.com/sdejongh/syncnorris/pkg/storage"
)

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
}

// PipelineConfig holds configuration for the pipeline
type PipelineConfig struct {
	MaxWorkers int
	QueueSize  int // Buffer size for the task queue
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
	if config.QueueSize < 100 {
		config.QueueSize = 100
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

	scanErr := p.scanSourceAndQueue(ctx)

	// Signal that scanning is complete
	p.scanComplete.Store(true)
	close(p.taskQueue)

	// Wait for all workers to finish
	workersWg.Wait()

	if scanErr != nil {
		report.Status = models.StatusFailed
		return report, scanErr
	}

	// Phase 5: Delete orphan files if requested
	if p.operation.DeleteOrphans {
		p.deleteOrphanFiles(ctx, report)
	}

	// Phase 6: Collect results and build report
	p.buildReport(report)

	// Complete formatter
	if p.formatter != nil {
		p.formatter.Complete(report)
	}

	// Finalize report
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

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
	destFiles, err := p.dest.List(ctx, "")
	if err != nil {
		return err
	}

	p.destFilesMu.Lock()
	defer p.destFilesMu.Unlock()

	for i := range destFiles {
		if destFiles[i].IsDir {
			// Skip root directory
			if destFiles[i].RelativePath != "." {
				p.destDirs[destFiles[i].RelativePath] = &destFiles[i]
			}
		} else {
			p.destFiles[destFiles[i].RelativePath] = &destFiles[i]
		}
	}

	return nil
}

// scanSourceAndQueue scans source files and adds them to the queue
func (p *Pipeline) scanSourceAndQueue(ctx context.Context) error {
	sourceFiles, err := p.source.List(ctx, "")
	if err != nil {
		return err
	}

	for _, f := range sourceFiles {
		// Skip directories
		if f.IsDir {
			continue
		}

		// Update totals
		p.totalFiles.Add(1)
		p.totalBytes.Add(f.Size)

		// Update formatter with new totals
		if p.formatter != nil {
			p.formatter.Progress(output.ProgressUpdate{
				Type:            "scan_progress",
				TotalFiles:      int(p.totalFiles.Load()),
				TotalBytes:      p.totalBytes.Load(),
			})
		}

		// Create task and add to queue
		task := NewFileTask(f.RelativePath, f.Size, f.ModTime)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.taskQueue <- task:
			// Task added to queue
		}
	}

	// Store source stats
	return nil
}

// runWorker is the worker goroutine that processes tasks
func (p *Pipeline) runWorker(ctx context.Context, workerID int, report *models.SyncReport, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
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

	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:        "file_start",
			FilePath:    task.RelativePath,
			TotalBytes:  task.Size,
			CurrentFile: fileIndex,
		})
	}

	// Step 1: Verify source file exists and is readable
	sourceReader, err := p.source.Read(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

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
	sourceReader.Close() // Close immediately, we'll reopen if needed

	// Step 2: Check if destination file exists
	p.destFilesMu.RLock()
	destInfo, destExists := p.destFiles[task.RelativePath]
	p.destFilesMu.RUnlock()

	if !destExists {
		// File doesn't exist in destination - copy it
		p.copyFile(ctx, workerID, task, report, fileIndex, startTime)
		return
	}

	// Step 3: File exists in both - compare them
	if p.formatter != nil {
		p.formatter.Progress(output.ProgressUpdate{
			Type:        "compare_start",
			FilePath:    task.RelativePath,
			TotalBytes:  task.Size,
			CurrentFile: fileIndex,
		})
	}

	comparison, err := p.comparator.Compare(ctx, p.source, p.dest, task.RelativePath, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

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
	if p.operation.DryRun {
		task.MarkCompleted(ResultCopied, task.Size, time.Since(startTime))
		report.Stats.FilesCopied.Add(1)
		p.processedBytes.Add(task.Size)
		p.addResult(task)

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

	// Get source metadata
	sourceInfo, err := p.source.Stat(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

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

	sourceInfo, err := p.source.Stat(ctx, task.RelativePath)
	if err != nil {
		task.MarkError(err, time.Since(startTime))
		report.Stats.FilesErrored.Add(1)
		p.recordError(report, task)
		p.addResult(task)

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
