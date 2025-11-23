package output

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
	"golang.org/x/term"
)

const (
	speedWindow = 3 * time.Second // Window for instantaneous speed calculation
)

// getMaxDisplayFiles returns maximum number of files to show based on OS
// Windows benefits from fewer lines to reduce flicker
func getMaxDisplayFiles() int {
	if runtime.GOOS == "windows" {
		return 3 // Show fewer files on Windows to minimize screen updates
	}
	return 5 // Show more files on Unix systems
}

// getUpdateInterval returns the progress update interval based on OS
// Windows terminals have higher latency with ANSI sequences, so we use a longer interval
func getUpdateInterval() time.Duration {
	if runtime.GOOS == "windows" {
		return 300 * time.Millisecond // Slower updates on Windows to reduce flicker
	}
	return 100 * time.Millisecond // Faster updates on Unix systems
}

// fileProgress tracks progress of an individual file
type fileProgress struct {
	path         string
	current      int64
	total        int64
	startTime    time.Time
	status       string // "copying", "hashing", "complete", "error"
	errorMessage string
}

// speedSample represents a point in time for speed calculation
type speedSample struct {
	timestamp time.Time
	bytes     int64
}

// ProgressFormatter formats output with progress bars
type ProgressFormatter struct {
	writer         io.Writer
	totalFiles     int
	totalBytes     int64
	processedFiles int
	processedBytes int64
	startTime      time.Time

	mu             sync.Mutex
	activeFiles    map[int]*fileProgress // fileIndex -> progress
	lastDisplay    time.Time
	displayLines   int // Number of lines currently displayed
	termWidth      int // Terminal width (for preventing line wrapping)

	// For instantaneous speed calculation
	speedSamples   []speedSample // History of bytes transferred
}

// NewProgressFormatter creates a new progress bar formatter
func NewProgressFormatter() *ProgressFormatter {
	return &ProgressFormatter{
		activeFiles: make(map[int]*fileProgress),
	}
}

// Start initializes the formatter
func (f *ProgressFormatter) Start(writer io.Writer, totalFiles int, totalBytes int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if writer == nil {
		writer = os.Stdout
	}

	f.writer = writer
	f.totalFiles = totalFiles
	f.totalBytes = totalBytes

	// Detect terminal width to prevent line wrapping issues
	// Try to get terminal width from stdout
	if file, ok := writer.(*os.File); ok {
		if width, _, err := term.GetSize(int(file.Fd())); err == nil && width > 0 {
			f.termWidth = width
		}
	}
	// Default to 120 if we couldn't detect (pipe, redirect, etc.)
	if f.termWidth == 0 {
		f.termWidth = 120
	}

	// Reset progress counters when starting a new phase
	f.processedFiles = 0
	f.processedBytes = 0

	// Only reset startTime on first call (when writer is being set)
	if f.startTime.IsZero() {
		f.startTime = time.Now()
	}
	f.lastDisplay = time.Now()

	// Initial display
	f.render()

	return nil
}

// Progress reports progress during sync
func (f *ProgressFormatter) Progress(update ProgressUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch update.Type {
	case "file_start":
		f.activeFiles[update.CurrentFile] = &fileProgress{
			path:      update.FilePath,
			current:   0,
			total:     update.TotalBytes,
			startTime: time.Now(),
			status:    "copying",
		}
		// Render immediately to show the file started
		f.render()
		f.lastDisplay = time.Now()

	case "compare_start":
		f.activeFiles[update.CurrentFile] = &fileProgress{
			path:      update.FilePath,
			current:   0,
			total:     update.TotalBytes,
			startTime: time.Now(),
			status:    "hashing",
		}
		// Render immediately to show the hashing icon
		f.render()
		f.lastDisplay = time.Now()

	case "file_progress":
		if fp, exists := f.activeFiles[update.CurrentFile]; exists {
			fp.current = update.BytesWritten
			if update.TotalBytes > 0 {
				fp.total = update.TotalBytes
			}
		}

	case "compare_complete":
		// Comparison is complete - remove from active display but don't count as processed
		// (files will be counted when actually transferred or marked as synchronized)
		if fp, exists := f.activeFiles[update.CurrentFile]; exists {
			fp.current = fp.total
			fp.status = "complete"

			// Remove from display after a short delay
			go func(fileIdx int) {
				time.Sleep(500 * time.Millisecond)
				f.mu.Lock()
				delete(f.activeFiles, fileIdx)
				f.mu.Unlock()
			}(update.CurrentFile)
		}
		// Don't increment processedFiles or processedBytes here

	case "file_complete":
		if fp, exists := f.activeFiles[update.CurrentFile]; exists {
			fp.current = fp.total
			fp.status = "complete"

			// Keep the file visible for a short time before removing
			// This allows users to see files that were compared quickly
			go func(fileIdx int) {
				time.Sleep(500 * time.Millisecond)
				f.mu.Lock()
				delete(f.activeFiles, fileIdx)
				f.mu.Unlock()
			}(update.CurrentFile)
		}
		f.processedFiles++
		f.processedBytes += update.BytesWritten

	case "file_error":
		if fp, exists := f.activeFiles[update.CurrentFile]; exists {
			fp.status = "error"
			if update.Error != nil {
				fp.errorMessage = update.Error.Error()
			}
		}
		f.processedFiles++
		delete(f.activeFiles, update.CurrentFile)
	}

	// Update display if enough time has passed (avoid flickering)
	// Use platform-specific interval (Windows needs longer intervals)
	now := time.Now()
	if now.Sub(f.lastDisplay) > getUpdateInterval() {
		f.render()
		f.lastDisplay = now
	}

	return nil
}

