package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/ratelimit"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// BidirectionalPipeline handles bidirectional synchronization
type BidirectionalPipeline struct {
	source      storage.Backend
	dest        storage.Backend
	comparator  compare.Comparator
	formatter   output.Formatter
	logger      logging.Logger
	operation   *models.SyncOperation
	config      PipelineConfig
	state       *SyncState
	rateLimiter *ratelimit.Limiter

	// Synchronization
	resultsMu sync.Mutex
}

// NewBidirectionalPipeline creates a new bidirectional sync pipeline
func NewBidirectionalPipeline(
	source, dest storage.Backend,
	comparator compare.Comparator,
	formatter output.Formatter,
	logger logging.Logger,
	operation *models.SyncOperation,
	config PipelineConfig,
) *BidirectionalPipeline {
	return &BidirectionalPipeline{
		source:     source,
		dest:       dest,
		comparator: comparator,
		formatter:  formatter,
		logger:     logger,
		operation:  operation,
		config:     config,
	}
}

// Run executes the bidirectional sync
func (p *BidirectionalPipeline) Run(ctx context.Context) (*models.SyncReport, error) {
	startTime := time.Now()

	// Initialize report
	report := &models.SyncReport{
		OperationID: p.operation.ID,
		SourcePath:  p.operation.SourcePath,
		DestPath:    p.operation.DestPath,
		Mode:        p.operation.Mode,
		DryRun:      p.operation.DryRun,
		StartTime:   startTime,
		Status:      models.StatusSuccess,
	}

	// Load previous state
	var err error
	p.state, err = LoadState(p.operation.SourcePath, p.operation.DestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load sync state: %w", err)
	}

	// Initialize rate limiter if bandwidth limit is set
	if p.operation.BandwidthLimit > 0 {
		p.rateLimiter = ratelimit.NewLimiter(p.operation.BandwidthLimit)
	}

	// Configure comparator with rate limiter if supported
	if p.rateLimiter != nil {
		if comp, ok := p.comparator.(compare.RateLimitedComparator); ok {
			comp.SetReaderWrapper(func(rc io.ReadCloser) io.ReadCloser {
				return ratelimit.NewReadCloser(ctx, rc, p.rateLimiter)
			})
		}
	}

	// Start formatter (we'll update totals after scanning)
	p.formatter.Start(os.Stdout, 0, 0, p.config.MaxWorkers)

	// Phase 1: Scan both sides and detect changes
	sourceFiles, destFiles, err := p.scanBothSides(ctx, report)
	if err != nil {
		p.formatter.Complete(report)
		return report, fmt.Errorf("scan failed: %w", err)
	}

	// Phase 2: Analyze changes and detect conflicts
	actions, conflicts := p.analyzeChanges(ctx, sourceFiles, destFiles, report)

	// Phase 3: Handle conflicts according to resolution strategy
	resolvedActions := p.resolveConflicts(ctx, conflicts, report)
	actions = append(actions, resolvedActions...)

	// Phase 4: Execute sync actions
	if err := p.executeActions(ctx, actions, report); err != nil {
		report.Status = models.StatusPartial
	}

	// Update state if not dry-run
	if !p.operation.DryRun {
		p.state.MarkSyncComplete()
		if err := p.state.Save(); err != nil {
			// Log error but don't fail the sync
			if p.logger != nil {
				p.logger.Error(ctx, "Failed to save sync state", err, logging.Fields{"path": p.operation.SourcePath})
			}
		}
	}

	// Finalize report
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	// Determine final status
	if report.Stats.FilesErrored.Load() > 0 {
		if report.Stats.FilesCopied.Load() > 0 || report.Stats.FilesUpdated.Load() > 0 {
			report.Status = models.StatusPartial
		} else {
			report.Status = models.StatusFailed
		}
	}

	p.formatter.Complete(report)

	return report, nil
}

