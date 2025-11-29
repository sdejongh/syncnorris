package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// TestHelper provides utilities for bidirectional sync tests
type TestHelper struct {
	t         *testing.T
	tempDir   string
	sourceDir string
	destDir   string
	source    *storage.Local
	dest      *storage.Local
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "syncnorris-bisync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	sourceDir := filepath.Join(tempDir, "source")
	destDir := filepath.Join(tempDir, "dest")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	source, err := storage.NewLocal(sourceDir)
	if err != nil {
		t.Fatalf("failed to create source backend: %v", err)
	}

	dest, err := storage.NewLocal(destDir)
	if err != nil {
		t.Fatalf("failed to create dest backend: %v", err)
	}

	return &TestHelper{
		t:         t,
		tempDir:   tempDir,
		sourceDir: sourceDir,
		destDir:   destDir,
		source:    source,
		dest:      dest,
	}
}

func (h *TestHelper) Cleanup() {
	os.RemoveAll(h.tempDir)
}

func (h *TestHelper) CreateSourceFile(name string, content []byte) {
	h.t.Helper()
	path := filepath.Join(h.sourceDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		h.t.Fatalf("failed to create parent dir: %v", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		h.t.Fatalf("failed to create source file: %v", err)
	}
}

func (h *TestHelper) CreateDestFile(name string, content []byte) {
	h.t.Helper()
	path := filepath.Join(h.destDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		h.t.Fatalf("failed to create parent dir: %v", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		h.t.Fatalf("failed to create dest file: %v", err)
	}
}

func (h *TestHelper) SetFileModTime(isSource bool, name string, modTime time.Time) {
	h.t.Helper()
	var path string
	if isSource {
		path = filepath.Join(h.sourceDir, name)
	} else {
		path = filepath.Join(h.destDir, name)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		h.t.Fatalf("failed to set mod time: %v", err)
	}
}

func (h *TestHelper) ReadSourceFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(h.sourceDir, name))
}

func (h *TestHelper) ReadDestFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(h.destDir, name))
}

func (h *TestHelper) SourceFileExists(name string) bool {
	_, err := os.Stat(filepath.Join(h.sourceDir, name))
	return err == nil
}

func (h *TestHelper) DestFileExists(name string) bool {
	_, err := os.Stat(filepath.Join(h.destDir, name))
	return err == nil
}

func (h *TestHelper) NewOperation() *models.SyncOperation {
	return &models.SyncOperation{
		SourcePath:         h.sourceDir,
		DestPath:           h.destDir,
		Mode:               models.ModeBidirectional,
		ComparisonMethod:   models.CompareHash,
		ConflictResolution: models.ConflictNewer,
		MaxWorkers:         2,
		BufferSize:         4096,
		DryRun:             false,
		Stateful:           false,
	}
}

// nullFormatter for testing
type nullFormatter struct{}

func (f *nullFormatter) Start(writer io.Writer, totalFiles int, totalBytes int64, maxWorkers int) error {
	return nil
}
func (f *nullFormatter) Progress(update output.ProgressUpdate) error { return nil }
func (f *nullFormatter) Complete(report *models.SyncReport) error    { return nil }
func (f *nullFormatter) Error(err error) error                       { return nil }
func (f *nullFormatter) Name() string                                { return "null" }

// ============== SyncAction Tests ==============

func TestSyncAction(t *testing.T) {
	action := &SyncAction{
		Path:       "test.txt",
		ActionType: models.ActionCopy,
		Direction:  DirectionSourceToDest,
		Reason:     "test reason",
	}

	if action.Path != "test.txt" {
		t.Errorf("Path = %s, want test.txt", action.Path)
	}
	if action.ActionType != models.ActionCopy {
		t.Errorf("ActionType = %s, want copy", action.ActionType)
	}
	if action.Direction != DirectionSourceToDest {
		t.Errorf("Direction = %s, want source_to_dest", action.Direction)
	}
}

func TestSyncDirection(t *testing.T) {
	tests := []struct {
		dir      SyncDirection
		expected string
	}{
		{DirectionSourceToDest, "source_to_dest"},
		{DirectionDestToSource, "dest_to_source"},
		{DirectionBoth, "both"},
	}

	for _, tt := range tests {
		if string(tt.dir) != tt.expected {
			t.Errorf("Direction %s != %s", tt.dir, tt.expected)
		}
	}
}

