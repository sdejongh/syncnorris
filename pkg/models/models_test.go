package models

import (
	"testing"
	"time"
)

// ============== FileEntry Tests ==============

func TestFileEntry(t *testing.T) {
	t.Run("CreateFileEntry", func(t *testing.T) {
		entry := &FileEntry{
			RelativePath: "dir/file.txt",
			AbsolutePath: "/home/user/sync/dir/file.txt",
			Size:         1024,
			ModTime:      time.Now(),
			IsDir:        false,
			Permissions:  0644,
			Hash:         "abc123",
			Location:     LocationBoth,
		}

		if entry.RelativePath != "dir/file.txt" {
			t.Errorf("RelativePath = %s, want dir/file.txt", entry.RelativePath)
		}
		if entry.Size != 1024 {
			t.Errorf("Size = %d, want 1024", entry.Size)
		}
		if entry.IsDir {
			t.Error("IsDir should be false")
		}
	})

	t.Run("DirectoryEntry", func(t *testing.T) {
		entry := &FileEntry{
			RelativePath: "subdir",
			IsDir:        true,
		}

		if !entry.IsDir {
			t.Error("IsDir should be true for directory")
		}
	})
}

func TestFileLocation(t *testing.T) {
	tests := []struct {
		location FileLocation
		expected string
	}{
		{LocationSource, "source"},
		{LocationDest, "dest"},
		{LocationBoth, "both"},
	}

	for _, tt := range tests {
		t.Run(string(tt.location), func(t *testing.T) {
			if string(tt.location) != tt.expected {
				t.Errorf("FileLocation = %s, want %s", string(tt.location), tt.expected)
			}
		})
	}
}

func TestAction(t *testing.T) {
	tests := []struct {
		action   Action
		expected string
	}{
		{ActionCopy, "copy"},
		{ActionUpdate, "update"},
		{ActionDelete, "delete"},
		{ActionSkip, "skip"},
		{ActionConflict, "conflict"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("Action = %s, want %s", string(tt.action), tt.expected)
			}
		})
	}
}

func TestFileOperation(t *testing.T) {
	entry := &FileEntry{
		RelativePath: "file.txt",
		Size:         1024,
	}

	op := &FileOperation{
		Entry:       entry,
		Action:      ActionCopy,
		Reason:      "file exists only in source",
		Error:       nil,
		BytesCopied: 1024,
		Duration:    time.Millisecond * 100,
	}

	if op.Entry != entry {
		t.Error("Entry reference mismatch")
	}
	if op.Action != ActionCopy {
		t.Errorf("Action = %s, want copy", op.Action)
	}
	if op.BytesCopied != 1024 {
		t.Errorf("BytesCopied = %d, want 1024", op.BytesCopied)
	}
}

// ============== SyncOperation Tests ==============

func TestSyncMode(t *testing.T) {
	tests := []struct {
		mode     SyncMode
		expected string
	}{
		{ModeOneWay, "oneway"},
		{ModeBidirectional, "bidirectional"},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("SyncMode = %s, want %s", string(tt.mode), tt.expected)
			}
		})
	}
}

func TestConflictResolution(t *testing.T) {
	tests := []struct {
		resolution ConflictResolution
		expected   string
	}{
		{ConflictAsk, "ask"},
		{ConflictSourceWins, "source-wins"},
		{ConflictDestWins, "dest-wins"},
		{ConflictNewer, "newer"},
		{ConflictBoth, "both"},
	}

	for _, tt := range tests {
		t.Run(string(tt.resolution), func(t *testing.T) {
			if string(tt.resolution) != tt.expected {
				t.Errorf("ConflictResolution = %s, want %s", string(tt.resolution), tt.expected)
			}
		})
	}
}

func TestComparisonMethod(t *testing.T) {
	tests := []struct {
		method   ComparisonMethod
		expected string
	}{
		{CompareNameSize, "namesize"},
		{CompareTimestamp, "timestamp"},
		{CompareBinary, "binary"},
		{CompareHash, "hash"},
		{CompareMD5, "md5"},
	}

	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			if string(tt.method) != tt.expected {
				t.Errorf("ComparisonMethod = %s, want %s", string(tt.method), tt.expected)
			}
		})
	}
}

