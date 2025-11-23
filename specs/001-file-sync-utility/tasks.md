# Tasks: File Synchronization Utility

**Input**: Design documents from `/specs/001-file-sync-utility/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Not requested in specification - tasks focus on implementation only

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4, US5)
- Include exact file paths in descriptions

## Path Conventions

- Single Go project: `cmd/`, `pkg/`, `internal/`, `tests/` at repository root
- Paths shown below follow Go standard project layout from plan.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [x] T001 Initialize Go module with `go mod init github.com/yourusername/syncnorris`
- [x] T002 Create directory structure per plan.md (cmd/, pkg/, internal/, tests/, config/, scripts/)
- [x] T003 [P] Add dependency github.com/spf13/cobra@latest for CLI framework
- [x] T004 [P] Add dependency github.com/spf13/viper@latest for YAML config
- [x] T005 [P] Add dependency github.com/rs/zerolog@latest for logging
- [x] T006 [P] Add dependency github.com/cheggaaa/pb/v3@latest for progress bars
- [x] T007 [P] Add dependency gopkg.in/yaml.v3@latest for YAML parsing
- [x] T008 Create config/config.example.yaml with default configuration structure
- [x] T009 Create Makefile with build, test, lint, and cross-compilation targets
- [x] T010 Create scripts/build.sh for cross-platform compilation (Linux, Windows, macOS)
- [x] T011 [P] Create scripts/test.sh for running all tests
- [x] T012 Create README.md with project overview and build instructions

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T013 [P] Define storage.Backend interface in pkg/storage/backend.go (List, Read, Write, Delete, Exists methods)
- [x] T014 [P] Define compare.Comparator interface in pkg/compare/comparator.go (Compare, Name methods)
- [x] T015 [P] Define output.Formatter interface in pkg/output/formatter.go (Format methods for human and JSON)
- [x] T016 [P] Define logging.Logger interface in pkg/logging/logger.go (Debug, Info, Warn, Error methods)
- [x] T017 [P] Create SyncOperation model in pkg/models/operation.go with all fields from data-model.md
- [x] T018 [P] Create FileEntry model in pkg/models/entry.go with all fields from data-model.md
- [x] T019 [P] Create SyncReport model in pkg/models/report.go with all fields from data-model.md
- [x] T020 [P] Create ComparisonResult model in pkg/models/comparison.go with all fields from data-model.md
- [x] T021 [P] Create Conflict model in pkg/models/conflict.go with all fields from data-model.md
- [x] T022 Implement local filesystem backend in pkg/storage/local.go (implements storage.Backend interface)
- [x] T023 Create configuration structures in pkg/config/config.go based on data-model.md Configuration entity
- [x] T024 Implement YAML config parsing in pkg/config/yaml.go using viper
- [x] T025 Create main CLI entry point in cmd/syncnorris/main.go with cobra root command
- [x] T026 [P] Implement global flags in internal/cli/flags.go (--config, --output, --log-format, etc.)
- [x] T027 [P] Implement cross-platform path utilities in internal/platform/paths.go (normalize, UNC handling)

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - One-way Folder Synchronization (Priority: P1) 🎯 MVP

**Goal**: Enable one-way synchronization from source to destination with hash comparison and progress reporting

**Independent Test**: Create source folder with test files, run one-way sync to destination, verify all files copied with matching content

### Implementation for User Story 1

- [x] T028 [P] [US1] Implement NameSize comparator in pkg/compare/namesize.go (quick comparison by filename and file size)
- [x] T029 [P] [US1] Implement Hash comparator in pkg/compare/hash.go (SHA-256 hash-based comparison with streaming for large files)
- [x] T030 [US1] Implement sync engine core in pkg/sync/engine.go (orchestrates scan, compare, transfer phases)
- [x] T031 [US1] Implement one-way sync logic in pkg/sync/oneway.go (handles source → dest synchronization)
- [x] T032 [US1] Implement parallel file transfer workers in pkg/sync/worker.go (worker pool pattern with configurable parallelism)
- [x] T033 [US1] Implement human-readable output formatter in pkg/output/human.go (file counts, transfer speeds)
- [x] T034 [US1] Implement progress bar rendering in pkg/output/progress.go using cheggaaa/pb (overall and per-file progress)
- [x] T035 [US1] Create sync command in internal/cli/commands.go with required flags (--source, --dest)
- [x] T036 [US1] Implement sync command handler that wires together engine, comparator, and output formatter
- [x] T037 [US1] Add command-line validation in internal/cli/validate.go (paths exist, not identical, not nested)

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently

---

## Phase 4: User Story 2 - Folder Comparison Without Sync (Priority: P2)

**Goal**: Enable dry-run comparison mode to preview changes without modifying files

**Independent Test**: Create two folders with different content, run comparison, verify tool identifies all differences without modifying files

### Implementation for User Story 2

- [ ] T038 [US2] Extend sync command with --dry-run flag support in internal/cli/commands.go
- [ ] T039 [US2] Modify sync engine in pkg/sync/engine.go to support comparison-only mode (set SyncOperation.DryRun=true)
- [ ] T040 [US2] Implement comparison result generation in pkg/sync/engine.go (categorize as additions, modifications, deletions)
- [ ] T041 [US2] Extend human output formatter in pkg/output/human.go to display comparison summary (5 additions, 3 modifications, 2 deletions)
- [ ] T042 [US2] Add detailed comparison output option showing file-by-file breakdown with reasons (size diff, hash diff, etc.)

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

---

## Phase 5: User Story 3 - Bidirectional Synchronization (Priority: P3)

**Goal**: Enable two-way sync with conflict detection and resolution

**Independent Test**: Create two folders with different changes on each side, run bidirectional sync, verify both changes merged correctly

### Implementation for User Story 3

- [ ] T043 [P] [US3] Create SyncState model in pkg/models/state.go for tracking last known good state (used for conflict detection)
- [ ] T044 [US3] Implement sync state persistence in pkg/sync/state.go (read/write .syncnorris/state.json)
- [ ] T045 [US3] Implement bidirectional sync logic in pkg/sync/bidirectional.go (three-way comparison using last sync state)
- [ ] T046 [US3] Implement conflict detection in pkg/sync/bidirectional.go (same file modified both sides, delete vs modify)
- [ ] T047 [US3] Implement conflict resolution strategies in pkg/sync/bidirectional.go (source-wins, dest-wins, newer, both, ask)
- [ ] T048 [US3] Extend sync command with --mode flag in internal/cli/commands.go (oneway vs bidirectional)
- [ ] T049 [US3] Extend sync command with --conflict flag for resolution strategy selection
- [ ] T050 [US3] Implement interactive conflict resolution prompts for --conflict=ask mode
- [ ] T051 [US3] Extend human output formatter to display conflict reports and resolutions

**Checkpoint**: At this point, User Stories 1, 2, AND 3 should all work independently

---

## Phase 6: User Story 4 - Multiple Comparison Methods (Priority: P4)

**Goal**: Enable users to choose between different comparison methods (namesize, timestamp, binary, hash) for speed vs accuracy tradeoffs

**Independent Test**: Modify file content without changing size/time, verify namesize misses change but hash detects it

### Implementation for User Story 4

- [ ] T052 [P] [US4] Implement Timestamp comparator in pkg/compare/timestamp.go (comparison by modification time)
- [ ] T053 [P] [US4] Implement Binary comparator in pkg/compare/binary.go (byte-by-byte comparison with first-diff-offset detection)
- [ ] T054 [US4] Extend sync command with --comparison flag in internal/cli/commands.go (namesize, timestamp, binary, hash)
- [ ] T055 [US4] Update sync engine to select comparator based on --comparison flag
- [ ] T056 [US4] Update human output to report which comparison method was used

**Checkpoint**: All comparison methods available, users can optimize for speed vs accuracy

---

## Phase 7: User Story 5 - JSON Output for Automation (Priority: P5)

**Goal**: Enable JSON output mode for programmatic parsing and automation scripts

**Independent Test**: Run any sync operation with --output=json, parse output programmatically, verify all metrics present

### Implementation for User Story 5

- [ ] T057 [P] [US5] Implement JSON output formatter in pkg/output/json.go (structured SyncReport output)
- [ ] T058 [P] [US5] Implement JSON progress events in pkg/output/json.go (newline-delimited JSON for streaming)
- [ ] T059 [US5] Update sync command handler to select formatter based on --output flag
- [ ] T060 [US5] Ensure JSON output includes all required fields (operation_id, status, summary, transfer stats, errors)
- [ ] T061 [US5] Add JSON schema examples to output formatter for documentation

**Checkpoint**: All user stories now support both human and JSON output modes

---

## Phase 8: Advanced Features & Polish

**Purpose**: Additional capabilities and cross-cutting improvements

### Storage Backend Extensions

- [ ] T062 [P] Implement SMB/Samba backend in pkg/storage/smb.go (mounted shares support)
- [ ] T063 [P] Implement NFS backend in pkg/storage/nfs.go (NFS mount support)
- [ ] T064 [P] Implement Windows UNC paths in pkg/storage/unc.go (\\server\share format)

### Logging & Configuration

- [ ] T065 [P] Implement JSON logger in pkg/logging/jsonlog.go using zerolog
- [ ] T066 [P] Implement text logger in pkg/logging/textlog.go (plain text format)
- [ ] T067 [P] Implement XML logger in pkg/logging/xmllog.go (wrap zerolog JSON output)
- [ ] T068 Wire logging throughout sync engine to log operations, errors, and progress

### Error Handling & Resilience

- [ ] T069 [P] Implement error handling for permission errors in pkg/sync/engine.go (graceful continue, report in SyncReport)
- [ ] T070 [P] Implement disk space checking in pkg/sync/engine.go (detect before transfer starts)
- [ ] T071 [P] Implement network interruption handling in pkg/sync/worker.go (retry logic, report failed files)
- [ ] T072 Implement resume functionality in pkg/sync/resume.go (track incomplete transfers, resume from checkpoint)

### Performance Features

- [ ] T073 Implement bandwidth throttling in pkg/sync/worker.go (limit bytes/sec per --bandwidth flag)
- [ ] T074 Optimize memory usage for large directory trees (stream file list, batch processing)

### Additional Commands

- [ ] T075 [P] Implement config command in internal/cli/commands.go (config show, config init, config validate subcommands)
- [ ] T076 [P] Implement version command in internal/cli/commands.go (show version, build date, Go version, platform)
- [ ] T077 Implement daemon mode in internal/platform/daemon.go (background execution with PID file)

### Cross-Cutting Concerns

- [ ] T078 [P] Add file exclusion support (glob patterns from config or --exclude flag)
- [ ] T079 [P] Implement symbolic link handling (follow or skip based on config)
- [ ] T080 [P] Add support for long file paths on Windows (>260 characters)
- [ ] T081 [P] Add support for special characters in filenames across platforms
- [ ] T082 Document all public APIs with godoc comments
- [ ] T083 Create comprehensive README with usage examples and quickstart
- [ ] T084 Add CI/CD configuration for cross-platform builds and tests

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-7)**: All depend on Foundational phase completion
  - User Story 1 (P1): Can start after Foundational - No dependencies on other stories
  - User Story 2 (P2): Can start after Foundational - No dependencies on other stories (reuses US1 comparators and engine)
  - User Story 3 (P3): Can start after Foundational - May integrate with US1/US2 but independently testable
  - User Story 4 (P4): Can start after Foundational - Extends US1 comparator selection
  - User Story 5 (P5): Can start after Foundational - Adds alternate output format to all stories
- **Advanced Features (Phase 8)**: Depends on desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1) - MVP**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2) - Comparison**: Reuses comparators from US1, but independently testable
- **User Story 3 (P3) - Bidirectional**: Builds on US1 sync engine, adds conflict detection
- **User Story 4 (P4) - Comparison Methods**: Extends US1 with more comparator implementations
- **User Story 5 (P5) - JSON Output**: Works with all stories, adds alternate output format

### Within Each User Story

- Models before services
- Services before CLI commands
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Once Foundational phase completes, user stories CAN be worked in parallel (if team capacity allows)
- Within each story, tasks marked [P] can run in parallel
- Different user stories can be worked on in parallel by different team members

---

## Parallel Example: User Story 1

```bash
# Launch comparator implementations in parallel (different files):
Task T028: Implement NameSize comparator in pkg/compare/namesize.go
Task T029: Implement Hash comparator in pkg/compare/hash.go

