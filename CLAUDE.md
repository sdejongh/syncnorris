# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**syncnorris** is a cross-platform file synchronization utility written in Go 1.24. It supports one-way sync (production-ready) and bidirectional sync (experimental) between directories, with multiple comparison methods and a concurrent pipeline architecture.

## Build & Development Commands

```bash
make build              # Build for current platform → dist/syncnorris
make build-all          # Cross-compile for linux/windows/darwin (amd64+arm64)
make test               # Full test suite with race detection and coverage
make test-unit          # Unit tests only (pkg/)
make test-integration   # Integration tests only (tests/)
make test-coverage      # Generate coverage.html report
make lint               # Run go vet + golangci-lint
make clean              # Remove build artifacts
make run                # Run from source
```

Run a single test:
```bash
go test -v -run TestName ./pkg/sync/...
```

Build injects version info via ldflags (`main.version`, `main.commit`, `main.date`).

## Architecture

### Execution Flow

CLI (Cobra) → `SyncOperation` model → `Engine` → Pipeline (one-way) or BidirectionalPipeline → `SyncReport`
GUI (Gio)  → `appState` widgets → `RunOptions` → `runSync()` → `Engine` → `GUIFormatter` → UI channel → `appState`

### Producer-Consumer Pipeline (`pkg/sync/pipeline.go`)

The core sync mechanism is a producer-consumer pipeline with streaming file discovery:
1. **Scanner** uses `Backend.Walk()` to stream files as they're discovered — no intermediate slice buffering. Workers start processing as soon as the first file is found.
2. **Task Queue** (buffered channel, capacity 1000) holds `FileTask` entries
3. **Worker Pool** (default 5 workers) processes tasks concurrently — compare, copy/update/delete, report progress

The destination is scanned first (map needed for comparisons), then source scanning and worker processing happen concurrently. Statistics use lock-free atomic counters (not mutexes) for concurrent updates.

### Package Responsibilities

| Package | Role |
|---------|------|
| `cmd/syncnorris` | Entry point, version injection |
| `internal/cli` | Cobra command implementations, flag parsing, validation |
| `internal/gui` | Gio-based GUI: app state/event loop, layout, formatter bridge, runner, settings persistence, theme, bandwidth tracker |
| `internal/platform` | OS-specific path handling |
| `pkg/sync` | Engine, pipeline, workers, bidirectional logic, state persistence, exclusion |
| `pkg/compare` | Comparator interface + implementations: hash (SHA-256), md5, binary, namesize, timestamp, composite |
| `pkg/storage` | `Backend` interface + local filesystem implementation |
| `pkg/models` | All data types: FileEntry, SyncOperation, SyncReport, Statistics, conflicts |
| `pkg/output` | Formatter interface + human (progress bars), JSON, differences report |
| `pkg/config` | YAML config loading, defaults, validation |
| `pkg/logging` | Logger interface + file logger (with rotation) and null logger |
| `pkg/ratelimit` | Token bucket bandwidth limiter wrapping io.ReadCloser |

### Key Interfaces

- **`storage.Backend`** (`pkg/storage/backend.go`): Abstraction over filesystem operations (Walk, List, Read, Write, Delete, Stat, etc.). `Walk` streams entries via callback; `List` is a wrapper that collects into a slice.
- **`compare.Comparator`** (`pkg/compare/comparator.go`): File comparison strategy with `Compare()` and `Name()`
- **`output.Formatter`** (`pkg/output/formatter.go`): Output display with Start/Progress/Complete/Error lifecycle
- **`logging.Logger`** (`pkg/logging/logger.go`): Structured logging with levels and fields

### Bidirectional Sync (`pkg/sync/bidirectional.go`)

Handles 9 file-state combinations, conflict detection (modify-modify, delete-modify, create-create), conflict resolution strategies (newer, source-wins, dest-wins, both), and optional state persistence via JSON files (~/.syncnorris/state) for incremental change detection.

## Key Design Decisions

- **Comparison methods** trade off speed vs. accuracy: namesize (fastest, metadata only) → timestamp → hash/md5 → binary (slowest, byte-exact). The composite comparator does a quick name+size check before expensive hashing.
- **Partial hashing** for files ≥1MB reads only the first 256KB, reducing I/O by ~95% for dissimilar files.
- **Parallel hashing** computes source and destination hashes concurrently (1.8-1.9x speedup).
- **Streaming scan** via `Backend.Walk()` feeds the task queue as files are discovered, eliminating the blocking `List()` call in the one-way pipeline. Bidirectional sync still uses `List()` since both sides must be fully scanned before analysis.
- **Progress throttling** limits display updates to 20/sec max to reduce overhead.
- Exit codes: 0 (success), 1 (partial), 2 (failed), 3 (cancelled).

