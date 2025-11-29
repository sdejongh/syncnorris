package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewLocal tests the Local backend constructor
func TestNewLocal(t *testing.T) {
	t.Run("ValidDirectory", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		local, err := NewLocal(tempDir)
		if err != nil {
			t.Fatalf("NewLocal() error = %v", err)
		}
		if local == nil {
			t.Fatal("NewLocal() returned nil")
		}
		defer local.Close()
	})

	t.Run("NonExistentPath", func(t *testing.T) {
		_, err := NewLocal("/nonexistent/path/that/does/not/exist")
		if err == nil {
			t.Error("NewLocal() should fail for non-existent path")
		}
	})

	t.Run("FileNotDirectory", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "syncnorris-file-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		_, err = NewLocal(tempFile.Name())
		if err == nil {
			t.Error("NewLocal() should fail for file path (not directory)")
		}
	})

	t.Run("RelativePath", func(t *testing.T) {
		// Create a temp dir and use relative path
		tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Change to temp dir parent and use relative path
		oldWd, _ := os.Getwd()
		os.Chdir(filepath.Dir(tempDir))
		defer os.Chdir(oldWd)

		relPath := filepath.Base(tempDir)
		local, err := NewLocal(relPath)
		if err != nil {
			t.Fatalf("NewLocal() should work with relative path: %v", err)
		}
		defer local.Close()
	})
}

// TestLocalList tests the List method
func TestLocalList(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test structure
	files := map[string][]byte{
		"file1.txt":        []byte("content1"),
		"file2.txt":        []byte("content2"),
		"subdir/file3.txt": []byte("content3"),
		"subdir/file4.txt": []byte("content4"),
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("ListAll", func(t *testing.T) {
		entries, err := local.List(ctx, "")
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		// Should have root dir + subdir + 4 files = 6 entries
		if len(entries) < 5 {
			t.Errorf("List() returned %d entries, expected at least 5", len(entries))
		}

		// Check that files are included
		fileCount := 0
		dirCount := 0
		for _, e := range entries {
			if e.IsDir {
				dirCount++
			} else {
				fileCount++
			}
		}
		if fileCount != 4 {
			t.Errorf("List() found %d files, expected 4", fileCount)
		}
	})

	t.Run("ListSubdir", func(t *testing.T) {
		entries, err := local.List(ctx, "subdir")
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		// Should have subdir itself + 2 files = 3 entries
		if len(entries) < 2 {
			t.Errorf("List() returned %d entries, expected at least 2 files", len(entries))
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := local.List(ctx, "")
		if err == nil {
			t.Error("List() should return error on cancelled context")
		}
	})
}

// TestLocalRead tests the Read method
func TestLocalRead(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("test content for reading")
	filePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("ReadExistingFile", func(t *testing.T) {
		reader, err := local.Read(ctx, "test.txt")
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		if !bytes.Equal(data, content) {
			t.Errorf("Read() content = %s, want %s", string(data), string(content))
		}
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		_, err := local.Read(ctx, "nonexistent.txt")
		if err == nil {
			t.Error("Read() should fail for non-existent file")
		}
	})
}

// TestLocalWrite tests the Write method
func TestLocalWrite(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("WriteNewFile", func(t *testing.T) {
		content := []byte("new file content")
		reader := bytes.NewReader(content)

		err := local.Write(ctx, "new.txt", reader, int64(len(content)), nil)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(filepath.Join(tempDir, "new.txt"))
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("File content = %s, want %s", string(data), string(content))
		}
	})

	t.Run("WriteWithSubdir", func(t *testing.T) {
		content := []byte("nested file content")
		reader := bytes.NewReader(content)

		err := local.Write(ctx, "subdir/nested.txt", reader, int64(len(content)), nil)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(filepath.Join(tempDir, "subdir/nested.txt"))
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("File content = %s, want %s", string(data), string(content))
		}
	})

	t.Run("WriteWithMetadata", func(t *testing.T) {
		content := []byte("metadata file content")
		reader := bytes.NewReader(content)
		modTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)

		metadata := &FileInfo{
			ModTime:     modTime,
			Permissions: 0600,
		}

		err := local.Write(ctx, "meta.txt", reader, int64(len(content)), metadata)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Verify metadata
		info, err := os.Stat(filepath.Join(tempDir, "meta.txt"))
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}

		if !info.ModTime().Truncate(time.Second).Equal(modTime) {
			t.Errorf("ModTime = %v, want %v", info.ModTime().Truncate(time.Second), modTime)
		}
		if info.Mode().Perm() != os.FileMode(0600) {
			t.Errorf("Permissions = %v, want %v", info.Mode().Perm(), os.FileMode(0600))
		}
	})

	t.Run("OverwriteFile", func(t *testing.T) {
		// Write initial content
		content1 := []byte("initial content")
		reader1 := bytes.NewReader(content1)
		err := local.Write(ctx, "overwrite.txt", reader1, int64(len(content1)), nil)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Overwrite with new content
		content2 := []byte("new content")
		reader2 := bytes.NewReader(content2)
		err = local.Write(ctx, "overwrite.txt", reader2, int64(len(content2)), nil)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Verify content was overwritten
		data, err := os.ReadFile(filepath.Join(tempDir, "overwrite.txt"))
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if !bytes.Equal(data, content2) {
			t.Errorf("File content = %s, want %s", string(data), string(content2))
		}
	})
}

