<!--
Sync Impact Report:
- Version change: 1.0.0 → 1.1.0
- Modified principles:
  * Principle III (Dual Output Interface): Enhanced with specific UI/UX requirements
  * Principle VI (Performance & Scalability): Added concrete optimization strategies
- Added sections:
  * Performance Implementation Details
  * User Experience Requirements
- Removed sections: N/A
- Templates requiring updates:
  ✅ plan-template.md (reviewed - compatible)
  ✅ spec-template.md (reviewed - compatible)
  ✅ tasks-template.md (reviewed - compatible)
- Follow-up TODOs: Update feature plans to incorporate new performance optimizations
- Key Changes Summary:
  * Composite comparison strategy (metadata-first, hash-on-demand)
  * Buffer pooling for memory efficiency
  * Parallel comparison operations
  * Enhanced progress reporting with instantaneous metrics
  * Metadata preservation for accurate incremental syncs
-->

# syncnorris Constitution

## Core Principles

### I. Cross-Platform File Synchronization

syncnorris MUST support synchronization between diverse storage types including:
- Local filesystems (Linux, Windows, macOS)
- Network shares (SMB/Samba, NFS)
- UNC paths (Windows network paths)
- Future storage backends through extensible architecture

**Rationale**: The tool's core value proposition is universal file synchronization across platforms and storage types, eliminating the need for platform-specific tools.

### II. Data Integrity & Verification

All file operations MUST ensure data integrity through:
- Binary comparison or cryptographic hashing for source/destination comparison
- Post-transfer validation of copied files
- Checksum verification to detect corruption
- Clear reporting of verification failures

**Rationale**: File synchronization tools must guarantee data correctness. Silent corruption or incomplete transfers are unacceptable in production environments.

### III. Dual Output Interface

Every operation MUST support two output modes:
- Human-readable format: Clear, visually appealing CLI output for interactive use with:
  - Real-time progress display showing active file operations in tabular format
  - Multiple progress bars (data transferred, files processed)
  - Instantaneous transfer rate (sliding window) and average rate
  - Visual status indicators (icons for copying, hashing, complete, error)
  - Alphabetically sorted file lists for stable display
- JSON format: Structured machine-parseable output for automation/scripting
- Mode selection via command-line flag (e.g., `--output=json` or `--output=human`)
- Detailed reports distinguishing files from directories in statistics

**Rationale**: Professional tools serve both human operators and automated systems. Modern CLI tools should provide rich, informative real-time feedback that respects the user's attention. Progress information should be actionable (instantaneous rates for current performance, ETA for planning) and stable (sorted lists prevent visual chaos).

### IV. Single Binary Distribution

The application MUST be distributed as:
- A single statically-linked executable binary
- No external runtime dependencies required
- Native binaries for Linux, Windows, and macOS
- Built using Go's cross-compilation capabilities

**Rationale**: Simplicity of deployment is critical. Users should download one file and run it without installing dependencies, runtimes, or frameworks.

### V. Extensible Architecture

The codebase MUST be designed for future extensibility:
- Clear separation of concerns (storage backends, comparison engines, output formatters)
- Well-defined interfaces for pluggable components
- Storage backend abstraction allowing new protocols without core changes
- Plugin or modular architecture where appropriate

**Rationale**: Initial implementation covers core use cases, but the tool must evolve. Architecture must accommodate new storage types, comparison algorithms, and features without major refactoring.

### VI. Performance & Scalability

The synchronization engine MUST be optimized for:
- Efficient handling of large file sets (millions of files)
- Parallel file operations where safe and beneficial (comparisons AND transfers)
- Minimal memory footprint for large directory trees
- Incremental synchronization (only changed files)
- Intelligent comparison strategy: quick metadata checks first, expensive hash calculations only when needed
- Buffer pooling to reduce memory allocations and GC pressure
- Metadata preservation (timestamps, permissions) to enable accurate incremental syncs
- Benchmarking against industry standards (rsync, rclone)

**Rationale**: Synchronization tools operate on large datasets. Poor performance makes the tool unusable for real-world scenarios. We respect the established performance standards of rsync and rclone. Smart optimizations like composite comparison (size check before hash) and buffer reuse can provide 10-40x speedups for re-sync scenarios.

## Cross-Platform Requirements

### Platform Support

syncnorris MUST provide native support for:
- **Linux**: Primary development and testing platform
- **Windows**: Full UNC path support, Windows-specific file attributes
- **macOS**: APFS and HFS+ filesystem compatibility

### Storage Protocol Support

Initial release MUST support:
- Local filesystem access (all platforms)
- SMB/Samba mounts
- NFS mounts
- UNC paths (Windows)

Future extensibility MUST allow for:
- Cloud storage providers (S3, Azure Blob, Google Cloud Storage)
- SFTP/SCP
- WebDAV
- Custom protocol plugins

### Build & Distribution

- Cross-compilation for all platforms using Go toolchain
- Automated CI/CD builds for each platform
- Signed binaries for security verification
- Static linking (CGO_ENABLED=0 for pure Go builds)

## Development Workflow

### Code Organization

- **Storage Backends**: Abstracted interfaces in `pkg/storage/`
- **Comparison Engine**: Hash/binary comparison logic in `pkg/compare/`
- **Output Formatters**: Human and JSON formatters in `pkg/output/`
- **CLI Interface**: Cobra/urfave/cli framework in `cmd/`
- **Core Sync Engine**: Orchestration logic in `pkg/sync/`