### GUI (`internal/gui/`)

Cross-platform graphical interface built with Gio (`gioui.org`), launched via `syncnorris gui`. Two-column layout: fixed-width config panel (left) + tabbed right panel (currently "Logs" tab). The GUI reuses the existing `Engine`/`Pipeline` layers via a `GUIFormatter` that implements `output.Formatter` and pushes events to the UI through a channel. Per-file action tracking (COPY/UPDATE/SKIP) is inferred from the pipeline event sequence (`compare_start`/`file_start`/`file_complete`). Settings and path history (last 10) are persisted in `~/.config/syncnorris/gui-settings.json` (or platform equivalent via `os.UserConfigDir()`). A `BandwidthTracker` (ring buffer, 60 samples at 1/sec) collects real-time throughput from cumulative bytes (completed + in-flight), displayed as a filled area chart with a dashed average line. Directory picking uses `github.com/ncruces/zenity` for native OS dialogs. The `gui` CLI command uses build tag `!nogui`; cross-compiled headless builds use `-tags nogui` with a stub command.

#### Pause / Resume (`pkg/sync/job`, `internal/gui/`)

One-way sync jobs support pause and resume, including automatic recovery after a crash or unexpected app close.

**Job persistence** — each run creates a `Job` record under `~/.config/syncnorris/jobs/<id>.json` (metadata) and `~/.config/syncnorris/jobs/<id>.log` (completion log). The completion log is an append-only TSV file; each line records a successfully synced file path, its size, and modification time. On resume, `scanSourceAndQueue` skips files whose completion-log entry matches the current on-disk stat (size + mtime), so already-synced work is not repeated.

**Instance lock** — `internal/gui/lock.go` acquires `~/.config/syncnorris/app.lock` via `github.com/gofrs/flock` before the Gio event loop starts. A second GUI instance exits immediately with an error banner rather than racing over the same job files.

**Atomic writes** — `storage.Local.Write` now writes to `<name>.syncnorris.partial` first and renames to the final path on success. This prevents partial files from appearing as complete entries in the destination and is active for all sync runs, not just paused ones.

**Pause semantics:**
- **Soft pause** — the pipeline drains the in-flight task queue and waits; no new tasks are dequeued. Workers finish their current file before stopping.
- **Hard pause** — the pipeline cancels all in-flight workers immediately via context cancellation. Partial files written so far are cleaned up by the `.partial` rename pattern (the rename never happened, so only the `.partial` file is left, which is ignored on re-scan).

**`PipelineConfig` fields added** — `Job *job.Job`, `JobStore *job.JobStore`, `PauseSoft <-chan struct{}`, `PauseHard <-chan struct{}`. All are optional; when nil the pipeline behaves identically to pre-v0.8.0.

## Dependencies

- `github.com/spf13/cobra` — CLI framework
- `github.com/cheggaaa/pb/v3` — Progress bars
- `github.com/google/uuid` — ID generation
- `gopkg.in/yaml.v3` — YAML config parsing
- `gioui.org` — Cross-platform GUI framework (Wayland, X11, Windows, macOS)
- `github.com/ncruces/zenity` — Native OS file/directory dialogs
- `github.com/gofrs/flock` — Cross-platform file locking (single-instance GUI lock)

## GUI Build Notes

- Gio requires system libs on Linux: `sudo apt install libwayland-dev libwayland-egl1 libxkbcommon-dev libxkbcommon-x11-dev libx11-dev libx11-xcb-dev libxcursor-dev libxfixes-dev libgles2-mesa-dev libegl1-mesa-dev libvulkan-dev`
- All files in `internal/gui/` have `//go:build !nogui` — excluded with `-tags nogui` for `CGO_ENABLED=0` cross-compilation
- Gio `app.MinSize()` panics if width or height is 0
- Wayland compositors may ignore programmatic `app.Size()` resize requests
- Windows `-H windowsgui` ldflags hides the console entirely — breaks CLI, don't use for dual-mode binaries
- CI uses native matrix builds (not GoReleaser) — one runner per OS for CGo support
- GitHub runners: use `macos-15` (not macos-13), `ubuntu-24.04-arm` for ARM64

## Release Workflow

Tags matching `v*` trigger `.github/workflows/release.yml`:
1. `test` job on ubuntu with GUI libs installed
2. `build` matrix: linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/amd64
3. `release` job: collect archives, checksums, create GitHub release
Archive naming: `syncnorris_v{VERSION}_{os}-{arch}.tar.gz` (or `.zip` for Windows)
