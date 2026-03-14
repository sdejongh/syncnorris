//go:build !nogui

package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	settingsDir  = "syncnorris"
	settingsFile = "gui-settings.json"
	maxHistory   = 10
)

// Settings holds persisted GUI preferences and path history.
type Settings struct {
	Mode       string `json:"mode"`
	Comparison string `json:"comparison"`
	Conflict   string `json:"conflict"`
	Workers    int    `json:"workers"`
	Excludes   string `json:"excludes"`
	DryRun     bool   `json:"dry_run"`
	Delete     bool   `json:"delete"`
	CreateDest bool   `json:"create_dest"`
	Stateful   bool   `json:"stateful"`

	SourceHistory []string `json:"source_history"`
	DestHistory   []string `json:"dest_history"`
}

// DefaultSettings returns settings matching the current GUI defaults.
func DefaultSettings() *Settings {
	return &Settings{
		Mode:       "oneway",
		Comparison: "hash",
		Conflict:   "newer",
		Workers:    5,
		Excludes:   "*.tmp, .git/, node_modules/",
	}
}

// settingsPath returns the platform-appropriate path for the settings file.
//
//	Linux:   ~/.config/syncnorris/gui-settings.json
//	macOS:   ~/Library/Application Support/syncnorris/gui-settings.json
//	Windows: %AppData%\syncnorris\gui-settings.json
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to ~/.config
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			return "", hErr
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, settingsDir, settingsFile), nil
}

// LoadSettings reads settings from disk.
// Returns default settings if the file doesn't exist.
func LoadSettings() *Settings {
	p, err := settingsPath()
	if err != nil {
		return DefaultSettings()
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return DefaultSettings()
	}

	s := DefaultSettings()
	if err := json.Unmarshal(data, s); err != nil {
		return DefaultSettings()
	}

	// Ensure valid values
	if s.Workers < 1 {
		s.Workers = 5
	}
	if s.Mode == "" {
		s.Mode = "oneway"
	}
	if s.Comparison == "" {
		s.Comparison = "hash"
	}
	if s.Conflict == "" {
		s.Conflict = "newer"
	}

	return s
}

// Save writes settings to disk, creating the directory if needed.
func (s *Settings) Save() error {
	p, err := settingsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, data, 0644)
}

// AddSourcePath adds a path to the source history (most recent first, max 10, no duplicates).
func (s *Settings) AddSourcePath(path string) {
	s.SourceHistory = addToHistory(s.SourceHistory, path)
}

// AddDestPath adds a path to the destination history (most recent first, max 10, no duplicates).
func (s *Settings) AddDestPath(path string) {
	s.DestHistory = addToHistory(s.DestHistory, path)
}

func addToHistory(history []string, path string) []string {
	if path == "" {
		return history
	}

	// Remove existing duplicate
	filtered := make([]string, 0, len(history))
	for _, h := range history {
		if h != path {
			filtered = append(filtered, h)
		}
	}

	// Prepend new entry
	result := append([]string{path}, filtered...)

	// Cap at maxHistory
	if len(result) > maxHistory {
		result = result[:maxHistory]
	}
	return result
}
