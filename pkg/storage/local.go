package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Local is a filesystem-based storage backend
type Local struct {
	rootPath string
}

// NewLocal creates a new local filesystem backend
func NewLocal(rootPath string) (*Local, error) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", absPath)
	}

	return &Local{rootPath: absPath}, nil
}

// List returns all files in the directory recursively
func (l *Local) List(ctx context.Context, path string) ([]FileInfo, error) {
	fullPath := filepath.Join(l.rootPath, path)
	var files []FileInfo

	err := filepath.WalkDir(fullPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(l.rootPath, p)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, FileInfo{
			Path:         p,
			Size:         info.Size(),
			ModTime:      info.ModTime(),
			IsDir:        info.IsDir(),
			Permissions:  uint32(info.Mode().Perm()),
			RelativePath: relPath,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return files, nil
}

// Read opens a file for reading
func (l *Local) Read(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(l.rootPath, path)

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// Write creates or overwrites a file
func (l *Local) Write(ctx context.Context, path string, reader io.Reader, size int64, metadata *FileInfo) error {
	fullPath := filepath.Join(l.rootPath, path)

	// Ensure parent directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if written != size {
		return fmt.Errorf("incomplete write: expected %d bytes, wrote %d", size, written)
	}

	// Preserve metadata if provided
	if metadata != nil {
		// Preserve modification time
		if !metadata.ModTime.IsZero() {
			if err := os.Chtimes(fullPath, metadata.ModTime, metadata.ModTime); err != nil {
				return fmt.Errorf("failed to set modification time: %w", err)
			}
		}

		// Preserve permissions
		if metadata.Permissions != 0 {
			if err := os.Chmod(fullPath, os.FileMode(metadata.Permissions)); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	}

	return nil
}

// Delete removes a file or directory
func (l *Local) Delete(ctx context.Context, path string) error {
	fullPath := filepath.Join(l.rootPath, path)

	err := os.RemoveAll(fullPath)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	return nil
}

// Exists checks if a file or directory exists
func (l *Local) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := filepath.Join(l.rootPath, path)

	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check existence: %w", err)
}

// Stat returns file metadata
func (l *Local) Stat(ctx context.Context, path string) (*FileInfo, error) {
	fullPath := filepath.Join(l.rootPath, path)

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	relPath, err := filepath.Rel(l.rootPath, fullPath)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Path:         fullPath,
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		IsDir:        info.IsDir(),
		Permissions:  uint32(info.Mode().Perm()),
		RelativePath: relPath,
	}, nil
}

// MkdirAll creates a directory and all necessary parents
func (l *Local) MkdirAll(ctx context.Context, path string) error {
	fullPath := filepath.Join(l.rootPath, path)

	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// Close releases resources (no-op for local filesystem)
func (l *Local) Close() error {
	return nil
}
