# Data Model: File Synchronization Utility

**Feature**: 001-file-sync-utility
**Date**: 2025-11-22
**Phase**: Phase 1 - Design

## Overview

This document defines the core data structures for the syncnorris file synchronization utility. These entities represent the domain model independent of implementation details, extracted from functional requirements in spec.md.

---

## Entity: SyncOperation

**Purpose**: Represents a synchronization task initiated by the user

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| OperationID | string (UUID) | Unique identifier for this sync operation | Required, UUID v4 format | N/A |
| SourcePath | string | Absolute path to source directory | Required, must exist | FR-001, FR-012-016 |
| DestPath | string | Absolute path to destination directory | Required, must be writable | FR-001, FR-012-016 |
| Direction | enum | Synchronization direction | OneWay or Bidirectional | FR-001, FR-002 |
| ComparisonMethod | enum | Method used to compare files | NameSize, Timestamp, Binary, or Hash | FR-006-009 |
| ConflictStrategy | enum | How to resolve conflicts (bidirectional only) | Ask, SourceWins, DestWins, Newer, Both | FR-004 |
| DryRun | boolean | If true, only report changes without modifying files | Default: false | FR-023 |
| StartTime | timestamp | When the operation started | Auto-set | SC-005 |
| EndTime | timestamp (nullable) | When the operation completed | Null if in progress | SC-005 |
| Status | enum | Current operation status | Pending, Running, Completed, Failed, Cancelled | FR-001-005 |
| TotalFiles | integer | Total number of files scanned | >= 0 | FR-017, FR-020 |
| ProcessedFiles | integer | Number of files processed so far | 0 <= value <= TotalFiles | FR-017, FR-020, SC-005 |
| CurrentFile | string (nullable) | Path of file currently being processed | Path or null | FR-021, SC-005 |

### Relationships

- Has many `FileEntry` records (files being tracked)
- Produces one `SyncReport` (summary of results)
- May have many `Conflict` records (if bidirectional sync)

### State Transitions

```
Pending → Running → Completed
           ↓
        Failed
           ↓
        Cancelled (user interruption)
```

**Business Rules**:
- Direction=OneWay → ConflictStrategy must be null
- Direction=Bidirectional → ConflictStrategy required
- Status=Running → CurrentFile must not be null
- Status=Completed or Failed → EndTime must not be null
- ProcessedFiles must never exceed TotalFiles

---

## Entity: FileEntry

**Purpose**: Represents a single file being tracked during a sync operation

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| EntryID | string (UUID) | Unique identifier for this file entry | Required, UUID v4 format | N/A |
| OperationID | string (UUID) | Parent sync operation | Required, foreign key | N/A |
| RelativePath | string | File path relative to source/dest root | Required, non-empty | FR-012-016 |
| Size | integer | File size in bytes | >= 0 | FR-006, FR-024 |
| ModTime | timestamp | Last modification time | Required | FR-007, FR-024 |
| Hash | string (nullable) | SHA-256 hash (if computed) | 64 hex chars or null | FR-009, FR-011 |
| State | enum | Current file state | New, Modified, Unchanged, Deleted, Error | FR-024 |
| Action | enum (nullable) | Action to perform | None, Copy, Update, Delete, Conflict | FR-001-003 |
| ErrorMessage | string (nullable) | Error details if State=Error | Max 500 chars | FR-026-030 |
| BytesTransferred | integer | Bytes copied (if Action=Copy or Update) | 0 <= value <= Size | SC-001, SC-005 |

### Relationships

- Belongs to one `SyncOperation`
- May have one `Conflict` record (if State=Conflict during bidirectional sync)

### State Values

| State | Meaning | Possible Actions |
|-------|---------|------------------|
| New | File exists in source but not destination | Copy |
| Modified | File exists in both but differs | Update |
| Unchanged | File identical in both locations | None |
| Deleted | File exists in destination but not source (oneway) | Delete (optional) |
| Error | File operation failed | None (logged) |

### Business Rules

- Hash is null unless ComparisonMethod=Hash
- State=Error → ErrorMessage must not be null
- Action=Copy or Update → BytesTransferred tracked during operation
- RelativePath must be normalized (use forward slashes internally, convert for OS display)

---

