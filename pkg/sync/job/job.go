// Package job provides persistence and lifecycle management for sync jobs,
// enabling pause/resume of one-way syncs across app restarts.
package job

import "time"

// Status represents the lifecycle state of a sync job.
type Status string

const (
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
)

// Options captures the subset of RunOptions needed to resume a
// one-way sync. It lives in this package (not internal/gui) to
// avoid an import cycle and to remain importable from the CLI.
type Options struct {
	Mode            string   `json:"mode"`
	Comparator      string   `json:"comparator"`
	Workers         int      `json:"workers"`
	BufferSize      int      `json:"buffer_size"`
	BandwidthLimit  int64    `json:"bandwidth_limit"`
	DeleteOrphans   bool     `json:"delete_orphans"`
	DryRun          bool     `json:"dry_run"`
	ExcludePatterns []string `json:"exclude_patterns"`
}

// Job holds persisted metadata for a one-way sync job, enabling
// pause and resume across application restarts.
type Job struct {
	ID            string    `json:"id"`
	Source        string    `json:"source"`
	Destination   string    `json:"destination"`
	Options       Options   `json:"options"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Heartbeat     time.Time `json:"heartbeat"`
	CompletionLog string    `json:"completion_log"`
}