# After comparators done, launch output formatters in parallel:
Task T033: Implement human output in pkg/output/human.go
Task T034: Implement progress bars in pkg/output/progress.go
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T012)
2. Complete Phase 2: Foundational (T013-T027) - CRITICAL GATE
3. Complete Phase 3: User Story 1 (T028-T037)
4. **STOP and VALIDATE**: Test User Story 1 independently
   - Create test source folder with 100 files
   - Run: `syncnorris sync --source ./test/src --dest ./test/dst`
   - Verify: All files copied, progress shown, report accurate
5. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready (27 tasks)
2. Add User Story 1 → Test independently → Deploy/Demo (MVP with 10 more tasks = 37 total)
3. Add User Story 2 → Test independently → Deploy/Demo (5 more tasks = 42 total)
4. Add User Story 3 → Test independently → Deploy/Demo (9 more tasks = 51 total)
5. Add User Story 4 → Test independently → Deploy/Demo (5 more tasks = 56 total)
6. Add User Story 5 → Test independently → Deploy/Demo (5 more tasks = 61 total)
7. Add Advanced Features as needed → Deploy/Demo (23 more tasks = 84 total)
8. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together (27 tasks)
2. Once Foundational is done:
   - Developer A: User Story 1 (T028-T037) - 10 tasks
   - Developer B: User Story 2 (T038-T042) - 5 tasks
   - Developer C: User Story 4 (T052-T056) - 5 tasks (can start in parallel with US1/US2)
