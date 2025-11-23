package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
)

// WriteDifferencesReport writes the differences report to a file
// Format can be "human" or "json"
func WriteDifferencesReport(report *models.SyncReport, filepath string, format string) error {
	if len(report.Differences) == 0 {
		// No differences - don't create empty file
		return nil
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create differences file: %w", err)
	}
	defer file.Close()

	switch format {
	case "json":
		return writeDifferencesJSON(report, file)
	default: // "human"
		return writeDifferencesHuman(report, file)
	}
}

// writeDifferencesHuman writes differences in human-readable format
func writeDifferencesHuman(report *models.SyncReport, w io.Writer) error {
	fmt.Fprintf(w, "Differences Report\n")
	fmt.Fprintf(w, "==================\n\n")
	fmt.Fprintf(w, "Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Source: %s\n", report.SourcePath)
	fmt.Fprintf(w, "Destination: %s\n", report.DestPath)
	fmt.Fprintf(w, "Mode: %s\n", report.Mode)
	fmt.Fprintf(w, "Dry Run: %v\n\n", report.DryRun)

	fmt.Fprintf(w, "Total Differences: %d\n\n", len(report.Differences))

	// Group by reason
	byReason := make(map[models.DifferenceReason][]models.FileDifference)
	for _, diff := range report.Differences {
		byReason[diff.Reason] = append(byReason[diff.Reason], diff)
	}

	// Display by category
	reasonOrder := []models.DifferenceReason{
		models.ReasonCopyError,
		models.ReasonUpdateError,
		models.ReasonOnlyInSource,
		models.ReasonOnlyInDest,
		models.ReasonHashDiff,
		models.ReasonContentDiff,
		models.ReasonSizeDiff,
		models.ReasonSkipped,
	}

	reasonLabels := map[models.DifferenceReason]string{
		models.ReasonCopyError:    "Copy Errors",
		models.ReasonUpdateError:  "Update Errors",
		models.ReasonOnlyInSource: "Only in Source",
		models.ReasonOnlyInDest:   "Only in Destination",
		models.ReasonHashDiff:     "Hash Differences",
		models.ReasonContentDiff:  "Content Differences",
		models.ReasonSizeDiff:     "Size Differences",
		models.ReasonSkipped:      "Skipped Files",
	}

	for _, reason := range reasonOrder {
		diffs, exists := byReason[reason]
		if !exists || len(diffs) == 0 {
			continue
		}

		label := fmt.Sprintf("%s (%d files)", reasonLabels[reason], len(diffs))
		fmt.Fprintf(w, "%s\n", label)
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", len(label)))

		for _, diff := range diffs {
			fmt.Fprintf(w, "  %s\n", diff.RelativePath)
			if diff.Details != "" {
				fmt.Fprintf(w, "    Details: %s\n", diff.Details)
			}

			if diff.SourceInfo != nil {
				fmt.Fprintf(w, "    Source:  %s", formatBytes(diff.SourceInfo.Size))
				if diff.SourceInfo.Hash != "" {
					fmt.Fprintf(w, ", hash: %s", diff.SourceInfo.Hash[:12])
				}
				fmt.Fprintf(w, "\n")
			}

			if diff.DestInfo != nil {
				fmt.Fprintf(w, "    Dest:    %s", formatBytes(diff.DestInfo.Size))
				if diff.DestInfo.Hash != "" {
					fmt.Fprintf(w, ", hash: %s", diff.DestInfo.Hash[:12])
				}
				fmt.Fprintf(w, "\n")
			}

			fmt.Fprintf(w, "\n")
		}

		fmt.Fprintf(w, "\n")
	}

	return nil
}

// writeDifferencesJSON writes differences in JSON format
func writeDifferencesJSON(report *models.SyncReport, w io.Writer) error {
	output := struct {
		Generated    string                   `json:"generated"`
		SourcePath   string                   `json:"source_path"`
		DestPath     string                   `json:"dest_path"`
		Mode         string                   `json:"mode"`
		DryRun       bool                     `json:"dry_run"`
		TotalCount   int                      `json:"total_count"`
		Differences  []models.FileDifference  `json:"differences"`
	}{
		Generated:   time.Now().Format(time.RFC3339),
		SourcePath:  report.SourcePath,
		DestPath:    report.DestPath,
		Mode:        string(report.Mode),
		DryRun:      report.DryRun,
		TotalCount:  len(report.Differences),
		Differences: report.Differences,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
