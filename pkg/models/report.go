package models

import (
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

	// Overall status
	Status SyncStatus
}

// Statistics holds sync operation metrics
type Statistics struct {
	// Files processed (unique paths)
	FilesScanned       int // Unique files across source and destination
	FilesCopied        int
	FilesUpdated       int
	FilesDeleted       int
	FilesSynchronized  int // Files already identical (no copy needed)
	FilesSkipped       int // Files skipped for other reasons (e.g., dest-only in one-way)
	FilesErrored       int

	// Source-specific counts
	SourceFilesScanned int
	SourceDirsScanned  int

	// Destination-specific counts
	DestFilesScanned int
	DestDirsScanned  int

	// Directories processed (unique paths)
	DirsScanned int // Unique directories across source and destination
	DirsCreated int
	DirsDeleted int

	// Data transfer
	BytesScanned     int64
	BytesTransferred int64

	// Performance
	AverageSpeed     int64 // bytes per second
	PeakSpeed        int64 // bytes per second
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