## Entity: SyncReport

**Purpose**: Summary of operation results

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| ReportID | string (UUID) | Unique identifier for this report | Required, UUID v4 format | N/A |
| OperationID | string (UUID) | Associated sync operation | Required, foreign key | N/A |
| FilesScanned | integer | Total files examined | >= 0 | FR-020, SC-004 |
| FilesCopied | integer | New files copied to destination | >= 0 | FR-001, FR-020 |
| FilesUpdated | integer | Existing files updated | >= 0 | FR-001, FR-020 |
| FilesDeleted | integer | Files removed from destination | >= 0 | FR-001 |
| FilesUnchanged | integer | Files skipped (identical) | >= 0 | FR-005 |
| FilesFailed | integer | Files that encountered errors | >= 0 | FR-026-030 |
| ConflictsDetected | integer | Conflicts found (bidirectional only) | >= 0 | FR-003 |
| TotalBytesTransferred | integer | Total data copied (bytes) | >= 0 | FR-017, FR-020, SC-001 |
| Duration | duration | Total operation time (seconds) | > 0 | FR-020, SC-001, SC-005 |
| AverageSpeed | float | Bytes per second | >= 0 | FR-017, FR-020, SC-005 |
| ErrorList | array of strings | Detailed error messages | Max 100 entries | FR-030 |

### Relationships

- Belongs to one `SyncOperation`

### Business Rules

- FilesScanned = FilesCopied + FilesUpdated + FilesDeleted + FilesUnchanged + FilesFailed
- ConflictsDetected > 0 only if Operation.Direction=Bidirectional
- AverageSpeed = TotalBytesTransferred / Duration (handle division by zero)
- ErrorList contains at most 100 most recent errors (truncate if more)

---

## Entity: ComparisonResult

**Purpose**: Outcome of comparing two folders (dry-run mode)

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| ComparisonID | string (UUID) | Unique identifier | Required, UUID v4 format | N/A |
| OperationID | string (UUID) | Parent sync operation | Required, foreign key | N/A |
| Additions | array of FileEntry | Files to be added | Non-null | FR-024 |
| Modifications | array of FileEntry | Files to be updated | Non-null | FR-024 |
| Deletions | array of FileEntry | Files to be removed | Non-null | FR-024 |
| Conflicts | array of Conflict | Files with conflicts (bidirectional) | Non-null | FR-003, FR-024 |
| TotalSize | integer | Total bytes that would be transferred | >= 0 | FR-025 |
| Unchanged | integer | Count of identical files (not listed) | >= 0 | FR-024 |

### Relationships

- Belongs to one `SyncOperation`
- References multiple `FileEntry` records
- May reference multiple `Conflict` records

### Business Rules

- Only populated when SyncOperation.DryRun = true
- Additions.length + Modifications.length + Deletions.length + Unchanged must equal TotalFiles
- Conflicts.length > 0 only if Direction=Bidirectional
- TotalSize = sum(Additions.Size) + sum(Modifications.Size)

---

## Entity: Conflict

**Purpose**: Represents a bidirectional sync conflict requiring resolution

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| ConflictID | string (UUID) | Unique identifier | Required, UUID v4 format | N/A |
| OperationID | string (UUID) | Parent sync operation | Required, foreign key | N/A |
| FilePath | string | Relative path of conflicting file | Required, non-empty | FR-003 |
| SourceState | enum | State of file at source | Exists, Modified, Deleted | FR-003 |
| DestState | enum | State of file at destination | Exists, Modified, Deleted | FR-003 |
| SourceHash | string (nullable) | SHA-256 of source file | 64 hex chars or null | FR-009 |
| DestHash | string (nullable) | SHA-256 of dest file | 64 hex chars or null | FR-009 |
| SourceModTime | timestamp (nullable) | Source modification time | Null if SourceState=Deleted | FR-007 |
| DestModTime | timestamp (nullable) | Dest modification time | Null if DestState=Deleted | FR-007 |
| Resolution | enum (nullable) | How conflict was resolved | KeepSource, KeepDest, KeepBoth, Skip, Pending | FR-004 |
| ResolvedAt | timestamp (nullable) | When resolution was applied | Null if Resolution=Pending | N/A |

### Relationships

