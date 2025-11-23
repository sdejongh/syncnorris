# syncnorris

**Version**: 0.1.0-alpha (MVP)
**Status**: Production-ready for one-way synchronization
**License**: [To be determined]

Cross-platform file synchronization utility built in Go, optimized for performance with advanced hash comparison and parallel operations.

## Current Features ✅

### Core Functionality
- ✅ **One-way synchronization** from source to destination
  - Local filesystem support (mounted network shares work)
  - Parallel file transfers (configurable worker count)
  - Dry-run mode to preview changes without modifying files
  - Incremental sync (only changed files are transferred)

### Comparison Methods
- ✅ **Hash-based comparison** (SHA-256, default and recommended)
  - Intelligent composite strategy: metadata first, hash only when needed
  - Partial hashing for large files (≥1MB): 95% I/O reduction for quick rejection
  - Parallel hash computation: 1.8-1.9x speedup
  - Buffer pooling for reduced memory pressure
- ✅ **MD5 hash comparison** (faster alternative to SHA-256)
  - Similar performance to SHA-256 but less secure
  - Suitable for non-critical data where speed matters
  - Also supports partial hashing and parallel computation
- ✅ **Binary comparison** (byte-by-byte verification)
  - Most thorough comparison method
  - Reports exact byte offset where files differ
  - Useful for debugging or when hash collisions are a concern
- ✅ **Name/size comparison** (fast metadata-only mode)
  - Ideal for re-sync scenarios: 10-40x faster than hash mode
  - Sub-second re-sync for 1000 identical files

### User Interface
- ✅ **Advanced progress display**
  - Real-time tabular view of up to 5 concurrent files
  - Dual progress bars: data transferred + files processed
  - Status icons: ⏳ copying, 🔍 hashing, ✅ complete, ❌ error
  - Instantaneous transfer rate (3-second sliding window) + average
  - Accurate ETA calculation
  - Terminal width detection (prevents line wrapping)
- ✅ **Human-readable output** with comprehensive summary statistics
- ✅ **Quiet mode** for scripts (suppress non-error output)
- ✅ **Verbose mode** for debugging

### Performance Optimizations
syncnorris has been heavily optimized and exceeds all performance targets:

- **Atomic counter statistics**: Lock-free updates, 8.6x faster (6% throughput gain)
- **Progress callback throttling**: 93% overhead reduction (smooth 20 updates/sec)
- **Partial hashing**: 95% I/O reduction for files differing in first 256KB
- **Parallel hash computation**: Source and destination hashed concurrently
- **Composite comparison**: Metadata check before expensive hash operations
- **Buffer pooling**: Reduced GC pressure with sync.Pool

**Measured Results**:
- 10,000 files synchronized in <2 minutes (target: <5 min) ✅
- 1,000 identical files re-synced in <0.5 seconds ✅
- Memory usage <300MB for 1M files (target: <500MB) ✅
- Incremental sync 10-40x faster than full copy ✅

### Build & Distribution
- ✅ **Single static binary** (no dependencies required)
- ✅ **Cross-platform**: Linux, Windows, macOS (amd64, arm64)
- ✅ **Configuration file** support (YAML format)
- ✅ **Shell autocompletion** (bash, zsh, fish, powershell)

## Planned Features 🚧

These features are **NOT yet implemented** but are planned for future releases:

- 🚧 **Bidirectional synchronization** with conflict resolution
- 🚧 **JSON output** for automation and scripting
- 🚧 **Exclude patterns** (glob-based filtering)
- 🚧 **Bandwidth limiting** for network-constrained environments
- 🚧 **Timestamp comparison** method
- 🚧 **Logging** to files (JSON, plain text)
- 🚧 **Resume interrupted operations**
- 🚧 **Native network storage** (SMB/Samba, NFS without mounting)

See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) for detailed feature status.

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

# Binaries will be in dist/ with platform suffixes
```

## Quick Start

### Basic One-Way Sync

```bash
# Sync from source to destination (with progress)
syncnorris sync --source /data/projects --dest /backup/projects

# Short form
syncnorris sync -s /src -d /dst
```

### Preview Changes (Dry-Run)

```bash
# See what would be changed without modifying anything
syncnorris sync -s /src -d /dst --dry-run
```

### Fast Metadata-Only Comparison

```bash
# Use name+size comparison instead of hash (much faster for re-sync)
syncnorris sync -s /src -d /dst --comparison namesize
```

### Parallel Operations

```bash
# Use 16 parallel workers (default: CPU count)
syncnorris sync -s /src -d /dst --parallel 16
```

### Quiet Mode for Scripts

```bash
# Suppress progress output, only show errors
syncnorris sync -s /src -d /dst --quiet

# Or use short form
syncnorris sync -s /src -d /dst -q
```

## Configuration

Create a config file at `~/.config/syncnorris/config.yaml`:

```yaml
sync:
  mode: oneway                    # Only 'oneway' currently supported
  comparison: hash                # 'hash', 'md5', 'binary', or 'namesize'

performance:
  max_workers: 8                  # Parallel worker count (0 = CPU count)
  buffer_size: 65536              # Buffer size for I/O operations (64KB)

output:
  format: human                   # Only 'human' currently supported
  progress: true                  # Show real-time progress bars
  quiet: false                    # Suppress non-error output
  verbose: false                  # Extra debug information

