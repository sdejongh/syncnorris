package compare

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sdejongh/syncnorris/pkg/storage"
)

// TestHelper provides utilities for comparator tests
type TestHelper struct {
	t       *testing.T
	tempDir string
	source  *storage.Local
	dest    *storage.Local
}

// NewTestHelper creates a new test helper with temporary directories
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "syncnorris-compare-test-*")
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
		t:       t,
		tempDir: tempDir,
		source:  source,
		dest:    dest,
	}
}

// Cleanup removes all temporary files
func (h *TestHelper) Cleanup() {
	os.RemoveAll(h.tempDir)
}

// CreateSourceFile creates a file in the source directory
func (h *TestHelper) CreateSourceFile(name string, content []byte) {
	h.t.Helper()
	path := filepath.Join(h.tempDir, "source", name)
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
	path := filepath.Join(h.tempDir, "dest", name)
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
		path = filepath.Join(h.tempDir, "source", name)
	} else {
		path = filepath.Join(h.tempDir, "dest", name)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		h.t.Fatalf("failed to set mod time: %v", err)
	}
}

// TestResultConstants verifies that Result constants are properly defined
func TestResultConstants(t *testing.T) {
	tests := []struct {
		result   Result
		expected string
	}{
		{Same, "same"},
		{Different, "different"},
		{SourceOnly, "source_only"},
		{DestOnly, "dest_only"},
		{Error, "error"},
	}

	for _, tt := range tests {
		t.Run(string(tt.result), func(t *testing.T) {
			if string(tt.result) != tt.expected {
				t.Errorf("Result constant %s has wrong value: got %s, want %s", tt.result, string(tt.result), tt.expected)
			}
		})
	}
}

// TestComparison verifies Comparison struct
func TestComparison(t *testing.T) {
	comp := &Comparison{
		SourcePath: "/source/file.txt",
		DestPath:   "/dest/file.txt",
		Result:     Same,
		Reason:     "files match",
		Error:      nil,
	}

	if comp.SourcePath != "/source/file.txt" {
		t.Errorf("SourcePath = %s, want /source/file.txt", comp.SourcePath)
	}
	if comp.DestPath != "/dest/file.txt" {
		t.Errorf("DestPath = %s, want /dest/file.txt", comp.DestPath)
	}
	if comp.Result != Same {
		t.Errorf("Result = %s, want %s", comp.Result, Same)
	}
	if comp.Reason != "files match" {
		t.Errorf("Reason = %s, want 'files match'", comp.Reason)
	}
	if comp.Error != nil {
		t.Errorf("Error = %v, want nil", comp.Error)
	}
}

