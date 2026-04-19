# syncnorris

Cross-platform file synchronization utility written in Go, with a concurrent pipeline, multiple comparison strategies, and an optional native GUI.

- **Status**: production-ready for one-way sync; bidirectional sync is experimental
- **Platforms**: Linux, macOS, Windows (amd64, arm64)
- **License**: MIT

## Features

### Synchronization

- One-way sync with parallel workers, dry-run mode, and optional orphan deletion (`--delete`)
- Bidirectional sync (experimental) with conflict detection and resolution strategies: `newer`, `source-wins`, `dest-wins`, `both`
- Optional state tracking (`--stateful`) for incremental bidirectional runs
- Streaming file discovery — workers start as soon as the first file is found
- Atomic writes via `.syncnorris.partial` rename pattern (no half-written files in the destination)
- Glob-based exclusion (`--exclude "*.log"`, repeatable)
- Bandwidth limiting (`--bandwidth 10M`)

### Comparison methods

| Method | Use case |
|--------|----------|
| `hash` (SHA-256, default) | Strong verification; partial hashing for files ≥1MB; parallel source/dest hashing |
| `md5` | Same optimizations as `hash`, slightly faster, weaker guarantee |
| `binary` | Byte-by-byte; reports exact differing offset |
| `namesize` | Metadata only; 10–40× faster for unchanged files |
| `timestamp` | Name + size + mtime; trusts timestamps |

### Graphical interface (`syncnorris gui`)

- Built with [Gio](https://gioui.org) — native rendering on Wayland, X11, Windows, and macOS
- Config panel (source/dest, comparison, workers, excludes, dry-run, delete, create-dest) + tabbed activity panel
- Live progress bar, bandwidth chart (60 s sliding window), color-coded per-file log
- Native directory picker, persistent settings and path history (`~/.config/syncnorris/gui-settings.json`)
- Single-instance file lock (`~/.config/syncnorris/app.lock`)
- **Pause / Resume** for one-way jobs, with automatic crash recovery:
  - *Soft pause* — finishes in-flight files, then waits
  - *Hard pause* — cancels immediately; partial files are cleaned up automatically
  - On restart, interrupted jobs can be resumed from where they stopped

### Output & logging

- Human-readable progress (tabular view of concurrent files, ETA, transfer rate) or JSON (`--output json`)
- Differences report (`--diff-report FILE`, human or JSON) listing copied, updated, skipped, deleted files and errors
- File logging (`--log-file`, `--log-format text|json`, `--log-level debug|info|warn|error`) with size-based rotation

## Installation

### Quick install

Linux and macOS:

```bash
curl -sSL https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/sdejongh/syncnorris/master/install.ps1 | iex
```

### Go

```bash
go install github.com/sdejongh/syncnorris/cmd/syncnorris@latest
```

### From source

```bash
git clone https://github.com/sdejongh/syncnorris.git
cd syncnorris
make build   # → dist/syncnorris
```

Pre-built archives are also available on the [Releases page](https://github.com/sdejongh/syncnorris/releases).

## Quick start

```bash
# Basic one-way sync
syncnorris sync -s /data/projects -d /backup/projects

# Preview changes only
syncnorris sync -s /src -d /dst --dry-run

# Fast re-sync (metadata comparison)
syncnorris sync -s /src -d /dst --comparison namesize

# Mirror source (delete orphans in destination)
syncnorris sync -s /src -d /dst --delete

# Launch the GUI
syncnorris gui
```

## Usage

### Commands

| Command | Purpose |
|---------|---------|
| `syncnorris sync` | Synchronize two directories |
| `syncnorris compare` | Compare directories without writing (alias for `sync --dry-run` with on-screen diff) |
| `syncnorris gui` | Launch the graphical interface |
| `syncnorris config` | Manage configuration |
| `syncnorris version` | Show version, commit, build date |

### Common flags

| Flag | Description |
|------|-------------|
| `-s, --source PATH` | Source directory (required) |
| `-d, --dest PATH` | Destination directory (required) |
| `--comparison METHOD` | `hash` (default), `md5`, `binary`, `namesize`, `timestamp` |
| `--dry-run` | Compare only, do not write |
| `--create-dest` | Create destination if missing |
| `--delete` | Delete orphan files in destination |
| `-p, --parallel N` | Worker count (default: 5) |
| `--exclude PATTERN` | Glob pattern to exclude (repeatable) |
| `-b, --bandwidth SIZE` | Bandwidth limit (e.g. `500K`, `10M`, `1G`) |
| `--mode MODE` | `oneway` (default) or `bidirectional` |
| `--conflict STRATEGY` | `newer`, `source-wins`, `dest-wins`, `both` (bidirectional only) |
| `--stateful` | Persist state between bidirectional runs |
| `--output FORMAT` | `human` (default) or `json` |
| `--diff-report FILE` | Write differences report to file |
| `--diff-format FORMAT` | `human` (default) or `json` |
| `--log-file PATH` | Enable file logging |
| `--log-format FORMAT` | `text` (default) or `json` |
| `--log-level LEVEL` | `debug`, `info` (default), `warn`, `error` |
| `-q, --quiet` | Suppress non-error output |
| `-v, --verbose` | Extra debug output |

Run `syncnorris <command> --help` for the full list.

## Configuration

Optional config file at `~/.config/syncnorris/config.yaml`:

```yaml
sync:
  mode: oneway
  comparison: hash

performance:
  max_workers: 8
  buffer_size: 65536
  bandwidth_limit: "0"      # "10M", "1G", or "0" for unlimited

output:
  format: human
  progress: true
  quiet: false
  verbose: false

exclude:
  - "*.log"
  - ".git/**"
  - "node_modules/**"
```

CLI flags override the config file.

## Bidirectional sync (experimental)

Bidirectional sync is functional but not yet production-ready. **Always test with `--dry-run` first.**

```bash
syncnorris sync -s /a -d /b --mode bidirectional --dry-run
syncnorris sync -s /a -d /b --mode bidirectional --conflict newer
syncnorris sync -s /a -d /b --mode bidirectional --conflict both --stateful
```

With `--conflict both`, conflicting files are kept under `.source-conflict` / `.dest-conflict` suffixes.

## Development

### Prerequisites

- Go 1.24+
- GNU Make (optional)
- GUI builds require system libraries on Linux:
  ```
  libwayland-dev libwayland-egl1 libxkbcommon-dev libxkbcommon-x11-dev
  libx11-dev libx11-xcb-dev libxcursor-dev libxfixes-dev
  libgles2-mesa-dev libegl1-mesa-dev libvulkan-dev
  ```
  (On Fedora: `wayland-devel libxkbcommon-devel libX11-devel libXcursor-devel mesa-libGLES-devel mesa-libEGL-devel`.)

### Build & test

```bash
make build              # current platform → dist/syncnorris
make build-all          # cross-compile (CLI only, no GUI)
make test               # full suite with race detection and coverage
make test-unit          # pkg/ only
make test-integration   # tests/ only
make lint               # go vet + golangci-lint
```

Headless builds (no GUI) use `-tags nogui` with `CGO_ENABLED=0`.

## Documentation

- [CHANGELOG.md](CHANGELOG.md) — version history
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) — detailed feature status
- [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) — dependency licenses

## Contributing

Contributions are welcome. Priority areas: testing and feedback on bidirectional sync, documentation improvements, bug reports.

## License

MIT — see [LICENSE](LICENSE).
