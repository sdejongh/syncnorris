package output

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
)

// JSONFormatter formats output as JSON for automation and scripting
type JSONFormatter struct {
	writer     io.Writer
	totalFiles int
	totalBytes int64
	startTime  time.Time
	events     []JSONEvent
}

// JSONEvent represents a single event in the JSON output stream
type JSONEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Data      any       `json:"data,omitempty"`
}

// JSONStartData represents the data for a start event
type JSONStartData struct {
	TotalFiles int   `json:"total_files"`
	TotalBytes int64 `json:"total_bytes"`
}

// JSONFileData represents file-related event data
type JSONFileData struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written,omitempty"`
	TotalBytes   int64  `json:"total_bytes,omitempty"`
	Error        string `json:"error,omitempty"`
}

// JSONReportData represents the final report data
type JSONReportData struct {
	Status      string               `json:"status"`
	Duration    string               `json:"duration"`
	DurationMs  int64                `json:"duration_ms"`
	Stats       JSONStatsData        `json:"stats"`
	Differences []JSONDifferenceData `json:"differences,omitempty"`
	Errors      []JSONErrorData      `json:"errors,omitempty"`
}

// JSONDifferenceData represents a file difference
type JSONDifferenceData struct {
	Path       string          `json:"path"`
	Reason     string          `json:"reason"`
	Details    string          `json:"details,omitempty"`
	SourceInfo *JSONFileInfoData `json:"source_info,omitempty"`
	DestInfo   *JSONFileInfoData `json:"dest_info,omitempty"`
}

// JSONFileInfoData represents file info in JSON
type JSONFileInfoData struct {
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Hash    string `json:"hash,omitempty"`
}

// JSONStatsData represents statistics in JSON format
type JSONStatsData struct {
	Scanned    JSONScannedData    `json:"scanned"`
	Operations JSONOperationsData `json:"operations"`
	Transfer   JSONTransferData   `json:"transfer"`
}

// JSONScannedData represents scanned files statistics
type JSONScannedData struct {
	SourceFiles int32 `json:"source_files"`
	SourceDirs  int32 `json:"source_dirs"`
	DestFiles   int32 `json:"dest_files"`
	DestDirs    int32 `json:"dest_dirs"`
	UniqueFiles int32 `json:"unique_files"`
	UniqueDirs  int32 `json:"unique_dirs"`
}

// JSONOperationsData represents operations statistics
type JSONOperationsData struct {
	FilesCopied       int32 `json:"files_copied"`
	FilesUpdated      int32 `json:"files_updated"`
	FilesDeleted      int32 `json:"files_deleted"`
	FilesSynchronized int32 `json:"files_synchronized"`
	FilesSkipped      int32 `json:"files_skipped"`
	FilesErrored      int32 `json:"files_errored"`
	DirsCreated       int32 `json:"dirs_created"`
	DirsDeleted       int32 `json:"dirs_deleted"`
}

// JSONTransferData represents transfer statistics
type JSONTransferData struct {
	BytesTransferred int64  `json:"bytes_transferred"`
	AverageSpeed     int64  `json:"average_speed_bytes_per_sec,omitempty"`
	AverageSpeedStr  string `json:"average_speed,omitempty"`
}

// JSONErrorData represents an error entry
type JSONErrorData struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{
		events: make([]JSONEvent, 0),
	}
}

// Start initializes the formatter
func (f *JSONFormatter) Start(writer io.Writer, totalFiles int, totalBytes int64, maxWorkers int) error {
	if writer == nil {
		writer = os.Stdout
	}
	f.writer = writer
	f.totalFiles = totalFiles
	f.totalBytes = totalBytes
	f.startTime = time.Now()

	// Record start event
	f.events = append(f.events, JSONEvent{
		Timestamp: time.Now(),
		Type:      "start",
		Data: JSONStartData{
			TotalFiles: totalFiles,
			TotalBytes: totalBytes,
		},
	})

	return nil
}