// scanBothSides scans source and destination to get current file lists
func (p *BidirectionalPipeline) scanBothSides(ctx context.Context, report *models.SyncReport) (map[string]*models.FileEntry, map[string]*models.FileEntry, error) {
	var sourceFiles, destFiles map[string]*models.FileEntry
	var sourceErr, destErr error
	var wg sync.WaitGroup

	wg.Add(2)

	// Scan source
	go func() {
		defer wg.Done()
		sourceFiles, sourceErr = p.scanSide(ctx, p.source, p.operation.SourcePath, report, true)
	}()

	// Scan destination
	go func() {
		defer wg.Done()
		destFiles, destErr = p.scanSide(ctx, p.dest, p.operation.DestPath, report, false)
	}()

	wg.Wait()

	if sourceErr != nil {
		return nil, nil, fmt.Errorf("source scan failed: %w", sourceErr)
	}
	if destErr != nil {
		return nil, nil, fmt.Errorf("destination scan failed: %w", destErr)
	}

	return sourceFiles, destFiles, nil
}

// scanSide scans one side of the sync
func (p *BidirectionalPipeline) scanSide(ctx context.Context, backend storage.Backend, rootPath string, report *models.SyncReport, isSource bool) (map[string]*models.FileEntry, error) {
	files := make(map[string]*models.FileEntry)

	entries, err := backend.List(ctx, "")
	if err != nil {
		return nil, err
	}

	for _, storageEntry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Apply exclude patterns
		if shouldExclude(storageEntry.RelativePath, p.operation.ExcludePatterns) {
			report.Stats.FilesSkipped.Add(1)
			continue
		}

		// Convert storage.FileInfo to models.FileEntry
		entry := &models.FileEntry{
			RelativePath: storageEntry.RelativePath,
			AbsolutePath: storageEntry.Path,
			Size:         storageEntry.Size,
			ModTime:      storageEntry.ModTime,
			IsDir:        storageEntry.IsDir,
			Permissions:  storageEntry.Permissions,
		}

		if entry.IsDir {
			if isSource {
				report.Stats.SourceDirsScanned.Add(1)
			} else {
				report.Stats.DestDirsScanned.Add(1)
			}
		} else {
			if isSource {
				report.Stats.SourceFilesScanned.Add(1)
			} else {
				report.Stats.DestFilesScanned.Add(1)
			}
		}

		files[entry.RelativePath] = entry
	}

	return files, nil
}

// analyzeChanges compares current state with previous state to determine actions
func (p *BidirectionalPipeline) analyzeChanges(ctx context.Context, sourceFiles, destFiles map[string]*models.FileEntry, report *models.SyncReport) ([]*SyncAction, []*models.Conflict) {
	var actions []*SyncAction
	var conflicts []*models.Conflict

	// Collect all unique paths
	allPaths := make(map[string]bool)
	for path := range sourceFiles {
		allPaths[path] = true
	}
	for path := range destFiles {
		allPaths[path] = true
	}
	for path := range p.state.Files {
		allPaths[path] = true
	}

	for path := range allPaths {
		select {
		case <-ctx.Done():
			return actions, conflicts
		default:
		}

		sourceEntry := sourceFiles[path]
		destEntry := destFiles[path]
		oldState := p.state.GetFileState(path)

		action, conflict := p.analyzeFile(path, sourceEntry, destEntry, oldState)

		if conflict != nil {
			conflicts = append(conflicts, conflict)
			// Note: conflicts are added to report after resolution in resolveConflicts
		} else if action != nil {
			actions = append(actions, action)
		}
	}

	return actions, conflicts
}

// SyncAction represents an action to perform during sync
type SyncAction struct {
	Path        string
	ActionType  models.Action
	Direction   SyncDirection
	SourceEntry *models.FileEntry
	DestEntry   *models.FileEntry
	Reason      string
}

// SyncDirection indicates which way to sync
type SyncDirection string

