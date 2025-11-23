# CLI Contract: syncnorris

**Feature**: 001-file-sync-utility
**Date**: 2025-11-22
**Phase**: Phase 1 - Design

## Overview

This document defines the command-line interface contract for the syncnorris file synchronization utility. All commands, flags, and output formats are derived from functional requirements in spec.md.

---

## Global Flags

Available for all commands:

| Flag | Short | Type | Default | Description | Source |
|------|-------|------|---------|-------------|--------|
| `--config` | `-c` | string | (auto-detected) | Path to YAML config file | Research.md #8 |
| `--output` | `-o` | enum | human | Output format: human, json | FR-017-019 |
| `--log-format` | | enum | json | Log format: json, text, xml | User input |
| `--log-level` | | enum | info | Log level: debug, info, warn, error | FR-030 |
| `--log-file` | | string | (stderr) | Log file path | FR-030 |
| `--quiet` | `-q` | boolean | false | Suppress progress output | FR-022 |
| `--verbose` | `-v` | boolean | false | Increase output verbosity | FR-030 |
| `--help` | `-h` | boolean | false | Show help message | Cobra default |
| `--version` | | boolean | false | Show version information | Cobra default |

---

## Commands

### 1. `syncnorris sync`

**Purpose**: Synchronize two folders (one-way or bidirectional)

**Usage**:
```bash
syncnorris sync --source <path> --dest <path> [flags]
```

**Required Flags**:

| Flag | Short | Type | Description | Source |
|------|-------|------|-------------|--------|
| `--source` | `-s` | string | Source directory path | FR-001, FR-012-016 |
| `--dest` | `-d` | string | Destination directory path | FR-001, FR-012-016 |

**Optional Flags**:

| Flag | Short | Type | Default | Description | Source |
|------|-------|------|---------|-------------|--------|
| `--mode` | `-m` | enum | oneway | Sync mode: oneway, bidirectional | FR-001-002 |
| `--comparison` | | enum | hash | Comparison method: namesize, timestamp, binary, hash | FR-006-009 |
| `--conflict` | | enum | ask | Conflict resolution: ask, source-wins, dest-wins, newer, both | FR-004 |
| `--dry-run` | | boolean | false | Compare only, don't sync | FR-023 |
| `--delete` | | boolean | false | Delete files from dest not in source (oneway only) | FR-001 |
| `--resume` | | boolean | false | Resume interrupted sync | FR-029 |
| `--parallel` | `-p` | integer | (CPU count) | Number of parallel workers | FR-031 |
| `--bandwidth` | `-b` | string | unlimited | Bandwidth limit (e.g., "10M", "1G") | FR-032 |
| `--exclude` | | string[] | [] | Glob patterns to exclude | Research.md #8 |

**Examples**:

```bash
# One-way sync with hash comparison
syncnorris sync --source /data/project --dest /backup/project

# Bidirectional sync with conflict detection
syncnorris sync -s /home/user/docs -d /mnt/nas/docs --mode bidirectional --conflict newer

# Dry-run to preview changes
syncnorris sync -s /src -d /dst --dry-run

# Sync with bandwidth limit and exclusions
syncnorris sync -s /data -d /backup --bandwidth 50M --exclude "*.tmp" --exclude ".git/"

# JSON output for automation
syncnorris sync -s /src -d /dst --output json
```

**Exit Codes**:
- `0`: Success, all files synchronized
- `1`: Partial success, some files failed (check errors in output)
- `2`: Operation failed (invalid arguments, source/dest inaccessible, etc.)
- `3`: User cancelled (Ctrl+C or conflict resolution aborted)

**Output (Human Mode)**:

```
Synchronizing: /data/project → /backup/project
Mode: One-way | Comparison: Hash | Workers: 8

Scanning directories...
Source: 10,000 files (5.1 GB)
Dest:   9,850 files (4.9 GB)

Changes detected:
  New files:      150 (215 MB)
  Modified files:  45 (180 MB)
  Unchanged:    9,800

Overall: [=========>          ] 45% | 4500/10000 files | 2.3 GB/5.1 GB | 5.2 MB/s | ETA: 2m15s
Current: [===================>] 95% | transferring: /path/to/large/file.zip

Sync complete!
Duration: 4m32s
Files copied: 150 | Files updated: 45 | Errors: 0
Total transferred: 395 MB (avg: 1.45 MB/s)
```

