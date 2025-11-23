#!/bin/bash
set -e

echo "=== Test du Throttling des Callbacks de Progression ==="
echo ""
echo "Ce test vérifie que les callbacks ne sont pas appelés à chaque lecture,"
echo "mais seulement tous les 64KB ou toutes les 50ms."
echo ""

# Create test directories
TEST_DIR="/tmp/syncnorris-throttle-test"
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

echo "Création de fichiers de test de différentes tailles..."
echo ""

# Create files of varying sizes to test throttling behavior
echo "- Petit fichier (10KB) - devrait avoir peu de callbacks"
dd if=/dev/urandom of="$SOURCE/small.bin" bs=1K count=10 2>/dev/null

echo "- Fichier moyen (500KB) - devrait avoir ~8 callbacks (500/64)"
dd if=/dev/urandom of="$SOURCE/medium.bin" bs=1K count=500 2>/dev/null

echo "- Gros fichier (5MB) - devrait avoir ~80 callbacks (5120/64)"
dd if=/dev/urandom of="$SOURCE/large.bin" bs=1M count=5 2>/dev/null

echo ""
echo "=== Test 1: Copie initiale avec throttling ==="
echo "Observez la fluidité des mises à jour de progression"
echo ""

./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison namesize

echo ""
echo "=== Test 2: Re-sync avec hash et throttling ==="
echo "Les callbacks de hash devraient aussi être throttlés"
echo ""

./dist/syncnorris sync -s "$SOURCE" -d "$DEST" --comparison hash

echo ""
echo "=== Test terminé ==="
echo ""
echo "Vérifications:"
echo "1. Les barres de progression ont-elles évolué de manière fluide?"
echo "2. L'affichage était-il stable (pas de scintillement)?"
echo "3. La performance était-elle acceptable?"
echo ""
echo "Avec le throttling, les callbacks sont limités à:"
echo "  - Maximum 1 par 50ms (20 callbacks/seconde)"
echo "  - OU 1 tous les 64KB lus"
echo "  - Plus le callback final à 100%"