const (
	DirectionSourceToDest SyncDirection = "source_to_dest"
	DirectionDestToSource SyncDirection = "dest_to_source"
	DirectionBoth         SyncDirection = "both" // For deletions that need to be propagated
)

// analyzeFile determines what action to take for a single file
func (p *BidirectionalPipeline) analyzeFile(path string, sourceEntry, destEntry *models.FileEntry, oldState *FileState) (*SyncAction, *models.Conflict) {
	sourceExists := sourceEntry != nil
	destExists := destEntry != nil

	// First sync: no previous state
	if oldState == nil {
		return p.analyzeFirstSync(path, sourceEntry, destEntry)
	}

	// Detect changes on each side
	sourceChange := p.detectChangeType(sourceEntry, oldState, true)
	destChange := p.detectChangeType(destEntry, oldState, false)

	// Both sides unchanged - nothing to do
	if sourceChange == ChangeNone && destChange == ChangeNone {
		return nil, nil
	}

	// Only source changed
	if sourceChange != ChangeNone && destChange == ChangeNone {
		return p.createActionFromChange(path, sourceEntry, destEntry, sourceChange, DirectionSourceToDest)
	}

	// Only dest changed
	if sourceChange == ChangeNone && destChange != ChangeNone {
		return p.createActionFromChange(path, sourceEntry, destEntry, destChange, DirectionDestToSource)
	}

	// Both sides changed - potential conflict
	conflict := &models.Conflict{
		Path:        path,
		SourceEntry: sourceEntry,
		DestEntry:   destEntry,
		DetectedAt:  time.Now(),
	}

	// Determine conflict type
	if sourceChange == ChangeDeleted && destChange == ChangeModified {
		conflict.Type = models.ConflictDeleteModify
	} else if sourceChange == ChangeModified && destChange == ChangeDeleted {
		conflict.Type = models.ConflictModifyDelete
	} else if sourceChange == ChangeCreated && destChange == ChangeCreated {
		conflict.Type = models.ConflictCreateCreate
	} else {
		conflict.Type = models.ConflictModifyModify
	}

	// Check if files are actually identical (same change made on both sides)
	if sourceExists && destExists && sourceEntry.Size == destEntry.Size {
		// Files might be identical - this isn't a conflict
		// Compare content if sizes match
		return &SyncAction{
			Path:        path,
			ActionType:  models.ActionSkip,
			SourceEntry: sourceEntry,
			DestEntry:   destEntry,
			Reason:      "files may be identical, needs verification",
		}, nil
	}

	return nil, conflict
}

// analyzeFirstSync handles the case when there's no previous state
func (p *BidirectionalPipeline) analyzeFirstSync(path string, sourceEntry, destEntry *models.FileEntry) (*SyncAction, *models.Conflict) {
	sourceExists := sourceEntry != nil
	destExists := destEntry != nil

	// File only in source - copy to dest
	if sourceExists && !destExists {
		return &SyncAction{
			Path:        path,
			ActionType:  models.ActionCopy,
			Direction:   DirectionSourceToDest,
			SourceEntry: sourceEntry,
			Reason:      "file only in source (first sync)",
		}, nil
	}

	// File only in dest - copy to source
	if !sourceExists && destExists {
		return &SyncAction{
			Path:        path,
			ActionType:  models.ActionCopy,
			Direction:   DirectionDestToSource,
			DestEntry:   destEntry,
			Reason:      "file only in destination (first sync)",
		}, nil
	}

	// File in both - check if they're identical or conflict
	if sourceExists && destExists {
		// If same size and modtime, assume identical
		if sourceEntry.Size == destEntry.Size {
			timeDiff := sourceEntry.ModTime.Sub(destEntry.ModTime)
			if timeDiff < time.Second && timeDiff > -time.Second {
				return &SyncAction{
					Path:        path,
					ActionType:  models.ActionSkip,
					SourceEntry: sourceEntry,
					DestEntry:   destEntry,
					Reason:      "files appear identical",
				}, nil
			}
		}

		// Files differ - conflict on first sync
		return nil, &models.Conflict{
			Path:        path,
			SourceEntry: sourceEntry,
			DestEntry:   destEntry,
			Type:        models.ConflictCreateCreate,
			DetectedAt:  time.Now(),
		}
	}

	return nil, nil
}

