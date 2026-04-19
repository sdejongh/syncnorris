package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultPartialSuffix is appended to file names while they are being written.
const defaultPartialSuffix = ".syncnorris.partial"

// Local is a filesystem-based storage backend
type Local struct {
	rootPath      string
	partialSuffix string
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

// Walk iterates over files recursively, calling fn for each entry found.
func (l *Local) Walk(ctx context.Context, path string, fn func(FileInfo) error) error {
	fullPath := filepath.Join(l.rootPath, path)

	return filepath.WalkDir(fullPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(l.rootPath, p)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		return fn(FileInfo{
			Path:         p,
			Size:         info.Size(),
			ModTime:      info.ModTime(),
			IsDir:        info.IsDir(),
			Permissions:  uint32(info.Mode().Perm()),
			RelativePath: relPath,
		})
	})
}

// List returns all files in the directory recursively
// Continues on permission errors, skipping inaccessible files/directories
func (l *Local) List(ctx context.Context, path string) ([]FileInfo, error) {
	var files []FileInfo

	err := l.Walk(ctx, path, func(fi FileInfo) error {
		files = append(files, fi)
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

// SetPartialSuffix overrides the default partial file suffix for this backend.
// Callers using jobs set this to ".syncnorris-<jobid8>.partial" so partial files
// from different jobs can be distinguished. Empty string restores the default.
func (l *Local) SetPartialSuffix(s string) { l.partialSuffix = s }

// Write creates or overwrites a file atomically via a .partial temp file followed
// by a rename. If the context is cancelled mid-write, the partial file is removed
// and the destination path is left untouched.
func (l *Local) Write(ctx context.Context, path string, reader io.Reader, size int64, metadata *FileInfo) error {
	fullPath := filepath.Join(l.rootPath, path)

	// Ensure parent directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	partialSuffix := l.partialSuffix
	if partialSuffix == "" {
		partialSuffix = defaultPartialSuffix
	}
	partialPath := fullPath + partialSuffix

	file, err := os.Create(partialPath)
	if err != nil {
		return fmt.Errorf("failed to create partial file: %w", err)
	}

	// Wrap reader with context-aware reader so io.Copy stops on cancel
	cancelReader := &ctxReader{ctx: ctx, r: reader}

	written, copyErr := io.Copy(file, cancelReader)
	closeErr := file.Close()

	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partialPath)
		if copyErr != nil {
			return fmt.Errorf("failed to write file: %w", copyErr)
		}
		return fmt.Errorf("failed to close partial file: %w", closeErr)
	}
	if written != size {
		_ = os.Remove(partialPath)
		return fmt.Errorf("incomplete write: expected %d bytes, wrote %d", size, written)
	}

	// Atomic rename into final path
	if err := os.Rename(partialPath, fullPath); err != nil {
		_ = os.Remove(partialPath)
		return fmt.Errorf("failed to finalize file: %w", err)
	}

	// Preserve metadata if provided (applied after rename on the final path)
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

// ctxReader wraps an io.Reader and checks context cancellation before each Read.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
	}
	return c.r.Read(p)
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
