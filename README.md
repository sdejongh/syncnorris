# syncnorris

Cross-platform file synchronization utility built in Go.

## Features

- **One-way and bidirectional synchronization** between local folders, network shares, and remote storage
- **Multiple comparison methods**: name/size, timestamp, binary, or hash-based (SHA-256)
- **Dual output modes**: Human-readable with progress bars or JSON for automation
- **Cross-platform**: Single static binary for Linux, Windows, and macOS
- **Performance optimized**: Parallel transfers, incremental sync, minimal memory footprint
- **Network storage support**: SMB/Samba, NFS, UNC paths

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/yourusername/syncnorris.git
cd syncnorris

# Build
make build

# The binary will be in dist/syncnorris
```

### Cross-Platform Builds

```bash
# Build for all platforms
make build-all

# Or use the build script
./scripts/build.sh
```

## Quick Start

### One-way Synchronization

```bash
# Sync from source to destination
syncnorris sync --source /data/project --dest /backup/project

# With hash comparison (default)
syncnorris sync -s /src -d /dst --comparison hash

# Dry-run to preview changes
syncnorris sync -s /src -d /dst --dry-run
```

### Bidirectional Synchronization

```bash
# Two-way sync with automatic conflict resolution
syncnorris sync -s /home/user/docs -d /mnt/nas/docs \
  --mode bidirectional --conflict newer
```

### JSON Output for Automation

```bash
# Get structured JSON output
syncnorris sync -s /src -d /dst --output json | jq .
```

## Configuration

Create a config file at `~/.config/syncnorris/config.yaml`:

```yaml
sync:
  mode: oneway
  comparison: hash
  conflict_resolution: ask

performance:
  max_workers: 8
  buffer_size: 65536
  bandwidth_limit: 0

output:
  format: human
  progress: true
  quiet: false

logging:
  enabled: true
  format: json
  level: info
  file: ""

exclude:
  - "*.tmp"
  - ".git/"
  - "node_modules/"
```

## Usage

### Commands

- `syncnorris sync` - Synchronize two folders
- `syncnorris compare` - Compare folders without syncing (dry-run)
- `syncnorris config` - Manage configuration
- `syncnorris version` - Show version information

### Sync Options

```
--source, -s      Source directory path (required)
--dest, -d        Destination directory path (required)
--mode, -m        Sync mode: oneway, bidirectional (default: oneway)
--comparison      Comparison method: namesize, timestamp, binary, hash (default: hash)
--conflict        Conflict resolution: ask, source-wins, dest-wins, newer, both
--dry-run         Compare only, don't sync
--parallel, -p    Number of parallel workers (default: CPU count)
--bandwidth, -b   Bandwidth limit (e.g., "10M", "1G")
--exclude         Glob patterns to exclude
--output, -o      Output format: human, json (default: human)
```

## Development

### Prerequisites

- Go 1.21 or later
- Make (optional)

### Building

```bash
# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Lint
make lint
```

### Project Structure

```
syncnorris/
├── cmd/syncnorris/      # Main CLI entry point
├── pkg/                 # Public packages
│   ├── storage/         # Storage backends
│   ├── compare/         # Comparison algorithms
│   ├── sync/            # Sync engine
│   ├── output/          # Output formatters
│   ├── logging/         # Logging implementations
│   ├── config/          # Configuration
│   └── models/          # Data models
├── internal/            # Private packages
│   ├── cli/             # CLI commands and flags
│   └── platform/        # Platform-specific code
├── tests/               # Tests
├── config/              # Configuration examples
└── scripts/             # Build and utility scripts
```

## Performance

- Handles millions of files with <500MB memory usage
- 10x faster incremental sync compared to full copy
- Parallel file transfers for optimal throughput
- Stream-based processing for files larger than RAM

## License

[License information here]

## Contributing

[Contributing guidelines here]
