#!/bin/bash
# SeasAGI Server (Community Edition) — Deploy Wrapper
# Delegates to the independent scripts/build.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

exec "$PROJECT_ROOT/scripts/build.sh" "$@"