### Testing Requirements

- Unit tests for all core components
- Integration tests for each storage backend
- Cross-platform testing via CI runners (Linux, Windows, macOS)
- Performance regression tests benchmarking against rsync/rclone
- Contract tests for JSON output schema stability

### Quality Gates

All code MUST:
- Pass `go vet` and `golangci-lint` checks
- Maintain test coverage above 80% for core sync logic
- Include godoc comments for exported functions
- Follow Go standard project layout
- Build cleanly with no warnings on all platforms

## Governance

This constitution defines the architectural and operational principles for syncnorris. All design decisions, code reviews, and feature implementations MUST comply with these principles.

### Amendment Process

1. Proposed changes documented with rationale
2. Impact analysis on existing code and users
3. Migration plan for breaking changes
4. Version increment per semantic versioning

### Compliance Review

- All pull requests MUST include constitutional compliance verification
- Design documents MUST reference applicable principles
- Any deviation from principles MUST be explicitly justified in the PR description
- Complexity additions require approval with documented rationale

### Version Control

This constitution follows semantic versioning:
- **MAJOR**: Backward-incompatible principle changes or removals
- **MINOR**: New principles or material expansions
- **PATCH**: Clarifications, wording improvements, typo fixes

## Performance Implementation Details

### Comparison Strategy

The comparison engine MUST implement a multi-stage approach:

1. **Stage 1 - Metadata Comparison**: Always check filename and size first (O(1) operations)
2. **Stage 2 - Conditional Hash Verification**: Only compute cryptographic hashes when:
   - User explicitly requests hash-based comparison mode
   - Metadata indicates files might be identical but verification is needed
3. **Optimization**: Files with different sizes are immediately marked as different without hash calculation

### Memory Management

- **Buffer Pooling**: Reuse allocated buffers via `sync.Pool` to reduce GC pressure during hash computation and file transfers
- **Streaming Operations**: Process files in chunks, never load entire files into memory
- **Configurable Buffer Sizes**: Allow tuning for different use cases (default 64KB)

### Parallel Execution

- **Comparison Phase**: Parallelize file comparisons using worker pools (default: CPU count)
- **Transfer Phase**: Support concurrent file transfers with configurable worker limit
- **Progress Reporting**: Non-blocking progress updates using channels and minimal locking

### Metadata Preservation

- **Timestamps**: Preserve modification times (mtime) during copy to enable fast incremental syncs
- **Permissions**: Copy file permissions when applicable and supported by destination filesystem
- **Verification**: Use preserved metadata for subsequent sync operations to avoid re-hashing identical files

## User Experience Requirements

### Progress Display

The human-readable output MUST provide:

1. **Active Operations Table**:
   - Column layout: Status Icon | Filename | Progress % | Bytes Copied | Total Size
   - Maximum 5 concurrent files displayed
   - Alphabetically sorted to prevent visual reordering
   - Status icons: ⏳ (copying), 🔍 (hashing), ✅ (complete), ❌ (error)

2. **Global Progress Bars**:
   - **Data Bar**: Bytes transferred with instantaneous speed and ETA
   - **Files Bar**: Number of files processed
   - Both bars show percentage completion

3. **Transfer Metrics**:
   - **Instantaneous Rate**: Calculated over 3-second sliding window for responsive feedback
   - **Average Rate**: Total operation average for overall performance assessment
   - **ETA**: Estimated time to completion based on instantaneous rate

4. **Final Report**:
   - Separate statistics for files and directories
   - Detailed error reporting with file paths and reasons
   - Average speed for the entire operation

### Real-time Feedback

- Progress updates MUST refresh at least 10 times per second (100ms intervals)
- Visual updates MUST use ANSI escape codes to update in-place without scrolling
- Long filenames MUST be truncated intelligently (show end of path)
- All byte sizes MUST be formatted in human-readable units (B, KB, MB, GB)

### Progress Counter Accuracy

The progress counters MUST adhere to the following requirements:

1. **Immediate Counting**: Files MUST be counted as soon as their status is determined
   - Synchronized files (identical): Count immediately during comparison phase
   - Files to transfer: Count during actual transfer
   - No counter resets between comparison and transfer phases

2. **Accurate Ratios**: Progress displays MUST never show impossible values
   - File count ratios MUST be valid (e.g., never "8/7 files")
   - Percentage MUST never exceed 100% (e.g., never "200% progress")
   - Byte counts MUST exclude files already counted as complete

3. **Event-Based Updates**:
   - `compare_start`: Signal start of file comparison (shows 🔍 icon)
   - `file_complete`: File synchronized or transferred (increment counters immediately)
   - `compare_complete`: File needs transfer (defer counting until transfer)
   - Events MUST render immediately without throttling for visual feedback

4. **Smooth Progression**: Progress bars MUST evolve continuously
   - Data progress: Updates as each file is verified or transferred
   - File count: Increments with each completed operation
   - No jarring jumps or resets visible to user

5. **Phase Continuity**: Single initialization for entire operation
   - Formatter initialized once before comparison phase
   - Counters persist across comparison and transfer phases
   - Total files/bytes remain constant throughout operation

**Version**: 1.2.0 | **Ratified**: 2025-11-22 | **Last Amended**: 2025-11-23
