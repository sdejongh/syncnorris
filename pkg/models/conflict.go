package models

import (
	"time"
)

// Conflict represents a file conflict in bidirectional sync
type Conflict struct {
	// Path is the relative path of the conflicting file
	Path string

	// SourceEntry is the source file state
	SourceEntry *FileEntry

	// DestEntry is the destination file state
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
