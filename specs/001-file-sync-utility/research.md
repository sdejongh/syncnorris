# Research: File Synchronization Utility

**Feature**: 001-file-sync-utility
**Date**: 2025-11-22
**Phase**: Phase 0 - Technology Selection & Best Practices

## Overview

This document captures research findings for technology choices and implementation patterns for the syncnorris file synchronization utility. All decisions support the constitutional requirements for cross-platform compatibility, performance, and extensible architecture.

## 1. CLI Framework Selection

### Decision: **Cobra** (github.com/spf13/cobra)

### Rationale

Cobra is the de facto standard for building modern CLI applications in Go and is used by prominent projects including:
- kubectl (Kubernetes CLI)
- Hugo (static site generator)
- GitHub CLI
- Docker CLI

**Key advantages**:
- **Nested subcommands**: Natural fit for our command structure (sync, compare, config, etc.)
- **Automatic help generation**: Reduces documentation burden
- **POSIX-compliant flags**: Professional UX expected by sysadmins
- **Flag completion**: Supports bash/zsh completion out of the box
- **Mature and stable**: v1.8+ with 10+ years of production use
- **Companion library (viper)**: YAML config integration (aligns with user requirement)

### Alternatives Considered

**urfave/cli (v2)**:
- Simpler API, less boilerplate
- Rejected because: Limited nested subcommand support, less feature-rich for complex CLIs

**spf13/pflag**:
- Low-level flag parsing only
- Rejected because: Would require building command routing manually; Cobra uses pflag internally anyway

### Implementation Impact

- Primary dependency: `github.com/spf13/cobra@latest`
- Config integration: `github.com/spf13/viper@latest` (YAML support, env vars, config file watching)
- Command structure:
  ```
  syncnorris sync --source /src --dest /dst --mode oneway
  syncnorris compare --source /src --dest /dst --method hash
  syncnorris config set logging.format json
  ```

---

## 2. Logging Library Selection

### Decision: **Zerolog** (github.com/rs/zerolog)

### Rationale

Zerolog provides the best combination of performance, structured logging, and multi-format output required for syncnorris.

**Key advantages**:
- **Zero-allocation JSON encoding**: Critical for high-performance sync operations with millions of files
- **Multiple output formats**: JSON (native), console (human-readable), can adapt for XML
- **Contextual logging**: Attach sync operation metadata to all log entries
- **Leveled logging**: Debug, Info, Warn, Error, Fatal
- **Sampling**: Reduce log volume for repetitive operations
- **Hook system**: Inject custom formatters (e.g., XML wrapper)

**Benchmark results** (from official comparisons):
- Zerolog: 0 allocs/op, ~200ns/op for JSON
- Zap: 5 allocs/op, ~1200ns/op
- Logrus: 17 allocs/op, ~3500ns/op

### Alternatives Considered

**Zap (uber-go/zap)**:
- Excellent performance, structured logging
- Rejected because: More allocations than zerolog, primarily JSON-focused (XML support would require custom encoders)

**Logrus (sirupsen/logrus)**:
- Popular, flexible formatters
- Rejected because: Significantly slower (3500ns vs 200ns per log), higher memory usage, not designed for high-throughput scenarios

### Implementation Impact

- Primary dependency: `github.com/rs/zerolog@latest`
- Log format selection via CLI flag: `--log-format json|text|xml`
- Custom XML encoder: Wrap zerolog JSON output with simple XML transformer for FR-030 compliance
- Example structured log entry:
  ```json
  {
    "level": "info",
    "time": "2025-11-22T10:30:00Z",
    "operation_id": "sync-001",
    "source": "/data/project",
    "dest": "/backup/project",
    "file": "src/main.go",
    "action": "copied",
    "bytes": 4096,
    "duration_ms": 45
  }
  ```

---

## 3. Progress Bar Library Selection

### Decision: **cheggaaa/pb** (v3)

### Rationale

cheggaaa/pb provides rich, customizable progress visualization with minimal dependencies, suitable for long-running sync operations.

**Key advantages**:
- **Multiple progress bars**: Show overall progress + current file progress simultaneously
- **Custom templates**: Fully customizable output format
- **Rate calculation**: Built-in ETA, transfer speed computation
- **Thread-safe**: Safe for concurrent operations
- **Terminal detection**: Auto-disables in non-TTY environments (e.g., when piped or in CI)
- **No external dependencies**: Pure Go

### Alternatives Considered

**schollz/progressbar**:
- Simpler API, good for basic use cases
- Rejected because: Less flexible for complex multi-file scenarios, limited template customization

**mpb (vbauerster/mpb)**:
- Very feature-rich, supports multiple bars
- Rejected because: Overly complex for our needs, steeper learning curve

### Implementation Impact

- Primary dependency: `github.com/cheggaaa/pb/v3@latest`
- Progress output example:
  ```
  Overall: [=========>          ] 45% | 4500/10000 files | 2.3 GB/5.1 GB | 5.2 MB/s | ETA: 2m15s
  Current: [===================>] 95% | transferring: /path/to/large/file.zip
  ```
