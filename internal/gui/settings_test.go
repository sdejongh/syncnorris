package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s.Mode != "oneway" {
		t.Errorf("expected oneway, got %s", s.Mode)
	}
	if s.Workers != 5 {
		t.Errorf("expected 5 workers, got %d", s.Workers)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-settings.json")

	// Patch settingsPath for test
	s := DefaultSettings()
	s.Mode = "bidirectional"
	s.Workers = 8
	s.SourceHistory = []string{"/tmp/src1", "/tmp/src2"}
	s.DestHistory = []string{"/tmp/dst1"}

	// Write manually
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Read back
	raw, _ := os.ReadFile(path)
	loaded := DefaultSettings()
	if err := json.Unmarshal(raw, loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.Mode != "bidirectional" {
		t.Errorf("expected bidirectional, got %s", loaded.Mode)
	}
	if loaded.Workers != 8 {
		t.Errorf("expected 8, got %d", loaded.Workers)
	}
	if len(loaded.SourceHistory) != 2 {
		t.Errorf("expected 2 source history, got %d", len(loaded.SourceHistory))
	}
}

func TestAddToHistory(t *testing.T) {
	var h []string

	// Add first entry
	h = addToHistory(h, "/a")
	if len(h) != 1 || h[0] != "/a" {
		t.Fatalf("expected [/a], got %v", h)
	}

	// Add second — prepend
	h = addToHistory(h, "/b")
	if len(h) != 2 || h[0] != "/b" || h[1] != "/a" {
		t.Fatalf("expected [/b, /a], got %v", h)
	}

	// Add duplicate — moves to front
	h = addToHistory(h, "/a")
	if len(h) != 2 || h[0] != "/a" || h[1] != "/b" {
		t.Fatalf("expected [/a, /b], got %v", h)
	}

	// Empty string ignored
	h = addToHistory(h, "")
	if len(h) != 2 {
		t.Fatalf("expected 2, got %d", len(h))
	}

	// Fill to max
	for i := 0; i < 15; i++ {
		h = addToHistory(h, filepath.Join("/path", string(rune('a'+i))))
	}
	if len(h) != maxHistory {
		t.Fatalf("expected %d, got %d", maxHistory, len(h))
	}
}
