# Partial Hash Optimization

**Date**: 2025-11-23
**Type**: I/O & CPU Optimization
**Impact**: Up to 95% reduction in hash computation for files that differ early

## Problem Statement

### Before Optimization

When comparing files using cryptographic hashing (SHA-256), the entire content of both files must be read and hashed, even if the files are obviously different from the beginning.

For a 5GB video file where only the first few KB differ:
- **Without partial hash**: Read and hash 5GB + 5GB = **10GB of I/O**
- **With partial hash**: Read and hash 256KB + 256KB = **512KB of I/O**
- **Speedup**: ~20,000x for this specific case

This is particularly wasteful for:
1. **Large media files** (videos, images, audio) with different headers/metadata
2. **Log files** where new entries are appended at the beginning
3. **Database dumps** with timestamps in headers
4. **Compiled binaries** with different build IDs in headers

## Solution: Two-Stage Partial Hashing

Implemented a two-stage hash comparison for large files:

### Stage 1: Partial Hash (First 256KB)
- Compute SHA-256 of only the first 256KB of each file
- If partial hashes differ → files are different, skip full hash
- If partial hashes match → proceed to Stage 2

### Stage 2: Full Hash (Entire File)
- Only executed if partial hashes match
- Ensures cryptographic verification of file identity
- Guarantees no false positives

### Threshold Strategy

```go
const (
    partialHashThreshold = 1 * 1024 * 1024  // 1MB
    partialHashSize = 256 * 1024             // 256KB
)
```

**File Size Decision Tree**:
```
File < 1MB
  └─> Full hash only (overhead not worth it)

File ≥ 1MB
  ├─> Partial hash (256KB)
  │   ├─> Different → Return "Different" (FAST PATH)
  │   └─> Same → Full hash → Return result
  └─> On partial hash error → Fallback to full hash
```

## Implementation Details

### New Method: `computePartialHash`

**File**: `pkg/compare/hash.go`

```go
func (c *HashComparator) computePartialHash(ctx context.Context, backend storage.Backend, path string) (string, error) {
    reader, err := backend.Read(ctx, path)
    if err != nil {
        return "", fmt.Errorf("failed to open file: %w", err)
    }
    defer reader.Close()

    hasher := sha256.New()
    bufPtr := c.bufferPool.Get().(*[]byte)
    buffer := *bufPtr
    defer c.bufferPool.Put(bufPtr)

    // Read up to partialHashSize bytes (256KB)
    var totalRead int64
    for totalRead < partialHashSize {
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        default:
        }

        n, err := reader.Read(buffer)
        if n > 0 {
            bytesToHash := int64(n)
            if totalRead+bytesToHash > partialHashSize {
                bytesToHash = partialHashSize - totalRead
            }
            hasher.Write(buffer[:bytesToHash])
            totalRead += bytesToHash
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return "", fmt.Errorf("failed to read file: %w", err)
        }
    }

    return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
```

**Key Features**:
- Uses buffer pool (no allocations)
- Respects context cancellation
- Stops reading after 256KB
- Handles files smaller than 256KB gracefully

### Modified Method: `Compare`

**File**: `pkg/compare/hash.go` (lines 103-128)

```go
// Partial hash optimization for large files
if c.enablePartialHash && sourceInfo.Size >= partialHashThreshold {
    sourcePartialHash, err := c.computePartialHash(ctx, source, sourcePath)
    if err != nil {
        // Fall back to full hash on error
    } else {
        destPartialHash, err := c.computePartialHash(ctx, dest, destPath)
        if err != nil {
            // Fall back to full hash on error
        } else {
            // Quick rejection if partial hashes differ
            if sourcePartialHash != destPartialHash {
                return &Comparison{
                    SourcePath: sourcePath,
                    DestPath:   destPath,
                    Result:     Different,
                    Reason:     "file partial hashes differ",
                }, nil
            }
            // Partial hashes match - continue to full hash
        }
    }
}

// Full hash computation (only if partial hash matched or wasn't used)
sourceHash, err := c.computeHash(ctx, source, sourcePath)
// ... rest of full hash comparison
```

## Performance Impact

### I/O Reduction

| File Size | Files Differ At | Without Partial | With Partial | Reduction |
|-----------|----------------|-----------------|--------------|-----------|
| 10 MB     | Start (0KB)    | 20 MB           | 512 KB       | 97.5%     |
| 100 MB    | Start (0KB)    | 200 MB          | 512 KB       | 99.7%     |
| 1 GB      | Start (0KB)    | 2 GB            | 512 KB       | 99.97%    |
| 5 GB      | Start (0KB)    | 10 GB           | 512 KB       | 99.99%    |

### Real-World Scenarios

#### Scenario 1: Video Library Sync (Different Encodings)
- **Dataset**: 100 video files, 2GB each, same content but different encoders
- **Difference**: Encoder writes different header metadata (first 1KB differs)
- **Without partial**: Hash 100 × 2GB × 2 = **400 GB of I/O**
- **With partial**: Hash 100 × 256KB × 2 = **50 MB of I/O**
- **Speedup**: ~8000x

#### Scenario 2: Log File Rotation
- **Dataset**: 50 log files, 500MB each, daily rotation with timestamps
- **Difference**: Timestamp in first line (first 50 bytes differ)
- **Without partial**: Hash 50 × 500MB × 2 = **50 GB of I/O**
- **With partial**: Hash 50 × 256KB × 2 = **25 MB of I/O**
- **Speedup**: ~2000x

#### Scenario 3: Identical Files (Worst Case)
- **Dataset**: 100 identical files, 2GB each
- **Without partial**: Hash 100 × 2GB × 2 = **400 GB of I/O**
- **With partial**: Hash (100 × 256KB × 2) + (100 × 2GB × 2) = **400.05 GB of I/O**
- **Overhead**: ~0.012% (negligible)

