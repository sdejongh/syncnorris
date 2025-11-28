package models

import (
	"time"
)

// SyncMode defines the synchronization direction
type SyncMode string

const (
	// ModeOneWay syncs from source to destination only
	ModeOneWay SyncMode = "oneway"
	// ModeBidirectional syncs in both directions
	ModeBidirectional SyncMode = "bidirectional"
)

// ConflictResolution defines how to handle conflicts
type ConflictResolution string

const (
	// ConflictAsk prompts the user for each conflict
	ConflictAsk ConflictResolution = "ask"
	// ConflictSourceWins always uses source version
	ConflictSourceWins ConflictResolution = "source-wins"
	// ConflictDestWins always uses destination version
	ConflictDestWins ConflictResolution = "dest-wins"
	// ConflictNewer uses the newer file
	ConflictNewer ConflictResolution = "newer"
	// ConflictBoth keeps both files with rename
	ConflictBoth ConflictResolution = "both"
)

// ComparisonMethod defines how files are compared
type ComparisonMethod string

const (
	// CompareNameSize compares by name and size only
	CompareNameSize ComparisonMethod = "namesize"
	// CompareTimestamp compares by modification time
	CompareTimestamp ComparisonMethod = "timestamp"
	// CompareBinary compares byte-by-byte
	CompareBinary ComparisonMethod = "binary"
	// CompareHash compares SHA-256 hashes
	CompareHash ComparisonMethod = "hash"
	// CompareMD5 compares MD5 hashes (faster than SHA-256, less secure)
	CompareMD5 ComparisonMethod = "md5"
)

// SyncOperation represents a sync operation configuration
type SyncOperation struct {
	ID                 string
	SourcePath         string
	DestPath           string
	Mode               SyncMode
	ComparisonMethod   ComparisonMethod
	ConflictResolution ConflictResolution
	ExcludePatterns    []string
	DryRun             bool
	DeleteOrphans      bool  // Delete files in destination that don't exist in source
	MaxWorkers         int
	BandwidthLimit     int64 // bytes per second, 0 = unlimited
	BufferSize         int
	CreatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
}

// Validate checks if the operation configuration is valid
func (op *SyncOperation) Validate() error {
	if op.SourcePath == "" {
		return &ValidationError{Field: "SourcePath", Message: "source path is required"}
	}
	if op.DestPath == "" {
		return &ValidationError{Field: "DestPath", Message: "destination path is required"}
	}
	if op.MaxWorkers < 1 {
		return &ValidationError{Field: "MaxWorkers", Message: "max workers must be at least 1"}
	}
	if op.BufferSize < 1024 {
		return &ValidationError{Field: "BufferSize", Message: "buffer size must be at least 1024 bytes"}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
