package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
	"github.com/sdejongh/syncnorris/pkg/sync"
)

// SyncFlags holds sync command flags
type SyncFlags struct {
	Source       string
	Dest         string
	Mode         string
	Comparison   string
	Conflict     string
	DryRun       bool
	CreateDest   bool
	Parallel     int
	Bandwidth    string
	Exclude      []string
	Output       string
	DiffReport   string
	DiffFormat   string
}

var syncFlags SyncFlags

// NewSyncCommand creates the sync command
func NewSyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize two folders",
		Long: `Synchronize files between source and destination directories.
Supports one-way and bidirectional sync with multiple comparison methods.`,
		RunE: runSync,
	}

	// Required flags
	cmd.Flags().StringVarP(&syncFlags.Source, "source", "s", "", "source directory path (required)")
	cmd.Flags().StringVarP(&syncFlags.Dest, "dest", "d", "", "destination directory path (required)")
	cmd.MarkFlagRequired("source")
	cmd.MarkFlagRequired("dest")

	// Optional flags
	cmd.Flags().StringVarP(&syncFlags.Mode, "mode", "m", "oneway", "sync mode: oneway, bidirectional")
	cmd.Flags().StringVar(&syncFlags.Comparison, "comparison", "hash", "comparison method: namesize, md5, binary, hash")
	cmd.Flags().StringVar(&syncFlags.Conflict, "conflict", "ask", "conflict resolution: ask, source-wins, dest-wins, newer, both")
	cmd.Flags().BoolVar(&syncFlags.DryRun, "dry-run", false, "compare only, don't sync")
	cmd.Flags().BoolVar(&syncFlags.CreateDest, "create-dest", false, "create destination directory if it doesn't exist")
	cmd.Flags().IntVarP(&syncFlags.Parallel, "parallel", "p", 0, "number of parallel workers (default: 5)")
	cmd.Flags().StringVarP(&syncFlags.Bandwidth, "bandwidth", "b", "", "bandwidth limit (e.g., \"10M\", \"1G\")")
	cmd.Flags().StringSliceVar(&syncFlags.Exclude, "exclude", []string{}, "glob patterns to exclude")
	cmd.Flags().StringVarP(&syncFlags.Output, "output", "o", "human", "output format: human, json")
	cmd.Flags().StringVar(&syncFlags.DiffReport, "diff-report", "", "write differences report to file")
	cmd.Flags().StringVar(&syncFlags.DiffFormat, "diff-format", "human", "differences report format: human, json")

	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate flags
	if err := validateSyncFlags(); err != nil {
		return err
	}

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command-line flags
	applyFlagsToConfig(cfg)

	// Create sync operation
	operation, err := createSyncOperation(cfg)
	if err != nil {
		return fmt.Errorf("failed to create sync operation: %w", err)
	}

	// Create storage backends
	source, err := storage.NewLocal(syncFlags.Source)
	if err != nil {
		return fmt.Errorf("failed to create source backend: %w", err)
	}
	defer source.Close()

	dest, err := storage.NewLocal(syncFlags.Dest)
	if err != nil {
		return fmt.Errorf("failed to create destination backend: %w", err)
	}
	defer dest.Close()

	// Create comparator
	var comparator compare.Comparator
	switch operation.ComparisonMethod {
	case models.CompareNameSize:
		// Fast: name+size only, no hash verification
		// Uses composite comparator without hash stage
		comparator = compare.NewCompositeComparator(false, cfg.Performance.BufferSize)

	case models.CompareHash:
		// Secure: SHA-256 hash comparison
		// Uses composite comparator with SHA-256 hash
		comparator = compare.NewCompositeComparator(true, cfg.Performance.BufferSize)

	case models.CompareMD5:
		// Fast hash: MD5 comparison (faster than SHA-256, less secure)
		// Suitable for non-critical data where speed matters
		comparator = compare.NewMD5Comparator(cfg.Performance.BufferSize)

	case models.CompareBinary:
		// Thorough: byte-by-byte comparison
		// Slowest but most precise (reports exact byte offset of difference)
		comparator = compare.NewBinaryComparator(cfg.Performance.BufferSize)

	default:
		return fmt.Errorf("unsupported comparison method: %s (use: namesize, md5, binary, hash)", operation.ComparisonMethod)
	}

	// Create output formatter
	var formatter output.Formatter
	if cfg.Output.Progress {
		formatter = output.NewProgressFormatter()
	} else {
		formatter = output.NewHumanFormatter()
	}

	// Create sync engine (logger is nil for now)
	engine := sync.NewEngine(source, dest, comparator, formatter, nil, operation)

	// Run sync
	report, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Write differences report if requested
	// Show report if:
	// - --diff-report is specified (write to file)
	// - --diff-format is explicitly set (write to stdout)
	if syncFlags.DiffReport != "" || cmd.Flags().Changed("diff-format") {
		if err := output.WriteDifferencesReport(report, syncFlags.DiffReport, syncFlags.DiffFormat); err != nil {
			return fmt.Errorf("failed to write differences report: %w", err)
		}
	}

	// Exit with appropriate code
	os.Exit(report.Status.ExitCode())
	return nil
}
