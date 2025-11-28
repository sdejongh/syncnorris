package compare

import (
	"context"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// CompositeComparator performs multi-stage comparison
// Stage 1: Quick name+size check
// Stage 2: Optional hash verification if enabled
type CompositeComparator struct {
	useHash        bool
	hashComp       *HashComparator
	bufferSize     int
	progressReport func(path string, current, total int64) // Optional progress callback
}

// NewCompositeComparator creates a smart comparator
// If useHash is true, performs hash verification when name+size match
// If false, considers files identical when name+size match
func NewCompositeComparator(useHash bool, bufferSize int) *CompositeComparator {
	var hashComp *HashComparator
	if useHash {
		hashComp = NewHashComparator(bufferSize)
	}
	return &CompositeComparator{
		useHash:    useHash,
		hashComp:   hashComp,
		bufferSize: bufferSize,
	}
}

// Compare performs intelligent comparison
func (c *CompositeComparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
	// Stage 1: Get file metadata (fast)
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

	// Quick check: if sizes differ, files are definitely different
	if sourceInfo.Size != destInfo.Size {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     "file sizes differ",
		}, nil
	}

	// Stage 2: If name+size match, do we need hash verification?
	if !c.useHash {
		// Trust name+size match
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Same,
			Reason:     "name and size match (hash check disabled)",
		}, nil
	}

	// Perform hash verification with progress reporting
	if c.progressReport != nil {
		c.hashComp.SetProgressCallback(c.progressReport)
	}
	return c.hashComp.Compare(ctx, source, dest, sourcePath, destPath)
}

// SetProgressCallback sets a callback for progress reporting during comparison
func (c *CompositeComparator) SetProgressCallback(callback func(path string, current, total int64)) {
	c.progressReport = callback
	if c.hashComp != nil {
		c.hashComp.SetProgressCallback(callback)
	}
}

// SetReaderWrapper sets a function to wrap readers (e.g., for rate limiting)
func (c *CompositeComparator) SetReaderWrapper(wrapper ReaderWrapper) {
	if c.hashComp != nil {
		c.hashComp.SetReaderWrapper(wrapper)
	}
}

// Name returns the comparator name
func (c *CompositeComparator) Name() string {
	if c.useHash {
		return "composite-hash"
	}
	return "composite-fast"
}