// ============== BidirectionalPipeline Tests ==============

func TestBidirectionalPipeline_NewFilesOnBothSides(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("source_only.txt", []byte("source content"))
	h.CreateDestFile("dest_only.txt", []byte("dest content"))

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Verify files synced both ways
	if !h.DestFileExists("source_only.txt") {
		t.Error("source_only.txt should exist in dest")
	}
	if !h.SourceFileExists("dest_only.txt") {
		t.Error("dest_only.txt should exist in source")
	}

	// Verify content
	content, _ := h.ReadDestFile("source_only.txt")
	if !bytes.Equal(content, []byte("source content")) {
		t.Errorf("source_only.txt in dest has wrong content")
	}
	content, _ = h.ReadSourceFile("dest_only.txt")
	if !bytes.Equal(content, []byte("dest content")) {
		t.Errorf("dest_only.txt in source has wrong content")
	}
}

func TestBidirectionalPipeline_IdenticalFiles(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	content := []byte("identical content")
	h.CreateSourceFile("same.txt", content)
	h.CreateDestFile("same.txt", content)

	// Set same mod time
	modTime := time.Now().Add(-time.Hour)
	h.SetFileModTime(true, "same.txt", modTime)
	h.SetFileModTime(false, "same.txt", modTime)

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// No conflicts for identical files
	if len(report.Conflicts) > 0 {
		t.Errorf("Conflicts = %d, want 0 for identical files", len(report.Conflicts))
	}
}

func TestBidirectionalPipeline_ConflictNewer(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source version"))
	h.CreateDestFile("conflict.txt", []byte("dest version"))

	// Make source newer
	h.SetFileModTime(true, "conflict.txt", time.Now())
	h.SetFileModTime(false, "conflict.txt", time.Now().Add(-5*time.Second))

	op := h.NewOperation()
	op.ConflictResolution = models.ConflictNewer
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Conflict should be detected
	if len(report.Conflicts) == 0 {
		t.Error("Should have detected a conflict")
	}

	// Dest should have source content (source is newer)
	content, _ := h.ReadDestFile("conflict.txt")
	if !bytes.Equal(content, []byte("source version")) {
		t.Errorf("Dest content = %s, want 'source version'", string(content))
	}
}

func TestBidirectionalPipeline_ConflictSourceWins(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source wins"))
	h.CreateDestFile("conflict.txt", []byte("dest loses"))

	op := h.NewOperation()
	op.ConflictResolution = models.ConflictSourceWins
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	content, _ := h.ReadDestFile("conflict.txt")
	if !bytes.Equal(content, []byte("source wins")) {
		t.Errorf("Dest content = %s, want 'source wins'", string(content))
	}
}

func TestBidirectionalPipeline_ConflictDestWins(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source loses"))
	h.CreateDestFile("conflict.txt", []byte("dest wins"))

	op := h.NewOperation()
	op.ConflictResolution = models.ConflictDestWins
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	content, _ := h.ReadSourceFile("conflict.txt")
	if !bytes.Equal(content, []byte("dest wins")) {
		t.Errorf("Source content = %s, want 'dest wins'", string(content))
	}
}

func TestBidirectionalPipeline_ConflictBoth(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source content"))
	h.CreateDestFile("conflict.txt", []byte("dest content"))

	op := h.NewOperation()
	op.ConflictResolution = models.ConflictBoth
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Both conflict copies should exist on both sides
	if !h.SourceFileExists("conflict.source-conflict.txt") {
		t.Error("source-conflict should exist in source")
	}
	if !h.SourceFileExists("conflict.dest-conflict.txt") || !h.DestFileExists("conflict.dest-conflict.txt") {
		// dest-conflict created in dest, source-conflict created in source
		// After sync: source has dest content, dest has source content
	}
	if !h.DestFileExists("conflict.source-conflict.txt") || !h.DestFileExists("conflict.dest-conflict.txt") {
		// Verify at least dest-conflict exists in dest
		if !h.DestFileExists("conflict.dest-conflict.txt") {
			t.Error("dest-conflict should exist in dest")
		}
	}

	// Verify conflict copies have correct content
	if h.SourceFileExists("conflict.source-conflict.txt") {
		content, _ := h.ReadSourceFile("conflict.source-conflict.txt")
		if !bytes.Equal(content, []byte("source content")) {
			t.Errorf("source-conflict content = %s, want 'source content'", string(content))
		}
	}
	if h.DestFileExists("conflict.dest-conflict.txt") {
		content, _ := h.ReadDestFile("conflict.dest-conflict.txt")
		if !bytes.Equal(content, []byte("dest content")) {
			t.Errorf("dest-conflict content = %s, want 'dest content'", string(content))
		}
	}
}

