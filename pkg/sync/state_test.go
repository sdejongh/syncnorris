package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ============== SyncState Tests ==============

func TestNewSyncState(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	if state.Version != stateFileVersion {
		t.Errorf("Version = %d, want %d", state.Version, stateFileVersion)
	}
	if state.SourcePath != "/source" {
		t.Errorf("SourcePath = %s, want /source", state.SourcePath)
	}
	if state.DestPath != "/dest" {
		t.Errorf("DestPath = %s, want /dest", state.DestPath)
	}
	if state.Files == nil {
		t.Error("Files map should be initialized")
	}
	if len(state.Files) != 0 {
		t.Errorf("Files should be empty, got %d entries", len(state.Files))
	}
}

func TestSyncState_UpdateFile(t *testing.T) {
	state := NewSyncState("/source", "/dest")
	modTime := time.Now()

	state.UpdateFile("test.txt", 1024, modTime, "hash123", true, true, false)

	fileState := state.GetFileState("test.txt")
	if fileState == nil {
		t.Fatal("GetFileState returned nil")
	}
	if fileState.RelativePath != "test.txt" {
		t.Errorf("RelativePath = %s, want test.txt", fileState.RelativePath)
	}
	if fileState.Size != 1024 {
		t.Errorf("Size = %d, want 1024", fileState.Size)
	}
	if !fileState.ModTime.Equal(modTime) {
		t.Errorf("ModTime mismatch")
	}
	if fileState.Hash != "hash123" {
		t.Errorf("Hash = %s, want hash123", fileState.Hash)
	}
	if !fileState.ExistsInSource {
		t.Error("ExistsInSource should be true")
	}
	if !fileState.ExistsInDest {
		t.Error("ExistsInDest should be true")
	}
	if fileState.IsDir {
		t.Error("IsDir should be false")
	}
}

func TestSyncState_UpdateFile_DeletesBothSides(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	// Add a file
	state.UpdateFile("test.txt", 1024, time.Now(), "", true, true, false)

	// Update to not exist on either side - should be removed
	state.UpdateFile("test.txt", 0, time.Time{}, "", false, false, false)

	if state.GetFileState("test.txt") != nil {
		t.Error("File should be removed when not existing on either side")
	}
}

func TestSyncState_RemoveFile(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	state.UpdateFile("test.txt", 1024, time.Now(), "", true, true, false)
	if state.GetFileState("test.txt") == nil {
		t.Fatal("File should exist before removal")
	}

	state.RemoveFile("test.txt")

	if state.GetFileState("test.txt") != nil {
		t.Error("File should be removed")
	}
}

func TestSyncState_GetFileState_NotFound(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	if state.GetFileState("nonexistent.txt") != nil {
		t.Error("GetFileState should return nil for non-existent file")
	}
}

func TestSyncState_MarkSyncComplete(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	if !state.IsFirstSync() {
		t.Error("Should be first sync before MarkSyncComplete")
	}

	state.MarkSyncComplete()

	if state.IsFirstSync() {
		t.Error("Should not be first sync after MarkSyncComplete")
	}
	if state.LastSyncTime.IsZero() {
		t.Error("LastSyncTime should be set")
	}
}

func TestSyncState_IsFirstSync(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	if !state.IsFirstSync() {
		t.Error("New state should be first sync")
	}

	state.LastSyncTime = time.Now()

	if state.IsFirstSync() {
		t.Error("Should not be first sync after setting LastSyncTime")
	}
}

// ============== State Persistence Tests ==============

