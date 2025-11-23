#!/bin/bash
# Test script for syncnorris

set -e

echo "Running all tests..."

# Run unit tests
echo "Running unit tests..."
go test ./tests/unit/... -v -race -coverprofile=coverage.out

# Run integration tests
echo "Running integration tests..."
go test ./tests/integration/... -v

# Display coverage
echo ""
echo "Coverage report:"
go tool cover -func=coverage.out | tail -n 1

echo ""
echo "All tests passed!"