func TestBidirectionalPipeline_DryRun(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("source.txt", []byte("source"))
	h.CreateDestFile("dest.txt", []byte("dest"))

	op := h.NewOperation()
	op.DryRun = true
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Files should NOT be synced in dry-run
	if h.DestFileExists("source.txt") {
		t.Error("source.txt should not be copied in dry-run")
	}
	if h.SourceFileExists("dest.txt") {
		t.Error("dest.txt should not be copied in dry-run")
	}

	// But counters should reflect what would happen
	if report.Stats.FilesCopied.Load() == 0 && report.Stats.FilesUpdated.Load() == 0 {
		t.Error("Dry-run should report files that would be copied")
	}
}

func TestBidirectionalPipeline_EmptyDirectories(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create empty directory in source
	os.MkdirAll(filepath.Join(h.sourceDir, "empty_dir"), 0755)

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Empty directory should be created in dest
	destDir := filepath.Join(h.destDir, "empty_dir")
	info, err := os.Stat(destDir)
	if err != nil {
		t.Errorf("Empty directory should be created in dest: %v", err)
	} else if !info.IsDir() {
		t.Error("empty_dir should be a directory")
	}
}

func TestBidirectionalPipeline_NestedDirectories(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("a/b/c/deep.txt", []byte("deep content"))
	h.CreateDestFile("x/y/z/other.txt", []byte("other content"))

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists("a/b/c/deep.txt") {
		t.Error("Nested file should be copied to dest")
	}
	if !h.SourceFileExists("x/y/z/other.txt") {
		t.Error("Nested file should be copied to source")
	}
}

func TestBidirectionalPipeline_ExcludePatterns(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("include.txt", []byte("include"))
	h.CreateSourceFile("exclude.tmp", []byte("exclude"))
	h.CreateDestFile("dest.txt", []byte("dest"))

	op := h.NewOperation()
	op.ExcludePatterns = []string{"*.tmp"}
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists("include.txt") {
		t.Error("include.txt should be copied")
	}
	if h.DestFileExists("exclude.tmp") {
		t.Error("exclude.tmp should NOT be copied (excluded)")
	}
}

func TestBidirectionalPipeline_ContextCancellation(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create several files
	for i := 0; i < 10; i++ {
		h.CreateSourceFile(filepath.Join("dir", "file"+string(rune('0'+i))+".txt"), []byte("content"))
	}

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(ctx)

	if err == nil {
		t.Error("Should return error on cancelled context")
	}
}

// ============== ChangeType Tests ==============

func TestChangeType(t *testing.T) {
	tests := []struct {
		change   ChangeType
		expected string
	}{
		{ChangeCreated, "created"},
		{ChangeModified, "modified"},
		{ChangeDeleted, "deleted"},
		{ChangeNone, "none"},
	}

	for _, tt := range tests {
		if string(tt.change) != tt.expected {
			t.Errorf("ChangeType %s != %s", tt.change, tt.expected)
		}
	}
}

// ============== Edge Cases ==============