// render displays the current state
func (f *ProgressFormatter) render() {
	if f.writer == nil {
		return
	}

	// Save cursor position on first render, then restore it on subsequent renders
	if f.displayLines == 0 {
		// First render - hide cursor to prevent flicker (especially on Windows)
		fmt.Fprint(f.writer, "\033[?25l") // Hide cursor
		f.renderContent()
		return
	}

	// Subsequent renders - move up and clear each line individually
	// Build all escape sequences into a single string to avoid buffering issues
	var escapeSeq strings.Builder

	// Hide cursor to reduce flicker (especially important on Windows)
	escapeSeq.WriteString("\033[?25l")

	for i := 0; i < f.displayLines; i++ {
		escapeSeq.WriteString("\033[1A") // Move up one line
		escapeSeq.WriteString("\033[2K") // Clear entire line
	}
	escapeSeq.WriteString("\r") // Move cursor to beginning of line

	// Write all escape sequences at once
	fmt.Fprint(f.writer, escapeSeq.String())

	// Flush if the writer supports it to ensure ANSI codes are executed
	// before we write new content
	if flusher, ok := f.writer.(interface{ Sync() error }); ok {
		flusher.Sync()
	}

	// Now render the new content
	f.renderContent()
}

// truncateLine ensures a line doesn't exceed terminal width
func (f *ProgressFormatter) truncateLine(line string) string {
	// Account for ANSI color codes which don't take visual space
	// For simplicity, just truncate based on rune count
	runes := []rune(line)
	if len(runes) > f.termWidth {
		return string(runes[:f.termWidth-3]) + "..."
	}
	return line
}

