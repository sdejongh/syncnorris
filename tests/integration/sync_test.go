package integration

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
	"github.com/sdejongh/syncnorris/pkg/sync"
)

// TestHelper provides utilities for integration tests
type TestHelper struct {
	t         *testing.T
	tempDir   string
	sourceDir string
	destDir   string
	source    *storage.Local
	dest      *storage.Local
}

// NewTestHelper creates a new integration test helper
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "syncnorris-integration-*")
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

// Cleanup removes all temporary files
func (h *TestHelper) Cleanup() {
	os.RemoveAll(h.tempDir)
}

// CreateSourceFile creates a file in the source directory
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

// CreateDestFile creates a file in the destination directory
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

// SetFileModTime sets the modification time for a file
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

// ReadDestFile reads a file from the destination directory
func (h *TestHelper) ReadDestFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(h.destDir, name))
}

// ReadSourceFile reads a file from the source directory
func (h *TestHelper) ReadSourceFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(h.sourceDir, name))
}

// DestFileExists checks if a file exists in the destination
func (h *TestHelper) DestFileExists(name string) bool {
	_, err := os.Stat(filepath.Join(h.destDir, name))
	return err == nil
}

// SourceFileExists checks if a file exists in the source
func (h *TestHelper) SourceFileExists(name string) bool {
	_, err := os.Stat(filepath.Join(h.sourceDir, name))
	return err == nil
}

// NewOperation creates a default sync operation for testing
func (h *TestHelper) NewOperation(mode models.SyncMode) *models.SyncOperation {
	return &models.SyncOperation{
		SourcePath:         h.sourceDir,
		DestPath:           h.destDir,
		Mode:               mode,
		ComparisonMethod:   models.CompareHash,
		ConflictResolution: models.ConflictNewer,
		MaxWorkers:         2,
		BufferSize:         4096,
		DryRun:             false,
	}
}

// nullFormatter is a minimal formatter for testing
type nullFormatter struct{}

func (f *nullFormatter) Start(writer io.Writer, totalFiles int, totalBytes int64, maxWorkers int) error {
	return nil
}
func (f *nullFormatter) Progress(update output.ProgressUpdate) error { return nil }
func (f *nullFormatter) Complete(report *models.SyncReport) error    { return nil }
func (f *nullFormatter) Error(err error) error                       { return nil }
func (f *nullFormatter) Name() string                                { return "null" }

var _ output.Formatter = (*nullFormatter)(nil)

// ============== One-Way Sync Tests ==============

func TestOneWaySync_EmptySource(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	op := h.NewOperation(models.ModeOneWay)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report == nil {
		t.Fatal("Run() returned nil report")
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}
}

