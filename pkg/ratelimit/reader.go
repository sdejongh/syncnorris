package ratelimit

import (
	"context"
	"io"
	"sync"
	"time"
)

// Reader wraps an io.Reader with bandwidth limiting
type Reader struct {
	reader    io.Reader
	limiter   *Limiter
	ctx       context.Context
}

// Limiter controls the rate of data transfer across multiple readers
type Limiter struct {
	bytesPerSecond int64
	mu             sync.Mutex
	tokens         int64     // Available tokens (bytes)
	lastUpdate     time.Time // Last time tokens were updated
	bucketSize     int64     // Maximum tokens (burst size)
}

// NewLimiter creates a new rate limiter with the specified bytes per second limit
// bucketSize allows for burst transfers up to that size
func NewLimiter(bytesPerSecond int64) *Limiter {
	if bytesPerSecond <= 0 {
		return nil // No limiting
	}

	// Bucket size is 1 second worth of data or 64KB minimum for smooth transfers
	bucketSize := bytesPerSecond
	if bucketSize < 65536 {
		bucketSize = 65536
	}

	return &Limiter{
		bytesPerSecond: bytesPerSecond,
		tokens:         bucketSize, // Start with full bucket
		lastUpdate:     time.Now(),
		bucketSize:     bucketSize,
	}
}

// NewReader wraps an io.Reader with rate limiting
func NewReader(ctx context.Context, reader io.Reader, limiter *Limiter) io.Reader {
	if limiter == nil {
		return reader // No limiting
	}
	return &Reader{
		reader:  reader,
		limiter: limiter,
		ctx:     ctx,
	}
}

// Read implements io.Reader with rate limiting using token bucket algorithm
func (r *Reader) Read(p []byte) (int, error) {
	// Check context cancellation
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	// Wait for tokens
	toRead := len(p)
	if toRead > int(r.limiter.bucketSize) {
		toRead = int(r.limiter.bucketSize)
	}

	r.limiter.waitForTokens(int64(toRead))

	// Read the data
	n, err := r.reader.Read(p[:toRead])
	if n > 0 {
		r.limiter.consumeTokens(int64(n))
	}

	return n, err
}

// waitForTokens blocks until enough tokens are available
func (l *Limiter) waitForTokens(needed int64) {
	for {
		l.mu.Lock()
		l.refillTokens()

		if l.tokens >= needed {
			l.mu.Unlock()
			return
		}

		// Calculate wait time
		deficit := needed - l.tokens
		waitTime := time.Duration(float64(deficit) / float64(l.bytesPerSecond) * float64(time.Second))
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}
		l.mu.Unlock()

		time.Sleep(waitTime)
	}
}

// refillTokens adds tokens based on elapsed time (must be called with lock held)
func (l *Limiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(l.lastUpdate)

	// Add tokens based on elapsed time
	tokensToAdd := int64(float64(elapsed) / float64(time.Second) * float64(l.bytesPerSecond))
	if tokensToAdd > 0 {
		l.tokens += tokensToAdd
		if l.tokens > l.bucketSize {
			l.tokens = l.bucketSize
		}
		l.lastUpdate = now
	}
}

// consumeTokens removes tokens after a read (must be called after waitForTokens)
func (l *Limiter) consumeTokens(n int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens -= n
	if l.tokens < 0 {
		l.tokens = 0
	}
}

// ReadCloser wraps an io.ReadCloser with rate limiting
type ReadCloser struct {
	Reader
	closer io.Closer
}

// NewReadCloser wraps an io.ReadCloser with rate limiting
func NewReadCloser(ctx context.Context, rc io.ReadCloser, limiter *Limiter) io.ReadCloser {
	if limiter == nil {
		return rc // No limiting
	}
	return &ReadCloser{
		Reader: Reader{
			reader:  rc,
			limiter: limiter,
			ctx:     ctx,
		},
		closer: rc,
	}
}

// Close implements io.Closer
func (rc *ReadCloser) Close() error {
	return rc.closer.Close()
}