- Belongs to one `SyncOperation`
- References one `FileEntry` (for the file in conflict)

### Conflict Types

| SourceState | DestState | Conflict Type | Example |
|-------------|-----------|---------------|---------|
| Modified | Modified | Divergent changes | Same file edited on both sides with different content |
| Deleted | Modified | Delete vs Modify | File removed from source but modified at dest |
| Modified | Deleted | Modify vs Delete | File modified at source but removed from dest |
| Deleted | Deleted | N/A | Not a conflict (both agree on deletion) |

### Business Rules

- SourceState=Deleted → SourceHash and SourceModTime must be null
- DestState=Deleted → DestHash and DestModTime must be null
- Resolution=Pending → ResolvedAt must be null
- Resolution!=Pending → ResolvedAt must not be null
- SourceState=Modified AND DestState=Modified → SourceHash != DestHash (otherwise not a conflict)

---

## Entity: Configuration

**Purpose**: User configuration loaded from YAML file or CLI flags

### Fields

| Field | Type | Description | Default | Source |
|-------|------|-------------|---------|--------|
| SyncMode | enum | Default sync direction | OneWay | FR-001-002 |
| ComparisonMethod | enum | Default comparison method | Hash | FR-006-009 |
| ConflictResolution | enum | Default conflict strategy | Ask | FR-004 |
| MaxWorkers | integer | Parallel operation limit | CPU count | FR-031 |
| BufferSize | integer | File I/O buffer size (bytes) | 65536 (64KB) | Research.md #5 |
| BandwidthLimit | integer | Max bytes/sec (0=unlimited) | 0 | FR-032 |
| OutputFormat | enum | Output mode | Human | FR-017-019 |
| ShowProgress | boolean | Display progress bars | true | FR-021 |
| QuietMode | boolean | Suppress non-error output | false | FR-022 |
| LoggingEnabled | boolean | Enable logging | true | FR-030 |
| LogFormat | enum | Log file format | JSON | User input |
| LogLevel | enum | Minimum log level | Info | FR-030 |
| LogFile | string (nullable) | Log file path | null (stderr) | FR-030 |
| FollowSymlinks | boolean | Follow symbolic links | false | Research.md #4 |
| ExcludePatterns | array of strings | Glob patterns to exclude | [] | Research.md #8 |

### Relationships

None (global configuration)

### Enum Definitions

```
SyncMode: OneWay | Bidirectional
ComparisonMethod: NameSize | Timestamp | Binary | Hash
ConflictResolution: Ask | SourceWins | DestWins | Newer | Both
OutputFormat: Human | JSON
LogFormat: JSON | Text | XML
LogLevel: Debug | Info | Warn | Error | Fatal
```

### Business Rules

- MaxWorkers must be > 0 (use runtime.NumCPU() if not specified)
- BufferSize must be > 0 and typically a power of 2
- BandwidthLimit >= 0 (0 means unlimited)
- QuietMode=true → ShowProgress must be false
- LogFile is validated for writability before operation starts

---

## Entity: SyncState (for Bidirectional Sync)

**Purpose**: Tracks last known good state for conflict detection

### Fields

| Field | Type | Description | Validation | Source |
|-------|------|-------------|------------|--------|
| FilePath | string | Relative path of tracked file | Required, non-empty, primary key | Research.md #7 |
| Hash | string | SHA-256 hash at last sync | Required, 64 hex chars | FR-009 |
| ModTime | timestamp | Modification time at last sync | Required | FR-007 |
| Size | integer | File size at last sync | >= 0 | FR-006 |
| LastSyncTime | timestamp | When this state was recorded | Required | N/A |
| Location | enum | Which side (Source or Dest) | Required | FR-002 |

### Relationships

None (stored separately for source and destination)

### Storage

- Persisted in `.syncnorris/state.json` in both source and destination directories
- JSON format for portability
- Updated after each successful sync
- Used during bidirectional sync to detect three-way conflicts

### Business Rules

- State file created on first sync
- State entries removed when file no longer exists on either side
- State comparison: if (current_hash != state.Hash) → file changed since last sync
- Conflict: if (source_changed AND dest_changed AND source_hash != dest_hash) → divergent modifications

---

## Data Flow Diagram

