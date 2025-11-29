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

// WriteDifferencesReport writes the differences report to a file or stdout
// If filepath is empty, writes to stdout
// Format can be "human" or "json"
func WriteDifferencesReport(report *models.SyncReport, filepath string, format string) error {
	var w io.Writer
	var shouldClose bool

	if filepath == "" {
		// Write to stdout
		w = os.Stdout
		shouldClose = false

		// Always display something to stdout, even if no differences
		if len(report.Differences) == 0 {
			fmt.Fprintln(w, "\nNo differences found - directories are synchronized.")
			return nil
		}

		// Add blank line before report for better readability
		fmt.Fprintln(w)
	} else {
		// Write to file (always create, even if no differences)
		file, err := os.Create(filepath)
		if err != nil {
			return fmt.Errorf("failed to create differences file: %w", err)
		}
		defer file.Close()
		w = file
		shouldClose = true
	}

	// Write the report
	var err error
	switch format {
	case "json":
		err = writeDifferencesJSON(report, w)
	default: // "human"
		err = writeDifferencesHuman(report, w)
	}

	if err != nil && shouldClose {
		return err
	}

	return err
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

	fmt.Fprintf(w, "Total Differences: %d\n", len(report.Differences))
	fmt.Fprintf(w, "Total Conflicts: %d\n\n", len(report.Conflicts))

	// Write conflicts section first (if any)
	if len(report.Conflicts) > 0 {
		writeConflictsHuman(report.Conflicts, w)
	}

	if len(report.Differences) == 0 && len(report.Conflicts) == 0 {
		fmt.Fprintf(w, "No differences found - directories are synchronized.\n")
		return nil
	}

	if len(report.Differences) == 0 {
		return nil
	}

	// Group by reason
	byReason := make(map[models.DifferenceReason][]models.FileDifference)
	for _, diff := range report.Differences {
		byReason[diff.Reason] = append(byReason[diff.Reason], diff)
	}

	// Display by category
	reasonOrder := []models.DifferenceReason{
		models.ReasonCopyError,
		models.ReasonUpdateError,
		models.ReasonDeleted,
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
		models.ReasonDeleted:      "Deleted from Destination",
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
		Generated      string                   `json:"generated"`
		SourcePath     string                   `json:"source_path"`
		DestPath       string                   `json:"dest_path"`
		Mode           string                   `json:"mode"`
		DryRun         bool                     `json:"dry_run"`
		TotalCount     int                      `json:"total_count"`
		ConflictCount  int                      `json:"conflict_count"`
		Differences    []models.FileDifference  `json:"differences"`
		Conflicts      []models.Conflict        `json:"conflicts,omitempty"`
	}{
		Generated:     time.Now().Format(time.RFC3339),
		SourcePath:    report.SourcePath,
		DestPath:      report.DestPath,
		Mode:          string(report.Mode),
		DryRun:        report.DryRun,
		TotalCount:    len(report.Differences),
		ConflictCount: len(report.Conflicts),
		Differences:   report.Differences,
		Conflicts:     report.Conflicts,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// writeConflictsHuman writes conflicts in human-readable format
func writeConflictsHuman(conflicts []models.Conflict, w io.Writer) {
	label := fmt.Sprintf("Conflicts Detected and Resolved (%d)", len(conflicts))
	fmt.Fprintf(w, "%s\n", label)
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", len(label)))

	// Group by conflict type
	byType := make(map[models.ConflictType][]models.Conflict)
	for _, c := range conflicts {
		byType[c.Type] = append(byType[c.Type], c)
	}

	typeLabels := map[models.ConflictType]string{
		models.ConflictModifyModify: "Both sides modified",
		models.ConflictDeleteModify: "Deleted on one side, modified on other",
		models.ConflictModifyDelete: "Modified on one side, deleted on other",
		models.ConflictCreateCreate: "Created on both sides",
	}

	typeOrder := []models.ConflictType{
		models.ConflictModifyModify,
		models.ConflictCreateCreate,
		models.ConflictDeleteModify,
		models.ConflictModifyDelete,
	}

	for _, ctype := range typeOrder {
		cs, exists := byType[ctype]
		if !exists || len(cs) == 0 {
			continue
		}

		fmt.Fprintf(w, "\n  %s:\n", typeLabels[ctype])
		for _, c := range cs {
			fmt.Fprintf(w, "    %s\n", c.Path)

			// Before resolution state
			fmt.Fprintf(w, "      Before resolution:\n")
			if c.SourceEntry != nil {
				fmt.Fprintf(w, "        Source: %s, modified %s\n", formatBytes(c.SourceEntry.Size), c.SourceEntry.ModTime.Format(time.RFC3339))
			} else {
				fmt.Fprintf(w, "        Source: (deleted)\n")
			}
			if c.DestEntry != nil {
				fmt.Fprintf(w, "        Dest:   %s, modified %s\n", formatBytes(c.DestEntry.Size), c.DestEntry.ModTime.Format(time.RFC3339))
			} else {
				fmt.Fprintf(w, "        Dest:   (deleted)\n")
			}

			// Resolution details
			fmt.Fprintf(w, "      Resolution:\n")
			fmt.Fprintf(w, "        Strategy: %s\n", c.Resolution)
			if c.Winner != "" {
				fmt.Fprintf(w, "        Winner:   %s\n", c.Winner)
			}
			if c.ResultDescription != "" {
				fmt.Fprintf(w, "        Result:   %s\n", c.ResultDescription)
			}
			if len(c.ConflictFiles) > 0 {
				fmt.Fprintf(w, "        Conflict copies created:\n")
				for _, cf := range c.ConflictFiles {
					fmt.Fprintf(w, "          - %s\n", cf)
				}
			}
		}
	}

	fmt.Fprintf(w, "\n")
}
