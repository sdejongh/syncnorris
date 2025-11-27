package sync

import (
	"time"
)

// TaskStatus represents the status of a file task in the pipeline
type TaskStatus string

const (
	// TaskPending indicates the task is waiting to be processed
	TaskPending TaskStatus = "pending"
	// TaskProcessing indicates the task is currently being processed by a worker
	TaskProcessing TaskStatus = "processing"
	// TaskCompleted indicates the task completed successfully
	TaskCompleted TaskStatus = "completed"
	// TaskError indicates the task failed with an error
	TaskError TaskStatus = "error"
)

// TaskResult represents what happened to the file
type TaskResult string

const (
	// ResultCopied indicates the file was copied (new file)
	ResultCopied TaskResult = "copied"
	// ResultUpdated indicates the file was updated (content changed)
	ResultUpdated TaskResult = "updated"
	// ResultSynchronized indicates the file was already identical
	ResultSynchronized TaskResult = "synchronized"
	// ResultSkipped indicates the file was skipped (dest-only, etc.)
	ResultSkipped TaskResult = "skipped"
	// ResultFailed indicates the file processing failed
	ResultFailed TaskResult = "failed"
)

// FileTask represents a file to be processed in the sync pipeline
type FileTask struct {
	// RelativePath is the path relative to the source root
	RelativePath string

	// Size is the file size in bytes (from source scan)
	Size int64

	// ModTime is the modification time (from source scan)
	ModTime time.Time

	// Status tracks the current state of this task
	Status TaskStatus

	// Result indicates what action was taken
	Result TaskResult

	// Error holds any error that occurred during processing
	Error error

	// BytesTransferred tracks how many bytes were actually transferred
	BytesTransferred int64

	// ProcessingDuration tracks how long the worker spent on this task
	ProcessingDuration time.Duration

	// WorkerID identifies which worker processed this task
	WorkerID int
}

// NewFileTask creates a new file task from scan data
func NewFileTask(relativePath string, size int64, modTime time.Time) *FileTask {
	return &FileTask{
		RelativePath: relativePath,
		Size:         size,
		ModTime:      modTime,
		Status:       TaskPending,
	}
}

// MarkProcessing marks the task as being processed by a worker
func (t *FileTask) MarkProcessing(workerID int) {
	t.Status = TaskProcessing
	t.WorkerID = workerID
}

// MarkCompleted marks the task as successfully completed
func (t *FileTask) MarkCompleted(result TaskResult, bytesTransferred int64, duration time.Duration) {
	t.Status = TaskCompleted
	t.Result = result
	t.BytesTransferred = bytesTransferred
	t.ProcessingDuration = duration
}

// MarkError marks the task as failed with an error
func (t *FileTask) MarkError(err error, duration time.Duration) {
	t.Status = TaskError
	t.Result = ResultFailed
	t.Error = err
	t.ProcessingDuration = duration
}