// TestLocalDelete tests the Delete method
func TestLocalDelete(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("DeleteFile", func(t *testing.T) {
		// Create file
		filePath := filepath.Join(tempDir, "delete.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		err := local.Delete(ctx, "delete.txt")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify file was deleted
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("File should be deleted")
		}
	})

	t.Run("DeleteDirectory", func(t *testing.T) {
		// Create directory with files
		subDir := filepath.Join(tempDir, "delete_dir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		err := local.Delete(ctx, "delete_dir")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify directory was deleted
		if _, err := os.Stat(subDir); !os.IsNotExist(err) {
			t.Error("Directory should be deleted")
		}
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		// Should not error for non-existent file
		err := local.Delete(ctx, "nonexistent.txt")
		if err != nil {
			t.Errorf("Delete() should not fail for non-existent file: %v", err)
		}
	})
}

// TestLocalExists tests the Exists method
func TestLocalExists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	if err := os.WriteFile(filepath.Join(tempDir, "exists.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("ExistingFile", func(t *testing.T) {
		exists, err := local.Exists(ctx, "exists.txt")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		exists, err := local.Exists(ctx, "nonexistent.txt")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false")
		}
	})

	t.Run("Directory", func(t *testing.T) {
		// Create directory
		if err := os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		exists, err := local.Exists(ctx, "subdir")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true for directory")
		}
	})
}

// TestLocalStat tests the Stat method
func TestLocalStat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("test content")
	filePath := filepath.Join(tempDir, "stat.txt")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("ExistingFile", func(t *testing.T) {
		info, err := local.Stat(ctx, "stat.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}

		if info.Size != int64(len(content)) {
			t.Errorf("Size = %d, want %d", info.Size, len(content))
		}
		if info.IsDir {
			t.Error("IsDir = true, want false")
		}
		if info.RelativePath != "stat.txt" {
			t.Errorf("RelativePath = %s, want stat.txt", info.RelativePath)
		}
		if info.ModTime.IsZero() {
			t.Error("ModTime should not be zero")
		}
	})

	t.Run("Directory", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		info, err := local.Stat(ctx, "subdir")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}

		if !info.IsDir {
			t.Error("IsDir = false, want true")
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		_, err := local.Stat(ctx, "nonexistent.txt")
		if err == nil {
			t.Error("Stat() should fail for non-existent file")
		}
	})
}

// TestLocalMkdirAll tests the MkdirAll method
func TestLocalMkdirAll(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	ctx := context.Background()

	t.Run("CreateSingleDir", func(t *testing.T) {
		err := local.MkdirAll(ctx, "newdir")
		if err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		info, err := os.Stat(filepath.Join(tempDir, "newdir"))
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !info.IsDir() {
			t.Error("Should be a directory")
		}
	})

	t.Run("CreateNestedDirs", func(t *testing.T) {
		err := local.MkdirAll(ctx, "level1/level2/level3")
		if err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		info, err := os.Stat(filepath.Join(tempDir, "level1/level2/level3"))
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !info.IsDir() {
			t.Error("Should be a directory")
		}
	})

	t.Run("ExistingDir", func(t *testing.T) {
		// Create dir first
		if err := os.MkdirAll(filepath.Join(tempDir, "existing"), 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		// Should not error
		err := local.MkdirAll(ctx, "existing")
		if err != nil {
			t.Fatalf("MkdirAll() error for existing dir = %v", err)
		}
	})
}

// TestBackendInterface verifies Local implements Backend interface
func TestBackendInterface(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "syncnorris-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	local, err := NewLocal(tempDir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	defer local.Close()

	// Verify interface implementation
	var _ Backend = local
}