// detectChangeType determines what kind of change occurred on one side
func (p *BidirectionalPipeline) detectChangeType(entry *models.FileEntry, oldState *FileState, isSource bool) ChangeType {
	exists := entry != nil

	var existedBefore bool
	if isSource {
		existedBefore = oldState.ExistsInSource
	} else {
		existedBefore = oldState.ExistsInDest
	}

	if !existedBefore && exists {
		return ChangeCreated
	}

	if existedBefore && !exists {
		return ChangeDeleted
	}

	if existedBefore && exists {
		// Check for modification
		if entry.Size != oldState.Size {
			return ChangeModified
		}
		if entry.ModTime.After(oldState.ModTime.Add(time.Second)) {
			return ChangeModified
		}
	}

	return ChangeNone
}

// createActionFromChange creates a sync action from a detected change
func (p *BidirectionalPipeline) createActionFromChange(path string, sourceEntry, destEntry *models.FileEntry, change ChangeType, direction SyncDirection) (*SyncAction, *models.Conflict) {
	switch change {
	case ChangeCreated, ChangeModified:
		actionType := models.ActionCopy
		if direction == DirectionSourceToDest && destEntry != nil {
			actionType = models.ActionUpdate
		} else if direction == DirectionDestToSource && sourceEntry != nil {
			actionType = models.ActionUpdate
		}

		return &SyncAction{
			Path:        path,
			ActionType:  actionType,
			Direction:   direction,
			SourceEntry: sourceEntry,
			DestEntry:   destEntry,
			Reason:      fmt.Sprintf("file %s on %s", change, direction),
		}, nil

	case ChangeDeleted:
		// Propagate deletion to the other side
		if direction == DirectionSourceToDest {
			// Deleted from source, delete from dest
			return &SyncAction{
				Path:        path,
				ActionType:  models.ActionDelete,
				Direction:   DirectionSourceToDest,
				DestEntry:   destEntry,
				Reason:      "deleted from source, propagating to destination",
			}, nil
		} else {
			// Deleted from dest, delete from source
			return &SyncAction{
				Path:        path,
				ActionType:  models.ActionDelete,
				Direction:   DirectionDestToSource,
				SourceEntry: sourceEntry,
				Reason:      "deleted from destination, propagating to source",
			}, nil
		}
	}

	return nil, nil
}

// resolveConflicts applies the conflict resolution strategy
func (p *BidirectionalPipeline) resolveConflicts(ctx context.Context, conflicts []*models.Conflict, report *models.SyncReport) []*SyncAction {
	var actions []*SyncAction

	for _, conflict := range conflicts {
		select {
		case <-ctx.Done():
			return actions
		default:
		}

		action := p.resolveConflict(conflict)
		if action != nil {
			actions = append(actions, action)
		}

		// Add the resolved conflict to the report
		report.Conflicts = append(report.Conflicts, *conflict)
	}

	return actions
}