func TestSyncOperationValidate(t *testing.T) {
	t.Run("ValidOperation", func(t *testing.T) {
		op := &SyncOperation{
			SourcePath: "/source",
			DestPath:   "/dest",
			MaxWorkers: 5,
			BufferSize: 4096,
		}

		err := op.Validate()
		if err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("EmptySourcePath", func(t *testing.T) {
		op := &SyncOperation{
			SourcePath: "",
			DestPath:   "/dest",
			MaxWorkers: 5,
			BufferSize: 4096,
		}

		err := op.Validate()
		if err == nil {
			t.Error("Validate() should fail for empty source path")
		}
		if ve, ok := err.(*ValidationError); ok {
			if ve.Field != "SourcePath" {
				t.Errorf("ValidationError.Field = %s, want SourcePath", ve.Field)
			}
		}
	})

	t.Run("EmptyDestPath", func(t *testing.T) {
		op := &SyncOperation{
			SourcePath: "/source",
			DestPath:   "",
			MaxWorkers: 5,
			BufferSize: 4096,
		}

		err := op.Validate()
		if err == nil {
			t.Error("Validate() should fail for empty dest path")
		}
		if ve, ok := err.(*ValidationError); ok {
			if ve.Field != "DestPath" {
				t.Errorf("ValidationError.Field = %s, want DestPath", ve.Field)
			}
		}
	})

	t.Run("ZeroWorkers", func(t *testing.T) {
		op := &SyncOperation{
			SourcePath: "/source",
			DestPath:   "/dest",
			MaxWorkers: 0,
			BufferSize: 4096,
		}

		err := op.Validate()
		if err == nil {
			t.Error("Validate() should fail for zero workers")
		}
	})

	t.Run("SmallBufferSize", func(t *testing.T) {
		op := &SyncOperation{
			SourcePath: "/source",
			DestPath:   "/dest",
			MaxWorkers: 5,
			BufferSize: 512, // Too small
		}

		err := op.Validate()
		if err == nil {
			t.Error("Validate() should fail for small buffer size")
		}
	})
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "TestField",
		Message: "test message",
	}

	expected := "TestField: test message"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}
}

func TestSyncOperationFields(t *testing.T) {
	now := time.Now()
	started := now.Add(-time.Minute)
	completed := now

	op := &SyncOperation{
		ID:                 "op-123",
		SourcePath:         "/source",
		DestPath:           "/dest",
		Mode:               ModeBidirectional,
		ComparisonMethod:   CompareHash,
		ConflictResolution: ConflictNewer,
		ExcludePatterns:    []string{"*.tmp", ".git"},
		DryRun:             true,
		DeleteOrphans:      true,
		MaxWorkers:         8,
		BandwidthLimit:     1024 * 1024, // 1 MB/s
		BufferSize:         65536,
		Stateful:           true,
		CreatedAt:          now,
		StartedAt:          &started,
		CompletedAt:        &completed,
	}

	if op.ID != "op-123" {
		t.Errorf("ID = %s, want op-123", op.ID)
	}
	if op.Mode != ModeBidirectional {
		t.Errorf("Mode = %s, want bidirectional", op.Mode)
	}
	if !op.DryRun {
		t.Error("DryRun should be true")
	}
	if !op.DeleteOrphans {
		t.Error("DeleteOrphans should be true")
	}
	if !op.Stateful {
		t.Error("Stateful should be true")
	}
	if len(op.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns length = %d, want 2", len(op.ExcludePatterns))
	}
}

// ============== Conflict Tests ==============

