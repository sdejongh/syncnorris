#!/bin/bash
set -e

echo "=== SyncNorris Progress Bar Test ==="
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-progress-test"
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

# Generate test files of varying sizes to see progress
echo "Generating test files..."
echo "- Creating 5 small files (100KB each)"
for i in {1..5}; do
    dd if=/dev/urandom of="$SOURCE/small_$i.bin" bs=100K count=1 2>/dev/null
done

echo "- Creating 3 medium files (5MB each)"
for i in {1..3}; do
    dd if=/dev/urandom of="$SOURCE/medium_$i.bin" bs=1M count=5 2>/dev/null
done

echo "- Creating 2 large files (20MB each)"
for i in {1..2}; do
    dd if=/dev/urandom of="$SOURCE/large_$i.bin" bs=1M count=20 2>/dev/null
done

echo ""
echo "=== Test 1: Sync with progress bar (default) ==="
echo "You should see:"
echo "  - Individual file progress at the top (up to 5 files, sorted alphabetically)"
echo "  - Data progress bar (bytes transferred) with speed and ETA"
echo "  - Files progress bar (number of files processed)"
echo ""
read -p "Press Enter to start..."

./dist/syncnorris sync -s "$SOURCE" -d "$DEST"

echo ""
echo "=== Test 2: Resync (should be instant with namesize) ==="
read -p "Press Enter to start..."

./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test Complete ==="