// resolveConflict resolves a single conflict based on the configured strategy
func (p *BidirectionalPipeline) resolveConflict(conflict *models.Conflict) *SyncAction {
	strategy := p.operation.ConflictResolution

	switch strategy {
	case models.ConflictSourceWins:
		conflict.Resolve(strategy, models.ActionCopy)
		return &SyncAction{
			Path:        conflict.Path,
			ActionType:  models.ActionUpdate,
			Direction:   DirectionSourceToDest,
			SourceEntry: conflict.SourceEntry,
			DestEntry:   conflict.DestEntry,
			Reason:      "conflict resolved: source wins",
		}

	case models.ConflictDestWins:
		conflict.Resolve(strategy, models.ActionCopy)
		return &SyncAction{
			Path:        conflict.Path,
			ActionType:  models.ActionUpdate,
			Direction:   DirectionDestToSource,
			SourceEntry: conflict.SourceEntry,
			DestEntry:   conflict.DestEntry,
			Reason:      "conflict resolved: destination wins",
		}

	case models.ConflictNewer:
		// Use the newer file
		if conflict.SourceEntry != nil && conflict.DestEntry != nil {
			if conflict.SourceEntry.ModTime.After(conflict.DestEntry.ModTime) {
				conflict.Resolve(strategy, models.ActionCopy)
				return &SyncAction{
					Path:        conflict.Path,
					ActionType:  models.ActionUpdate,
					Direction:   DirectionSourceToDest,
					SourceEntry: conflict.SourceEntry,
					DestEntry:   conflict.DestEntry,
					Reason:      "conflict resolved: newer file (source)",
				}
			} else {
				conflict.Resolve(strategy, models.ActionCopy)
				return &SyncAction{
					Path:        conflict.Path,
					ActionType:  models.ActionUpdate,
					Direction:   DirectionDestToSource,
					SourceEntry: conflict.SourceEntry,
					DestEntry:   conflict.DestEntry,
					Reason:      "conflict resolved: newer file (destination)",
				}
			}
		}
		// Handle delete-modify conflicts with newer strategy
		if conflict.SourceEntry == nil && conflict.DestEntry != nil {
			conflict.Resolve(strategy, models.ActionCopy)
			return &SyncAction{
				Path:        conflict.Path,
				ActionType:  models.ActionCopy,
				Direction:   DirectionDestToSource,
				DestEntry:   conflict.DestEntry,
				Reason:      "conflict resolved: keeping modified file",
			}
		}
		if conflict.SourceEntry != nil && conflict.DestEntry == nil {
			conflict.Resolve(strategy, models.ActionCopy)
			return &SyncAction{
				Path:        conflict.Path,
				ActionType:  models.ActionCopy,
				Direction:   DirectionSourceToDest,
				SourceEntry: conflict.SourceEntry,
				Reason:      "conflict resolved: keeping modified file",
			}
		}

	case models.ConflictBoth:
		// Keep both files with renamed conflict copy
		// This is more complex - we need to create a renamed copy
		conflict.Resolve(strategy, models.ActionConflict)
		return &SyncAction{
			Path:        conflict.Path,
			ActionType:  models.ActionConflict,
			Direction:   DirectionBoth,
			SourceEntry: conflict.SourceEntry,
			DestEntry:   conflict.DestEntry,
			Reason:      "conflict: keeping both files",
		}

	case models.ConflictAsk:
		// In non-interactive mode, skip conflicts
		// TODO: Implement interactive conflict resolution
		return nil

	default:
		// Skip unresolved conflicts
		return nil
	}

	return nil
}

// executeActions performs all sync actions
func (p *BidirectionalPipeline) executeActions(ctx context.Context, actions []*SyncAction, report *models.SyncReport) error {
	// Sort actions: directories first, then files (for proper creation order)
	sort.Slice(actions, func(i, j int) bool {
		iIsDir := (actions[i].SourceEntry != nil && actions[i].SourceEntry.IsDir) ||
			(actions[i].DestEntry != nil && actions[i].DestEntry.IsDir)
		jIsDir := (actions[j].SourceEntry != nil && actions[j].SourceEntry.IsDir) ||
			(actions[j].DestEntry != nil && actions[j].DestEntry.IsDir)

		if iIsDir != jIsDir {
			return iIsDir // Directories first
		}
		return actions[i].Path < actions[j].Path
	})

	var hasErrors bool

	for _, action := range actions {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := p.executeAction(ctx, action, report)
		if err != nil {
			hasErrors = true
			report.Stats.FilesErrored.Add(1)
			report.Errors = append(report.Errors, models.SyncError{
				FilePath:  action.Path,
				Operation: action.ActionType,
				Error:     err.Error(),
				Timestamp: time.Now(),
			})

			p.resultsMu.Lock()
			report.Differences = append(report.Differences, models.FileDifference{
				RelativePath: action.Path,
				Reason:       models.ReasonCopyError,
				Details:      err.Error(),
			})
			p.resultsMu.Unlock()
		}
	}

	if hasErrors {
		return fmt.Errorf("some actions failed")
	}
	return nil
}