- Integration with output formatters:
  - Human mode: Show progress bars
  - JSON mode: Emit progress events as JSON objects
  - Quiet mode: Suppress progress, only show errors

---

## 4. Go Cross-Platform Best Practices

### File Path Handling

**Decision**: Use `filepath` package exclusively for path operations

**Rationale**:
- `filepath.Join()` automatically uses correct separator (/ on Unix, \ on Windows)
- `filepath.Clean()` normalizes paths across platforms
- `filepath.Abs()` resolves relative to absolute paths portably

**Implementation**:
- Never hardcode path separators
- Use `filepath.ToSlash()` for storage/comparison (normalize to /)
- Use `filepath.FromSlash()` when constructing OS-specific paths

### Windows UNC Path Support

**Decision**: Detect and handle UNC paths with special logic

**Pattern**:
```go
func isUNCPath(path string) bool {
    return runtime.GOOS == "windows" && strings.HasPrefix(path, `\\`)
}

func normalizeUNCPath(path string) string {
    if !isUNCPath(path) {
        return filepath.Clean(path)
    }
    // Preserve \\ prefix for UNC paths
    cleaned := filepath.Clean(path)
    if !strings.HasPrefix(cleaned, `\\`) {
        return `\\` + cleaned
    }
    return cleaned
}
```

### Cross-Platform Testing

**Decision**: Use build tags and CI matrix

**Pattern**:
- Build tags for platform-specific code: `//go:build windows`, `//go:build !windows`
- GitHub Actions matrix: test on ubuntu-latest, windows-latest, macos-latest
- Integration tests with Docker containers for Linux-only features (NFS mounts)

---

## 5. File Hashing for Large Files

### Decision: Stream-based hashing with io.Copy

**Rationale**:
- Prevents loading entire file into memory
- Works with files larger than available RAM
- Leverages io.Copy's internal buffering (32KB default)

**Pattern**:
```go
func computeFileSHA256(path string) (string, error) {
    file, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer file.Close()

    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", err
    }

    return hex.EncodeToString(hasher.Sum(nil)), nil
}
```

**Performance considerations**:
- Buffer size: Use 64KB chunks for network filesystems (reduces syscalls)
- Parallelism: Hash multiple files concurrently (limit: CPU count × 2)
- Caching: Store hashes in memory map during comparison phase

---

## 6. Parallel File Operations

### Decision: Worker pool pattern with semaphore

