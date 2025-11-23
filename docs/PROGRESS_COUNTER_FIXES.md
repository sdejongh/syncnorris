# Progress Counter Accuracy Fixes

**Date**: 2025-11-23
**Session**: Progress Counter Debugging and Corrections

## Overview

This document details the fixes applied to resolve progress counter inconsistencies in syncnorris. These issues manifested as confusing displays like "8/7 files", "0/6 files", or "200% progress".

## Problems Identified

### Issue #1: Inconsistent File Counters
**Symptom**: Display showed impossible values like "(8/7 files)" or "(0/6 files)"

**Root Cause**:
- The `Start()` method was called twice during a sync operation:
  1. Once before the comparison phase with all files to process
  2. Once before the transfer phase with only files needing transfer
- The second call updated `totalFiles` but did NOT reset `processedFiles`
- Result: `processedFiles` from comparison phase + new `totalFiles` = invalid ratio

**Example**:
```
Comparison: totalFiles=7, processedFiles accumulates to 7
Transfer:   totalFiles=2 (only new files), processedFiles still at 7
Display:    (8/2 files) ❌
```

**Solution**: Reset both `processedFiles` and `processedBytes` in `Start()` when beginning a new phase.

```go
// pkg/output/progress.go:73-75
// Reset progress counters when starting a new phase
f.processedFiles = 0
f.processedBytes = 0
```

### Issue #2: Double Counting Bytes
**Symptom**: Progress showed impossible percentages like "200% 150 B/75 B"

**Root Cause**:
- Files marked as "complete" remained in `activeFiles` for 500ms (for visibility)
- The `render()` function calculated current bytes as: `processedBytes + sum(activeFiles.current)`
- Files with status="complete" were counted in BOTH `processedBytes` AND `activeFiles`

**Example**:
```
processedBytes = 75 B (from 5 completed files)
activeFiles contains the same 5 files (visible for 500ms)
currentBytes = 75 + 75 = 150 B
Display: 150 B/75 B = 200% ❌
```

**Solution**: Exclude files with `status="complete"` from active byte calculations.

```go
// pkg/output/progress.go:274-277
currentBytes := f.processedBytes
for _, fp := range f.activeFiles {
    if fp.status != "complete" {  // Don't double-count
        currentBytes += fp.current
    }
}
```

### Issue #3: Progress Bars Not Evolving During Comparison
**Symptom**:
- Progress stayed at "0% 0 B/105 B (0/7 files)" during entire comparison phase
- Then jumped to "100%" when transfer completed
- User couldn't see that 5 files were already synchronized

**Root Cause**:
- After comparing files, engine sent `compare_complete` event for ALL files
- `compare_complete` event removed files from display but did NOT increment counters
- Synchronized files were only counted later in `worker.Execute()`

**Example**:
```
7 files to sync, 5 already synchronized, 2 need transfer:
- Comparison completes: 7 files verified, 0 counted ❌
- Transfer starts: "0/2 files" showing
- Transfer completes: "2/2 files"
User never saw the 5 synchronized files
```

**Solution**: Send different events based on comparison result:
- Files **identical**: Send `file_complete` immediately (count now)
- Files **different**: Send `compare_complete` (count during transfer)

```go
// pkg/sync/engine.go:406-438
if comparison.Result == compare.Same {
    action = models.ActionSkip
    reason = "files are identical"

    // Count immediately
    e.formatter.Progress(output.ProgressUpdate{
        Type:         "file_complete",
        FilePath:     path,
        BytesWritten: sourceInfo.Size,
        CurrentFile:  idx,
    })
} else {
    action = models.ActionUpdate
    reason = comparison.Reason

    // Will be counted during transfer
    e.formatter.Progress(output.ProgressUpdate{
        Type:         "compare_complete",
        FilePath:     path,
        BytesWritten: sourceInfo.Size,
        CurrentFile:  idx,
    })
}
```

### Issue #4: Hash Progress Visibility
**Symptom**: Hash comparison icon (🔍) and progress not visible

**Root Cause**:
- Progress updates were throttled to 100ms intervals
- For fast hash operations, the `compare_start` event was throttled and never rendered

**Solution**: Force immediate render on `compare_start` and `file_start` events.

```go
// pkg/output/progress.go:99-109
case "compare_start":
    f.activeFiles[update.CurrentFile] = &fileProgress{
        path:      update.FilePath,
        status:    "hashing",
        // ...
    }
    // Render immediately to show the hashing icon
    f.render()
    f.lastDisplay = time.Now()
```

## Event Flow

