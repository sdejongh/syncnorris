package models

import (
	"sync/atomic"
	"time"
)

// SyncReport represents the results of a sync operation
type SyncReport struct {
	// Operation details
	OperationID string
	SourcePath  string
	DestPath    string
	Mode        SyncMode
	DryRun      bool

	// Timing
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// Statistics
	Stats Statistics

	// File operations performed
	Operations []FileOperation

	// Conflicts encountered
	Conflicts []Conflict

	// Errors encountered
	Errors []SyncError

	// Differences remaining after sync/compare
	Differences []FileDifference

	// Overall status
	Status SyncStatus
}

// Statistics holds sync operation metrics
// Uses atomic counters for thread-safe concurrent updates
type Statistics struct {
	// Files processed (unique paths)
	FilesScanned       atomic.Int32 // Unique files across source and destination
	FilesCopied        atomic.Int32
	FilesUpdated       atomic.Int32
	FilesDeleted       atomic.Int32
	FilesSynchronized  atomic.Int32 // Files already identical (no copy needed)
	FilesSkipped       atomic.Int32 // Files skipped for other reasons (e.g., dest-only in one-way)
	FilesErrored       atomic.Int32

	// Source-specific counts
	SourceFilesScanned atomic.Int32
	SourceDirsScanned  atomic.Int32

	// Destination-specific counts
	DestFilesScanned atomic.Int32
	DestDirsScanned  atomic.Int32

	// Directories processed (unique paths)
	DirsScanned atomic.Int32 // Unique directories across source and destination
	DirsCreated atomic.Int32
	DirsDeleted atomic.Int32

	// Data transfer
	BytesScanned     atomic.Int64
	BytesTransferred atomic.Int64

	// Performance
	AverageSpeed     atomic.Int64 // bytes per second
	PeakSpeed        atomic.Int64 // bytes per second
}

// SyncStatus represents the overall result
type SyncStatus string

const (
	// StatusSuccess indicates all operations completed successfully
	StatusSuccess SyncStatus = "success"
	// StatusPartial indicates some operations failed
	StatusPartial SyncStatus = "partial"
	// StatusFailed indicates the sync operation failed
	StatusFailed SyncStatus = "failed"
	// StatusCancelled indicates the operation was cancelled
	StatusCancelled SyncStatus = "cancelled"
)

// SyncError represents an error during sync
type SyncError struct {
	FilePath  string
	Operation Action
	Error     string
	Timestamp time.Time
}

// ExitCode returns the appropriate exit code for the sync status
func (s SyncStatus) ExitCode() int {
	switch s {
	case StatusSuccess:
		return 0
	case StatusPartial:
		return 1
	case StatusFailed:
		return 2
	case StatusCancelled:
		return 3
	default:
		return 2
	}
}

// FileDifference represents a file that remains different after sync/compare
type FileDifference struct {
	RelativePath string           `json:"relative_path"`
	Reason       DifferenceReason `json:"reason"`
	Details      string           `json:"details,omitempty"`
	SourceInfo   *FileInfo        `json:"source_info,omitempty"` // nil if file doesn't exist in source
	DestInfo     *FileInfo        `json:"dest_info,omitempty"`   // nil if file doesn't exist in dest
}

// DifferenceReason indicates why files remain different
type DifferenceReason string

const (
	// ReasonCopyError indicates file copy failed
	ReasonCopyError DifferenceReason = "copy_error"
	// ReasonUpdateError indicates file update failed
	ReasonUpdateError DifferenceReason = "update_error"
	// ReasonSizeDiff indicates files have different sizes
	ReasonSizeDiff DifferenceReason = "size_different"
	// ReasonHashDiff indicates files have different hashes
	ReasonHashDiff DifferenceReason = "hash_different"
	// ReasonContentDiff indicates files have different content (binary comparison)
	ReasonContentDiff DifferenceReason = "content_different"
	// ReasonOnlyInSource indicates file exists only in source (not copied yet or copy failed)
	ReasonOnlyInSource DifferenceReason = "only_in_source"
	// ReasonOnlyInDest indicates file exists only in destination (one-way mode)
	ReasonOnlyInDest DifferenceReason = "only_in_dest"
	// ReasonSkipped indicates file was intentionally skipped
	ReasonSkipped DifferenceReason = "skipped"
)

// FileInfo holds metadata about a file for difference reporting
type FileInfo struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Hash    string    `json:"hash,omitempty"` // may be empty
}
