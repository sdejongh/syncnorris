package models

import (
	"time"
)

// Conflict represents a file conflict in bidirectional sync
type Conflict struct {
	// Path is the relative path of the conflicting file
	Path string

	// SourceEntry is the source file state (before resolution)
	SourceEntry *FileEntry

	// DestEntry is the destination file state (before resolution)
	DestEntry *FileEntry

	// Type categorizes the conflict
	Type ConflictType

	// DetectedAt is when the conflict was detected
	DetectedAt time.Time

	// Resolution is how the conflict was resolved (if resolved)
	Resolution ConflictResolution

	// ResolvedAction is the action taken (if resolved)
	ResolvedAction Action

	// ResolvedAt is when the conflict was resolved
	ResolvedAt *time.Time

	// Winner indicates which side won the conflict (source, dest, or both)
	Winner string `json:"winner,omitempty"`

	// ResultDescription describes the outcome of the resolution
	ResultDescription string `json:"result_description,omitempty"`

	// ConflictFiles lists any additional files created (e.g., .source-conflict, .dest-conflict)
	ConflictFiles []string `json:"conflict_files,omitempty"`
}

// ConflictType categorizes different kinds of conflicts
type ConflictType string

const (
	// ConflictModifyModify indicates both sides modified the file
	ConflictModifyModify ConflictType = "modify-modify"
	// ConflictDeleteModify indicates one side deleted, other modified
	ConflictDeleteModify ConflictType = "delete-modify"
	// ConflictModifyDelete indicates one side modified, other deleted
	ConflictModifyDelete ConflictType = "modify-delete"
	// ConflictCreateCreate indicates same file created on both sides
	ConflictCreateCreate ConflictType = "create-create"
)

// IsResolved returns true if the conflict has been resolved
func (c *Conflict) IsResolved() bool {
	return c.ResolvedAt != nil
}

// Resolve marks the conflict as resolved
func (c *Conflict) Resolve(resolution ConflictResolution, action Action) {
	c.Resolution = resolution
	c.ResolvedAction = action
	now := time.Now()
	c.ResolvedAt = &now
}

// ResolveWithDetails marks the conflict as resolved with additional details
func (c *Conflict) ResolveWithDetails(resolution ConflictResolution, action Action, winner string, description string, conflictFiles []string) {
	c.Resolution = resolution
	c.ResolvedAction = action
	c.Winner = winner
	c.ResultDescription = description
	c.ConflictFiles = conflictFiles
	now := time.Now()
	c.ResolvedAt = &now
}
