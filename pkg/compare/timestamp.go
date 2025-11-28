package compare

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// TimestampComparator compares files by name, size, and modification time
type TimestampComparator struct{}

// NewTimestampComparator creates a new timestamp comparator
func NewTimestampComparator() *TimestampComparator {
	return &TimestampComparator{}
}

// Compare compares two files by name, size, and modification time
// Files are considered the same if they have the same name, size, and the source
// modification time is not newer than the destination modification time
func (c *TimestampComparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
	sourceInfo, err := source.Stat(ctx, sourcePath)
	if err != nil {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     SourceOnly,
			Reason:     "file exists only in source",
		}, nil
	}

	destInfo, err := dest.Stat(ctx, destPath)
	if err != nil {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     SourceOnly,
			Reason:     "file exists only in destination",
		}, nil
	}

	// Compare file names
	sourceName := filepath.Base(sourcePath)
	destName := filepath.Base(destPath)
	if sourceName != destName {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     "file names differ",
		}, nil
	}

	// Compare file sizes
	if sourceInfo.Size != destInfo.Size {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     fmt.Sprintf("file sizes differ (source: %d, dest: %d)", sourceInfo.Size, destInfo.Size),
		}, nil
	}

	// Compare modification times
	// Source is considered newer if its ModTime is after the destination's ModTime
	// We use a 1-second tolerance to account for filesystem timestamp precision differences
	timeDiff := sourceInfo.ModTime.Sub(destInfo.ModTime)
	if timeDiff > 1e9 { // More than 1 second newer
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     fmt.Sprintf("source is newer (source: %s, dest: %s)", sourceInfo.ModTime.Format("2006-01-02 15:04:05"), destInfo.ModTime.Format("2006-01-02 15:04:05")),
		}, nil
	}

	return &Comparison{
		SourcePath: sourcePath,
		DestPath:   destPath,
		Result:     Same,
		Reason:     "name, size, and timestamp match",
	}, nil
}

// Name returns the comparator name
func (c *TimestampComparator) Name() string {
	return "timestamp"
}
