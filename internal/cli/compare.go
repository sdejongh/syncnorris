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

// NewCompareCommand creates the compare command
func NewCompareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare folders without syncing (dry-run)",
		Long: `Compare source and destination folders and report differences
without performing any file operations. This is equivalent to sync --dry-run.`,
		RunE: runCompare,
	}

	// Reuse sync flags for comparison
	cmd.Flags().StringVarP(&syncFlags.Source, "source", "s", "", "source directory path (required)")
	cmd.Flags().StringVarP(&syncFlags.Dest, "dest", "d", "", "destination directory path (required)")
	cmd.MarkFlagRequired("source")
	cmd.MarkFlagRequired("dest")

	cmd.Flags().StringVar(&syncFlags.Comparison, "comparison", "hash", "comparison method: namesize, md5, binary, hash")
	cmd.Flags().StringSliceVar(&syncFlags.Exclude, "exclude", []string{}, "glob patterns to exclude")
	cmd.Flags().StringVarP(&syncFlags.Output, "output", "o", "human", "output format: human, json")
	cmd.Flags().StringVar(&syncFlags.DiffReport, "diff-report", "", "write differences report to file")
	cmd.Flags().StringVar(&syncFlags.DiffFormat, "diff-format", "human", "differences report format: human, json")

	return cmd
}

func runCompare(cmd *cobra.Command, args []string) error {
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

	// Force dry-run mode for compare command
	syncFlags.DryRun = true

	// Create sync operation (with dry-run enabled)
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
		comparator = compare.NewCompositeComparator(false, cfg.Performance.BufferSize)

	case models.CompareHash:
		// Secure: SHA-256 hash comparison
		comparator = compare.NewCompositeComparator(true, cfg.Performance.BufferSize)

	case models.CompareMD5:
		// Fast hash: MD5 comparison
		comparator = compare.NewMD5Comparator(cfg.Performance.BufferSize)

	case models.CompareBinary:
		// Thorough: byte-by-byte comparison
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

	// Run comparison (dry-run sync)
	report, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("comparison failed: %w", err)
	}

	// Write differences report if requested
	if syncFlags.DiffReport != "" {
		if err := output.WriteDifferencesReport(report, syncFlags.DiffReport, syncFlags.DiffFormat); err != nil {
			return fmt.Errorf("failed to write differences report: %w", err)
		}
	}

	// Exit with appropriate code
	os.Exit(report.Status.ExitCode())
	return nil
}
