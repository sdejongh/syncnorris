package storage

import (
	"context"
	"io"
	"time"
)

// FileInfo represents metadata about a file
type FileInfo struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsDir        bool
	Permissions  uint32
	RelativePath string
}

// Backend defines the interface for storage operations
// Implementations include local filesystem, SMB, NFS, etc.
type Backend interface {
	// List returns all files in the specified directory recursively
	List(ctx context.Context, path string) ([]FileInfo, error)

	// Read opens a file for reading
	Read(ctx context.Context, path string) (io.ReadCloser, error)

	// Write creates or overwrites a file with the given content
	// If metadata is provided, attempts to preserve timestamps and permissions
	Write(ctx context.Context, path string, reader io.Reader, size int64, metadata *FileInfo) error

	// Delete removes a file or directory
	Delete(ctx context.Context, path string) error

	// Exists checks if a file or directory exists
	Exists(ctx context.Context, path string) (bool, error)

	// Stat returns file metadata
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// MkdirAll creates a directory and all necessary parents
	MkdirAll(ctx context.Context, path string) error

	// Close releases any resources held by the backend
	Close() error
}