func TestOneWaySync_CopyNewFiles(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create files in source
	h.CreateSourceFile("file1.txt", []byte("content1"))
	h.CreateSourceFile("file2.txt", []byte("content2"))
	h.CreateSourceFile("subdir/file3.txt", []byte("content3"))

	op := h.NewOperation(models.ModeOneWay)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Verify files were copied
	for _, name := range []string{"file1.txt", "file2.txt", "subdir/file3.txt"} {
		if !h.DestFileExists(name) {
			t.Errorf("File %s should exist in destination", name)
		}
	}

	// Verify content
	content, err := h.ReadDestFile("file1.txt")
	if err != nil {
		t.Fatalf("ReadDestFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("content1")) {
		t.Errorf("file1.txt content = %s, want content1", string(content))
	}
}

func TestOneWaySync_UpdateModifiedFiles(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create initial file in both locations
	h.CreateSourceFile("file.txt", []byte("new content"))
	h.CreateDestFile("file.txt", []byte("old content"))

	op := h.NewOperation(models.ModeOneWay)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Verify content was updated
	content, err := h.ReadDestFile("file.txt")
	if err != nil {
		t.Fatalf("ReadDestFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("new content")) {
		t.Errorf("file.txt content = %s, want 'new content'", string(content))
	}
}

func TestOneWaySync_SkipIdenticalFiles(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	content := []byte("identical content")
	h.CreateSourceFile("identical.txt", content)
	h.CreateDestFile("identical.txt", content)

	op := h.NewOperation(models.ModeOneWay)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// File should still exist and be unchanged
	destContent, err := h.ReadDestFile("identical.txt")
	if err != nil {
		t.Fatalf("ReadDestFile() error = %v", err)
	}
	if !bytes.Equal(destContent, content) {
		t.Error("Content should be unchanged")
	}
}

func TestOneWaySync_DeleteOrphans(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create file only in destination (orphan)
	h.CreateDestFile("orphan.txt", []byte("orphan content"))
	h.CreateSourceFile("keep.txt", []byte("keep this"))

	op := h.NewOperation(models.ModeOneWay)
	op.DeleteOrphans = true
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Orphan should be deleted
	if h.DestFileExists("orphan.txt") {
		t.Error("orphan.txt should be deleted")
	}
	// keep.txt should be copied
	if !h.DestFileExists("keep.txt") {
		t.Error("keep.txt should exist")
	}
}

func TestOneWaySync_DryRun(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("new.txt", []byte("new content"))
	h.CreateDestFile("existing.txt", []byte("existing content"))

	op := h.NewOperation(models.ModeOneWay)
	op.DryRun = true
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// In dry-run, new.txt should NOT be created
	if h.DestFileExists("new.txt") {
		t.Error("new.txt should not exist in dry-run mode")
	}
	// existing.txt should still be there
	if !h.DestFileExists("existing.txt") {
		t.Error("existing.txt should still exist")
	}

	// Report should still have stats
	if report == nil {
		t.Fatal("Report should not be nil")
	}
}

func TestOneWaySync_ExcludePatterns(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("include.txt", []byte("include"))
	h.CreateSourceFile("exclude.tmp", []byte("exclude"))
	h.CreateSourceFile("subdir/.git/config", []byte("git config"))

	op := h.NewOperation(models.ModeOneWay)
	op.ExcludePatterns = []string{"*.tmp", ".git"}
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	_, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// include.txt should be copied
	if !h.DestFileExists("include.txt") {
		t.Error("include.txt should be copied")
	}
	// exclude.tmp should be excluded
	if h.DestFileExists("exclude.tmp") {
		t.Error("exclude.tmp should not be copied (excluded)")
	}
}

func TestOneWaySync_ContextCancellation(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create some files
	for i := 0; i < 10; i++ {
		h.CreateSourceFile(filepath.Join("subdir", "file"+string(rune('0'+i))+".txt"), []byte("content"))
	}

	op := h.NewOperation(models.ModeOneWay)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	_, err := engine.Run(ctx)

	// Should return an error due to cancellation
	if err == nil {
		t.Error("Run() should return error on cancelled context")
	}
}

func TestOneWaySync_MultipleMethods(t *testing.T) {
	methods := []models.ComparisonMethod{
		models.CompareNameSize,
		models.CompareTimestamp,
		models.CompareHash,
	}

	for _, method := range methods {
		t.Run(string(method), func(t *testing.T) {
			h := NewTestHelper(t)
			defer h.Cleanup()

			h.CreateSourceFile("file.txt", []byte("content"))

			op := h.NewOperation(models.ModeOneWay)
			op.ComparisonMethod = method

			var comparator compare.Comparator
			switch method {
			case models.CompareNameSize:
				comparator = compare.NewNameSizeComparator()
			case models.CompareTimestamp:
				comparator = compare.NewTimestampComparator()
			case models.CompareHash:
				comparator = compare.NewHashComparator(4096)
			}

			formatter := &nullFormatter{}
			engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
			report, err := engine.Run(context.Background())

			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if report.Status != models.StatusSuccess {
				t.Errorf("Status = %s, want success", report.Status)
			}
			if !h.DestFileExists("file.txt") {
				t.Error("file.txt should be copied")
			}
		})
	}
}

// ============== Bidirectional Sync Tests ==============

func TestBidirectionalSync_NewFilesOnBothSides(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create different files on each side
	h.CreateSourceFile("source_only.txt", []byte("source content"))
	h.CreateDestFile("dest_only.txt", []byte("dest content"))

	op := h.NewOperation(models.ModeBidirectional)
	op.Stateful = false // Stateless mode
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// Both files should exist on both sides
	if !h.DestFileExists("source_only.txt") {
		t.Error("source_only.txt should be copied to dest")
	}
	if !h.SourceFileExists("dest_only.txt") {
		t.Error("dest_only.txt should be copied to source")
	}
}

func TestBidirectionalSync_ConflictNewer(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	// Create same file on both sides with different content
	h.CreateSourceFile("conflict.txt", []byte("source version"))
	h.CreateDestFile("conflict.txt", []byte("dest version"))

	// Make source newer
	h.SetFileModTime(true, "conflict.txt", time.Now())
	h.SetFileModTime(false, "conflict.txt", time.Now().Add(-5*time.Second))

	op := h.NewOperation(models.ModeBidirectional)
	op.ConflictResolution = models.ConflictNewer
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Dest should have source content (source is newer)
	content, err := h.ReadDestFile("conflict.txt")
	if err != nil {
		t.Fatalf("ReadDestFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("source version")) {
		t.Errorf("conflict.txt content = %s, want 'source version'", string(content))
	}

	// Check conflict was detected
	if len(report.Conflicts) == 0 {
		t.Error("Should have detected a conflict")
	}
}

func TestBidirectionalSync_ConflictSourceWins(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source wins"))
	h.CreateDestFile("conflict.txt", []byte("dest loses"))

	op := h.NewOperation(models.ModeBidirectional)
	op.ConflictResolution = models.ConflictSourceWins
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	_, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Dest should have source content
	content, err := h.ReadDestFile("conflict.txt")
	if err != nil {
		t.Fatalf("ReadDestFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("source wins")) {
		t.Errorf("conflict.txt content = %s, want 'source wins'", string(content))
	}
}

func TestBidirectionalSync_ConflictDestWins(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source loses"))
	h.CreateDestFile("conflict.txt", []byte("dest wins"))

	op := h.NewOperation(models.ModeBidirectional)
	op.ConflictResolution = models.ConflictDestWins
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	_, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Source should have dest content
	content, err := h.ReadSourceFile("conflict.txt")
	if err != nil {
		t.Fatalf("ReadSourceFile() error = %v", err)
	}
	if !bytes.Equal(content, []byte("dest wins")) {
		t.Errorf("conflict.txt content = %s, want 'dest wins'", string(content))
	}
}

func TestBidirectionalSync_ConflictBoth(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("conflict.txt", []byte("source content"))
	h.CreateDestFile("conflict.txt", []byte("dest content"))

	op := h.NewOperation(models.ModeBidirectional)
	op.ConflictResolution = models.ConflictBoth
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	_, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Both sides should have both versions now
	// Original files should still exist
	if !h.SourceFileExists("conflict.txt") {
		t.Error("conflict.txt should exist in source")
	}
	if !h.DestFileExists("conflict.txt") {
		t.Error("conflict.txt should exist in dest")
	}

	// Conflict copies should exist
	if !h.SourceFileExists("conflict.source-conflict.txt") && !h.SourceFileExists("conflict.dest-conflict.txt") {
		t.Error("Conflict copy should exist in source")
	}
	if !h.DestFileExists("conflict.source-conflict.txt") && !h.DestFileExists("conflict.dest-conflict.txt") {
		t.Error("Conflict copy should exist in dest")
	}
}

func TestBidirectionalSync_DryRun(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	h.CreateSourceFile("source.txt", []byte("source"))
	h.CreateDestFile("dest.txt", []byte("dest"))

	op := h.NewOperation(models.ModeBidirectional)
	op.DryRun = true
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Files should NOT be synced in dry-run mode
	if h.DestFileExists("source.txt") {
		t.Error("source.txt should not be copied in dry-run")
	}
	if h.SourceFileExists("dest.txt") {
		t.Error("dest.txt should not be copied in dry-run")
	}

	// Report should still have stats
	if report == nil {
		t.Fatal("Report should not be nil")
	}
}

func TestBidirectionalSync_IdenticalFiles(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	content := []byte("identical content")
	h.CreateSourceFile("same.txt", content)
	h.CreateDestFile("same.txt", content)

	op := h.NewOperation(models.ModeBidirectional)
	comparator := compare.NewHashComparator(4096)
	formatter := &nullFormatter{}

	engine := sync.NewEngine(h.source, h.dest, comparator, formatter, nil, op)
	report, err := engine.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Status != models.StatusSuccess {
		t.Errorf("Status = %s, want success", report.Status)
	}

	// No conflicts should be detected for identical files
	if len(report.Conflicts) > 0 {
		t.Errorf("Conflicts count = %d, want 0 for identical files", len(report.Conflicts))
	}
}
