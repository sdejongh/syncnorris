package models

// ComparisonResult represents the outcome of comparing two files
type ComparisonResult struct {
	// SourceEntry is the file in the source location
	SourceEntry *FileEntry

	// DestEntry is the file in the destination location
	DestEntry *FileEntry

	// Match indicates if the files are considered identical
	Match bool

	// Reason explains why files differ or match
	Reason string

	// Method is the comparison method used
	Method ComparisonMethod

	// RecommendedAction is the suggested operation
	RecommendedAction Action

	// Conflict is populated if files conflict in bidirectional mode
	Conflict *Conflict
}

// DifferenceType categorizes why files differ
type DifferenceType string

const (
	// DiffSize indicates different file sizes
	DiffSize DifferenceType = "size"
	// DiffModTime indicates different modification times
	DiffModTime DifferenceType = "modtime"
	// DiffContent indicates different content
	DiffContent DifferenceType = "content"
	// DiffHash indicates different hashes
	DiffHash DifferenceType = "hash"
	// DiffMissing indicates file missing on one side
	DiffMissing DifferenceType = "missing"
	// DiffNone indicates files are identical
	DiffNone DifferenceType = "none"
)