// Progress reports progress during sync
func (f *JSONFormatter) Progress(update ProgressUpdate) error {
	// JSON formatter doesn't output progress events in real-time
	// to keep the output clean and parseable
	// Progress is accumulated and can be optionally included in the final report
	return nil
}

// Complete finalizes output and displays summary as JSON
func (f *JSONFormatter) Complete(report *models.SyncReport) error {
	if f.writer == nil {
		f.writer = io.Discard
	}

	// Calculate average speed
	var avgSpeed int64
	var avgSpeedStr string
	if report.Duration.Seconds() > 0 {
		avgSpeed = int64(float64(report.Stats.BytesTransferred.Load()) / report.Duration.Seconds())
		avgSpeedStr = formatBytes(avgSpeed) + "/s"
	}

	// Build errors list
	var errors []JSONErrorData
	for _, err := range report.Errors {
		errors = append(errors, JSONErrorData{
			Path:  err.FilePath,
			Error: err.Error,
		})
	}

	// Build differences list
	var differences []JSONDifferenceData
	for _, diff := range report.Differences {
		diffData := JSONDifferenceData{
			Path:    diff.RelativePath,
			Reason:  string(diff.Reason),
			Details: diff.Details,
		}
		if diff.SourceInfo != nil {
			diffData.SourceInfo = &JSONFileInfoData{
				Size:    diff.SourceInfo.Size,
				ModTime: diff.SourceInfo.ModTime.Format(time.RFC3339),
				Hash:    diff.SourceInfo.Hash,
			}
		}
		if diff.DestInfo != nil {
			diffData.DestInfo = &JSONFileInfoData{
				Size:    diff.DestInfo.Size,
				ModTime: diff.DestInfo.ModTime.Format(time.RFC3339),
				Hash:    diff.DestInfo.Hash,
			}
		}
		differences = append(differences, diffData)
	}

	// Create report data
	reportData := JSONReportData{
		Status:      string(report.Status),
		Duration:   report.Duration.Round(time.Millisecond).String(),
		DurationMs: report.Duration.Milliseconds(),
		Stats: JSONStatsData{
			Scanned: JSONScannedData{
				SourceFiles: report.Stats.SourceFilesScanned.Load(),
				SourceDirs:  report.Stats.SourceDirsScanned.Load(),
				DestFiles:   report.Stats.DestFilesScanned.Load(),
				DestDirs:    report.Stats.DestDirsScanned.Load(),
				UniqueFiles: report.Stats.FilesScanned.Load(),
				UniqueDirs:  report.Stats.DirsScanned.Load(),
			},
			Operations: JSONOperationsData{
				FilesCopied:       report.Stats.FilesCopied.Load(),
				FilesUpdated:      report.Stats.FilesUpdated.Load(),
				FilesDeleted:      report.Stats.FilesDeleted.Load(),
				FilesSynchronized: report.Stats.FilesSynchronized.Load(),
				FilesSkipped:      report.Stats.FilesSkipped.Load(),
				FilesErrored:      report.Stats.FilesErrored.Load(),
				DirsCreated:       report.Stats.DirsCreated.Load(),
				DirsDeleted:       report.Stats.DirsDeleted.Load(),
			},
			Transfer: JSONTransferData{
				BytesTransferred: report.Stats.BytesTransferred.Load(),
				AverageSpeed:     avgSpeed,
				AverageSpeedStr:  avgSpeedStr,
			},
		},
		Differences: differences,
		Errors:      errors,
	}

	// Add complete event
	f.events = append(f.events, JSONEvent{
		Timestamp: time.Now(),
		Type:      "complete",
		Data:      reportData,
	})

	// Output as JSON
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")

	// Output the final report directly (not wrapped in events)
	return encoder.Encode(reportData)
}

// Error reports an error
func (f *JSONFormatter) Error(err error) error {
	f.events = append(f.events, JSONEvent{
		Timestamp: time.Now(),
		Type:      "error",
		Data: map[string]string{
			"error": err.Error(),
		},
	})
	return nil
}

// Name returns the formatter name
func (f *JSONFormatter) Name() string {
	return "json"
}
