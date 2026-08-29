#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLIENT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== SeasAGI Dev Mode ==="
echo "Starting Wails dev server..."
cd "$CLIENT_ROOT/src-app"
wails dev