func TestSyncState_SaveAndLoad(t *testing.T) {
	// Use temp directories
	tempDir, err := os.MkdirTemp("", "syncnorris-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourcePath := filepath.Join(tempDir, "source")
	destPath := filepath.Join(tempDir, "dest")

	// Create state and add some files
	state := NewSyncState(sourcePath, destPath)
	modTime := time.Now().Truncate(time.Second) // Truncate for JSON roundtrip

	state.UpdateFile("file1.txt", 100, modTime, "hash1", true, true, false)
	state.UpdateFile("file2.txt", 200, modTime, "hash2", true, false, false)
	state.UpdateFile("dir/file3.txt", 300, modTime, "", true, true, false)
	state.MarkSyncComplete()

	// Save
	err = state.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := LoadState(sourcePath, destPath)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	// Verify
	if loaded.Version != state.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, state.Version)
	}
	if loaded.SourcePath != sourcePath {
		t.Errorf("SourcePath = %s, want %s", loaded.SourcePath, sourcePath)
	}
	if loaded.DestPath != destPath {
		t.Errorf("DestPath = %s, want %s", loaded.DestPath, destPath)
	}
	if len(loaded.Files) != 3 {
		t.Errorf("Files count = %d, want 3", len(loaded.Files))
	}

	// Verify file1
	file1 := loaded.GetFileState("file1.txt")
	if file1 == nil {
		t.Fatal("file1.txt not found")
	}
	if file1.Size != 100 {
		t.Errorf("file1 Size = %d, want 100", file1.Size)
	}
	if file1.Hash != "hash1" {
		t.Errorf("file1 Hash = %s, want hash1", file1.Hash)
	}

	// Verify file2
	file2 := loaded.GetFileState("file2.txt")
	if file2 == nil {
		t.Fatal("file2.txt not found")
	}
	if !file2.ExistsInSource {
		t.Error("file2 should exist in source")
	}
	if file2.ExistsInDest {
		t.Error("file2 should not exist in dest")
	}
}

func TestLoadState_NonExistent(t *testing.T) {
	// Use unique paths that don't have existing state
	sourcePath := "/tmp/nonexistent-source-12345"
	destPath := "/tmp/nonexistent-dest-12345"

	state, err := LoadState(sourcePath, destPath)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	// Should return new empty state
	if state == nil {
		t.Fatal("LoadState should return new state")
	}
	if len(state.Files) != 0 {
		t.Error("Should be empty state")
	}
	if !state.IsFirstSync() {
		t.Error("Should be first sync")
	}
}

func TestClearState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourcePath := filepath.Join(tempDir, "source")
	destPath := filepath.Join(tempDir, "dest")

	// Create and save state
	state := NewSyncState(sourcePath, destPath)
	state.UpdateFile("test.txt", 100, time.Now(), "", true, true, false)
	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Clear state
	err = ClearState(sourcePath, destPath)
	if err != nil {
		t.Fatalf("ClearState() error = %v", err)
	}

	// Loading should return empty state
	loaded, err := LoadState(sourcePath, destPath)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(loaded.Files) != 0 {
		t.Error("State should be cleared")
	}
}

func TestClearState_NonExistent(t *testing.T) {
	// Should not error for non-existent state
	err := ClearState("/tmp/nonexistent-1", "/tmp/nonexistent-2")
	if err != nil {
		t.Errorf("ClearState should not error for non-existent: %v", err)
	}
}

// ============== DetectChange Tests ==============

func TestSyncState_DetectChange_Created(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	change := state.DetectChange("new.txt", 100, time.Now(), true, SideSource)

	if change != ChangeCreated {
		t.Errorf("Change = %s, want created", change)
	}
}

func TestSyncState_DetectChange_Deleted(t *testing.T) {
	state := NewSyncState("/source", "/dest")
	state.UpdateFile("old.txt", 100, time.Now(), "", true, true, false)

	change := state.DetectChange("old.txt", 0, time.Time{}, false, SideSource)

	if change != ChangeDeleted {
		t.Errorf("Change = %s, want deleted", change)
	}
}

func TestSyncState_DetectChange_Modified_Size(t *testing.T) {
	state := NewSyncState("/source", "/dest")
	state.UpdateFile("file.txt", 100, time.Now(), "", true, true, false)

	// Different size
	change := state.DetectChange("file.txt", 200, time.Now(), true, SideSource)

	if change != ChangeModified {
		t.Errorf("Change = %s, want modified", change)
	}
}

