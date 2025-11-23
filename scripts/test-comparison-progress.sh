#!/bin/bash
set -e

echo "=== SyncNorris - Test de progression pendant la comparaison ==="
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-comparison-test"
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
mkdir -p "$SOURCE/subdir1" "$SOURCE/subdir2" "$DEST"

echo "Création de fichiers de test..."

# Create files in source
for i in {1..5}; do
    dd if=/dev/urandom of="$SOURCE/file_${i}.bin" bs=1M count=10 2>/dev/null
done

# Create subdirectories with files
for i in {1..3}; do
    dd if=/dev/urandom of="$SOURCE/subdir1/data_${i}.bin" bs=1M count=5 2>/dev/null
done

for i in {1..2}; do
    dd if=/dev/urandom of="$SOURCE/subdir2/large_${i}.bin" bs=1M count=20 2>/dev/null
done

echo ""
echo "=== Test 1: Sync initial (copie) ==="
echo "Vous devriez voir les fichiers avec l'icône ⏳ (copie en cours)"
echo ""
./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test 2: Re-sync avec comparaison hash ==="
echo "Vous devriez voir les fichiers avec l'icône 🔍 (calcul de hash en cours)"
echo "Les fichiers devraient apparaître dans la liste pendant la comparaison"
echo ""
read -p "Appuyez sur Entrée pour continuer..."

./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash

echo ""
echo "=== Rapport final ==="
echo "Le rapport devrait distinguer :"
echo "  - Fichiers scannés vs Dossiers scannés"
echo "  - Débit moyen affiché"
echo ""
echo "Test terminé !"
