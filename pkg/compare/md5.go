package compare

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// MD5Comparator compares files using MD5 hash
// MD5 is faster than SHA-256 but less secure - suitable for non-critical data
type MD5Comparator struct {
	bufferSize        int
	bufferPool        *sync.Pool
	progressReport    func(path string, current, total int64) // Optional progress callback
	enablePartialHash bool                                     // Enable partial hashing optimization
	readerWrapper     ReaderWrapper                            // Optional reader wrapper (e.g., for rate limiting)
}

// NewMD5Comparator creates a new MD5-based comparator
func NewMD5Comparator(bufferSize int) *MD5Comparator {
	if bufferSize < 4096 {
		bufferSize = 4096
	}
	return &MD5Comparator{
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
func (c *MD5Comparator) SetPartialHashEnabled(enabled bool) {
	c.enablePartialHash = enabled
}

// SetProgressCallback sets the progress reporting callback
func (c *MD5Comparator) SetProgressCallback(callback func(path string, current, total int64)) {
	c.progressReport = callback
}

// SetReaderWrapper sets a function to wrap readers (e.g., for rate limiting)
func (c *MD5Comparator) SetReaderWrapper(wrapper ReaderWrapper) {
	c.readerWrapper = wrapper
}

// Compare compares two files using MD5 hash
func (c *MD5Comparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
	// Check if source exists
	sourceExists, err := source.Exists(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check source existence: %w", err)
	}
	if !sourceExists {
		return &Comparison{
			Result: Different,
			Reason: "source file does not exist",
		}, nil
	}

	// Check if destination exists
	destExists, err := dest.Exists(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check destination existence: %w", err)
	}
	if !destExists {
		return &Comparison{
			Result: Different,
			Reason: "destination file does not exist",
		}, nil
	}

	// Get file info
	sourceInfo, err := source.Stat(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat source: %w", err)
	}

	destInfo, err := dest.Stat(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat destination: %w", err)
	}

	// Quick check: if sizes differ, files are different
	if sourceInfo.Size != destInfo.Size {
		return &Comparison{
			Result: Different,
			Reason: fmt.Sprintf("size mismatch: source=%d, dest=%d", sourceInfo.Size, destInfo.Size),
		}, nil
	}

	// Try partial hash first for large files (same as SHA-256 comparator)
	if c.enablePartialHash && sourceInfo.Size >= partialHashThreshold {
		// Compute partial hashes in parallel
		var sourcePartialHash, destPartialHash string
		var sourcePartialErr, destPartialErr error
		var wg sync.WaitGroup

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

		if sourcePartialErr != nil {
			return nil, fmt.Errorf("failed to compute source partial hash: %w", sourcePartialErr)
		}
		if destPartialErr != nil {
			return nil, fmt.Errorf("failed to compute destination partial hash: %w", destPartialErr)
		}

		// If partial hashes differ, files are definitely different (quick rejection)
		if sourcePartialHash != destPartialHash {
			return &Comparison{
				Result: Different,
				Reason: "MD5 partial hash mismatch (first 256KB differ)",
			}, nil
		}
	}

	// Compute full MD5 hashes in parallel
	var sourceHash, destHash string
	var sourceErr, destErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		sourceHash, sourceErr = c.computeHash(ctx, source, sourcePath, sourceInfo.Size)
	}()
	go func() {
		defer wg.Done()
		destHash, destErr = c.computeHash(ctx, dest, destPath, destInfo.Size)
	}()
	wg.Wait()

	if sourceErr != nil {
		return nil, fmt.Errorf("failed to compute source hash: %w", sourceErr)
	}
	if destErr != nil {
		return nil, fmt.Errorf("failed to compute destination hash: %w", destErr)
	}

	// Compare hashes
	if sourceHash == destHash {
		return &Comparison{
			Result: Same,
			Reason: "MD5 hashes match",
		}, nil
	}

	return &Comparison{
		Result: Different,
		Reason: "MD5 hash mismatch",
	}, nil
}

// computePartialHash computes MD5 hash of first 256KB of file
func (c *MD5Comparator) computePartialHash(ctx context.Context, backend storage.Backend, path string) (string, error) {
	reader, err := backend.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer reader.Close()

	// Apply reader wrapper if set (e.g., for rate limiting)
	if c.readerWrapper != nil {
		reader = c.readerWrapper(reader)
	}

	hash := md5.New()
	bufPtr := c.bufferPool.Get().(*[]byte)
	defer c.bufferPool.Put(bufPtr)
	buf := *bufPtr

	bytesRead := int64(0)
	for bytesRead < partialHashSize {
		n, err := reader.Read(buf)
		if n > 0 {
			toWrite := n
			if bytesRead+int64(n) > partialHashSize {
				toWrite = int(partialHashSize - bytesRead)
			}
			hash.Write(buf[:toWrite])
			bytesRead += int64(toWrite)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// computeHash computes MD5 hash of entire file with progress reporting
func (c *MD5Comparator) computeHash(ctx context.Context, backend storage.Backend, path string, fileSize int64) (string, error) {
	reader, err := backend.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer reader.Close()

	// Apply reader wrapper if set (e.g., for rate limiting)
	if c.readerWrapper != nil {
		reader = c.readerWrapper(reader)
	}

	hash := md5.New()
	bufPtr := c.bufferPool.Get().(*[]byte)
	defer c.bufferPool.Put(bufPtr)
	buf := *bufPtr

	// Progress reporting with throttling
	const (
		progressReportInterval = 50 * time.Millisecond
		progressReportBytes    = 64 * 1024 // 64KB
	)

	var bytesRead int64
	var lastReported int64
	var lastReportTime time.Time

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
			bytesRead += int64(n)

			// Throttled progress reporting
			if c.progressReport != nil {
				bytesSinceLastReport := bytesRead - lastReported
				timeSinceLastReport := time.Since(lastReportTime)
				shouldReport := bytesSinceLastReport >= progressReportBytes ||
					timeSinceLastReport >= progressReportInterval ||
					err != nil

				if shouldReport {
					c.progressReport(path, bytesRead, fileSize)
					lastReported = bytesRead
					lastReportTime = time.Now()
				}
			}
		}
		if err == io.EOF {
			// Final progress report
			if c.progressReport != nil && bytesRead > lastReported {
				c.progressReport(path, bytesRead, fileSize)
			}
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Name returns the comparator name
func (c *MD5Comparator) Name() string {
	return "md5"
}
