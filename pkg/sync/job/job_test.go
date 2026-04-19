package job

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJobJSONRoundtrip(t *testing.T) {
	original := &Job{
		ID:          "a3f9b2e1",
		Source:      "/src",
		Destination: "/dest",
		Options: Options{
			Mode:            "oneway",
			Comparator:      "hash",
			Workers:         5,
			BandwidthLimit:  0,
			DeleteOrphans:   false,
			ExcludePatterns: []string{".git", "*.tmp"},
		},
		Status:        StatusRunning,
		CreatedAt:     time.Date(2026, 4, 19, 10, 15, 0, 0, time.UTC),
		Heartbeat:     time.Date(2026, 4, 19, 10, 22, 34, 0, time.UTC),
		CompletionLog: "/tmp/a3f9b2e1.log",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Status != StatusRunning {
		t.Errorf("Status mismatch: got %q", decoded.Status)
	}
	if len(decoded.Options.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns len mismatch: got %d", len(decoded.Options.ExcludePatterns))
	}
	if !decoded.Heartbeat.Equal(original.Heartbeat) {
		t.Errorf("Heartbeat mismatch")
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestJobStatusConstants(t *testing.T) {
	if StatusRunning != "running" {
		t.Errorf("StatusRunning got %q", StatusRunning)
	}
	if StatusPaused != "paused" {
		t.Errorf("StatusPaused got %q", StatusPaused)
	}
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted got %q", StatusCompleted)
	}
}