func TestConflictType(t *testing.T) {
	tests := []struct {
		ctype    ConflictType
		expected string
	}{
		{ConflictModifyModify, "modify-modify"},
		{ConflictDeleteModify, "delete-modify"},
		{ConflictModifyDelete, "modify-delete"},
		{ConflictCreateCreate, "create-create"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ctype), func(t *testing.T) {
			if string(tt.ctype) != tt.expected {
				t.Errorf("ConflictType = %s, want %s", string(tt.ctype), tt.expected)
			}
		})
	}
}

func TestConflictIsResolved(t *testing.T) {
	t.Run("Unresolved", func(t *testing.T) {
		conflict := &Conflict{
			Path:       "file.txt",
			Type:       ConflictModifyModify,
			DetectedAt: time.Now(),
		}

		if conflict.IsResolved() {
			t.Error("IsResolved() should return false for unresolved conflict")
		}
	})

	t.Run("Resolved", func(t *testing.T) {
		now := time.Now()
		conflict := &Conflict{
			Path:       "file.txt",
			Type:       ConflictModifyModify,
			DetectedAt: time.Now(),
			ResolvedAt: &now,
		}

		if !conflict.IsResolved() {
			t.Error("IsResolved() should return true for resolved conflict")
		}
	})
}

func TestConflictResolve(t *testing.T) {
	conflict := &Conflict{
		Path:       "file.txt",
		Type:       ConflictModifyModify,
		DetectedAt: time.Now(),
	}

	if conflict.IsResolved() {
		t.Error("Conflict should not be resolved initially")
	}

	conflict.Resolve(ConflictNewer, ActionCopy)

	if !conflict.IsResolved() {
		t.Error("Conflict should be resolved after Resolve()")
	}
	if conflict.Resolution != ConflictNewer {
		t.Errorf("Resolution = %s, want newer", conflict.Resolution)
	}
	if conflict.ResolvedAction != ActionCopy {
		t.Errorf("ResolvedAction = %s, want copy", conflict.ResolvedAction)
	}
	if conflict.ResolvedAt == nil {
		t.Error("ResolvedAt should not be nil")
	}
}

func TestConflictResolveWithDetails(t *testing.T) {
	conflict := &Conflict{
		Path: "file.txt",
		Type: ConflictModifyModify,
		SourceEntry: &FileEntry{
			RelativePath: "file.txt",
			Size:         100,
		},
		DestEntry: &FileEntry{
			RelativePath: "file.txt",
			Size:         200,
		},
		DetectedAt: time.Now(),
	}

	conflictFiles := []string{"file.source-conflict.txt", "file.dest-conflict.txt"}
	conflict.ResolveWithDetails(
		ConflictBoth,
		ActionCopy,
		"both",
		"Both versions kept",
		conflictFiles,
	)

	if !conflict.IsResolved() {
		t.Error("Conflict should be resolved")
	}
	if conflict.Winner != "both" {
		t.Errorf("Winner = %s, want both", conflict.Winner)
	}
	if conflict.ResultDescription != "Both versions kept" {
		t.Errorf("ResultDescription = %s, want 'Both versions kept'", conflict.ResultDescription)
	}
	if len(conflict.ConflictFiles) != 2 {
		t.Errorf("ConflictFiles length = %d, want 2", len(conflict.ConflictFiles))
	}
}

func TestConflictWithEntries(t *testing.T) {
	sourceEntry := &FileEntry{
		RelativePath: "doc.txt",
		Size:         1000,
		ModTime:      time.Now().Add(-time.Hour),
	}
	destEntry := &FileEntry{
		RelativePath: "doc.txt",
		Size:         1500,
		ModTime:      time.Now(),
	}

	conflict := &Conflict{
		Path:        "doc.txt",
		SourceEntry: sourceEntry,
		DestEntry:   destEntry,
		Type:        ConflictModifyModify,
		DetectedAt:  time.Now(),
	}

	if conflict.SourceEntry.Size != 1000 {
		t.Errorf("SourceEntry.Size = %d, want 1000", conflict.SourceEntry.Size)
	}
	if conflict.DestEntry.Size != 1500 {
		t.Errorf("DestEntry.Size = %d, want 1500", conflict.DestEntry.Size)
	}
}