**Output (JSON Mode)**:

```json
{
  "operation": "sync",
  "operation_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "source": "/data/project",
  "dest": "/backup/project",
  "mode": "oneway",
  "comparison": "hash",
  "dry_run": false,
  "started_at": "2025-11-22T10:30:00Z",
  "ended_at": "2025-11-22T10:34:32Z",
  "summary": {
    "files_scanned": 10000,
    "files_copied": 150,
    "files_updated": 45,
    "files_deleted": 0,
    "files_unchanged": 9800,
    "files_failed": 0,
    "conflicts_detected": 0
  },
  "transfer": {
    "total_bytes": 414253056,
    "duration_seconds": 272,
    "average_speed_bps": 1522869
  },
  "errors": []
}
```

---

### 2. `syncnorris compare`

**Purpose**: Compare two folders without synchronizing

**Usage**:
```bash
syncnorris compare --source <path> --dest <path> [flags]
```

**Required Flags**:

| Flag | Short | Type | Description | Source |
|------|-------|------|-------------|--------|
| `--source` | `-s` | string | Source directory path | FR-023 |
| `--dest` | `-d` | string | Destination directory path | FR-023 |

**Optional Flags**:

| Flag | Short | Type | Default | Description | Source |
|------|-------|------|---------|-------------|--------|
| `--comparison` | | enum | hash | Comparison method: namesize, timestamp, binary, hash | FR-006-009 |
| `--detailed` | | boolean | false | Show file-by-file breakdown | FR-024 |

**Examples**:

```bash
# Quick comparison by name and size
syncnorris compare -s /src -d /dst --comparison namesize

# Detailed hash-based comparison
syncnorris compare -s /src -d /dst --comparison hash --detailed

# JSON output for scripting
syncnorris compare -s /src -d /dst --output json
```

**Exit Codes**:
- `0`: Folders are identical
- `1`: Folders differ
- `2`: Comparison failed (invalid arguments, paths inaccessible, etc.)

**Output (Human Mode)**:

```
Comparing: /src vs /dst
Method: SHA-256 hash

Scanning directories...
Source: 1,000 files (2.5 GB)
Dest:   1,050 files (2.6 GB)

Differences detected:
  New files (in source):     15 (25 MB)
  Modified files:             8 (12 MB)
  Deleted (missing in src):  65 (110 MB)

Total changes: 88 files (147 MB would be transferred)

Folders are NOT identical.
```

**Output (JSON Mode)**:

```json
{
  "operation": "compare",
  "source": "/src",
  "dest": "/dst",
  "comparison": "hash",
  "identical": false,
  "changes": {
    "additions": [
      {"path": "docs/new-file.md", "size": 4096},
      {"path": "src/feature.go", "size": 12288}
    ],
    "modifications": [
      {"path": "config.yaml", "size": 1024, "reason": "hash differs"}
    ],
    "deletions": [
      {"path": "old-file.txt", "size": 2048}
    ]
  },
  "summary": {
    "additions": 15,
    "modifications": 8,
    "deletions": 65,
    "unchanged": 912,
    "total_size_to_transfer": 154140672
  }
}
```

---

### 3. `syncnorris config`

**Purpose**: Manage configuration

**Subcommands**:
- `config show`: Display current configuration
- `config init`: Create default config file
- `config validate`: Validate config file

**Usage**:

```bash
# Show current configuration
syncnorris config show

# Create default config at ~/.config/syncnorris/config.yaml
syncnorris config init

# Validate specific config file
syncnorris config validate --config /path/to/config.yaml
```

**Output (config show)**:

```yaml
# syncnorris configuration

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
  file: /var/log/syncnorris.log

exclude:
  - "*.tmp"
  - ".git/"
  - "node_modules/"
```

