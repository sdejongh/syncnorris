package compare

import (
	"context"
	"path/filepath"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// NameSizeComparator compares files by name and size only
type NameSizeComparator struct{}

// NewNameSizeComparator creates a new name/size comparator
func NewNameSizeComparator() *NameSizeComparator {
	return &NameSizeComparator{}
}

// Compare compares two files by name and size
func (c *NameSizeComparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
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
			Result:     DestOnly,
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
			Reason:     "file sizes differ",
		}, nil
	}

	return &Comparison{
		SourcePath: sourcePath,
		DestPath:   destPath,
		Result:     Same,
		Reason:     "name and size match",
	}, nil
}

// Name returns the comparator name
func (c *NameSizeComparator) Name() string {
	return "namesize"
}