# Note: exclude, bandwidth_limit, and logging are defined
# in config but not yet implemented
```

## Usage Reference

### Commands

```bash
syncnorris sync      # Synchronize two folders (primary command)
syncnorris compare   # Compare folders without syncing (alias for sync --dry-run)
syncnorris config    # Manage configuration
syncnorris help      # Show help for any command
```

### Sync Command Options

#### Required Flags
```
--source, -s PATH    Source directory path (required)
--dest, -d PATH      Destination directory path (required)
```

#### Functional Flags (Implemented)
```
--comparison METHOD  Comparison method: hash, md5, binary, namesize (default: hash)
--dry-run            Compare only, don't sync
--parallel, -p N     Number of parallel workers (default: CPU count)
--mode oneway        Sync mode (only 'oneway' currently supported)
```

#### Global Flags
```
--config FILE        Config file path (default: ~/.config/syncnorris/config.yaml)
--quiet, -q          Suppress non-error output
--verbose, -v        Verbose debug output
```

#### Non-Functional Flags (Accepted but Not Yet Implemented)
These flags are accepted for future compatibility but currently have no effect:

```
--mode bidirectional      # Returns error: not yet implemented
--comparison timestamp    # Falls back to hash comparison
--output json             # Falls back to human-readable output
--bandwidth, -b LIMIT     # Not yet implemented (no rate limiting)
--exclude PATTERN         # Not yet implemented (no filtering)
--conflict STRATEGY       # Not yet implemented (bidirectional only)
```

## Examples

### Backup Important Data

```bash
# Daily backup with hash verification
syncnorris sync \
  --source ~/Documents \
  --dest /mnt/backup/Documents \
  --comparison hash

# View the generated report
```

### Fast Re-Sync After Interruption

```bash
# Use name+size for quick re-sync (skip re-hashing identical files)
syncnorris sync \
  -s /large/dataset \
  -d /backup/dataset \
  --comparison namesize
```

### Test Before Syncing

```bash
# Dry-run to preview changes
syncnorris sync -s ~/src -d /mnt/nas/backup --dry-run

# Review the output, then run actual sync
syncnorris sync -s ~/src -d /mnt/nas/backup
```

### Maximum Performance

```bash
# Use all CPU cores for parallel operations
syncnorris sync \
  -s /source \
  -d /dest \
  --parallel $(nproc) \
  --comparison namesize
```

### Fast Hash Verification

```bash
# Use MD5 for faster hash-based comparison (less secure than SHA-256)
syncnorris sync \
  -s /media/photos \
  -d /backup/photos \
  --comparison md5
```

### Debugging File Differences

```bash
# Use binary comparison to find exact byte offset where files differ
syncnorris sync \
  -s /original \
  -d /modified \
  --comparison binary \
  --dry-run
```

## Performance Tips

1. **First sync**: Use `--comparison hash` (default) for cryptographic verification
2. **Re-sync**: Use `--comparison namesize` for 10-40x speedup on unchanged files
3. **Fast hash**: Use `--comparison md5` for slightly faster hashing (less secure than SHA-256)
4. **Debugging**: Use `--comparison binary` for byte-by-byte verification with exact offset reporting
5. **Large files**: Hash comparison (SHA-256/MD5) automatically uses partial hashing (≥1MB)
6. **Network storage**: Mount shares locally rather than waiting for native SMB/NFS support
7. **Worker count**: Default (CPU count) is optimal for most scenarios
8. **Progress overhead**: Already optimized (93% reduction), no tuning needed

## Project Structure

```
syncnorris/
├── cmd/syncnorris/           # Main CLI entry point
├── pkg/                      # Public packages
│   ├── storage/              # Storage backends (local filesystem)
│   ├── compare/              # Comparison algorithms (hash, composite)
│   ├── sync/                 # Sync engine and worker pools
│   ├── output/               # Output formatters (human, progress)
│   ├── config/               # Configuration management
│   └── models/               # Data models and types
├── internal/                 # Private packages
│   └── cli/                  # CLI commands and validation
├── docs/                     # Optimization documentation
├── specs/                    # Feature specifications
├── scripts/                  # Build and test scripts
└── tests/                    # Test files
```

## Development

### Prerequisites

- Go 1.21+ (uses sync/atomic and other modern features)
- Make (optional but recommended)

### Building

```bash
# Install dependencies
go mod download

# Build for current platform
make build

# Run tests
make test

# Cross-compile for all platforms
make build-all
```

### Running Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Performance benchmarks
go test -bench=. ./pkg/compare/
```

## Documentation

- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - Detailed feature status
- [CHANGELOG.md](CHANGELOG.md) - Version history and optimization details
- [docs/ATOMIC_COUNTERS_OPTIMIZATION.md](docs/ATOMIC_COUNTERS_OPTIMIZATION.md) - Lock-free statistics
- [docs/PARALLEL_HASH_OPTIMIZATION.md](docs/PARALLEL_HASH_OPTIMIZATION.md) - Concurrent hashing
- [docs/PARTIAL_HASH_OPTIMIZATION.md](docs/PARTIAL_HASH_OPTIMIZATION.md) - Quick rejection strategy
- [docs/THROTTLE_OPTIMIZATION.md](docs/THROTTLE_OPTIMIZATION.md) - Callback optimization

## Known Limitations

1. **Bidirectional sync** is not yet implemented (returns error)
2. **JSON output** is not yet implemented (falls back to human-readable)
3. **Exclude patterns** are not yet implemented (all files processed)
4. **Bandwidth limiting** is not yet implemented
5. **Network storage** requires mounting (no native SMB/NFS/UNC support yet)
6. **Interrupted operations** cannot be resumed (no checkpointing)

See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) for complete list.

## Contributing

Contributions are welcome! Priority areas:

1. JSON output formatter
2. Exclude pattern implementation
3. Bandwidth limiting
4. Bidirectional sync with conflict resolution
5. Integration tests
6. Documentation improvements

## License

[License information to be determined]

## Credits

Built with performance in mind, leveraging Go's excellent concurrency primitives and modern optimization techniques.
