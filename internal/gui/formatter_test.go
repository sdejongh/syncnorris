package gui

import (
	"fmt"
	"testing"
	"time"

	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
)

func TestGUIFormatterStart(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)

	err := f.Start(nil, 42, 1024*1024, 5)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Type != eventLog {
			t.Fatalf("expected eventLog, got %d", ev.Type)
		}
		if ev.Log.Level != "INFO" {
			t.Fatalf("expected INFO level, got %s", ev.Log.Level)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestGUIFormatterCopySequence(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 10, 1000, 2)
	drain(ch)

	// COPY sequence: file_start → file_complete (no compare_start)
	f.Progress(output.ProgressUpdate{Type: "file_start", FilePath: "new.go", TotalBytes: 500, CurrentFile: 1})
	drain(ch) // progress event

	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "new.go", BytesWritten: 500, TotalBytes: 500, CurrentFile: 1})

	ev := expectLog(t, ch)
	if ev.Log.Level != "COPY" {
		t.Errorf("expected COPY, got %s", ev.Log.Level)
	}
}

func TestGUIFormatterUpdateSequence(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 10, 1000, 2)
	drain(ch)

	// UPDATE sequence: compare_start → file_start → file_complete
	f.Progress(output.ProgressUpdate{Type: "compare_start", FilePath: "existing.go", TotalBytes: 500, CurrentFile: 1})
	drain(ch)

	f.Progress(output.ProgressUpdate{Type: "file_start", FilePath: "existing.go", TotalBytes: 500, CurrentFile: 1})
	drain(ch)

	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "existing.go", BytesWritten: 500, TotalBytes: 500, CurrentFile: 1})

	ev := expectLog(t, ch)
	if ev.Log.Level != "UPDATE" {
		t.Errorf("expected UPDATE, got %s", ev.Log.Level)
	}
}

func TestGUIFormatterSkipSequence(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 10, 1000, 2)
	drain(ch)

	// SKIP sequence: compare_start → file_complete (no file_start)
	f.Progress(output.ProgressUpdate{Type: "compare_start", FilePath: "same.go", TotalBytes: 200, CurrentFile: 1})
	drain(ch)

	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "same.go", BytesWritten: 200, TotalBytes: 200, CurrentFile: 1})

	ev := expectLog(t, ch)
	if ev.Log.Level != "SKIP" {
		t.Errorf("expected SKIP, got %s", ev.Log.Level)
	}
}

func TestGUIFormatterErrorSequence(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 10, 1000, 2)
	drain(ch)

	f.Progress(output.ProgressUpdate{
		Type:        "file_error",
		FilePath:    "bad.go",
		CurrentFile: 1,
		Error:       fmt.Errorf("permission denied"),
	})

	ev := expectLog(t, ch)
	if ev.Log.Level != "ERROR" {
		t.Errorf("expected ERROR, got %s", ev.Log.Level)
	}
}

func TestGUIFormatterRunningStats(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 10, 1000, 2)
	drain(ch)

	// Simulate: 1 copy, 1 update, 1 skip
	// COPY
	f.Progress(output.ProgressUpdate{Type: "file_start", FilePath: "a.go", CurrentFile: 1})
	drain(ch)
	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "a.go", BytesWritten: 100, TotalBytes: 100, CurrentFile: 1})
	drain(ch) // log + progress

	// UPDATE
	f.Progress(output.ProgressUpdate{Type: "compare_start", FilePath: "b.go", CurrentFile: 2})
	drain(ch)
	f.Progress(output.ProgressUpdate{Type: "file_start", FilePath: "b.go", CurrentFile: 2})
	drain(ch)
	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "b.go", BytesWritten: 100, TotalBytes: 100, CurrentFile: 2})
	drain(ch)

	// SKIP
	f.Progress(output.ProgressUpdate{Type: "compare_start", FilePath: "c.go", CurrentFile: 3})
	drain(ch)
	f.Progress(output.ProgressUpdate{Type: "file_complete", FilePath: "c.go", BytesWritten: 100, TotalBytes: 100, CurrentFile: 3})

	// Last progress event should have stats
	events := drainAll(ch)
	var lastProgress *ProgressState
	for _, ev := range events {
		if ev.Type == eventProgress && ev.Progress != nil {
			lastProgress = ev.Progress
		}
	}

	if lastProgress == nil {
		t.Fatal("no progress event with stats found")
	}
	if lastProgress.Stats.Copied != 1 {
		t.Errorf("expected 1 copied, got %d", lastProgress.Stats.Copied)
	}
	if lastProgress.Stats.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", lastProgress.Stats.Updated)
	}
	if lastProgress.Stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", lastProgress.Stats.Skipped)
	}
}

func TestGUIFormatterComplete(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)
	_ = f.Start(nil, 5, 500, 1)
	drain(ch)

	report := &models.SyncReport{
		Status:   models.StatusSuccess,
		Duration: 2 * time.Second,
	}
	report.Stats.FilesCopied.Store(3)
	report.Stats.FilesUpdated.Store(1)
	report.Stats.FilesSynchronized.Store(1)

	err := f.Complete(report)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	// Should get summary log entries then eventComplete
	events := drainAll(ch)
	hasComplete := false
	hasSummaryLog := false
	for _, ev := range events {
		if ev.Type == eventComplete && ev.Report != nil {
			hasComplete = true
		}
		if ev.Type == eventLog && ev.Log != nil && ev.Log.Level == "INFO" {
			hasSummaryLog = true
		}
	}
	if !hasComplete {
		t.Error("expected eventComplete")
	}
	if !hasSummaryLog {
		t.Error("expected summary log entry")
	}
}

func TestGUIFormatterError(t *testing.T) {
	ch := make(chan UIEvent, 100)
	f := NewGUIFormatter(ch)

	testErr := fmt.Errorf("test error")
	err := f.Error(testErr)
	if err != nil {
		t.Fatalf("Error returned error: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Type != eventError {
			t.Fatalf("expected eventError, got %d", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestGUIFormatterName(t *testing.T) {
	ch := make(chan UIEvent, 1)
	f := NewGUIFormatter(ch)
	if f.Name() != "gui" {
		t.Fatalf("expected name 'gui', got '%s'", f.Name())
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestResolveAction(t *testing.T) {
	tests := []struct {
		name     string
		state    *fileState
		expected string
	}{
		{"nil state", nil, "DONE"},
		{"copy only", &fileState{hadCopy: true}, "COPY"},
		{"compare then copy", &fileState{hadCompare: true, hadCopy: true}, "UPDATE"},
		{"compare only", &fileState{hadCompare: true}, "SKIP"},
		{"neither", &fileState{}, "DONE"},
	}
	for _, tt := range tests {
		got := resolveAction(tt.state)
		if got != tt.expected {
			t.Errorf("resolveAction(%s) = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

// --- test helpers ---

func drain(ch chan UIEvent) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainAll(ch chan UIEvent) []UIEvent {
	var events []UIEvent
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
		default:
			return events
		}
	}
}

func expectLog(t *testing.T, ch chan UIEvent) UIEvent {
	t.Helper()
	for {
		select {
		case ev := <-ch:
			if ev.Type == eventLog {
				return ev
			}
			// skip progress events
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for log event")
			return UIEvent{}
		}
	}
}