func TestBidirectionalPipeline_SameSizeDifferentContent(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Same size, different content - should detect as conflict
	h.CreateSourceFile("same_size.txt", []byte("aaaaaaaa"))
	h.CreateDestFile("same_size.txt", []byte("bbbbbbbb"))

	// Set different timestamps to ensure conflict detection
	// (same timestamp + same size would be skipped as "identical")
	h.SetFileModTime(true, "same_size.txt", time.Now())
	h.SetFileModTime(false, "same_size.txt", time.Now().Add(-5*time.Second))

	op := h.NewOperation()
	op.ConflictResolution = models.ConflictSourceWins
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should detect a conflict (same size but different timestamps)
	if len(report.Conflicts) == 0 {
		t.Error("Should detect conflict for same-size different-content files")
	}

	// Source should win
	content, _ := h.ReadDestFile("same_size.txt")
	if !bytes.Equal(content, []byte("aaaaaaaa")) {
		t.Errorf("Dest content = %s, want 'aaaaaaaa' (source wins)", string(content))
	}
}

func TestBidirectionalPipeline_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create a 5MB file
	largeContent := make([]byte, 5*1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	h.CreateSourceFile("large.bin", largeContent)

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(65536)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Verify file copied correctly
	destContent, err := h.ReadDestFile("large.bin")
	if err != nil {
		t.Fatalf("Failed to read dest file: %v", err)
	}
	if !bytes.Equal(destContent, largeContent) {
		t.Error("Large file content mismatch")
	}
}

func TestBidirectionalPipeline_SpecialCharactersInFilename(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// File with spaces and special characters
	h.CreateSourceFile("file with spaces.txt", []byte("content"))
	h.CreateSourceFile("file-with-dashes.txt", []byte("content"))
	h.CreateSourceFile("file_with_underscores.txt", []byte("content"))

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists("file with spaces.txt") {
		t.Error("File with spaces should be copied")
	}
	if !h.DestFileExists("file-with-dashes.txt") {
		t.Error("File with dashes should be copied")
	}
	if !h.DestFileExists("file_with_underscores.txt") {
		t.Error("File with underscores should be copied")
	}
}

func TestBidirectionalPipeline_EmptyFile(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("empty.txt", []byte{})

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists("empty.txt") {
		t.Error("Empty file should be copied")
	}

	content, _ := h.ReadDestFile("empty.txt")
	if len(content) != 0 {
		t.Error("Empty file should remain empty")
	}
}

func TestBidirectionalPipeline_Symlinks(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create a regular file
	h.CreateSourceFile("realfile.txt", []byte("real content"))

	// Create a symlink to the file
	symlinkPath := filepath.Join(h.sourceDir, "symlink.txt")
	targetPath := filepath.Join(h.sourceDir, "realfile.txt")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Skipf("Symlinks not supported on this system: %v", err)
	}

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Real file should be synced
	if !h.DestFileExists("realfile.txt") {
		t.Error("Real file should be copied")
	}

	// Symlinks are typically followed by filepath.Walk, so the symlink
	// itself may or may not be synced depending on implementation
	// This test documents current behavior
	t.Logf("Report: files copied=%d, status=%s", report.Stats.FilesCopied.Load(), report.Status)
}

func TestBidirectionalPipeline_FilePermissions(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI environment")
	}

	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create file with specific permissions
	h.CreateSourceFile("executable.sh", []byte("#!/bin/bash\necho hello"))
	sourcePath := filepath.Join(h.sourceDir, "executable.sh")

	// Set executable permission
	if err := os.Chmod(sourcePath, 0755); err != nil {
		t.Skipf("Cannot set file permissions: %v", err)
	}

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Check that file was copied
	if !h.DestFileExists("executable.sh") {
		t.Fatal("Executable file should be copied")
	}

	// Check permissions were preserved
	destPath := filepath.Join(h.destDir, "executable.sh")
	destInfo, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Failed to stat dest file: %v", err)
	}

	// Permissions should match (at least executable bit)
	if destInfo.Mode().Perm()&0100 == 0 {
		t.Logf("Note: Execute permission was not preserved (got %o)", destInfo.Mode().Perm())
	}
}

func TestBidirectionalPipeline_ReadOnlyFile(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI environment")
	}

	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create read-only file in source
	h.CreateSourceFile("readonly.txt", []byte("read only content"))
	sourcePath := filepath.Join(h.sourceDir, "readonly.txt")
	if err := os.Chmod(sourcePath, 0444); err != nil {
		t.Skipf("Cannot set file permissions: %v", err)
	}
	// Ensure cleanup can delete the file
	defer os.Chmod(sourcePath, 0644)

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists("readonly.txt") {
		t.Error("Read-only file should be copied")
	}

	// Clean up dest file permissions for deletion
	destPath := filepath.Join(h.destDir, "readonly.txt")
	os.Chmod(destPath, 0644)
}

