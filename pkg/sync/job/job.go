// Package job provides persistence and lifecycle management for sync jobs,
// enabling pause/resume of one-way syncs across app restarts.
package job

import "time"

type Status string

const (
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
)

// Options captures the user-selected run configuration we need to
// restore on resume. Mirrors internal/gui.RunOptions but lives here
// to avoid a dependency on the GUI package.
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
