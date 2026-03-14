//go:build !nogui

package gui

import (
	"sync"
	"time"
)

const bwWindowSize = 60 // 60 samples = 1 minute of history at 1 sample/sec

// BandwidthSample holds one second of bandwidth data.
type BandwidthSample struct {
	BytesPerSec float64
	Time        time.Time
}

// BandwidthTracker collects bandwidth samples over a sliding window.
type BandwidthTracker struct {
	mu             sync.Mutex
	samples        [bwWindowSize]BandwidthSample
	head           int // next write position (ring buffer)
	count          int // number of valid samples
	lastBytes      int64
	lastSampleTime time.Time
	totalBytes     int64
	startTime      time.Time
}

// NewBandwidthTracker creates a new tracker.
func NewBandwidthTracker() *BandwidthTracker {
	now := time.Now()
	return &BandwidthTracker{
		lastSampleTime: now,
		startTime:      now,
	}
}

// Update records the current cumulative bytes transferred.
// Call this frequently; it auto-samples once per second.
func (bt *BandwidthTracker) Update(totalBytes int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	bt.totalBytes = totalBytes
	now := time.Now()
	elapsed := now.Sub(bt.lastSampleTime)

	if elapsed < time.Second {
		return
	}

	// Compute bytes/sec since last sample
	delta := totalBytes - bt.lastBytes
	secs := elapsed.Seconds()
	bps := float64(delta) / secs

	bt.samples[bt.head] = BandwidthSample{
		BytesPerSec: bps,
		Time:        now,
	}
	bt.head = (bt.head + 1) % bwWindowSize
	if bt.count < bwWindowSize {
		bt.count++
	}

	bt.lastBytes = totalBytes
	bt.lastSampleTime = now
}

// Current returns the most recent bandwidth sample (bytes/sec).
func (bt *BandwidthTracker) Current() float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if bt.count == 0 {
		return 0
	}
	idx := (bt.head - 1 + bwWindowSize) % bwWindowSize
	return bt.samples[idx].BytesPerSec
}

// Average returns the average bandwidth since the start (bytes/sec).
func (bt *BandwidthTracker) Average() float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	elapsed := time.Since(bt.startTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(bt.totalBytes) / elapsed
}

// Samples returns up to the last bwWindowSize samples (oldest first).
func (bt *BandwidthTracker) Samples() []BandwidthSample {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if bt.count == 0 {
		return nil
	}

	result := make([]BandwidthSample, bt.count)
	start := (bt.head - bt.count + bwWindowSize) % bwWindowSize
	for i := 0; i < bt.count; i++ {
		result[i] = bt.samples[(start+i)%bwWindowSize]
	}
	return result
}

// Reset clears all samples and counters.
func (bt *BandwidthTracker) Reset() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	now := time.Now()
	bt.count = 0
	bt.head = 0
	bt.lastBytes = 0
	bt.totalBytes = 0
	bt.lastSampleTime = now
	bt.startTime = now
}
