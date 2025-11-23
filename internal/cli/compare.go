package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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

	return cmd
}

func runCompare(cmd *cobra.Command, args []string) error {
	// TODO: Implement compare logic
	fmt.Println("Compare command placeholder - will be implemented in Phase 3")
	return nil
}
