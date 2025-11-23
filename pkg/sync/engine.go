package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// Engine orchestrates the sync operation
type Engine struct {
	source     storage.Backend
	dest       storage.Backend
	comparator compare.Comparator
	formatter  output.Formatter
	logger     logging.Logger
	operation  *models.SyncOperation
}

// NewEngine creates a new sync engine
func NewEngine(
	source, dest storage.Backend,
	comparator compare.Comparator,
	formatter output.Formatter,
	logger logging.Logger,
	operation *models.SyncOperation,
) *Engine {
	return &Engine{
		source:     source,
		dest:       dest,
		comparator: comparator,
		formatter:  formatter,
		logger:     logger,
		operation:  operation,
	}
}

// Run executes the sync operation
func (e *Engine) Run(ctx context.Context) (*models.SyncReport, error) {
	startTime := time.Now()
	report := &models.SyncReport{
		OperationID: e.operation.ID,
		SourcePath:  e.operation.SourcePath,
		DestPath:    e.operation.DestPath,
		Mode:        e.operation.Mode,
		DryRun:      e.operation.DryRun,
		StartTime:   startTime,
		Status:      models.StatusSuccess,
	}

	if e.logger != nil {
		e.logger.Info(ctx, "Starting sync operation", logging.Fields{
			"operation_id": e.operation.ID,
			"source":       e.operation.SourcePath,
			"dest":         e.operation.DestPath,
			"mode":         e.operation.Mode,
			"comparison":   e.operation.ComparisonMethod,
			"dry_run":      e.operation.DryRun,
		})
	}

	// Phase 1: Scan both sides
	if e.logger != nil {
		e.logger.Info(ctx, "Scanning source directory", nil)
	}
	sourceFiles, err := e.source.List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to scan source: %w", err)
	}

	if e.logger != nil {
		e.logger.Info(ctx, "Scanning destination directory", nil)
	}
	destFiles, err := e.dest.List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to scan destination: %w", err)
	}

	// Count files and directories separately
	sourceFileCount := 0
	sourceDirCount := 0
	for _, f := range sourceFiles {
		if f.IsDir {
			sourceDirCount++
		} else {
			sourceFileCount++
		}
	}

	destFileCount := 0
	destDirCount := 0
	for _, f := range destFiles {
		if f.IsDir {
			destDirCount++
		} else {
			destFileCount++
		}
	}

	// Store source and destination counts separately (using atomic Store)
	report.Stats.SourceFilesScanned.Store(int32(sourceFileCount))
	report.Stats.SourceDirsScanned.Store(int32(sourceDirCount))
	report.Stats.DestFilesScanned.Store(int32(destFileCount))
	report.Stats.DestDirsScanned.Store(int32(destDirCount))

	// Count unique paths for total scanned
	uniqueFilePaths := make(map[string]bool)
	uniqueDirPaths := make(map[string]bool)

	for _, f := range sourceFiles {
		if f.IsDir {
			uniqueDirPaths[f.RelativePath] = true
		} else {
			uniqueFilePaths[f.RelativePath] = true
		}
	}

	for _, f := range destFiles {
		if f.IsDir {
			uniqueDirPaths[f.RelativePath] = true
		} else {
			uniqueFilePaths[f.RelativePath] = true
		}
	}

	report.Stats.FilesScanned.Store(int32(len(uniqueFilePaths)))
	report.Stats.DirsScanned.Store(int32(len(uniqueDirPaths)))

	// Phase 2: Compare files
	if e.logger != nil {
		e.logger.Info(ctx, "Comparing files", logging.Fields{
			"source_files": len(sourceFiles),
			"dest_files":   len(destFiles),
		})
	}

	// Count total files to process (for progress display during comparison)
	totalFilesToProcess := 0
	totalBytesToProcess := int64(0)
	for _, f := range sourceFiles {
		if !f.IsDir {
			totalFilesToProcess++
			totalBytesToProcess += f.Size
		}
	}

	// Initialize formatter before comparison phase
	if e.formatter != nil {
		e.formatter.Start(nil, totalFilesToProcess, totalBytesToProcess)
	}

	operations, err := e.planOperations(ctx, sourceFiles, destFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to plan operations: %w", err)
	}

	report.Operations = operations

	// Calculate total bytes to transfer (for actual copy/update operations)
	var totalBytes int64
	var totalFiles int
	for _, op := range operations {
		if op.Action == models.ActionCopy || op.Action == models.ActionUpdate {
			totalBytes += op.Entry.Size
			totalFiles++
		}
	}

	// Phase 3: Execute operations (unless dry-run)
	if e.operation.DryRun {
		if e.logger != nil {
			e.logger.Info(ctx, "Dry-run mode: skipping execution", nil)
		}

		// Count operations by action type for dry-run report
		// Use the same logic as worker.go to distinguish synchronized vs skipped
		for _, op := range operations {
			switch op.Action {
			case models.ActionCopy:
				report.Stats.FilesCopied.Add(1)
			case models.ActionUpdate:
				report.Stats.FilesUpdated.Add(1)
			case models.ActionSkip:
				// Distinguish between synchronized (identical) and skipped (excluded/other)
				if op.Reason == "files are identical" {
					report.Stats.FilesSynchronized.Add(1)
				} else {
					report.Stats.FilesSkipped.Add(1)
				}
			case models.ActionDelete:
				// File deletions would be counted here in bidirectional mode
			}
		}

		// Display comparison results in dry-run mode
		if e.formatter != nil {
			e.formatter.Complete(report)
		}
	} else {
		// Don't reinitialize the formatter - keep the comparison phase counters
		// Synchronized files will be counted as they are processed

		if e.logger != nil {
			e.logger.Info(ctx, "Executing operations", logging.Fields{
				"total_operations": len(operations),
				"total_bytes":      totalBytes,
			})
		}

		// Execute based on sync mode
		switch e.operation.Mode {
		case models.ModeOneWay:
			err = e.executeOneWay(ctx, operations, report)
		case models.ModeBidirectional:
			err = fmt.Errorf("bidirectional sync not yet implemented")
		default:
			err = fmt.Errorf("unsupported sync mode: %s", e.operation.Mode)
		}

		if err != nil {
			report.Status = models.StatusFailed
			return report, err
		}

		if e.formatter != nil {
			e.formatter.Complete(report)
		}
	}

	// Finalize report
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	if len(report.Errors) > 0 {
		if int(report.Stats.FilesErrored.Load()) == len(operations) {
			report.Status = models.StatusFailed
		} else {
			report.Status = models.StatusPartial
		}
	}

	if e.logger != nil {
		e.logger.Info(ctx, "Sync operation completed", logging.Fields{
			"duration":           report.Duration.String(),
			"status":             report.Status,
			"files_copied":       report.Stats.FilesCopied.Load(),
			"files_updated":      report.Stats.FilesUpdated.Load(),
			"files_skipped":      report.Stats.FilesSkipped.Load(),
			"files_errored":      report.Stats.FilesErrored.Load(),
			"bytes_transferred":  report.Stats.BytesTransferred.Load(),
		})
	}

	return report, nil
}