---

### 4. `syncnorris version`

**Purpose**: Show version information

**Usage**:
```bash
syncnorris version
```

**Output**:

```
syncnorris version 1.0.0
Built: 2025-11-22T10:00:00Z
Go version: go1.21.5
Platform: linux/amd64
```

**JSON Output**:

```json
{
  "version": "1.0.0",
  "built_at": "2025-11-22T10:00:00Z",
  "go_version": "go1.21.5",
  "platform": "linux/amd64",
  "commit": "abc1234"
}
```

---

### 5. `syncnorris daemon`

**Purpose**: Run syncnorris in background/daemon mode

**Usage**:
```bash
syncnorris daemon --config /etc/syncnorris/config.yaml [flags]
```

**Flags**:

| Flag | Type | Default | Description | Source |
|------|------|---------|-------------|--------|
| `--config` | string | (required) | Path to YAML config file | User input |
| `--pid-file` | string | /var/run/syncnorris.pid | PID file path | Research.md #9 |
| `--interval` | duration | 1h | Sync interval (e.g., "5m", "1h", "24h") | User input |

**Examples**:

```bash
# Run as daemon, sync every hour
syncnorris daemon --config /etc/syncnorris/config.yaml --interval 1h

# Custom PID file and interval
syncnorris daemon --config /etc/syncnorris/config.yaml --interval 30m --pid-file /tmp/syncnorris.pid
```

**Behavior**:
- Detaches from terminal (Unix: double-fork, Windows: background process)
- Writes PID to specified file
- Runs sync operation every `--interval` duration
- Logs to file specified in config (not stdout/stderr)
- Handles SIGTERM for graceful shutdown
- Exits if config file becomes invalid or inaccessible

**Signal Handling**:
- `SIGTERM`, `SIGINT`: Graceful shutdown (finish current sync, then exit)
- `SIGHUP`: Reload configuration file
- `SIGUSR1`: Trigger immediate sync (bypass interval)

---

## Environment Variables

| Variable | Description | Default | Source |
|----------|-------------|---------|--------|
| `SYNCNORRIS_CONFIG` | Path to config file | (search order) | Research.md #8 |
| `SYNCNORRIS_LOG_LEVEL` | Override log level | (from config) | FR-030 |
| `SYNCNORRIS_OUTPUT` | Override output format | (from config) | FR-017-019 |
| `SYNCNORRIS_MAX_WORKERS` | Override parallel workers | (CPU count) | FR-031 |

**Config file search order** (if `--config` not specified):
1. `./syncnorris.yaml`
2. `~/.config/syncnorris/config.yaml`
3. `/etc/syncnorris/config.yaml` (Linux/macOS)
4. `C:\ProgramData\syncnorris\config.yaml` (Windows)

---

## Progress Output Format (Human Mode)

### Real-time Progress Bar

```
Overall: [=========>          ] 45% | 4500/10000 files | 2.3 GB/5.1 GB | 5.2 MB/s | ETA: 2m15s
Current: [===================>] 95% | transferring: /path/to/large/file.zip
```

**Components**:
- Overall progress: Percentage, file count, data transferred, transfer speed, ETA
- Current file: Individual file progress, current action, file path

**Update Frequency**: At least 1 Hz (FR-021, SC-005)

### Streaming JSON Progress (JSON Mode)

When `--output json`, emit newline-delimited JSON events:

```json
{"event":"start","operation_id":"550e8400...","source":"/src","dest":"/dst","total_files":10000}
{"event":"progress","files_processed":1000,"bytes_transferred":104857600,"current_file":"/path/to/file.txt"}
{"event":"progress","files_processed":2000,"bytes_transferred":209715200,"current_file":"/path/to/another.bin"}
{"event":"error","file":"/path/to/error.txt","error":"permission denied"}
{"event":"complete","summary":{...}}
```

---

## Error Handling

### Error Output (Human Mode)