### Before Fixes
```
1. Comparison Phase:
   - Compare file → send compare_complete (no counting)
   - Progress: 0% (0/7 files) ❌

2. Transfer Phase:
   - Start() resets totalFiles=2, keeps processedFiles=0
   - Copy file 1 → processedFiles=1 → (1/2 files) ✓
   - Copy file 2 → processedFiles=2 → (2/2 files) ✓
   - Execute() counts synchronized files → processedFiles=7 → (7/2 files) ❌
```

### After Fixes
```
1. Comparison Phase:
   - Start(totalFiles=7, totalBytes=105)
   - Compare file (identical) → file_complete → 1/7, 15 B/105 B ✓
   - Compare file (identical) → file_complete → 2/7, 30 B/105 B ✓
   - Compare file (different) → compare_complete (no counting yet)
   - Compare file (identical) → file_complete → 3/7, 45 B/105 B ✓
   - Continue... → 5/7, 75 B/105 B ✓

2. Transfer Phase:
   - No second Start() call (keeps existing counters)
   - Copy file 1 → file_complete → 6/7, 90 B/105 B ✓
   - Copy file 2 → file_complete → 7/7, 105 B/105 B ✓
```

## Test Results

### Test Case 1: Partial Sync (5 synchronized, 2 new)
```bash
# 7 total files: 5 already identical, 2 need copying

Progress during comparison:
  29% 30 B/105 B (2/7 files)  ✓
  43% 45 B/105 B (3/7 files)  ✓
  57% 60 B/105 B (4/7 files)  ✓
  71% 75 B/105 B (5/7 files)  ✓

Progress during transfer:
  71% 75 B/105 B (5/7 files)  ✓ (no reset)
  86% 90 B/105 B (6/7 files)  ✓
  100% 105 B/105 B (7/7 files) ✓

Summary:
  Files synchronized: 5
  Files copied: 2
  Total: 7 ✓
```

### Test Case 2: All Files Synchronized
```bash
# 7 total files: all identical, 0 need copying

Progress during comparison:
  0% 0 B/105 B (0/7 files)
  29% 30 B/105 B (2/7 files)  ✓
  43% 45 B/105 B (3/7 files)  ✓
  57% 60 B/105 B (4/7 files)  ✓
  71% 75 B/105 B (5/7 files)  ✓
  86% 90 B/105 B (6/7 files)  ✓
  100% 105 B/105 B (7/7 files) ✓

Summary:
  Files synchronized: 7
  Files copied: 0
  Transfer: 0 B ✓
```

### Test Case 3: Hash Progress Visibility
```bash
# 3 large files (100 MB each) being compared with hash

Display shows:
🔍  large_file_1.bin    0.0%    0 B      100.0 MiB
🔍  large_file_2.bin    0.0%    0 B      100.0 MiB
🔍  large_file_3.bin    0.0%    0 B      100.0 MiB

[Progress updates during hashing...]

🔍  large_file_1.bin    57.8%   57.8 MiB  100.0 MiB
🔍  large_file_2.bin    56.9%   56.9 MiB  100.0 MiB
🔍  large_file_3.bin    57.5%   57.5 MiB  100.0 MiB

Data: 57% 172.2 MiB/300.0 MiB @ 1.7 GiB/s ✓
```

## Files Modified

### Core Changes
1. **pkg/output/progress.go**:
   - Added counter reset in `Start()` (lines 73-75)
   - Exclude complete files from active bytes (lines 274-277)
   - Added `compare_complete` event handler (lines 127-142)
   - Immediate render on `compare_start` and `file_start` (lines 96-97, 108-109)

2. **pkg/sync/engine.go**:
   - Conditional event sending based on comparison result (lines 406-438)
   - Removed duplicate `Start()` call before transfer phase (line 184)
   - Move error event notification into comparison error handling (lines 365-377)

3. **pkg/sync/worker.go**:
   - Removed duplicate `file_complete` notification for synchronized files (lines 70-72)
   - Synchronized files already counted during comparison phase

## Benefits

1. **Visual Continuity**: Progress evolves smoothly from 0% to 100% without resets
2. **Accurate Counts**: Never shows impossible ratios like "8/7 files"
3. **Immediate Feedback**: Users see synchronized files being counted in real-time
4. **Better UX**: No confusing jumps or percentage over 100%
5. **Truthful Display**: Progress bars accurately reflect work completed vs remaining

## Migration Impact

- **Breaking Changes**: None
- **API Changes**: None (internal implementation only)
- **Behavior Changes**: Progress display is more accurate and responsive
- **Performance**: No impact (same number of operations, just better reporting)