3. After MVP stories complete:
   - Developer D: User Story 3 (T043-T051) - 9 tasks (needs US1 as foundation)
   - Developer E: User Story 5 (T057-T061) - 5 tasks
4. Stories complete and integrate independently

---

## Task Count Summary

| Phase | Task Count | Parallel Tasks | Description |
|-------|------------|----------------|-------------|
| Phase 1: Setup | 12 | 6 | Project initialization |
| Phase 2: Foundational | 15 | 10 | Core infrastructure (BLOCKS user stories) |
| Phase 3: User Story 1 (P1) 🎯 | 10 | 2 | One-way sync - MVP |
| Phase 4: User Story 2 (P2) | 5 | 0 | Comparison/dry-run mode |
| Phase 5: User Story 3 (P3) | 9 | 1 | Bidirectional sync |
| Phase 6: User Story 4 (P4) | 5 | 2 | Multiple comparison methods |
| Phase 7: User Story 5 (P5) | 5 | 2 | JSON output |
| Phase 8: Advanced Features | 23 | 15 | Storage backends, logging, error handling, etc. |
| **TOTAL** | **84** | **38** | **45% parallelizable** |

**MVP Scope** (Recommended): Phases 1-3 = **37 tasks** for fully functional one-way sync with hash comparison and progress reporting

