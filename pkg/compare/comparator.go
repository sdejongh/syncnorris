package compare

import (
	"context"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// Result represents the outcome of comparing two files
type Result string

const (
	// Same indicates files are identical
	Same Result = "same"
	// Different indicates files differ
	Different Result = "different"
	// SourceOnly indicates file exists only in source
	SourceOnly Result = "source_only"
	// DestOnly indicates file exists only in destination
	DestOnly Result = "dest_only"
	// Error indicates comparison failed
	Error Result = "error"
)

// Comparison holds the result of comparing two files
type Comparison struct {
	SourcePath string
	DestPath   string
	Result     Result
	Reason     string
	Error      error
}

// Comparator defines the interface for file comparison algorithms
type Comparator interface {
	// Compare compares two files and returns the result
	Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error)

	// Name returns the name of the comparison method
	Name() string
}
