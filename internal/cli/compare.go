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
	cmd.Flags().IntVarP(&syncFlags.Parallel, "parallel", "p", 0, "number of parallel workers (default: 5)")
	cmd.Flags().StringVarP(&syncFlags.Bandwidth, "bandwidth", "b", "", "bandwidth limit (e.g., \"10M\", \"1G\")")
	cmd.Flags().BoolVar(&syncFlags.Delete, "delete", false, "include files that would be deleted from destination")

	// Logging flags
	cmd.Flags().StringVar(&syncFlags.LogFile, "log-file", "", "write logs to file (enables logging)")
	cmd.Flags().StringVar(&syncFlags.LogFormat, "log-format", "text", "log format: text, json")
	cmd.Flags().StringVar(&syncFlags.LogLevel, "log-level", "info", "log level: debug, info, warn, error")

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

	case models.CompareTimestamp:
		// Fast: name+size+timestamp comparison
		comparator = compare.NewTimestampComparator()

	default:
		return fmt.Errorf("unsupported comparison method: %s (use: namesize, timestamp, md5, binary, hash)", operation.ComparisonMethod)
	}

	// Create output formatter
	var formatter output.Formatter
	switch syncFlags.Output {
	case "json":
		formatter = output.NewJSONFormatter()
	default:
		if cfg.Output.Progress {
			formatter = output.NewProgressFormatter()
		} else {
			formatter = output.NewHumanFormatter()
		}
	}

	// Create logger
	logger, err := createLogger(syncFlags.LogFile, syncFlags.LogFormat, syncFlags.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Close()

	// Create sync engine
	engine := sync.NewEngine(source, dest, comparator, formatter, logger, operation)

	// Run comparison (dry-run sync)
	report, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("comparison failed: %w", err)
	}

	// Write differences report for compare command
	// In JSON output mode, skip the human-readable diff report (JSON formatter handles it)
	if syncFlags.Output != "json" {
		// If no file specified, write to stdout
		if err := output.WriteDifferencesReport(report, syncFlags.DiffReport, syncFlags.DiffFormat); err != nil {
			return fmt.Errorf("failed to write differences report: %w", err)
		}
	} else if syncFlags.DiffReport != "" {
		// In JSON mode with explicit diff-report file, write JSON diff to file
		if err := output.WriteDifferencesReport(report, syncFlags.DiffReport, "json"); err != nil {
			return fmt.Errorf("failed to write differences report: %w", err)
		}
	}

	// Exit with appropriate code
	os.Exit(report.Status.ExitCode())
	return nil
}
