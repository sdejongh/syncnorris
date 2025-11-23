package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sdejongh/syncnorris/pkg/config"
	"github.com/sdejongh/syncnorris/pkg/models"
)

// validateSyncFlags validates the sync command flags
func validateSyncFlags() error {
	// Validate paths exist
	if _, err := os.Stat(syncFlags.Source); os.IsNotExist(err) {
		return fmt.Errorf("source path does not exist: %s", syncFlags.Source)
	}

	// Destination can not exist yet (will be created), but parent must exist
	destInfo, err := os.Stat(syncFlags.Dest)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to access destination path: %w", err)
	}
	if destInfo != nil && !destInfo.IsDir() {
		return fmt.Errorf("destination path exists but is not a directory: %s", syncFlags.Dest)
	}

	// Validate paths are not identical
	sourceAbs, err := filepath.Abs(syncFlags.Source)
	if err != nil {
		return fmt.Errorf("failed to resolve source path: %w", err)
	}

	destAbs, err := filepath.Abs(syncFlags.Dest)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %w", err)
	}

	if sourceAbs == destAbs {
		return fmt.Errorf("source and destination cannot be the same: %s", sourceAbs)
	}

	// Validate paths are not nested
	if strings.HasPrefix(destAbs, sourceAbs+string(filepath.Separator)) {
		return fmt.Errorf("destination cannot be inside source directory")
	}
	if strings.HasPrefix(sourceAbs, destAbs+string(filepath.Separator)) {
		return fmt.Errorf("source cannot be inside destination directory")
	}

	// Validate sync mode
	validModes := map[string]bool{
		"oneway":        true,
		"bidirectional": true,
	}
	if !validModes[syncFlags.Mode] {
		return fmt.Errorf("invalid sync mode: %s (valid: oneway, bidirectional)", syncFlags.Mode)
	}

	// Validate comparison method
	validComparisons := map[string]bool{
		"namesize":  true,
		"timestamp": true,
		"binary":    true,
		"hash":      true,
		"md5":       true,
	}
	if !validComparisons[syncFlags.Comparison] {
		return fmt.Errorf("invalid comparison method: %s (valid: namesize, timestamp, binary, hash, md5)", syncFlags.Comparison)
	}

	// Validate conflict resolution
	validConflicts := map[string]bool{
		"ask":         true,
		"source-wins": true,
		"dest-wins":   true,
		"newer":       true,
		"both":        true,
	}
	if !validConflicts[syncFlags.Conflict] {
		return fmt.Errorf("invalid conflict resolution: %s (valid: ask, source-wins, dest-wins, newer, both)", syncFlags.Conflict)
	}

	return nil
}

// loadConfig loads configuration from file or returns default
func loadConfig() (*config.Config, error) {
	if globalFlags.ConfigFile != "" {
		return config.LoadFromFile(globalFlags.ConfigFile)
	}
	return config.LoadDefault()
}

// applyFlagsToConfig overrides config values with command-line flags
func applyFlagsToConfig(cfg *config.Config) {
	// Sync mode
	if syncFlags.Mode != "" {
		cfg.Sync.Mode = models.SyncMode(syncFlags.Mode)
	}

	// Comparison method
	if syncFlags.Comparison != "" {
		cfg.Sync.Comparison = models.ComparisonMethod(syncFlags.Comparison)
	}

	// Conflict resolution
	if syncFlags.Conflict != "" {
		cfg.Sync.ConflictResolution = models.ConflictResolution(syncFlags.Conflict)
	}

	// Parallel workers
	if syncFlags.Parallel > 0 {
		cfg.Performance.MaxWorkers = syncFlags.Parallel
	} else if cfg.Performance.MaxWorkers == 0 {
		cfg.Performance.MaxWorkers = runtime.NumCPU()
	}

	// Exclude patterns
	if len(syncFlags.Exclude) > 0 {
		cfg.Exclude = syncFlags.Exclude
	}

	// Output format
	if syncFlags.Output != "" {
		cfg.Output.Format = syncFlags.Output
	}

	// Disable progress in quiet mode
	if globalFlags.Quiet {
		cfg.Output.Progress = false
		cfg.Output.Quiet = true
	}

	// Enable progress in verbose mode
	if globalFlags.Verbose {
		cfg.Output.Progress = true
	}
}

// createSyncOperation creates a sync operation from configuration
func createSyncOperation(cfg *config.Config) (*models.SyncOperation, error) {
	operation := &models.SyncOperation{
		ID:                 uuid.New().String(),
		SourcePath:         syncFlags.Source,
		DestPath:           syncFlags.Dest,
		Mode:               cfg.Sync.Mode,
		ComparisonMethod:   cfg.Sync.Comparison,
		ConflictResolution: cfg.Sync.ConflictResolution,
		ExcludePatterns:    cfg.Exclude,
		DryRun:             syncFlags.DryRun,
		MaxWorkers:         cfg.Performance.MaxWorkers,
		BandwidthLimit:     cfg.Performance.BandwidthLimit,
		BufferSize:         cfg.Performance.BufferSize,
		CreatedAt:          time.Now(),
	}

	if err := operation.Validate(); err != nil {
		return nil, err
	}

	return operation, nil
}