// planOperations determines what operations need to be performed
func (e *Engine) planOperations(ctx context.Context, sourceFiles, destFiles []storage.FileInfo) ([]models.FileOperation, error) {
	// Create maps for quick lookup
	sourceMap := make(map[string]*storage.FileInfo)
	destMap := make(map[string]*storage.FileInfo)

	for i := range sourceFiles {
		sourceMap[sourceFiles[i].RelativePath] = &sourceFiles[i]
	}
	for i := range destFiles {
		destMap[destFiles[i].RelativePath] = &destFiles[i]
	}

	var operations []models.FileOperation
	var mu sync.Mutex

	// Process all unique paths
	allPaths := make(map[string]bool)
	for path := range sourceMap {
		allPaths[path] = true
	}
	for path := range destMap {
		allPaths[path] = true
	}

	// Convert to slice for parallel processing
	pathList := make([]string, 0, len(allPaths))
	for path := range allPaths {
		pathList = append(pathList, path)
	}

	// Create file index mapping for progress reporting
	pathToIndex := make(map[string]int)
	fileIndex := 0
	for _, path := range pathList {
		info := sourceMap[path]
		if info == nil {
			info = destMap[path]
		}
		// Only count files, not directories
		if info != nil && !info.IsDir {
			pathToIndex[path] = fileIndex
			fileIndex++
		}
	}

	// Setup progress callback for comparator if it supports it
	if comp, ok := e.comparator.(interface {
		SetProgressCallback(func(path string, current, total int64))
	}); ok {
		comp.SetProgressCallback(func(path string, current, total int64) {
			if e.formatter != nil {
				mu.Lock()
				idx, exists := pathToIndex[path]
				mu.Unlock()
				if exists {
					// Send progress update
					e.formatter.Progress(output.ProgressUpdate{
						Type:         "file_progress",
						FilePath:     path,
						BytesWritten: current,
						TotalBytes:   total,
						CurrentFile:  idx,
					})
				}
			}
		})
	}

	// Use worker pool for parallel comparisons
	maxWorkers := e.operation.MaxWorkers
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	var wg sync.WaitGroup
	pathChan := make(chan string, len(pathList))

	// Send all paths to channel
	for _, path := range pathList {
		pathChan <- path
	}
	close(pathChan)

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathChan {
				sourceInfo := sourceMap[path]
				destInfo := destMap[path]

				// Skip directories
				if (sourceInfo != nil && sourceInfo.IsDir) || (destInfo != nil && destInfo.IsDir) {
					continue
				}

				var action models.Action
				var reason string

				if sourceInfo != nil && destInfo == nil {
					// File exists only in source
					action = models.ActionCopy
					reason = "file exists only in source"
				} else if sourceInfo == nil && destInfo != nil {
					// File exists only in dest (skip in one-way mode)
					action = models.ActionSkip
					reason = "file exists only in destination"
				} else {
					// File exists in both - compare
					// Notify formatter that comparison is starting
					if e.formatter != nil {
						mu.Lock()
						idx := pathToIndex[path]
						mu.Unlock()
						e.formatter.Progress(output.ProgressUpdate{
							Type:        "compare_start",
							FilePath:    path,
							TotalBytes:  sourceInfo.Size,
							CurrentFile: idx,
						})
					}

					comparison, err := e.comparator.Compare(ctx, e.source, e.dest, path, path)

					if err != nil {
						// Notify formatter of comparison error
						if e.formatter != nil {
							mu.Lock()
							idx := pathToIndex[path]
							mu.Unlock()
							e.formatter.Progress(output.ProgressUpdate{
								Type:        "file_error",
								FilePath:    path,
								CurrentFile: idx,
								Error:       err,
							})
						}
						mu.Lock()
						operations = append(operations, models.FileOperation{
							Entry: &models.FileEntry{
								RelativePath: path,
								AbsolutePath: sourceInfo.Path,
								Size:         sourceInfo.Size,
								ModTime:      sourceInfo.ModTime,
								IsDir:        sourceInfo.IsDir,
							},
							Action: models.ActionSkip,
							Reason: "comparison failed",
							Error:  err,
						})
						mu.Unlock()
						continue
					}

					// Determine action and notify formatter based on comparison result
					if comparison.Result == compare.Same {
						action = models.ActionSkip
						reason = "files are identical"

						// Files are identical - count them immediately as synchronized
						if e.formatter != nil {
							mu.Lock()
							idx := pathToIndex[path]
							mu.Unlock()
							e.formatter.Progress(output.ProgressUpdate{
								Type:         "file_complete",
								FilePath:     path,
								BytesWritten: sourceInfo.Size,
								CurrentFile:  idx,
							})
						}
					} else {
						action = models.ActionUpdate
						reason = comparison.Reason

						// Files are different - will be transferred later
						if e.formatter != nil {
							mu.Lock()
							idx := pathToIndex[path]
							mu.Unlock()
							e.formatter.Progress(output.ProgressUpdate{
								Type:         "compare_complete",
								FilePath:     path,
								BytesWritten: sourceInfo.Size,
								CurrentFile:  idx,
							})
						}
					}
				}

				// Use sourceInfo if available, otherwise destInfo
				var entry *models.FileEntry
				if sourceInfo != nil {
					entry = &models.FileEntry{
						RelativePath: path,
						AbsolutePath: sourceInfo.Path,
						Size:         sourceInfo.Size,
						ModTime:      sourceInfo.ModTime,
						IsDir:        sourceInfo.IsDir,
					}
				} else {
					entry = &models.FileEntry{
						RelativePath: path,
						AbsolutePath: destInfo.Path,
						Size:         destInfo.Size,
						ModTime:      destInfo.ModTime,
						IsDir:        destInfo.IsDir,
					}
				}

				mu.Lock()
				operations = append(operations, models.FileOperation{
					Entry:  entry,
					Action: action,
					Reason: reason,
				})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	return operations, nil
}

// executeOneWay executes one-way sync operations
func (e *Engine) executeOneWay(ctx context.Context, operations []models.FileOperation, report *models.SyncReport) error {
	worker := NewWorker(e.source, e.dest, e.operation.MaxWorkers)
	return worker.Execute(ctx, operations, report, e.formatter)
}