```
User Command
    ↓
Configuration (YAML + CLI flags)
    ↓
SyncOperation created
    ↓
Scan source and dest → FileEntry records
    ↓
Compare using ComparisonMethod → ComparisonResult
    ↓
Detect conflicts (if bidirectional) → Conflict records
    ↓
Resolve conflicts (if any) → update Resolution
    ↓
Execute sync (if not DryRun)
    ↓
Update SyncState (if bidirectional)
    ↓
Generate SyncReport
    ↓
Output (Human or JSON)
```

---

## Validation Rules Summary

### Cross-Entity Constraints

1. **SyncOperation + FileEntry**: ProcessedFiles must equal count of FileEntry records with Action != None
2. **SyncOperation + SyncReport**: Report.FilesScanned must equal Operation.TotalFiles
3. **SyncOperation + Conflict**: Conflicts only exist if Direction=Bidirectional
4. **ComparisonResult + SyncOperation**: ComparisonResult only exists if DryRun=true
5. **FileEntry + Conflict**: If FileEntry.State=Error, no Conflict record should exist

### Data Integrity

- All UUID fields must be valid UUID v4 format
- All timestamp fields must be in UTC
- All file paths must be normalized (forward slashes internally)
- All hash values must be lowercase hexadecimal SHA-256 (64 characters)
- All size/count fields must be non-negative

---

## Storage Considerations

**In-Memory** (during operation):
- SyncOperation (current operation state)
- FileEntry records (working set)
- Conflict records (if applicable)
- Progress tracking

**Persistent** (between operations):
- Configuration (YAML file)
- SyncState (JSON file in `.syncnorris/`)
- Logs (JSON/Text/XML files if enabled)

**Output** (after operation):
- SyncReport (JSON if OutputFormat=JSON)
- ComparisonResult (if DryRun=true)

---

## Performance Considerations

### Memory Efficiency

- **FileEntry streaming**: For 1M+ files, process in batches of 10K to stay under 500MB limit (SC-009)
- **Hash caching**: Store computed hashes in map[string]string during comparison phase
- **Conflict buffering**: Keep conflicts in memory (typically << 1% of total files)

### Concurrency

- **FileEntry processing**: Parallelize comparison operations (read-only, thread-safe)
- **SyncReport updates**: Use atomic counters for FilesScanned, FilesCopied, etc.
- **Progress updates**: Throttle UI updates to max 1/second (SC-005)

---

## JSON Schema Examples

### SyncReport (JSON Output)

```json
{
  "operation_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "summary": {
    "files_scanned": 10000,
    "files_copied": 150,
    "files_updated": 45,
    "files_deleted": 0,
    "files_unchanged": 9800,
    "files_failed": 5,
    "conflicts_detected": 0
  },
  "transfer": {
    "total_bytes": 524288000,
    "duration_seconds": 185,
    "average_speed_mbps": 22.7
  },
  "errors": [
    "/path/to/file1.txt: permission denied",
    "/path/to/file2.dat: disk full"
  ],
  "timestamp": "2025-11-22T10:35:00Z"
}
```

### ComparisonResult (Dry-Run Output)

```json
{
  "operation_id": "550e8400-e29b-41d4-a716-446655440001",
  "dry_run": true,
  "changes": {
    "additions": [
      {"path": "docs/new-file.md", "size": 4096}
    ],
    "modifications": [
      {"path": "src/main.go", "size": 8192, "reason": "content differs (hash)"}
    ],
    "deletions": [
      {"path": "old-file.txt", "size": 2048}
    ],
    "conflicts": []
  },
  "summary": {
    "total_changes": 3,
    "total_size": 14336,
    "unchanged": 9997
  }
}
```

---

## Conclusion

This data model provides a complete representation of all entities required for the syncnorris file synchronization utility, extracted from the functional requirements and informed by research findings. All entities are designed to be:

- **Technology-agnostic**: No implementation details (database, ORM, etc.)
- **Testable**: Clear validation rules and business logic
- **Extensible**: Can accommodate future features (e.g., additional comparison methods, storage backends)
- **Performant**: Designed for large-scale operations (1M+ files, <500MB memory)

Next phase: Define CLI contracts and API surface in `/contracts/`.