// executeAction performs a single sync action
func (p *BidirectionalPipeline) executeAction(ctx context.Context, action *SyncAction, report *models.SyncReport) error {
	if p.operation.DryRun {
		// Just report what would happen
		p.reportDryRunAction(action, report)
		return nil
	}

	switch action.ActionType {
	case models.ActionCopy:
		return p.executeCopy(ctx, action, report)

	case models.ActionUpdate:
		return p.executeUpdate(ctx, action, report)

	case models.ActionDelete:
		return p.executeDelete(ctx, action, report)

	case models.ActionSkip:
		report.Stats.FilesSynchronized.Add(1)
		return nil

	case models.ActionConflict:
		return p.executeConflictBoth(ctx, action, report)
	}

	return nil
}

// executeCopy copies a file in the specified direction
func (p *BidirectionalPipeline) executeCopy(ctx context.Context, action *SyncAction, report *models.SyncReport) error {
	var srcBackend, dstBackend storage.Backend
	var srcEntry *models.FileEntry

	if action.Direction == DirectionSourceToDest {
		srcBackend = p.source
		dstBackend = p.dest
		srcEntry = action.SourceEntry
	} else {
		srcBackend = p.dest
		dstBackend = p.source
		srcEntry = action.DestEntry
	}

	if srcEntry == nil {
		return fmt.Errorf("source entry is nil")
	}

	if srcEntry.IsDir {
		// Create directory
		err := dstBackend.MkdirAll(ctx, action.Path)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		report.Stats.DirsCreated.Add(1)
	} else {
		// Copy file
		reader, err := srcBackend.Read(ctx, action.Path)
		if err != nil {
			return fmt.Errorf("failed to read source: %w", err)
		}
		defer reader.Close()

		// Apply rate limiting
		var readerToUse io.Reader = reader
		if p.rateLimiter != nil {
			readerToUse = ratelimit.NewReader(ctx, reader, p.rateLimiter)
		}

		metadata := &storage.FileInfo{
			Size:        srcEntry.Size,
			ModTime:     srcEntry.ModTime,
			Permissions: srcEntry.Permissions,
		}

		err = dstBackend.Write(ctx, action.Path, readerToUse, srcEntry.Size, metadata)
		if err != nil {
			return fmt.Errorf("failed to write destination: %w", err)
		}

		report.Stats.FilesCopied.Add(1)
		report.Stats.BytesTransferred.Add(srcEntry.Size)
	}

	// Update state
	p.updateStateForFile(action.Path, srcEntry, true, true)

	return nil
}

// executeUpdate updates an existing file
func (p *BidirectionalPipeline) executeUpdate(ctx context.Context, action *SyncAction, report *models.SyncReport) error {
	err := p.executeCopy(ctx, action, report)
	if err != nil {
		return err
	}

	// Adjust stats (executeCopy incremented FilesCopied)
	report.Stats.FilesCopied.Add(-1)
	report.Stats.FilesUpdated.Add(1)

	return nil
}

// executeDelete deletes a file from the target side
func (p *BidirectionalPipeline) executeDelete(ctx context.Context, action *SyncAction, report *models.SyncReport) error {
	var backend storage.Backend
	if action.Direction == DirectionSourceToDest {
		backend = p.dest
	} else {
		backend = p.source
	}

	err := backend.Delete(ctx, action.Path)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	report.Stats.FilesDeleted.Add(1)

	// Update state - file no longer exists on either side
	p.state.RemoveFile(action.Path)

	return nil
}

