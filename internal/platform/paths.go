package platform

import (
	"path/filepath"
	"runtime"
	"strings"
)

// NormalizePath normalizes a path for the current platform
func NormalizePath(path string) string {
	// Convert to platform-specific separators
	normalized := filepath.Clean(path)

	// On Windows, ensure UNC paths are preserved
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(path, "\\\\") && !strings.HasPrefix(normalized, "\\\\") {
			normalized = "\\\\" + normalized
		}
	}

	return normalized
}

// IsUNCPath checks if a path is a UNC path (Windows network share)
func IsUNCPath(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.HasPrefix(path, "\\\\") || strings.HasPrefix(path, "//")
}

// IsAbsolute checks if a path is absolute
func IsAbsolute(path string) bool {
	if IsUNCPath(path) {
		return true
	}
	return filepath.IsAbs(path)
}

// Join joins path elements with platform-specific separator
func Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Split splits a path into directory and file
func Split(path string) (dir, file string) {
	return filepath.Split(path)
}

// Rel returns a relative path from base to target
func Rel(basepath, targetpath string) (string, error) {
	return filepath.Rel(basepath, targetpath)
}

// Ext returns the file extension
func Ext(path string) string {
	return filepath.Ext(path)
}

// Base returns the last element of path
func Base(path string) string {
	return filepath.Base(path)
}

// Dir returns all but the last element of path
func Dir(path string) string {
	return filepath.Dir(path)
}

// ParseUNCPath parses a UNC path into host and share components
// Returns empty strings if not a UNC path
func ParseUNCPath(path string) (host, share, relPath string) {
	if !IsUNCPath(path) {
		return "", "", ""
	}

	// Remove leading slashes
	trimmed := strings.TrimPrefix(path, "\\\\")
	trimmed = strings.TrimPrefix(trimmed, "//")

	// Split into components
	parts := strings.SplitN(trimmed, string(filepath.Separator), 3)

	if len(parts) >= 1 {
		host = parts[0]
	}
	if len(parts) >= 2 {
		share = parts[1]
	}
	if len(parts) >= 3 {
		relPath = parts[2]
	}

	return host, share, relPath
}

// ValidatePath checks if a path is valid for the current platform
func ValidatePath(path string) error {
	if path == "" {
		return &PathError{Path: path, Message: "path is empty"}
	}

	// Check for invalid characters based on OS
	if runtime.GOOS == "windows" {
		invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*"}
		for _, char := range invalidChars {
			if strings.Contains(path, char) && !IsUNCPath(path) {
				return &PathError{Path: path, Message: "path contains invalid character: " + char}
			}
		}
	}

	return nil
}

// PathError represents a path validation error
type PathError struct {
	Path    string
	Message string
}

func (e *PathError) Error() string {
	return "invalid path '" + e.Path + "': " + e.Message
}
