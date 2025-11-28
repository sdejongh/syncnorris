package compare

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// Partial hashing configuration
const (
	// Minimum file size to enable partial hashing (1MB)
	partialHashThreshold = 1 * 1024 * 1024
	// Size of partial hash to compute (256KB)
	partialHashSize = 256 * 1024
)

// HashComparator compares files using SHA-256 hash
type HashComparator struct {
	bufferSize        int
	bufferPool        *sync.Pool
	progressReport    func(path string, current, total int64) // Optional progress callback
	enablePartialHash bool                                     // Enable partial hashing optimization
	readerWrapper     ReaderWrapper                            // Optional reader wrapper (e.g., for rate limiting)
}

// NewHashComparator creates a new hash-based comparator
func NewHashComparator(bufferSize int) *HashComparator {
	if bufferSize < 4096 {
		bufferSize = 4096
	}
	return &HashComparator{
		bufferSize:        bufferSize,
		enablePartialHash: true, // Enabled by default
		bufferPool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, bufferSize)
				return &buf
			},
		},
	}
}

// SetPartialHashEnabled enables or disables partial hashing optimization
func (c *HashComparator) SetPartialHashEnabled(enabled bool) {
	c.enablePartialHash = enabled
}

// Compare compares two files using SHA-256 hash
func (c *HashComparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
	// Check if source exists
	sourceExists, err := source.Exists(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check source existence: %w", err)
	}
	if !sourceExists {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     DestOnly,
			Reason:     "file exists only in destination",
		}, nil
	}

	// Check if destination exists
	destExists, err := dest.Exists(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check destination existence: %w", err)
	}
	if !destExists {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     SourceOnly,
			Reason:     "file exists only in source",
		}, nil
	}

	// Get file info to check sizes first (quick check)
	sourceInfo, err := source.Stat(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat source file: %w", err)
	}

	destInfo, err := dest.Stat(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat destination file: %w", err)
	}

	// If sizes differ, files are different
	if sourceInfo.Size != destInfo.Size {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     "file sizes differ",
		}, nil
	}

	// Partial hash optimization for large files (parallel execution)
	// If files are large enough and partial hashing is enabled,
	// compute partial hashes first for quick rejection
	if c.enablePartialHash && sourceInfo.Size >= partialHashThreshold {
		var sourcePartialHash, destPartialHash string
		var sourcePartialErr, destPartialErr error
		var wg sync.WaitGroup

		// Compute both partial hashes in parallel
		wg.Add(2)
		go func() {
			defer wg.Done()
			sourcePartialHash, sourcePartialErr = c.computePartialHash(ctx, source, sourcePath)
		}()
		go func() {
			defer wg.Done()
			destPartialHash, destPartialErr = c.computePartialHash(ctx, dest, destPath)
		}()
		wg.Wait()

		// Only use partial hash results if both succeeded
		if sourcePartialErr == nil && destPartialErr == nil {
			// If partial hashes differ, files are different - no need for full hash
			if sourcePartialHash != destPartialHash {
				return &Comparison{
					SourcePath: sourcePath,
					DestPath:   destPath,
					Result:     Different,
					Reason:     "file partial hashes differ",
				}, nil
			}
			// Partial hashes match - continue to full hash verification
		}
		// If either partial hash fails, fall back to full hash (don't fail the comparison)
	}

	// Compute full hashes in parallel
	var sourceHash, destHash string
	var sourceHashErr, destHashErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		sourceHash, sourceHashErr = c.computeHash(ctx, source, sourcePath)
	}()
	go func() {
		defer wg.Done()
		destHash, destHashErr = c.computeHash(ctx, dest, destPath)
	}()
	wg.Wait()

	// Check for errors
	if sourceHashErr != nil {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Error,
			Reason:     "failed to compute source hash",
			Error:      sourceHashErr,
		}, sourceHashErr
	}
	if destHashErr != nil {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Error,
			Reason:     "failed to compute destination hash",
			Error:      destHashErr,
		}, destHashErr
	}

	// Compare hashes
	if sourceHash != destHash {
		return &Comparison{
			SourcePath: sourcePath,
			DestPath:   destPath,
			Result:     Different,
			Reason:     "file hashes differ",
		}, nil
	}

	return &Comparison{
		SourcePath: sourcePath,
		DestPath:   destPath,
		Result:     Same,
		Reason:     "file hashes match",
	}, nil
}

// computeHash computes SHA-256 hash of a file using streaming
func (c *HashComparator) computeHash(ctx context.Context, backend storage.Backend, path string) (string, error) {
	// Get file size for progress reporting
	fileInfo, err := backend.Stat(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fileInfo.Size

	reader, err := backend.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer reader.Close()

	// Apply reader wrapper if set (e.g., for rate limiting)
	if c.readerWrapper != nil {
		reader = c.readerWrapper(reader)
	}

	hasher := sha256.New()

	// Get buffer from pool
	bufPtr := c.bufferPool.Get().(*[]byte)
	buffer := *bufPtr
	defer c.bufferPool.Put(bufPtr)

	// Progress throttling variables
	const (
		progressReportInterval = 50 * time.Millisecond // Minimum time between reports
		progressReportBytes    = 64 * 1024             // Minimum bytes between reports (64KB)
	)
	var totalRead int64
	var lastReported int64
	lastReportTime := time.Now()

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			hasher.Write(buffer[:n])
			totalRead += int64(n)

			// Report progress if callback is set (with throttling)
			if c.progressReport != nil {
				bytesSinceLastReport := totalRead - lastReported
				timeSinceLastReport := time.Since(lastReportTime)
				shouldReport := bytesSinceLastReport >= progressReportBytes ||
					timeSinceLastReport >= progressReportInterval ||
					err != nil // Always report on completion or error

				if shouldReport {
					c.progressReport(path, totalRead, fileSize)
					lastReported = totalRead
					lastReportTime = time.Now()
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	// Ensure final progress report shows 100% completion
	if c.progressReport != nil && totalRead > lastReported {
		c.progressReport(path, totalRead, fileSize)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// computePartialHash computes SHA-256 hash of the first partialHashSize bytes of a file
// This is used for quick rejection of different files without computing full hash
func (c *HashComparator) computePartialHash(ctx context.Context, backend storage.Backend, path string) (string, error) {
	reader, err := backend.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer reader.Close()

	// Apply reader wrapper if set (e.g., for rate limiting)
	if c.readerWrapper != nil {
		reader = c.readerWrapper(reader)
	}

	hasher := sha256.New()

	// Get buffer from pool
	bufPtr := c.bufferPool.Get().(*[]byte)
	buffer := *bufPtr
	defer c.bufferPool.Put(bufPtr)

	// Read up to partialHashSize bytes
	var totalRead int64
	for totalRead < partialHashSize {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			// Only hash up to the limit
			bytesToHash := int64(n)
			if totalRead+bytesToHash > partialHashSize {
				bytesToHash = partialHashSize - totalRead
			}
			hasher.Write(buffer[:bytesToHash])
			totalRead += bytesToHash
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// SetProgressCallback sets a callback for progress reporting during hash calculation
func (c *HashComparator) SetProgressCallback(callback func(path string, current, total int64)) {
	c.progressReport = callback
}

// SetReaderWrapper sets a function to wrap readers (e.g., for rate limiting)
func (c *HashComparator) SetReaderWrapper(wrapper ReaderWrapper) {
	c.readerWrapper = wrapper
}

// Name returns the comparator name
func (c *HashComparator) Name() string {
	return "hash"
}
