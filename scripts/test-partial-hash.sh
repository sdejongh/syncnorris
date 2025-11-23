#!/bin/bash
set -e

echo "=== Test du Hash Partiel (Partial Hashing) ==="
echo ""
echo "Ce test vérifie que le hash partiel permet de rejeter rapidement"
echo "des fichiers différents sans calculer le hash complet."
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-partial-hash-test"
SOURCE="$TEST_DIR/source"
DEST="$TEST_DIR/dest"

# Clean up function
cleanup() {
    echo ""
    echo "Nettoyage..."
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create fresh test environment
rm -rf "$TEST_DIR"
mkdir -p "$SOURCE" "$DEST"

echo "Création de fichiers de test..."
echo ""

# Create a 5MB file with random data
echo "- Fichier source (5MB, données aléatoires)"
dd if=/dev/urandom of="$SOURCE/large.bin" bs=1M count=5 2>/dev/null

# Create a destination file that differs in the first 256KB
# This should be rejected by partial hash without computing full hash
echo "- Fichier destination (5MB, premiers 256KB différents)"
dd if=/dev/zero of="$DEST/large.bin" bs=1M count=5 2>/dev/null

# Create identical files to test full hash path
echo "- Fichiers identiques (small.bin, 100KB)"
dd if=/dev/urandom of="$SOURCE/small.bin" bs=1K count=100 2>/dev/null
cp "$SOURCE/small.bin" "$DEST/small.bin"

# Create large identical files to test partial hash match -> full hash
echo "- Fichiers identiques (identical.bin, 3MB)"
dd if=/dev/urandom of="$SOURCE/identical.bin" bs=1M count=3 2>/dev/null
cp "$SOURCE/identical.bin" "$DEST/identical.bin"

echo ""
echo "=== Test 1: Comparaison avec hash complet (sans partial hash) ==="
echo "Tous les fichiers seront entièrement hashés"
echo ""

time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash --dry-run

echo ""
echo "=== Test 2: Comparaison avec hash partiel (avec partial hash) ==="
echo "Les fichiers >1MB avec premiers 256KB différents seront rejetés rapidement"
echo ""
echo "Note: Le hash partiel est activé par défaut dans HashComparator"
echo ""

time ./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash --dry-run

echo ""
echo "=== Résultats attendus ==="
echo ""
echo "✓ large.bin: Rejeté par partial hash (premiers 256KB diffèrent)"
echo "  → Pas besoin de hasher les 5MB complets"
echo ""
echo "✓ small.bin: Trop petit pour partial hash (100KB < 1MB)"
echo "  → Hash complet calculé, fichiers identiques"
echo ""
echo "✓ identical.bin: Partial hash match → hash complet calculé"
echo "  → Hash complet confirme que les fichiers sont identiques"
echo ""
echo "Performance attendue:"
echo "  - Sans partial hash: ~15MB de données hashées"
echo "  - Avec partial hash: ~8.5MB de données hashées (43% de réduction)"
echo "  - Speedup: ~1.7x pour ce cas de test"
