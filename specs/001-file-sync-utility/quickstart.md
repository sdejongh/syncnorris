# Quickstart Guide: syncnorris File Synchronization Utility

**Feature**: 001-file-sync-utility
**Date**: 2025-11-22
**Phase**: Phase 1 - Design

## Overview

This quickstart guide provides step-by-step instructions for developers to get started with implementing and testing the syncnorris file synchronization utility. It covers project setup, building, running basic operations, and validating the implementation against requirements.

---

## Prerequisites

### Development Environment

- **Go**: Version 1.21 or later ([download](https://go.dev/dl/))
- **Git**: For version control
- **Make**: For build automation (optional but recommended)
- **Text editor/IDE**: VS Code with Go extension, GoLand, or vim/emacs

### System Requirements

- **Linux**, **Windows**, or **macOS** (all supported)
- Minimum 2 GB RAM
- 100 MB free disk space for development
- Network connectivity for downloading dependencies

### Verify Installation

```bash
# Check Go version
go version
# Expected: go version go1.21.x or later

# Check Git
git --version

# Check Make (optional)
make --version
```

---

## Step 1: Project Initialization

### Create Project Structure

```bash
# Create project directory
mkdir -p ~/projects/syncnorris
cd ~/projects/syncnorris

# Initialize Go module
go mod init github.com/yourusername/syncnorris

# Create directory structure (from plan.md)
mkdir -p cmd/syncnorris
mkdir -p pkg/{storage,compare,sync,output,logging,config,models}
mkdir -p internal/{cli,platform}
mkdir -p tests/{integration,unit,testdata/fixtures}
mkdir -p config scripts

# Create placeholder files
touch cmd/syncnorris/main.go
touch pkg/storage/backend.go
touch pkg/models/operation.go
touch config/config.example.yaml
touch README.md
```

### Add Dependencies

```bash
# Add CLI framework (Cobra + Viper)
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest

# Add logging (Zerolog)
go get github.com/rs/zerolog@latest

# Add progress bars (cheggaaa/pb)
go get github.com/cheggaaa/pb/v3@latest

# Add YAML support
go get gopkg.in/yaml.v3@latest

# Tidy dependencies
go mod tidy
```

---

## Step 2: Minimal Working Example

### Create main.go

```go
// cmd/syncnorris/main.go
package main

import (
    "fmt"
    "os"
    "github.com/spf13/cobra"
)

var (
    version = "dev"
    rootCmd = &cobra.Command{
        Use:   "syncnorris",
        Short: "Cross-platform file synchronization utility",
        Long:  "syncnorris synchronizes files between local folders, network shares, and remote storage",
    }

    syncCmd = &cobra.Command{
        Use:   "sync",
        Short: "Synchronize two folders",
        RunE:  runSync,
    }

    versionCmd = &cobra.Command{
        Use:   "version",
        Short: "Show version information",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("syncnorris version %s\n", version)
        },
    }
)

var (
    sourcePath string
    destPath   string
    dryRun     bool
)

func init() {
    syncCmd.Flags().StringVarP(&sourcePath, "source", "s", "", "Source directory (required)")
    syncCmd.Flags().StringVarP(&destPath, "dest", "d", "", "Destination directory (required)")
    syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compare only, don't sync")
    syncCmd.MarkFlagRequired("source")
    syncCmd.MarkFlagRequired("dest")

    rootCmd.AddCommand(syncCmd)
    rootCmd.AddCommand(versionCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
    if dryRun {
        fmt.Printf("DRY RUN: Would sync %s → %s\n", sourcePath, destPath)
    } else {
        fmt.Printf("Syncing %s → %s\n", sourcePath, destPath)
    }
    return nil
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Build and Test

```bash
# Build the binary
go build -o syncnorris cmd/syncnorris/main.go

# Test basic commands
./syncnorris --help
./syncnorris version
./syncnorris sync --help

# Test with minimal arguments
./syncnorris sync --source /tmp/test-src --dest /tmp/test-dst --dry-run
```

**Expected Output**:
```
DRY RUN: Would sync /tmp/test-src → /tmp/test-dst
```

---

## Step 3: Implement Core Abstractions

### Storage Interface (pkg/storage/backend.go)

```go
package storage

import (
    "io"
    "time"
)

// FileInfo represents basic file metadata
type FileInfo struct {
    Path    string
    Size    int64
    ModTime time.Time
    IsDir   bool
}

// Backend defines the interface for storage backends
type Backend interface {
    // List returns all files in the given path recursively
    List(path string) ([]FileInfo, error)

    // Read opens a file for reading
    Read(path string) (io.ReadCloser, error)

    // Write creates or overwrites a file
    Write(path string, reader io.Reader) error

    // Delete removes a file
    Delete(path string) error

    // Exists checks if a file exists
    Exists(path string) (bool, error)
}
```

### Comparator Interface (pkg/compare/comparator.go)

```go
package compare

import "github.com/yourusername/syncnorris/pkg/storage"

// Result represents comparison outcome for a single file
type Result struct {
    Path       string
    Status     Status
    SourceInfo *storage.FileInfo
    DestInfo   *storage.FileInfo
}

// Status represents file comparison state
type Status int

const (
    StatusNew       Status = iota // File only in source
    StatusModified                // File in both but different
    StatusUnchanged               // Files identical
    StatusDeleted                 // File only in dest
)

// Comparator defines interface for file comparison
type Comparator interface {
    // Compare determines if two files are different
    Compare(sourcePath, destPath string) (Status, error)

    // Name returns the comparison method name
    Name() string
}
```

---

## Step 4: Implement Name/Size Comparator (MVP)

### Create pkg/compare/namesize.go

```go
package compare

import (
    "os"
)

type NameSizeComparator struct{}

func NewNameSizeComparator() *NameSizeComparator {
    return &NameSizeComparator{}
}

func (c *NameSizeComparator) Name() string {
    return "namesize"
}

func (c *NameSizeComparator) Compare(sourcePath, destPath string) (Status, error) {
    sourceInfo, sourceErr := os.Stat(sourcePath)
    destInfo, destErr := os.Stat(destPath)

    // Dest doesn't exist -> New
    if destErr != nil && os.IsNotExist(destErr) {
        return StatusNew, nil
    }

    if destErr != nil {
        return StatusUnchanged, destErr
    }

    // Source doesn't exist -> Deleted
    if sourceErr != nil && os.IsNotExist(sourceErr) {
        return StatusDeleted, nil
    }

    if sourceErr != nil {
        return StatusUnchanged, sourceErr
    }

    // Compare size
    if sourceInfo.Size() != destInfo.Size() {
        return StatusModified, nil
    }

    return StatusUnchanged, nil
}
```

### Write Unit Test (tests/unit/compare_test.go)

```go
package compare_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/yourusername/syncnorris/pkg/compare"
)

func TestNameSizeComparator(t *testing.T) {
    tmpDir := t.TempDir()

    // Create test files
    srcFile := filepath.Join(tmpDir, "source.txt")
    dstFile := filepath.Join(tmpDir, "dest.txt")

    // Test: File only in source (New)
    os.WriteFile(srcFile, []byte("hello"), 0644)

    comp := compare.NewNameSizeComparator()
    status, err := comp.Compare(srcFile, dstFile)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if status != compare.StatusNew {
        t.Errorf("expected StatusNew, got %v", status)
    }

    // Test: Files with same size (Unchanged)
    os.WriteFile(dstFile, []byte("world"), 0644) // Same length
    status, err = comp.Compare(srcFile, dstFile)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if status != compare.StatusUnchanged {
        t.Errorf("expected StatusUnchanged, got %v", status)
    }

    // Test: Files with different sizes (Modified)
    os.WriteFile(dstFile, []byte("longer content"), 0644)
    status, err = comp.Compare(srcFile, dstFile)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if status != compare.StatusModified {
        t.Errorf("expected StatusModified, got %v", status)
    }
}
```

### Run Tests

```bash
go test ./tests/unit/... -v
```

**Expected Output**:
```
=== RUN   TestNameSizeComparator
--- PASS: TestNameSizeComparator (0.00s)
PASS
ok      github.com/yourusername/syncnorris/tests/unit   0.003s
```

---

## Step 5: Cross-Platform Build

### Create Makefile

```makefile
# Makefile
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
BUILD_DIR := dist

.PHONY: all
all: clean build-all

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

.PHONY: build
build:
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris cmd/syncnorris/main.go

.PHONY: build-all
build-all:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-linux-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-linux-arm64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-windows-amd64.exe cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-darwin-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-darwin-arm64 cmd/syncnorris/main.go

.PHONY: test
test:
	go test ./... -v -race -coverprofile=coverage.out

.PHONY: lint
lint:
	go vet ./...
	golangci-lint run

.PHONY: run
run:
	go run cmd/syncnorris/main.go
```

### Build for All Platforms

```bash
make build-all
ls -lh dist/
```

**Expected Output**:
```
-rwxr-xr-x  1 user  staff   8.1M Nov 22 10:00 syncnorris-darwin-amd64
-rwxr-xr-x  1 user  staff   7.9M Nov 22 10:00 syncnorris-darwin-arm64
-rwxr-xr-x  1 user  staff   8.3M Nov 22 10:00 syncnorris-linux-amd64
-rwxr-xr-x  1 user  staff   8.0M Nov 22 10:00 syncnorris-linux-arm64
-rwxr-xr-x  1 user  staff   8.5M Nov 22 10:00 syncnorris-windows-amd64.exe
```

---

## Step 6: Integration Test

### Create Integration Test (tests/integration/sync_test.go)

```go
package integration_test

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func TestBasicSync(t *testing.T) {
    // Setup: create temp directories
    tmpDir := t.TempDir()
    srcDir := filepath.Join(tmpDir, "source")
    dstDir := filepath.Join(tmpDir, "dest")

    os.MkdirAll(srcDir, 0755)
    os.MkdirAll(dstDir, 0755)

    // Create test files in source
    testFiles := map[string]string{
        "file1.txt": "content 1",
        "file2.txt": "content 2",
        "subdir/file3.txt": "content 3",
    }

    for path, content := range testFiles {
        fullPath := filepath.Join(srcDir, path)
        os.MkdirAll(filepath.Dir(fullPath), 0755)
        os.WriteFile(fullPath, []byte(content), 0644)
    }

    // Run syncnorris
    cmd := exec.Command("../../dist/syncnorris",
        "sync",
        "--source", srcDir,
        "--dest", dstDir,
    )
    output, err := cmd.CombinedOutput()

    if err != nil {
        t.Fatalf("sync failed: %v\nOutput: %s", err, output)
    }

    // Verify: all files copied to dest
    for path, expectedContent := range testFiles {
        destPath := filepath.Join(dstDir, path)
        actualContent, err := os.ReadFile(destPath)

        if err != nil {
            t.Errorf("file %s not found in dest: %v", path, err)
            continue
        }

        if string(actualContent) != expectedContent {
            t.Errorf("file %s: expected %q, got %q", path, expectedContent, actualContent)
        }
    }

    t.Logf("Sync output:\n%s", output)
}
```

### Run Integration Test

```bash
# Build first
make build

# Run integration tests
go test ./tests/integration/... -v
```

---

## Step 7: Validate Against Requirements

### Test Checklist (from spec.md)

| Requirement | Test Command | Expected Result | Status |
|-------------|--------------|-----------------|--------|
| FR-001: One-way sync | `syncnorris sync -s /src -d /dst` | Files copied | [ ] |
| FR-006: Name/size comparison | `syncnorris sync --comparison namesize` | Fast comparison | [ ] |
| FR-017: Human output | `syncnorris sync -s /src -d /dst` | Progress bar shown | [ ] |
| FR-018: JSON output | `syncnorris sync -s /src -d /dst --output json` | Valid JSON | [ ] |
| FR-023: Dry-run mode | `syncnorris sync --dry-run -s /src -d /dst` | No changes made | [ ] |
| SC-008: Cross-platform | Build on Linux, Windows, macOS | All binaries work | [ ] |

### Manual Validation

```bash
# Create test directories
mkdir -p /tmp/sync-test/{source,dest}
echo "test content" > /tmp/sync-test/source/file.txt

# Test: Basic sync
./dist/syncnorris sync --source /tmp/sync-test/source --dest /tmp/sync-test/dest

# Verify: File copied
cat /tmp/sync-test/dest/file.txt
# Expected: "test content"

# Test: Dry-run mode
./dist/syncnorris sync --source /tmp/sync-test/source --dest /tmp/sync-test/dest --dry-run

# Verify: No changes made, only comparison reported
```

---

## Step 8: Configuration File

### Create config/config.example.yaml

```yaml
# syncnorris configuration example

sync:
  mode: oneway              # oneway | bidirectional
  comparison: hash          # namesize | timestamp | binary | hash
  conflict_resolution: ask  # ask | source-wins | dest-wins | newer | both

performance:
  max_workers: 8            # Parallel file operations (0 = CPU count)
  buffer_size: 65536        # 64KB chunks for file I/O
  bandwidth_limit: 0        # Bytes/sec (0 = unlimited)

output:
  format: human             # human | json
  progress: true            # Show progress bars
  quiet: false              # Suppress non-error output

logging:
  enabled: true
  format: json              # json | text | xml
  level: info               # debug | info | warn | error
  file: ""                  # Empty = stderr only

exclude:
  - "*.tmp"
  - ".git/"
  - "node_modules/"
  - ".DS_Store"
```

### Test Configuration

```bash
# Copy example config
cp config/config.example.yaml ~/.config/syncnorris/config.yaml

# Run with config
./dist/syncnorris sync --source /tmp/test-src --dest /tmp/test-dst
```

---

## Next Steps

### Phase 2: Core Implementation

1. **Implement remaining comparators**:
   - Timestamp (pkg/compare/timestamp.go)
   - Binary (pkg/compare/binary.go)
   - Hash (pkg/compare/hash.go)

2. **Implement storage backends**:
   - Local filesystem (pkg/storage/local.go)
   - SMB/Samba (pkg/storage/smb.go)
   - NFS (pkg/storage/nfs.go)

3. **Implement sync engine**:
   - One-way sync (pkg/sync/oneway.go)
   - Bidirectional sync (pkg/sync/bidirectional.go)
   - Parallel workers (pkg/sync/worker.go)

4. **Implement output formatters**:
   - Human-readable (pkg/output/human.go)
   - JSON (pkg/output/json.go)
   - Progress bars (pkg/output/progress.go)

5. **Implement logging**:
   - JSON logger (pkg/logging/jsonlog.go)
   - Text logger (pkg/logging/textlog.go)
   - XML logger (pkg/logging/xmllog.go)

### Phase 3: Advanced Features

1. Bidirectional sync with conflict detection
2. Resume interrupted operations
3. Daemon mode
4. Bandwidth throttling
5. Cross-platform CI/CD pipeline

### Phase 4: Distribution

1. Package for Linux (apt, snap)
2. Package for macOS (Homebrew)
3. Package for Windows (Chocolatey)
4. Binary releases on GitHub
5. Docker image (optional)

---

## Troubleshooting

### Build Errors

**Problem**: `go: cannot find module providing package X`

**Solution**:
```bash
go mod tidy
go get <package>
```

**Problem**: `permission denied` when running binary

**Solution**:
```bash
chmod +x dist/syncnorris
```

### Test Failures

**Problem**: Tests fail with "permission denied"

**Solution**: Ensure test directories are writable
```bash
chmod -R 755 tests/testdata
```

**Problem**: Cross-compilation fails on Windows

**Solution**: Use WSL or Git Bash with proper PATH

---

## Additional Resources

- **Cobra docs**: https://github.com/spf13/cobra
- **Zerolog docs**: https://github.com/rs/zerolog
- **Go standard project layout**: https://github.com/golang-standards/project-layout
- **Feature spec**: [spec.md](spec.md)
- **Implementation plan**: [plan.md](plan.md)
- **Data model**: [data-model.md](data-model.md)
- **CLI contract**: [contracts/cli-contract.md](contracts/cli-contract.md)

---

## Success Criteria

You've successfully completed the quickstart when:

- [x] Project structure created
- [x] Dependencies installed
- [x] Minimal CLI runs (`syncnorris --help`)
- [x] Unit tests pass
- [x] Cross-platform builds succeed
- [x] Integration test passes
- [x] Basic sync works end-to-end

**Ready for implementation**: Proceed to `/speckit.tasks` to generate detailed task breakdown.