// renderContent renders the actual progress display
func (f *ProgressFormatter) renderContent() {
	lines := 0

	// Build all content in memory to reduce flicker (especially on Windows)
	var content strings.Builder

	// Show active files, sorted alphabetically
	// Platform-specific: Windows shows fewer files to reduce flicker
	maxFiles := getMaxDisplayFiles()

	// First, collect and sort the active files
	type indexedFile struct {
		index int
		fp    *fileProgress
	}
	var sortedFiles []indexedFile
	for idx, fp := range f.activeFiles {
		sortedFiles = append(sortedFiles, indexedFile{idx, fp})
	}

	// Sort by file path alphabetically
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].fp.path < sortedFiles[j].fp.path
	})

	// Display header if there are active files
	if len(sortedFiles) > 0 {
		header1 := fmt.Sprintf("%-3s  %-50s  %8s  %12s  %12s",
			"", "File", "Progress", "Copied", "Total")
		header2 := fmt.Sprintf("%-3s  %-50s  %8s  %12s  %12s",
			"", "────────────────────────────────────────────────", "────────", "────────────", "────────────")
		content.WriteString(f.truncateLine(header1) + "\n")
		content.WriteString(f.truncateLine(header2) + "\n")
		lines += 2
	}

	count := 0
	for _, item := range sortedFiles {
		if count >= maxFiles {
			break
		}

		fp := item.fp
		percent := float64(0)
		if fp.total > 0 {
			percent = float64(fp.current) / float64(fp.total) * 100
		}

		// Truncate filename if too long
		filename := fp.path
		maxFilenameLen := 50
		if len(filename) > maxFilenameLen {
			filename = "..." + filename[len(filename)-maxFilenameLen+3:]
		}

		statusIcon := "📄"
		switch fp.status {
		case "copying":
			statusIcon = "⏳"
		case "complete":
			statusIcon = "✅"
		case "error":
			statusIcon = "❌"
		case "hashing":
			statusIcon = "🔍"
		}

		// Format with aligned columns
		fileLine := fmt.Sprintf("%s  %-50s  %7.1f%%  %12s  %12s",
			statusIcon,
			filename,
			percent,
			formatBytes(fp.current),
			formatBytes(fp.total),
		)
		content.WriteString(f.truncateLine(fileLine) + "\n")
		lines++
		count++
	}

	// Add separator if there are active files
	if count > 0 {
		content.WriteString("\n")
		lines++
	}

	// Calculate current bytes including in-progress files
	// Don't count files with status="complete" as they're already in processedBytes
	currentBytes := f.processedBytes
	for _, fp := range f.activeFiles {
		if fp.status != "complete" {
			currentBytes += fp.current
		}
	}

	// Record current sample for speed calculation
	now := time.Now()
	f.speedSamples = append(f.speedSamples, speedSample{
		timestamp: now,
		bytes:     currentBytes,
	})

	// Remove samples older than the window
	cutoff := now.Add(-speedWindow)
	validSamples := f.speedSamples[:0]
	for _, sample := range f.speedSamples {
		if sample.timestamp.After(cutoff) {
			validSamples = append(validSamples, sample)
		}
	}
	f.speedSamples = validSamples

	// Calculate instantaneous speed (bytes/sec over the window)
	instantSpeed := int64(0)
	if len(f.speedSamples) >= 2 {
		oldest := f.speedSamples[0]
		newest := f.speedSamples[len(f.speedSamples)-1]
		duration := newest.timestamp.Sub(oldest.timestamp).Seconds()
		if duration > 0 {
			bytesDiff := newest.bytes - oldest.bytes
			instantSpeed = int64(float64(bytesDiff) / duration)
		}
	}

	// Calculate average speed (for ETA and display)
	elapsed := time.Since(f.startTime).Seconds()
	avgSpeed := int64(0)
	if elapsed > 0 {
		avgSpeed = int64(float64(currentBytes) / elapsed)
	}

	// Use instantaneous speed if available, otherwise fall back to average
	displaySpeed := instantSpeed
	if displaySpeed == 0 {
		displaySpeed = avgSpeed
	}

	// Calculate ETA using instantaneous speed for better accuracy
	eta := ""
	if displaySpeed > 0 && f.totalBytes > currentBytes {
		remaining := f.totalBytes - currentBytes
		etaSeconds := float64(remaining) / float64(displaySpeed)
		eta = formatDuration(time.Duration(etaSeconds) * time.Second)
	}

	barWidth := 40

	// First progress bar: Bytes transferred
	bytesPercent := float64(0)
	if f.totalBytes > 0 {
		bytesPercent = float64(currentBytes) / float64(f.totalBytes) * 100
	}

	bytesFilled := int(float64(barWidth) * bytesPercent / 100)
	if bytesFilled > barWidth {
		bytesFilled = barWidth
	}

	bytesBar := ""
	for i := 0; i < barWidth; i++ {
		if i < bytesFilled {
			bytesBar += "█"
		} else {
			bytesBar += "░"
		}
	}

	// Build complete data line
	dataLine := fmt.Sprintf("Data:    [%s] %3.0f%% %s/%s",
		bytesBar,
		bytesPercent,
		formatBytes(currentBytes),
		formatBytes(f.totalBytes),
	)

	// Display instantaneous speed and average in parentheses
	if displaySpeed > 0 {
		dataLine += fmt.Sprintf(" @ %s/s", formatBytes(displaySpeed))
		if avgSpeed > 0 && avgSpeed != displaySpeed {
			dataLine += fmt.Sprintf(" (avg: %s/s)", formatBytes(avgSpeed))
		}
	}

	if eta != "" {
		dataLine += fmt.Sprintf(" ETA: %s", eta)
	}

	content.WriteString(f.truncateLine(dataLine) + "\n")
	lines++

	// Second progress bar: Files processed
	filesPercent := float64(0)
	if f.totalFiles > 0 {
		filesPercent = float64(f.processedFiles) / float64(f.totalFiles) * 100
	}

	filesFilled := int(float64(barWidth) * filesPercent / 100)
	if filesFilled > barWidth {
		filesFilled = barWidth
	}

	filesBar := ""
	for i := 0; i < barWidth; i++ {
		if i < filesFilled {
			filesBar += "█"
		} else {
			filesBar += "░"
		}
	}

	filesLine := fmt.Sprintf("Files:   [%s] %3.0f%% (%d/%d files)",
		filesBar,
		filesPercent,
		f.processedFiles,
		f.totalFiles,
	)
	content.WriteString(f.truncateLine(filesLine) + "\n")
	lines++

	f.displayLines = lines

	// Write all content at once to minimize flicker
	fmt.Fprint(f.writer, content.String())
}

