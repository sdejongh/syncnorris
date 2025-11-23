#!/bin/bash
set -e

echo "=== Test de progression pendant le calcul de hash ==="
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-hash-progress"
SOURCE="$TEST_DIR/source"
DEST="$TEST_DIR/dest"

# Clean up function
cleanup() {
    echo "Nettoyage..."
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create fresh test environment
rm -rf "$TEST_DIR"
mkdir -p "$SOURCE" "$DEST"

echo "Création de gros fichiers pour voir la progression..."
echo ""

# Create large files (100MB each) to ensure hash calculation is visible
echo "- Création de 3 fichiers de 100MB chacun..."
for i in {1..3}; do
    dd if=/dev/urandom of="$SOURCE/large_file_$i.bin" bs=1M count=100 2>/dev/null &
done
wait

echo ""
echo "=== Test 1: Premier sync (copie) ==="
echo "Les fichiers devraient apparaître avec l'icône ⏳"
echo ""
./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test 2: Re-sync avec hash (IMPORTANT) ==="
echo "Les fichiers devraient apparaître avec l'icône 🔍"
echo "ET vous devriez voir la progression du calcul de hash (0% → 100%)"
echo ""
read -p "Appuyez sur Entrée pour lancer le test avec hash..."

./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash

echo ""
echo "=== Avez-vous vu la progression pendant le hash ? ==="
echo "Si NON, il y a un problème avec les callbacks de progression"