func TestSyncState_DetectChange_Modified_Time(t *testing.T) {
	state := NewSyncState("/source", "/dest")
	oldTime := time.Now().Add(-time.Hour)
	state.UpdateFile("file.txt", 100, oldTime, "", true, true, false)

	// Same size, newer time
	change := state.DetectChange("file.txt", 100, time.Now(), true, SideSource)

	if change != ChangeModified {
		t.Errorf("Change = %s, want modified", change)
	}
}

func TestSyncState_DetectChange_None(t *testing.T) {
	state := NewSyncState("/source", "/dest")
	modTime := time.Now()
	state.UpdateFile("file.txt", 100, modTime, "", true, true, false)

	// Same size and time
	change := state.DetectChange("file.txt", 100, modTime, true, SideSource)

	if change != ChangeNone {
		t.Errorf("Change = %s, want none", change)
	}
}

func TestSyncState_DetectChange_NonExistent(t *testing.T) {
	state := NewSyncState("/source", "/dest")

	// File doesn't exist and wasn't tracked
	change := state.DetectChange("nonexistent.txt", 0, time.Time{}, false, SideSource)

	if change != ChangeNone {
		t.Errorf("Change = %s, want none", change)
	}
}

// ============== FileState Tests ==============

func TestFileState(t *testing.T) {
	fs := &FileState{
		RelativePath:   "path/to/file.txt",
		Size:           1024,
		ModTime:        time.Now(),
		Hash:           "abc123",
		ExistsInSource: true,
		ExistsInDest:   false,
		IsDir:          false,
	}

	if fs.RelativePath != "path/to/file.txt" {
		t.Errorf("RelativePath = %s", fs.RelativePath)
	}
	if fs.Size != 1024 {
		t.Errorf("Size = %d", fs.Size)
	}
	if !fs.ExistsInSource {
		t.Error("ExistsInSource should be true")
	}
	if fs.ExistsInDest {
		t.Error("ExistsInDest should be false")
	}
}

// ============== ChangeType Tests ==============

func TestChangeType_Values(t *testing.T) {
	tests := []struct {
		ct       ChangeType
		expected string
	}{
		{ChangeCreated, "created"},
		{ChangeModified, "modified"},
		{ChangeDeleted, "deleted"},
		{ChangeNone, "none"},
	}

	for _, tt := range tests {
		if string(tt.ct) != tt.expected {
			t.Errorf("ChangeType %s != %s", tt.ct, tt.expected)
		}
	}
}

// ============== ChangeSide Tests ==============

func TestChangeSide_Values(t *testing.T) {
	tests := []struct {
		cs       ChangeSide
		expected string
	}{
		{SideSource, "source"},
		{SideDest, "dest"},
		{SideBoth, "both"},
	}

	for _, tt := range tests {
		if string(tt.cs) != tt.expected {
			t.Errorf("ChangeSide %s != %s", tt.cs, tt.expected)
		}
	}
}

// ============== hashPaths Tests ==============

func TestHashPaths(t *testing.T) {
	// Same paths should produce same hash
	hash1 := hashPaths("/source", "/dest")
	hash2 := hashPaths("/source", "/dest")
	if hash1 != hash2 {
		t.Error("Same paths should produce same hash")
	}

	// Different paths should produce different hash
	hash3 := hashPaths("/other", "/dest")
	if hash1 == hash3 {
		t.Error("Different paths should produce different hash")
	}

	// Order matters
	hash4 := hashPaths("/dest", "/source")
	if hash1 == hash4 {
		t.Error("Path order should matter")
	}

	// Hash should be deterministic
	if len(hash1) != 16 {
		t.Errorf("Hash length = %d, want 16", len(hash1))
	}
}

// ============== getStateFilePath Tests ==============

func TestGetStateFilePath(t *testing.T) {
	path := getStateFilePath("/source", "/dest")

	if path == "" {
		t.Error("Path should not be empty")
	}

	// Should end with .json
	if filepath.Ext(path) != ".json" {
		t.Errorf("Path should end with .json: %s", path)
	}

	// Should be in syncnorris/state directory
	if filepath.Base(filepath.Dir(path)) != "state" {
		t.Errorf("Should be in state directory: %s", path)
	}
}
