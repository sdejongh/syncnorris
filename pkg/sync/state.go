package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncState represents the state of a synchronization pair
// This is persisted to track changes between syncs
type SyncState struct {
	// Version for state file format compatibility
	Version int `json:"version"`

	// SourcePath and DestPath identify the sync pair
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`

	// LastSyncTime is when the last successful sync completed
	LastSyncTime time.Time `json:"last_sync_time"`

	// Files tracks the state of each file at last sync
	Files map[string]*FileState `json:"files"`
}

// FileState represents the state of a single file at last sync
type FileState struct {
	// RelativePath is the path relative to sync root
	RelativePath string `json:"relative_path"`

	// Size at last sync
	Size int64 `json:"size"`

	// ModTime at last sync
	ModTime time.Time `json:"mod_time"`

	// Hash at last sync (optional, may be empty)
	Hash string `json:"hash,omitempty"`

	// ExistsInSource at last sync
	ExistsInSource bool `json:"exists_in_source"`

	// ExistsInDest at last sync
	ExistsInDest bool `json:"exists_in_dest"`

	// IsDir indicates if this is a directory
	IsDir bool `json:"is_dir"`
}

// FileChange represents a change detected since last sync
type FileChange struct {
	RelativePath string
	ChangeType   ChangeType
	Side         ChangeSide
	OldState     *FileState // nil if file didn't exist
	NewSize      int64
	NewModTime   time.Time
	NewHash      string
	IsDir        bool
}

// ChangeType categorizes the type of change
type ChangeType string

const (
	ChangeCreated  ChangeType = "created"
	ChangeModified ChangeType = "modified"
	ChangeDeleted  ChangeType = "deleted"
	ChangeNone     ChangeType = "none"
)

// ChangeSide indicates which side the change occurred on
type ChangeSide string

const (
	SideSource ChangeSide = "source"
	SideDest   ChangeSide = "dest"
	SideBoth   ChangeSide = "both"
)

const (
	stateFileVersion = 1
	stateFileName    = ".syncnorris-state.json"
)

// NewSyncState creates a new empty sync state
func NewSyncState(sourcePath, destPath string) *SyncState {
	return &SyncState{
		Version:    stateFileVersion,
		SourcePath: sourcePath,
		DestPath:   destPath,
		Files:      make(map[string]*FileState),
	}
}

// LoadState loads the sync state from the state file
// Returns a new empty state if the file doesn't exist
func LoadState(sourcePath, destPath string) (*SyncState, error) {
	statePath := getStateFilePath(sourcePath, destPath)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No previous state, return new empty state
			return NewSyncState(sourcePath, destPath), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Check version compatibility
	if state.Version > stateFileVersion {
		return nil, fmt.Errorf("state file version %d is newer than supported version %d", state.Version, stateFileVersion)
	}

	// Ensure Files map is initialized
	if state.Files == nil {
		state.Files = make(map[string]*FileState)
	}

	return &state, nil
}

// Save persists the sync state to the state file
func (s *SyncState) Save() error {
	statePath := getStateFilePath(s.SourcePath, s.DestPath)

	// Ensure directory exists
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write atomically using temp file
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to finalize state file: %w", err)
	}

	return nil
}

// UpdateFile updates the state for a single file after sync
func (s *SyncState) UpdateFile(relativePath string, size int64, modTime time.Time, hash string, existsInSource, existsInDest, isDir bool) {
	if !existsInSource && !existsInDest {
		// File was deleted from both sides, remove from state
		delete(s.Files, relativePath)
		return
	}

	s.Files[relativePath] = &FileState{
		RelativePath:   relativePath,
		Size:           size,
		ModTime:        modTime,
		Hash:           hash,
		ExistsInSource: existsInSource,
		ExistsInDest:   existsInDest,
		IsDir:          isDir,
	}
}

// RemoveFile removes a file from the state
func (s *SyncState) RemoveFile(relativePath string) {
	delete(s.Files, relativePath)
}

// GetFileState returns the state of a file, or nil if not tracked
func (s *SyncState) GetFileState(relativePath string) *FileState {
	return s.Files[relativePath]
}

// MarkSyncComplete updates the last sync time
func (s *SyncState) MarkSyncComplete() {
	s.LastSyncTime = time.Now()
}

// IsFirstSync returns true if this is the first sync (no previous state)
func (s *SyncState) IsFirstSync() bool {
	return s.LastSyncTime.IsZero()
}

// DetectChange determines what kind of change occurred for a file
func (s *SyncState) DetectChange(relativePath string, currentSize int64, currentModTime time.Time, exists bool, side ChangeSide) ChangeType {
	oldState := s.GetFileState(relativePath)

	if oldState == nil {
		// File wasn't tracked before
		if exists {
			return ChangeCreated
		}
		return ChangeNone
	}

	// Check if file existed on this side before
	var existedBefore bool
	if side == SideSource {
		existedBefore = oldState.ExistsInSource
	} else {
		existedBefore = oldState.ExistsInDest
	}

	if !existedBefore && exists {
		return ChangeCreated
	}

	if existedBefore && !exists {
		return ChangeDeleted
	}

	if existedBefore && exists {
		// Check for modification
		if currentSize != oldState.Size || currentModTime.After(oldState.ModTime) {
			return ChangeModified
		}
	}

	return ChangeNone
}

// getStateFilePath returns the path to the state file
// State is stored in the user's config directory
func getStateFilePath(sourcePath, destPath string) string {
	// Create a unique identifier for this sync pair
	// Use hash of paths to avoid filesystem issues with special characters
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to home directory
		configDir, _ = os.UserHomeDir()
		configDir = filepath.Join(configDir, ".config")
	}

	stateDir := filepath.Join(configDir, "syncnorris", "state")

	// Create a deterministic filename from the paths
	// Use a simple hash to avoid path length issues
	pairID := hashPaths(sourcePath, destPath)

	return filepath.Join(stateDir, pairID+".json")
}

// hashPaths creates a deterministic identifier for a source/dest pair
func hashPaths(source, dest string) string {
	// Normalize paths
	source = filepath.Clean(source)
	dest = filepath.Clean(dest)

	// Simple hash using FNV-1a algorithm
	h := uint64(14695981039346656037)
	for _, c := range source + "|" + dest {
		h ^= uint64(c)
		h *= 1099511628211
	}

	return fmt.Sprintf("%016x", h)
}

// ClearState removes the state file for a sync pair
func ClearState(sourcePath, destPath string) error {
	statePath := getStateFilePath(sourcePath, destPath)
	err := os.Remove(statePath)
	if os.IsNotExist(err) {
		return nil // Already doesn't exist
	}
	return err
}
