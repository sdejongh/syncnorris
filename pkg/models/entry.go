package models

import (
	"time"
)

// FileEntry represents a file in a sync operation
type FileEntry struct {
	// RelativePath is the path relative to the sync root
	RelativePath string

	// AbsolutePath is the full path on the filesystem
	AbsolutePath string

	// Size in bytes
	Size int64

	// ModTime is the last modification time
	ModTime time.Time

	// IsDir indicates if this is a directory
	IsDir bool

	// Permissions are the file mode bits
	Permissions uint32

	// Hash is the SHA-256 hash (optional, computed on demand)
	Hash string

	// Location indicates where the file exists
	Location FileLocation
}

// FileLocation indicates which side(s) a file exists on
type FileLocation string

const (
	// LocationSource indicates file exists in source only
	LocationSource FileLocation = "source"
	// LocationDest indicates file exists in destination only
	LocationDest FileLocation = "dest"
	// LocationBoth indicates file exists in both locations
	LocationBoth FileLocation = "both"
)

// Action represents what should be done with a file
type Action string

const (
	// ActionCopy copies file from source to destination
	ActionCopy Action = "copy"
	// ActionUpdate updates existing file in destination
	ActionUpdate Action = "update"
	// ActionDelete deletes file from destination
	ActionDelete Action = "delete"
	// ActionSkip skips the file (no change needed)
	ActionSkip Action = "skip"
	// ActionConflict indicates a conflict requiring resolution
	ActionConflict Action = "conflict"
)

// FileOperation represents a planned operation on a file
type FileOperation struct {
	Entry    *FileEntry
	Action   Action
	Reason   string
	Error    error
	BytesCopied int64
	Duration time.Duration
}
