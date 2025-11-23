#!/bin/bash
set -e

echo "=== SyncNorris Performance Test ==="
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-perf-test"
SOURCE="$TEST_DIR/source"
DEST="$TEST_DIR/dest"

# Clean up function
cleanup() {
    echo "Cleaning up test directories..."
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create fresh test environment
rm -rf "$TEST_DIR"
mkdir -p "$SOURCE" "$DEST"

# Generate test files
echo "Generating 100 test files (1MB each)..."
for i in {1..100}; do
    dd if=/dev/urandom of="$SOURCE/file_$i.bin" bs=1M count=1 2>/dev/null
done

echo ""
echo "=== Test 1: Initial sync (copy all files) ==="
time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test 2: Second sync with namesize (should be instant - no changes) ==="
time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test 3: Second sync with hash (slower - hashes all files) ==="
time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash

echo ""
echo "=== Test 4: Modify one file and resync ==="
dd if=/dev/urandom of="$SOURCE/file_50.bin" bs=1M count=1 2>/dev/null
echo "Modified file_50.bin"
time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test Complete ==="
echo "Key observations:"
echo "- Test 2 should be very fast (metadata-only comparison)"
echo "- Test 3 will be slower (hashes all 100MB)"
echo "- Test 4 should detect and copy only the modified file"