// executeConflictBoth handles the "keep both" conflict resolution
func (p *BidirectionalPipeline) executeConflictBoth(ctx context.Context, action *SyncAction, report *models.SyncReport) error {
	// Create conflict copies on both sides
	// source_file -> dest_file.source-conflict
	// dest_file -> source_file.dest-conflict

	basePath := action.Path
	ext := filepath.Ext(basePath)
	nameWithoutExt := basePath[:len(basePath)-len(ext)]

	if action.SourceEntry != nil {
		// Copy source version to dest with .source-conflict suffix
		conflictPath := nameWithoutExt + ".source-conflict" + ext

		reader, err := p.source.Read(ctx, basePath)
		if err != nil {
			return fmt.Errorf("failed to read source for conflict copy: %w", err)
		}
		defer reader.Close()

		metadata := &storage.FileInfo{
			Size:        action.SourceEntry.Size,
			ModTime:     action.SourceEntry.ModTime,
			Permissions: action.SourceEntry.Permissions,
		}

		err = p.dest.Write(ctx, conflictPath, reader, action.SourceEntry.Size, metadata)
		if err != nil {
			return fmt.Errorf("failed to write source conflict copy: %w", err)
		}
	}

	if action.DestEntry != nil {
		// Copy dest version to source with .dest-conflict suffix
		conflictPath := nameWithoutExt + ".dest-conflict" + ext

		reader, err := p.dest.Read(ctx, basePath)
		if err != nil {
			return fmt.Errorf("failed to read dest for conflict copy: %w", err)
		}
		defer reader.Close()

		metadata := &storage.FileInfo{
			Size:        action.DestEntry.Size,
			ModTime:     action.DestEntry.ModTime,
			Permissions: action.DestEntry.Permissions,
		}

		err = p.source.Write(ctx, conflictPath, reader, action.DestEntry.Size, metadata)
		if err != nil {
			return fmt.Errorf("failed to write dest conflict copy: %w", err)
		}
	}

	return nil
}

// reportDryRunAction reports what would happen in dry-run mode
func (p *BidirectionalPipeline) reportDryRunAction(action *SyncAction, report *models.SyncReport) {
	var reason models.DifferenceReason
	var details string

	switch action.ActionType {
	case models.ActionCopy:
		reason = models.ReasonOnlyInSource
		if action.Direction == DirectionDestToSource {
			reason = models.ReasonOnlyInDest
		}
		details = fmt.Sprintf("would copy %s (%s)", action.Path, action.Direction)

	case models.ActionUpdate:
		reason = models.ReasonContentDiff
		details = fmt.Sprintf("would update %s (%s)", action.Path, action.Direction)

	case models.ActionDelete:
		reason = models.ReasonDeleted
		details = fmt.Sprintf("would delete %s (dry-run)", action.Path)

	case models.ActionSkip:
		return // Don't report skipped files

	case models.ActionConflict:
		reason = models.ReasonContentDiff
		details = "conflict: would keep both files"
	}

	p.resultsMu.Lock()
	report.Differences = append(report.Differences, models.FileDifference{
		RelativePath: action.Path,
		Reason:       reason,
		Details:      details,
	})
	p.resultsMu.Unlock()
}

// updateStateForFile updates the state after a successful file operation
func (p *BidirectionalPipeline) updateStateForFile(path string, entry *models.FileEntry, existsInSource, existsInDest bool) {
	if entry == nil {
		p.state.RemoveFile(path)
		return
	}

	p.state.UpdateFile(
		path,
		entry.Size,
		entry.ModTime,
		entry.Hash,
		existsInSource,
		existsInDest,
		entry.IsDir,
	)
}