**Full Feature Set**: All 84 tasks for complete syncnorris implementation

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence

---

## Testing Strategy (Manual Validation)

While automated tests are not included in this task list (not requested in spec), each user story should be manually validated:

**User Story 1 (MVP)**:
```bash
# Create test data
mkdir -p test/{source,dest}
echo "test" > test/source/file1.txt
echo "data" > test/source/file2.txt

# Run sync
./syncnorris sync --source test/source --dest test/dest

# Verify
ls test/dest/  # Should contain file1.txt and file2.txt
cat test/dest/file1.txt  # Should output "test"
```

**User Story 2 (Comparison)**:
```bash
# Modify source
echo "changed" > test/source/file1.txt

# Run comparison
./syncnorris sync --source test/source --dest test/dest --dry-run

# Verify: Should report 1 modification, 0 additions, 0 deletions, no files changed
```

**User Story 3 (Bidirectional)**:
```bash
# Make different changes on each side
echo "change A" > test/source/fileA.txt
echo "change B" > test/dest/fileB.txt

# Run bidirectional
./syncnorris sync --source test/source --dest test/dest --mode bidirectional

# Verify: Both sides should have fileA.txt and fileB.txt
```

**User Story 4 (Comparison Methods)**:
```bash
# Test fast namesize comparison
./syncnorris sync --source test/source --dest test/dest --comparison namesize --dry-run

# Test thorough hash comparison
./syncnorris sync --source test/source --dest test/dest --comparison hash --dry-run
```

**User Story 5 (JSON Output)**:
```bash
# Run with JSON output
./syncnorris sync --source test/source --dest test/dest --output json

# Verify: Valid JSON output
./syncnorris sync --source test/source --dest test/dest --output json | jq .
```