**Rationale**:
- Control concurrency to avoid overwhelming filesystem/network
- Graceful error handling (don't stop entire operation on single file failure)
- Progress reporting from multiple goroutines

**Pattern**:
```go
type FileTask struct {
    Source string
    Dest   string
}

func parallelCopy(tasks []FileTask, maxWorkers int) error {
    sem := make(chan struct{}, maxWorkers)
    errChan := make(chan error, len(tasks))
    var wg sync.WaitGroup

    for _, task := range tasks {
        wg.Add(1)
        go func(t FileTask) {
            defer wg.Done()
            sem <- struct{}{}        // Acquire
            defer func() { <-sem }() // Release

            if err := copyFile(t.Source, t.Dest); err != nil {
                errChan <- fmt.Errorf("%s: %w", t.Source, err)
            }
        }(task)
    }

    wg.Wait()
    close(errChan)

    // Collect errors
    var errs []error
    for err := range errChan {
        errs = append(errs, err)
    }

    if len(errs) > 0 {
        return fmt.Errorf("encountered %d errors: %v", len(errs), errs)
    }
    return nil
}
```

---

## 7. Bidirectional Sync Conflict Detection

### Decision: Three-way comparison with last-sync state tracking

**Rationale**:
- Cannot reliably detect conflicts without knowing "last known good state"
- Must track: file hash, modification time, size at last sync
- Store in hidden `.syncnorris` directory in both source and dest

**Pattern**:
```go
type SyncState struct {
    FilePath     string
    Hash         string
    ModTime      time.Time
    Size         int64
    LastSyncTime time.Time
}

// Conflict detection logic:
// 1. Load last sync state for both sides
// 2. For each file:
//    - Changed on both sides + different hashes → CONFLICT
//    - Deleted on A, modified on B → CONFLICT
//    - Modified on A, deleted on B → CONFLICT
//    - Changed on one side only → SYNC
```

**Conflict resolution strategies** (user-configurable):
- `--conflict=ask`: Prompt user (interactive mode)
- `--conflict=source-wins`: Always keep source version
- `--conflict=dest-wins`: Always keep destination version
- `--conflict=newer`: Keep file with newer modification time
- `--conflict=both`: Keep both, rename with suffix (e.g., `.conflict.source`, `.conflict.dest`)

---

## 8. Configuration File Structure (YAML)

### Decision: Hierarchical YAML with sane defaults

**Example config.yaml**:
```yaml
# syncnorris configuration file

# Default sync behavior
sync:
  mode: oneway              # oneway | bidirectional
  comparison: hash          # namesize | timestamp | binary | hash
  conflict_resolution: ask  # ask | source-wins | dest-wins | newer | both

# Performance tuning
performance:
  max_workers: 8            # Parallel file operations (0 = CPU count)
  buffer_size: 65536        # 64KB chunks for file I/O
  bandwidth_limit: 0        # Bytes/sec (0 = unlimited)

# Output configuration
output:
  format: human             # human | json
  progress: true            # Show progress bars
  quiet: false              # Suppress non-error output

# Logging configuration
logging:
  enabled: true
  format: json              # json | text | xml
  level: info               # debug | info | warn | error
  file: /var/log/syncnorris.log  # Empty = stderr only

# Storage backends
storage:
  local:
    follow_symlinks: false
  smb:
    timeout: 30s            # Connection timeout
  nfs:
    timeout: 30s

# Exclusions (glob patterns)
exclude:
  - "*.tmp"
  - ".git/"
  - "node_modules/"
  - ".DS_Store"
```

**Implementation**:
- Viper handles YAML parsing, env vars, defaults
- Override via CLI flags: `--sync.mode=bidirectional`
- Config file search order: `./syncnorris.yaml`, `~/.config/syncnorris/config.yaml`, `/etc/syncnorris/config.yaml`

---

## 9. Daemon/Background Execution

### Decision: Use OS-specific process management

**Rationale**:
- Linux: systemd units (most distributions)
- macOS: launchd plists
- Windows: Services API or Task Scheduler

**Implementation approach**:
- CLI flag: `--daemon` or `--background`
- Daemon mode:
  - Detach from terminal (Unix: double-fork, Windows: CreateProcess with DETACHED_PROCESS)
  - Write PID file: `/var/run/syncnorris.pid`
  - Redirect stdout/stderr to log file
  - Handle SIGTERM for graceful shutdown
- Provide example service files in `config/` directory

**Example systemd unit**:
```ini
[Unit]
Description=syncnorris file synchronization
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/syncnorris sync --config /etc/syncnorris/config.yaml
Restart=on-failure
User=syncuser

[Install]
WantedBy=multi-user.target
```

---

## 10. Build and Distribution Strategy

### Decision: Makefile + GitHub Actions for cross-compilation

**Build targets**:
- linux/amd64
- linux/arm64
- windows/amd64
- darwin/amd64 (Intel Mac)
- darwin/arm64 (Apple Silicon)

**Makefile snippet**:
```makefile
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all
all: build-linux build-windows build-darwin

.PHONY: build-linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/syncnorris-linux-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/syncnorris-linux-arm64 cmd/syncnorris/main.go

.PHONY: build-windows
build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/syncnorris-windows-amd64.exe cmd/syncnorris/main.go

.PHONY: build-darwin
build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/syncnorris-darwin-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/syncnorris-darwin-arm64 cmd/syncnorris/main.go
```

**Distribution**:
- GitHub Releases with automated binary uploads
- Checksums (SHA-256) for verification
- Optional: Package for apt (Debian/Ubuntu), homebrew (macOS), chocolatey (Windows)

---

## Summary of Decisions

| Category | Decision | Key Rationale |
|----------|----------|---------------|
| CLI Framework | Cobra | Industry standard, nested commands, viper integration |
| Logging | Zerolog | Zero-allocation JSON, best performance, multi-format |
| Progress Bars | cheggaaa/pb | Flexible templates, multi-bar support, thread-safe |
| Path Handling | filepath package | Cross-platform normalization, UNC support |
| File Hashing | Stream-based (io.Copy) | Handles files larger than RAM |
| Concurrency | Worker pool + semaphore | Controlled parallelism, error collection |
| Bidirectional Sync | Three-way state tracking | Reliable conflict detection |
| Config Format | YAML via viper | User requirement, hierarchy, defaults |
| Daemon Mode | OS-specific services | Native integration (systemd, launchd, Windows Services) |
| Build Process | Makefile + GitHub Actions | Automated cross-compilation, reproducible |

## Dependencies Summary

**Direct dependencies** (go.mod):
```
github.com/spf13/cobra v1.8+
github.com/spf13/viper v1.18+
github.com/rs/zerolog v1.31+
github.com/cheggaaa/pb/v3 v3.1+
gopkg.in/yaml.v3 v3.0+
```

**Standard library** (no external deps):
- crypto/sha256 (hashing)
- io, os, filepath (file operations)
- encoding/json, encoding/xml (output formats)
- sync (concurrency primitives)
- runtime (platform detection)

**Total dependency count**: 5 external + standard library
**Static binary size estimate**: 8-12 MB (after stripping symbols with -ldflags="-s -w")

---

## Next Steps

With all technology choices resolved, we can proceed to:
1. **Phase 1**: Define data models and CLI contract
2. **Implementation**: Begin with core abstractions (storage interface, comparator interface)
3. **Testing**: Set up CI matrix for cross-platform validation

All decisions align with constitutional requirements:
- ✅ Cross-platform compatibility
- ✅ Single binary distribution
- ✅ Extensible architecture
- ✅ High performance
- ✅ Dual output modes
- ✅ Data integrity