// TestNameSizeComparator tests the name/size comparator
func TestNameSizeComparator(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	comparator := NewNameSizeComparator()
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if comparator.Name() != "namesize" {
			t.Errorf("Name() = %s, want namesize", comparator.Name())
		}
	})

	t.Run("IdenticalFiles", func(t *testing.T) {
		content := []byte("identical content")
		h.CreateSourceFile("identical.txt", content)
		h.CreateDestFile("identical.txt", content)

		result, err := comparator.Compare(ctx, h.source, h.dest, "identical.txt", "identical.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Same {
			t.Errorf("Result = %s, want %s", result.Result, Same)
		}
	})

	t.Run("DifferentSizes", func(t *testing.T) {
		h.CreateSourceFile("diff_size.txt", []byte("short"))
		h.CreateDestFile("diff_size.txt", []byte("much longer content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "diff_size.txt", "diff_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
		if result.Reason != "file sizes differ" {
			t.Errorf("Reason = %s, want 'file sizes differ'", result.Reason)
		}
	})

	t.Run("SourceOnly", func(t *testing.T) {
		h.CreateSourceFile("source_only.txt", []byte("content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "source_only.txt", "source_only.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != DestOnly {
			t.Errorf("Result = %s, want %s (file in dest doesn't exist)", result.Result, DestOnly)
		}
	})

	t.Run("DestOnly", func(t *testing.T) {
		h.CreateDestFile("dest_only.txt", []byte("content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "dest_only.txt", "dest_only.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != SourceOnly {
			t.Errorf("Result = %s, want %s (file in source doesn't exist)", result.Result, SourceOnly)
		}
	})

	t.Run("SameSizeDifferentContent", func(t *testing.T) {
		// namesize doesn't check content, so same size should be Same
		h.CreateSourceFile("same_size.txt", []byte("content1"))
		h.CreateDestFile("same_size.txt", []byte("content2"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "same_size.txt", "same_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Same {
			t.Errorf("Result = %s, want %s (namesize only checks size)", result.Result, Same)
		}
	})
}

// TestTimestampComparator tests the timestamp comparator
func TestTimestampComparator(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	comparator := NewTimestampComparator()
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if comparator.Name() != "timestamp" {
			t.Errorf("Name() = %s, want timestamp", comparator.Name())
		}
	})

	t.Run("IdenticalFiles", func(t *testing.T) {
		content := []byte("identical content")
		h.CreateSourceFile("identical.txt", content)
		h.CreateDestFile("identical.txt", content)

		// Set same mod time (2 seconds ago)
		modTime := time.Now().Add(-2 * time.Second)
		h.SetFileModTime(true, "identical.txt", modTime)
		h.SetFileModTime(false, "identical.txt", modTime)

		result, err := comparator.Compare(ctx, h.source, h.dest, "identical.txt", "identical.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Same {
			t.Errorf("Result = %s, want %s", result.Result, Same)
		}
	})

	t.Run("SourceNewer", func(t *testing.T) {
		content := []byte("content")
		h.CreateSourceFile("newer.txt", content)
		h.CreateDestFile("newer.txt", content)

		// Source is 5 seconds newer
		h.SetFileModTime(true, "newer.txt", time.Now())
		h.SetFileModTime(false, "newer.txt", time.Now().Add(-5*time.Second))

		result, err := comparator.Compare(ctx, h.source, h.dest, "newer.txt", "newer.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s (source is newer)", result.Result, Different)
		}
	})

	t.Run("DestNewer", func(t *testing.T) {
		content := []byte("content")
		h.CreateSourceFile("dest_newer.txt", content)
		h.CreateDestFile("dest_newer.txt", content)

		// Dest is newer (source is older)
		h.SetFileModTime(true, "dest_newer.txt", time.Now().Add(-5*time.Second))
		h.SetFileModTime(false, "dest_newer.txt", time.Now())

		result, err := comparator.Compare(ctx, h.source, h.dest, "dest_newer.txt", "dest_newer.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		// Dest being newer means source is not newer, so they're considered the same
		if result.Result != Same {
			t.Errorf("Result = %s, want %s (dest newer means no update needed)", result.Result, Same)
		}
	})

	t.Run("DifferentSizes", func(t *testing.T) {
		h.CreateSourceFile("diff_size.txt", []byte("short"))
		h.CreateDestFile("diff_size.txt", []byte("much longer content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "diff_size.txt", "diff_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
	})
}

// TestHashComparator tests the SHA-256 hash comparator
func TestHashComparator(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	comparator := NewHashComparator(4096)
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if comparator.Name() != "hash" {
			t.Errorf("Name() = %s, want hash", comparator.Name())
		}
	})

	t.Run("IdenticalFiles", func(t *testing.T) {
		content := []byte("identical content for hash test")
		h.CreateSourceFile("hash_identical.txt", content)
		h.CreateDestFile("hash_identical.txt", content)

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_identical.txt", "hash_identical.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Same {
			t.Errorf("Result = %s, want %s", result.Result, Same)
		}
	})

	t.Run("DifferentContent", func(t *testing.T) {
		h.CreateSourceFile("hash_diff.txt", []byte("content1"))
		h.CreateDestFile("hash_diff.txt", []byte("content2"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_diff.txt", "hash_diff.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
	})

	t.Run("DifferentSizes", func(t *testing.T) {
		h.CreateSourceFile("hash_size.txt", []byte("short"))
		h.CreateDestFile("hash_size.txt", []byte("much longer content here"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_size.txt", "hash_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
		if result.Reason != "file sizes differ" {
			t.Errorf("Reason = %s, want 'file sizes differ' (size check before hash)", result.Reason)
		}
	})

	t.Run("SourceOnly", func(t *testing.T) {
		h.CreateSourceFile("hash_source.txt", []byte("content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_source.txt", "hash_source.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != SourceOnly {
			t.Errorf("Result = %s, want %s", result.Result, SourceOnly)
		}
	})

	t.Run("DestOnly", func(t *testing.T) {
		h.CreateDestFile("hash_dest.txt", []byte("content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_dest.txt", "hash_dest.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != DestOnly {
			t.Errorf("Result = %s, want %s", result.Result, DestOnly)
		}
	})

	t.Run("SameSizeDifferentContent", func(t *testing.T) {
		// Same size but different content - hash should detect
		h.CreateSourceFile("hash_same_size.txt", []byte("abcdefgh"))
		h.CreateDestFile("hash_same_size.txt", []byte("12345678"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "hash_same_size.txt", "hash_same_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s (hash detects content diff)", result.Result, Different)
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		// Create a large file to ensure context check is reached
		largeContent := make([]byte, 1024*1024) // 1MB
		h.CreateSourceFile("hash_cancel.txt", largeContent)
		h.CreateDestFile("hash_cancel.txt", largeContent)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := comparator.Compare(ctx, h.source, h.dest, "hash_cancel.txt", "hash_cancel.txt")
		if err == nil {
			t.Error("Compare() should return error on cancelled context")
		}
	})
}

// TestBinaryComparator tests the byte-by-byte comparator
func TestBinaryComparator(t *testing.T) {
	h := NewTestHelper(t)
	defer h.Cleanup()

	comparator := NewBinaryComparator(4096)
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if comparator.Name() != "binary" {
			t.Errorf("Name() = %s, want binary", comparator.Name())
		}
	})

	t.Run("IdenticalFiles", func(t *testing.T) {
		content := []byte("identical content for binary test")
		h.CreateSourceFile("binary_identical.txt", content)
		h.CreateDestFile("binary_identical.txt", content)

		result, err := comparator.Compare(ctx, h.source, h.dest, "binary_identical.txt", "binary_identical.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Same {
			t.Errorf("Result = %s, want %s", result.Result, Same)
		}
	})

	t.Run("DifferentContent", func(t *testing.T) {
		h.CreateSourceFile("binary_diff.txt", []byte("aaaaaaaaaa"))
		h.CreateDestFile("binary_diff.txt", []byte("aaaaXaaaaa"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "binary_diff.txt", "binary_diff.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
		// Should report the byte offset
		if result.Reason == "" {
			t.Error("Reason should contain byte offset info")
		}
	})

	t.Run("DifferentAtStart", func(t *testing.T) {
		h.CreateSourceFile("binary_start.txt", []byte("Xbcdefghij"))
		h.CreateDestFile("binary_start.txt", []byte("abcdefghij"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "binary_start.txt", "binary_start.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
	})

	t.Run("DifferentSizes", func(t *testing.T) {
		h.CreateSourceFile("binary_size.txt", []byte("short"))
		h.CreateDestFile("binary_size.txt", []byte("much longer content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "binary_size.txt", "binary_size.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
	})

	t.Run("SourceDoesNotExist", func(t *testing.T) {
		h.CreateDestFile("binary_nodest.txt", []byte("content"))

		result, err := comparator.Compare(ctx, h.source, h.dest, "binary_nosource.txt", "binary_nodest.txt")
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if result.Result != Different {
			t.Errorf("Result = %s, want %s", result.Result, Different)
		}
	})
}

// TestComparatorInterface verifies all comparators implement the interface
func TestComparatorInterface(t *testing.T) {
	comparators := []Comparator{
		NewNameSizeComparator(),
		NewTimestampComparator(),
		NewHashComparator(4096),
		NewBinaryComparator(4096),
	}

	for _, c := range comparators {
		t.Run(c.Name(), func(t *testing.T) {
			// Just verify they implement the interface
			var _ Comparator = c
		})
	}
}