### CPU Impact

Partial hashing adds minimal CPU overhead:
- **SHA-256 on 256KB**: ~1-2ms on modern CPUs
- **I/O time savings**: Often 100-1000ms for large files
- **Net benefit**: Still significant even with CPU cost

## Threshold Tuning

### Why 1MB Threshold?

Files must be ≥1MB to enable partial hashing:
- **Small files**: Overhead of two separate reads not worth it
- **Large files**: Benefit increases with file size
- **Break-even point**: Around 500KB-1MB depending on I/O speed

### Why 256KB Partial Size?

The 256KB partial hash size balances:

**Too Small (e.g., 4KB)**:
- ❌ May miss differences that occur after the first few KB
- ❌ Headers/metadata might not fit entirely in 4KB

**Too Large (e.g., 10MB)**:
- ❌ Reduces benefit for files only slightly larger than threshold
- ❌ Still significant I/O for files in 10-100MB range

**256KB Sweet Spot**:
- ✅ Covers most file headers and metadata
- ✅ Still provides 95%+ reduction for files >5MB
- ✅ Fast enough to compute (1-5ms) that overhead is negligible

## Testing

### Test Script: `scripts/test-partial-hash.sh`

Creates three test scenarios:

1. **large.bin** (5MB, different at start)
   - Source: Random data
   - Dest: All zeros
   - Expected: Rejected by partial hash

2. **small.bin** (100KB, identical)
   - Source: Random data
   - Dest: Copy of source
   - Expected: Full hash (too small for partial)

3. **identical.bin** (3MB, identical)
   - Source: Random data
   - Dest: Copy of source
   - Expected: Partial hash match → full hash confirms

**Run Test**:
```bash
bash scripts/test-partial-hash.sh
```

**Success Criteria**:
- ✅ large.bin shows reason "file partial hashes differ"
- ✅ small.bin is compared with full hash
- ✅ identical.bin passes both partial and full hash
- ✅ Overall time is faster for datasets with early differences

## Edge Cases Handled

### 1. Files Smaller Than 256KB
- **Issue**: Can't read 256KB if file is only 100KB
- **Solution**: Read loop stops at EOF naturally
- **Result**: Partial hash = full hash for small files

### 2. Partial Hash I/O Error
- **Issue**: Network interruption during partial hash read
- **Solution**: Error is caught, falls back to full hash
- **Result**: Graceful degradation, no comparison failure

### 3. Files Exactly 1MB
- **Issue**: Edge case at threshold boundary
- **Solution**: Uses `>=` comparison, so 1MB files get partial hash
- **Result**: Consistent behavior at boundary

### 4. Partial Hash Match, Full Hash Differs
- **Issue**: Mathematically possible but extremely unlikely (hash collision)
- **Solution**: Full hash always computed after partial match
- **Result**: Cryptographically secure, no false negatives

### 5. Context Cancellation During Partial Hash
- **Issue**: User cancels operation mid-read
- **Solution**: Check `ctx.Done()` in read loop
- **Result**: Immediate cancellation, no resource leak

## Configuration

### Enable/Disable Partial Hashing

Partial hashing is **enabled by default** but can be controlled:

```go
comparator := compare.NewHashComparator(bufferSize)

// Disable partial hashing (always use full hash)
comparator.SetPartialHashEnabled(false)

// Re-enable partial hashing
comparator.SetPartialHashEnabled(true)
```

**When to Disable**:
- Paranoid mode (want full hash always, even if slower)
- Testing/debugging (to compare with full hash behavior)
- Regulatory compliance (some standards may require full file hashing)

## Files Modified

1. **pkg/compare/hash.go**
   - Lines 14-20: Added partial hash constants
   - Line 27: Added `enablePartialHash bool` field
   - Lines 47-50: Added `SetPartialHashEnabled()` method
   - Lines 103-128: Added partial hash logic in `Compare()`
   - Lines 247-292: Added `computePartialHash()` method

2. **scripts/test-partial-hash.sh** (new)
   - Automated test for partial hash behavior
   - Creates files with different characteristics
   - Measures performance difference

3. **CHANGELOG.md**
   - Lines 39-55: Documented partial hashing optimization

## Future Improvements

### Possible Enhancements

1. **Adaptive Partial Size**:
   - Larger partial hash for very large files (e.g., 1MB for 1TB files)
   - Smaller partial hash for files near threshold
   - Trade-off between I/O savings and reliability

2. **Multi-Stage Partial Hashing**:
   - Stage 1: Hash first 4KB (ultra-fast rejection)
   - Stage 2: Hash first 256KB (fast rejection)
   - Stage 3: Full hash (cryptographic verification)
   - Each stage only if previous matches

3. **Content-Aware Partial Hashing**:
   - Different strategies for different file types
   - Videos: Hash first frame + random middle frames
   - Logs: Hash first and last N lines
   - Archives: Hash file list + first file

4. **Statistical Tracking**:
   - Track how often partial hash rejects files
   - Track average I/O savings
   - Report in sync summary

## Conclusion

Partial hashing provides:
- **Up to 99.99% I/O reduction** for large files that differ early
- **Negligible overhead** (0.01%) for identical files
- **No false negatives** (full hash always verifies)
- **Graceful fallback** on errors

This optimization is particularly effective for:
- Large media libraries with different encodings
- Log files with timestamps
- Database dumps with metadata
- Versioned binary files

Combined with other optimizations (throttling, parallel comparison, composite strategy), partial hashing helps syncnorris achieve high performance even on massive datasets.