```
Error: Failed to synchronize
  - /path/to/file1.txt: permission denied
  - /path/to/file2.dat: disk full (need 500 MB, have 100 MB available)
  - /network/share/file3.bin: network unreachable

3 files failed. See log for details: /var/log/syncnorris.log
```

### Error Output (JSON Mode)

```json
{
  "status": "partial_failure",
  "errors": [
    {
      "file": "/path/to/file1.txt",
      "error": "permission denied",
      "code": "EPERM"
    },
    {
      "file": "/path/to/file2.dat",
      "error": "disk full (need 500 MB, have 100 MB available)",
      "code": "ENOSPC"
    },
    {
      "file": "/network/share/file3.bin",
      "error": "network unreachable",
      "code": "ENETUNREACH"
    }
  ],
  "errors_count": 3
}
```

**Error Codes** (Unix errno + custom codes):
- `EPERM`: Permission denied
- `ENOENT`: No such file or directory
- `ENOSPC`: No space left on device
- `ENETUNREACH`: Network is unreachable
- `ETIMEDOUT`: Connection timed out
- `CONFLICT`: Bidirectional sync conflict detected
- `HASH_MISMATCH`: Post-transfer verification failed

---

## Validation Rules

### Path Validation

- Must be absolute paths (relative paths rejected with error)
- Source must exist and be readable
- Dest must be writable (created if missing, requires parent directory to exist)
- Cannot be identical (source == dest)
- Cannot be nested (source inside dest or vice versa)

### Flag Validation

- `--mode bidirectional` requires `--conflict` to be specified
- `--delete` only valid with `--mode oneway` (rejected for bidirectional)
- `--resume` requires existing `.syncnorris/state.json` (otherwise ignored with warning)
- `--bandwidth` format: `<number><unit>` where unit = K, M, G (e.g., "10M", "1G")
- `--parallel` must be > 0 and <= 128 (capped to prevent resource exhaustion)

### Conflict Validation

- `--conflict=ask` requires interactive terminal (TTY detected, otherwise error)
- `--conflict=ask` incompatible with `--output json` (use different strategy for automation)

---

## Backward Compatibility

### Version Guarantees

- **Major version changes (1.x → 2.x)**: CLI contract may change (flags renamed, removed, behavior changed)
- **Minor version changes (1.0 → 1.1)**: Additive only (new flags, new commands, but existing behavior preserved)
- **Patch version changes (1.0.0 → 1.0.1)**: No CLI contract changes (bug fixes only)

### Deprecated Flags

If flags are deprecated in future versions:
- Old flag continues to work with deprecation warning
- Minimum 2 minor versions before removal
- Example: `--output-format` deprecated in favor of `--output` (1.1.0), removed in 1.3.0

---

## Testing Contract

### Manual Testing

```bash
# Sanity test: one-way sync
syncnorris sync -s ./test/source -d ./test/dest

# Expected: All files from source copied to dest, exit code 0

# Test: dry-run mode
syncnorris sync -s ./test/source -d ./test/dest --dry-run

# Expected: Output shows changes but no files modified

# Test: JSON output
syncnorris sync -s ./test/source -d ./test/dest --output json | jq .

# Expected: Valid JSON, parseable by jq

# Test: invalid source
syncnorris sync -s /nonexistent -d /tmp/dest

# Expected: Error message, exit code 2
```

### Automated Testing

Contract tests will verify:
- All flags are recognized (no "unknown flag" errors for documented flags)
- Help output (`--help`) includes all documented flags
- JSON output is valid JSON and matches schema
- Exit codes match documented values
- Error messages are actionable (include file path, error reason, suggested fix)

---

## Summary

This CLI contract defines:
- **5 primary commands**: sync, compare, config, version, daemon
- **20+ flags**: Global and command-specific
- **2 output formats**: Human-readable and JSON
- **5 exit codes**: Success, partial failure, failure, user cancel, invalid args
- **Cross-platform**: Works identically on Linux, Windows, macOS (path format differences handled internally)

All contracts are testable, documented, and aligned with constitutional requirements for usability, automation, and extensibility.