// Complete finalizes output and displays summary
func (f *ProgressFormatter) Complete(report *models.SyncReport) error {
	f.mu.Lock()

	// Calculate final average speed for the summary
	elapsed := time.Since(f.startTime).Seconds()
	avgSpeed := int64(0)
	if elapsed > 0 {
		avgSpeed = int64(float64(report.Stats.BytesTransferred.Load()) / elapsed)
	}

	f.mu.Unlock()

	// Final render
	f.mu.Lock()
	f.render()
	f.mu.Unlock()

	// Show cursor again (was hidden to prevent flicker)
	fmt.Fprint(f.writer, "\033[?25h")

	// Move past the progress display
	fmt.Fprintf(f.writer, "\n")

	// Display summary with average speed
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Sync completed in %s\n", report.Duration.Round(time.Millisecond))
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Summary:\n")
	fmt.Fprintf(f.writer, "  Scanned:\n")
	fmt.Fprintf(f.writer, "    Source:         %d files, %d dirs\n", report.Stats.SourceFilesScanned.Load(), report.Stats.SourceDirsScanned.Load())
	fmt.Fprintf(f.writer, "    Destination:    %d files, %d dirs\n", report.Stats.DestFilesScanned.Load(), report.Stats.DestDirsScanned.Load())
	fmt.Fprintf(f.writer, "    Unique paths:   %d files, %d dirs\n", report.Stats.FilesScanned.Load(), report.Stats.DirsScanned.Load())
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "  Operations:\n")
	fmt.Fprintf(f.writer, "    Files copied:       %d\n", report.Stats.FilesCopied.Load())
	fmt.Fprintf(f.writer, "    Files updated:      %d\n", report.Stats.FilesUpdated.Load())
	fmt.Fprintf(f.writer, "    Files synchronized: %d\n", report.Stats.FilesSynchronized.Load())
	fmt.Fprintf(f.writer, "    Files skipped:      %d\n", report.Stats.FilesSkipped.Load())
	fmt.Fprintf(f.writer, "    Files errored:      %d\n", report.Stats.FilesErrored.Load())
	fmt.Fprintf(f.writer, "    Dirs created:       %d\n", report.Stats.DirsCreated.Load())
	fmt.Fprintf(f.writer, "    Dirs deleted:       %d\n", report.Stats.DirsDeleted.Load())
	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "  Transfer:\n")
	fmt.Fprintf(f.writer, "    Data:           %s\n", formatBytes(report.Stats.BytesTransferred.Load()))

	if avgSpeed > 0 {
		fmt.Fprintf(f.writer, "    Average speed:  %s/s\n", formatBytes(avgSpeed))
	}

	fmt.Fprintf(f.writer, "\n")
	fmt.Fprintf(f.writer, "Status: %s\n", report.Status)

	if len(report.Errors) > 0 {
		fmt.Fprintf(f.writer, "\nErrors:\n")
		for _, err := range report.Errors {
			fmt.Fprintf(f.writer, "  %s: %s\n", err.FilePath, err.Error)
		}
	}

	return nil
}

// Error reports an error
func (f *ProgressFormatter) Error(err error) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.writer != nil {
		fmt.Fprintf(f.writer, "\n❌ Error: %v\n", err)
	}
	return nil
}

// Name returns the formatter name
func (f *ProgressFormatter) Name() string {
	return "progress"
}