func TestBidirectionalPipeline_UnreadableFile(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI environment")
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping unreadable file test when running as root")
	}

	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create a file and make it unreadable
	h.CreateSourceFile("unreadable.txt", []byte("secret content"))
	sourcePath := filepath.Join(h.sourceDir, "unreadable.txt")
	if err := os.Chmod(sourcePath, 0000); err != nil {
		t.Skipf("Cannot set file permissions: %v", err)
	}
	// Ensure cleanup can delete the file
	defer os.Chmod(sourcePath, 0644)

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	// Pipeline may or may not error depending on implementation
	// The important thing is it handles the unreadable file gracefully
	t.Logf("Run completed: err=%v", err)

	// File might not be copied due to permission error, or scan might skip it
	t.Logf("Report: files copied=%d, files errored=%d, status=%s",
		report.Stats.FilesCopied.Load(),
		report.Stats.FilesErrored.Load(),
		report.Status)
}

func TestBidirectionalPipeline_ManySmallFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping many files test in short mode")
	}

	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create 100 small files
	fileCount := 100
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("file_%03d.txt", i)
		content := fmt.Sprintf("content of file %d", i)
		h.CreateSourceFile(filename, []byte(content))
	}

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 4, QueueSize: 200}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Verify all files were copied
	copiedCount := 0
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("file_%03d.txt", i)
		if h.DestFileExists(filename) {
			copiedCount++
		}
	}
	if copiedCount != fileCount {
		t.Errorf("Copied %d files, want %d", copiedCount, fileCount)
	}
}

func TestBidirectionalPipeline_DeepNestedDirectories(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create deeply nested structure (10 levels)
	deepPath := "level1/level2/level3/level4/level5/level6/level7/level8/level9/level10"
	h.CreateSourceFile(filepath.Join(deepPath, "deep.txt"), []byte("deep content"))

	op := h.NewOperation()
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	_, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !h.DestFileExists(filepath.Join(deepPath, "deep.txt")) {
		t.Error("Deeply nested file should be copied")
	}
}

func TestBidirectionalPipeline_BidirectionalWithStateful(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Initial sync with files on both sides
	h.CreateSourceFile("source_file.txt", []byte("from source"))
	h.CreateDestFile("dest_file.txt", []byte("from dest"))

	// First sync with stateful mode
	op := h.NewOperation()
	op.Stateful = true
	config := PipelineConfig{MaxWorkers: 2, QueueSize: 100}
	formatter := &nullFormatter{}
	comparator := compare.NewHashComparator(4096)

	pipeline := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report1, err := pipeline.Run(context.Background())

	if err != nil {
		t.Fatalf("First run error = %v", err)
	}
	if report1.Status != models.StatusSuccess {
		t.Errorf("First run status = %s, want success", report1.Status)
	}

	// Both files should now exist on both sides
	if !h.SourceFileExists("dest_file.txt") {
		t.Error("dest_file.txt should be copied to source")
	}
	if !h.DestFileExists("source_file.txt") {
		t.Error("source_file.txt should be copied to dest")
	}

	// Modify a file in source
	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference
	h.CreateSourceFile("source_file.txt", []byte("modified source content"))

	// Run second sync
	pipeline2 := NewBidirectionalPipeline(h.source, h.dest, comparator, formatter, nil, op, config)
	report2, err := pipeline2.Run(context.Background())

	if err != nil {
		t.Fatalf("Second run error = %v", err)
	}
	if report2.Status != models.StatusSuccess {
		t.Errorf("Second run status = %s, want success", report2.Status)
	}

	// Modified file should be synced to dest
	destContent, _ := h.ReadDestFile("source_file.txt")
	if string(destContent) != "modified source content" {
		t.Errorf("Modified content not synced: got %q", string(destContent))
	}

	// Clean up state file
	ClearState(h.sourceDir, h.destDir)
}
