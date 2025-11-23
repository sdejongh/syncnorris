#!/bin/bash
# Cross-platform build script for syncnorris

set -e

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS="-s -w -X main.version=$VERSION"
BUILD_DIR="dist"

echo "Building syncnorris version: $VERSION"

# Clean previous builds
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Build for all platforms
echo "Building for Linux (amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$BUILD_DIR/syncnorris-linux-amd64" cmd/syncnorris/main.go

echo "Building for Linux (arm64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o "$BUILD_DIR/syncnorris-linux-arm64" cmd/syncnorris/main.go

echo "Building for Windows (amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$BUILD_DIR/syncnorris-windows-amd64.exe" cmd/syncnorris/main.go

echo "Building for macOS (amd64)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$BUILD_DIR/syncnorris-darwin-amd64" cmd/syncnorris/main.go

echo "Building for macOS (arm64)..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o "$BUILD_DIR/syncnorris-darwin-arm64" cmd/syncnorris/main.go

echo ""
echo "Build complete! Binaries:"
ls -lh "$BUILD_DIR"
