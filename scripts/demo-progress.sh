#!/bin/bash
set -e

echo "=== SyncNorris Progress Display Demo ==="
echo ""
echo "This will create test files and show the new columnar progress display."
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-demo"
SOURCE="$TEST_DIR/source"
DEST="$TEST_DIR/dest"

# Clean up function
cleanup() {
    echo "Cleaning up..."
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create fresh test environment
rm -rf "$TEST_DIR"
mkdir -p "$SOURCE/subdir1" "$SOURCE/subdir2" "$DEST"

# Generate files with descriptive names
echo "Creating test files..."

# Small files
for i in {1..3}; do
    dd if=/dev/urandom of="$SOURCE/small_file_${i}.bin" bs=100K count=1 2>/dev/null
done

# Medium files in subdirectories
for i in {1..3}; do
    dd if=/dev/urandom of="$SOURCE/subdir1/medium_file_${i}.bin" bs=1M count=5 2>/dev/null
done

# Large files
for i in {1..2}; do
    dd if=/dev/urandom of="$SOURCE/subdir2/large_file_${i}.bin" bs=1M count=20 2>/dev/null
done

# Very long filename to test truncation
dd if=/dev/urandom of="$SOURCE/this_is_a_very_long_filename_that_should_be_truncated_in_display.bin" bs=1M count=10 2>/dev/null

echo ""
echo "Starting sync with columnar progress display..."
echo ""

./dist/syncnorris sync -s "$SOURCE" -d "$DEST"

echo ""
echo "Demo complete!"
