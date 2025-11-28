package compare

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// BinaryComparator compares files byte-by-byte
// This is the most thorough comparison but also the slowest
// Useful for detecting exact byte offset where files differ
type BinaryComparator struct {
	bufferSize     int
	bufferPool     *sync.Pool
	progressReport func(path string, current, total int64) // Optional progress callback
	readerWrapper  ReaderWrapper                           // Optional reader wrapper (e.g., for rate limiting)
}

// NewBinaryComparator creates a new byte-by-byte comparator
func NewBinaryComparator(bufferSize int) *BinaryComparator {
	if bufferSize < 4096 {
		bufferSize = 4096
	}
	return &BinaryComparator{
		bufferSize: bufferSize,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, bufferSize)
				return &buf
			},
		},
	}
}

// SetProgressCallback sets the progress reporting callback
func (c *BinaryComparator) SetProgressCallback(callback func(path string, current, total int64)) {
	c.progressReport = callback
}

// SetReaderWrapper sets a function to wrap readers (e.g., for rate limiting)
func (c *BinaryComparator) SetReaderWrapper(wrapper ReaderWrapper) {
	c.readerWrapper = wrapper
}

// Compare compares two files byte-by-byte
func (c *BinaryComparator) Compare(ctx context.Context, source, dest storage.Backend, sourcePath, destPath string) (*Comparison, error) {
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

	// Open both files for reading
	sourceReader, err := source.Read(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceReader.Close()

	destReader, err := dest.Read(ctx, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open destination file: %w", err)
	}
	defer destReader.Close()

	// Apply reader wrapper if set (e.g., for rate limiting)
	var sourceReaderWrapped io.Reader = sourceReader
	var destReaderWrapped io.Reader = destReader
	if c.readerWrapper != nil {
		sourceReaderWrapped = c.readerWrapper(sourceReader)
		destReaderWrapped = c.readerWrapper(destReader)
	}

	// Get buffers from pool
	sourceBufPtr := c.bufferPool.Get().(*[]byte)
	defer c.bufferPool.Put(sourceBufPtr)
	sourceBuf := *sourceBufPtr

	destBufPtr := c.bufferPool.Get().(*[]byte)
	defer c.bufferPool.Put(destBufPtr)
	destBuf := *destBufPtr

	// Progress reporting with throttling
	const (
		progressReportInterval = 50 * time.Millisecond
		progressReportBytes    = 64 * 1024 // 64KB
	)

	var bytesCompared int64
	var lastReported int64
	var lastReportTime time.Time
	var diffOffset int64 = -1 // First byte where files differ

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Read from both files
		sourceN, sourceErr := sourceReaderWrapped.Read(sourceBuf)
		destN, destErr := destReaderWrapped.Read(destBuf)

		// Both should read the same amount (or both EOF)
		if sourceN != destN {
			return &Comparison{
				Result: Different,
				Reason: fmt.Sprintf("read mismatch at offset %d", bytesCompared),
			}, nil
		}

		if sourceN > 0 {
			// Compare the buffers
			if !bytes.Equal(sourceBuf[:sourceN], destBuf[:destN]) {
				// Find exact byte offset where they differ
				for i := 0; i < sourceN; i++ {
					if sourceBuf[i] != destBuf[i] {
						diffOffset = bytesCompared + int64(i)
						break
					}
				}
				return &Comparison{
					Result: Different,
					Reason: fmt.Sprintf("binary content differs at byte offset %d", diffOffset),
				}, nil
			}

			bytesCompared += int64(sourceN)

			// Throttled progress reporting
			if c.progressReport != nil {
				bytesSinceLastReport := bytesCompared - lastReported
				timeSinceLastReport := time.Since(lastReportTime)
				shouldReport := bytesSinceLastReport >= progressReportBytes ||
					timeSinceLastReport >= progressReportInterval

				if shouldReport {
					c.progressReport(sourcePath, bytesCompared, sourceInfo.Size)
					lastReported = bytesCompared
					lastReportTime = time.Now()
				}
			}
		}

		// Check for errors
		if sourceErr == io.EOF && destErr == io.EOF {
			// Final progress report
			if c.progressReport != nil && bytesCompared > lastReported {
				c.progressReport(sourcePath, bytesCompared, sourceInfo.Size)
			}
			break // Both files ended at the same point
		}

		if sourceErr == io.EOF && destErr != io.EOF {
			return &Comparison{
				Result: Different,
				Reason: fmt.Sprintf("source ended at %d but destination continues", bytesCompared),
			}, nil
		}

		if sourceErr != io.EOF && destErr == io.EOF {
			return &Comparison{
				Result: Different,
				Reason: fmt.Sprintf("destination ended at %d but source continues", bytesCompared),
			}, nil
		}

		if sourceErr != nil {
			return nil, fmt.Errorf("failed to read source: %w", sourceErr)
		}
		if destErr != nil {
			return nil, fmt.Errorf("failed to read destination: %w", destErr)
		}
	}

	// Files are identical
	return &Comparison{
		Result: Same,
		Reason: fmt.Sprintf("binary content matches (%d bytes)", bytesCompared),
	}, nil
}

// Name returns the comparator name
func (c *BinaryComparator) Name() string {
	return "binary"
}
