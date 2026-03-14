package gui

import (
	"testing"

	"github.com/sdejongh/syncnorris/pkg/models"
)

func TestBuildOperationDefaults(t *testing.T) {
	opts := RunOptions{
		Source:     "/tmp/src",
		Dest:       "/tmp/dst",
		Mode:       "oneway",
		Comparison: "hash",
		Conflict:   "newer",
		Workers:    5,
		BufferSize: 65536,
	}

	op, err := buildOperation(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.SourcePath != "/tmp/src" {
		t.Errorf("expected source /tmp/src, got %s", op.SourcePath)
	}
	if op.Mode != models.ModeOneWay {
		t.Errorf("expected oneway, got %s", op.Mode)
	}
	if op.ComparisonMethod != models.CompareHash {
		t.Errorf("expected hash, got %s", op.ComparisonMethod)
	}
	if op.MaxWorkers != 5 {
		t.Errorf("expected 5 workers, got %d", op.MaxWorkers)
	}
}

func TestBuildOperationValidation(t *testing.T) {
	opts := RunOptions{
		Source: "",
		Dest:   "/tmp/dst",
	}
	_, err := buildOperation(opts)
	if err == nil {
		t.Fatal("expected validation error for empty source")
	}
}

func TestBuildOperationExcludes(t *testing.T) {
	opts := RunOptions{
		Source:     "/tmp/src",
		Dest:       "/tmp/dst",
		Mode:       "oneway",
		Comparison: "hash",
		Conflict:   "newer",
		Workers:    5,
		BufferSize: 65536,
		Excludes:   "*.tmp, .git/, node_modules/",
	}
	op, err := buildOperation(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.ExcludePatterns) != 3 {
		t.Errorf("expected 3 exclude patterns, got %d: %v", len(op.ExcludePatterns), op.ExcludePatterns)
	}
}

func TestBuildOperationWorkerDefaults(t *testing.T) {
	opts := RunOptions{
		Source:     "/tmp/src",
		Dest:       "/tmp/dst",
		Mode:       "oneway",
		Comparison: "hash",
		Conflict:   "newer",
		Workers:    0, // should default to 5
		BufferSize: 0, // should default to 65536
	}
	op, err := buildOperation(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.MaxWorkers != 5 {
		t.Errorf("expected 5 workers, got %d", op.MaxWorkers)
	}
	if op.BufferSize != 65536 {
		t.Errorf("expected 65536 buffer, got %d", op.BufferSize)
	}
}

func TestParseExcludes(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"  ", 0},
		{"*.tmp", 1},
		{"*.tmp, .git/", 2},
		{"*.tmp, .git/, node_modules/, *.log", 4},
		{" *.tmp , .git/ , ", 2}, // trailing comma, spaces
	}
	for _, tt := range tests {
		got := parseExcludes(tt.input)
		if len(got) != tt.expected {
			t.Errorf("parseExcludes(%q) = %v (len %d), want len %d", tt.input, got, len(got), tt.expected)
		}
	}
}

func TestBuildComparator(t *testing.T) {
	methods := []models.ComparisonMethod{
		models.CompareNameSize,
		models.CompareHash,
		models.CompareMD5,
		models.CompareBinary,
		models.CompareTimestamp,
	}
	for _, m := range methods {
		c, err := buildComparator(m, 65536)
		if err != nil {
			t.Errorf("buildComparator(%s) failed: %v", m, err)
		}
		if c == nil {
			t.Errorf("buildComparator(%s) returned nil", m)
		}
	}

	// Invalid method
	_, err := buildComparator("invalid", 65536)
	if err == nil {
		t.Error("expected error for invalid comparison method")
	}
}
