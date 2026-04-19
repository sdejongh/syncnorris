package job

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompletionLogAppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cl, err := NewCompletionLog(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	entries := []CompletionEntry{
		{Path: "a/b.txt", Size: 100, MTime: time.Unix(1000, 0), Hash: "abc123"},
		{Path: "c/d.png", Size: 2048, MTime: time.Unix(2000, 0), Hash: ""},
		{Path: "weird\ttab.txt", Size: 50, MTime: time.Unix(3000, 0), Hash: "-"}, // tab in path must be rejected or escaped
	}

	if err := cl.Append(entries[0]); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := cl.Append(entries[1]); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	// Tab in path: expect an error to keep format simple
	if err := cl.Append(entries[2]); err == nil {
		t.Error("expected error for tab in path")
	}

	if err := cl.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	loaded, err := LoadCompletionLog(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded["a/b.txt"].Size != 100 {
		t.Errorf("size mismatch")
	}
	if loaded["c/d.png"].Hash != "" {
		t.Errorf("expected empty hash for dash marker, got %q", loaded["c/d.png"].Hash)
	}
}

func TestCompletionLogTolerateMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.log")

	// Handcraft a file with one good and two bad lines
	content := "good/file.txt\t100\t1000000000\t-\nnot_enough_fields\nalso/good.txt\t200\t2000000000\tdeadbeef\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCompletionLog(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 good entries, got %d", len(loaded))
	}
}

func TestCompletionLogMissingFileReturnsEmpty(t *testing.T) {
	loaded, err := LoadCompletionLog(filepath.Join(t.TempDir(), "nope.log"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(loaded))
	}
}

func TestCompletionLogBatchFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.log")

	cl, err := NewCompletionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	for i := 0; i < 150; i++ {
		if err := cl.Append(CompletionEntry{
			Path:  filepath.Join("f", string(rune('a'+i%26))),
			Size:  int64(i),
			MTime: time.Unix(int64(i), 0),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Force flush
	if err := cl.Flush(); err != nil {
		t.Fatal(err)
	}

	// Read raw, count lines
	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines < 150 {
		t.Errorf("expected at least 150 lines flushed, got %d", lines)
	}
}
