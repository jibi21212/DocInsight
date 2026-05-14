#!/bin/bash
# Reset the test environment to a clean state before E2E runs
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SCRIPT_DIR"

echo "Resetting test environment..."

# Remove database
rm -f ./docinsight.db
rm -f ./backend/docinsight.db

# Clear uploads
rm -rf ./uploads/*

# Clear test logs
rm -f ./test-logs/backend.log

# Ensure directories exist
mkdir -p ./test-logs
mkdir -p ./uploads

echo "Test environment reset complete"
