package sync

import (
	"path/filepath"
	"strings"
)

// shouldExclude checks if a path should be excluded based on the given patterns
// Patterns support:
//   - Simple glob patterns: *.tmp, *.log
//   - Directory patterns: .git/, node_modules/
//   - Path patterns: build/*, **/test/*
//   - Negation: !important.log (not yet supported)
func shouldExclude(relativePath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	// Normalize path separators for cross-platform support
	normalizedPath := filepath.ToSlash(relativePath)
	baseName := filepath.Base(relativePath)

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		// Normalize pattern
		normalizedPattern := filepath.ToSlash(pattern)

		// Check if it's a directory pattern (ends with /)
		isDirPattern := strings.HasSuffix(normalizedPattern, "/")
		if isDirPattern {
			// Remove trailing slash for matching
			dirPattern := strings.TrimSuffix(normalizedPattern, "/")
			// Check if path starts with or contains the directory
			if strings.HasPrefix(normalizedPath, dirPattern+"/") ||
				normalizedPath == dirPattern ||
				strings.Contains(normalizedPath, "/"+dirPattern+"/") {
				return true
			}
			continue
		}

		// Check for ** (matches any path depth)
		if strings.Contains(normalizedPattern, "**") {
			// Convert ** pattern to a simple check
			// **/pattern matches pattern at any level
			parts := strings.Split(normalizedPattern, "**/")
			if len(parts) == 2 && parts[0] == "" {
				// Pattern like **/foo or **/foo/*
				suffix := parts[1]
				if matchGlob(baseName, suffix) {
					return true
				}
				// Also check if path ends with the pattern
				if strings.HasSuffix(normalizedPath, "/"+suffix) || normalizedPath == suffix {
					return true
				}
				// Check with glob on full path
				if matchGlobPath(normalizedPath, suffix) {
					return true
				}
			}
			continue
		}

		// Check if pattern contains path separator
		if strings.Contains(normalizedPattern, "/") {
			// Pattern applies to full path
			matched, _ := filepath.Match(normalizedPattern, normalizedPath)
			if matched {
				return true
			}
			// Also try matching from the end (for patterns like build/*)
			if strings.HasSuffix(normalizedPath, normalizedPattern) {
				return true
			}
		} else {
			// Pattern applies to basename only
			matched, _ := filepath.Match(normalizedPattern, baseName)
			if matched {
				return true
			}
		}
	}

	return false
}

// matchGlob performs simple glob matching on a single path component
func matchGlob(name, pattern string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// matchGlobPath checks if any component of the path matches the pattern
func matchGlobPath(path, pattern string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if matchGlob(part, pattern) {
			return true
		}
	}
	return false
}
