#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLATFORM="${1:-}"

if [ -n "$PLATFORM" ]; then
    # Explicit platform: dispatch based on GOOS part
    GOOS="${PLATFORM%%/*}"
    case "$GOOS" in
        darwin)  exec "$SCRIPT_DIR/build-macos.sh" "$PLATFORM" ;;
        linux)   exec "$SCRIPT_DIR/build-linux.sh" "$PLATFORM" ;;
        windows) exec "$SCRIPT_DIR/build-windows.sh" "$PLATFORM" ;;
        *)
            echo "Unsupported platform: $PLATFORM"
            echo "Expected format: GOOS/GOARCH (e.g. darwin/amd64, linux/amd64, windows/amd64)"
            exit 1
            ;;
    esac
fi

# No platform arg: auto-detect OS
case "$(uname -s)" in
    Darwin)
        exec "$SCRIPT_DIR/build-macos.sh"
        ;;
    Linux)
        exec "$SCRIPT_DIR/build-linux.sh"
        ;;
    CYGWIN*|MINGW*|MSYS*)
        exec "$SCRIPT_DIR/build-windows.sh"
        ;;
    *)
        echo "Unsupported OS: $(uname -s)"
        echo "Use one of the platform-specific scripts directly:"
        echo "  bash scripts/build-macos.sh   (macOS)"
        echo "  bash scripts/build-linux.sh   (Linux)"
        echo "  bash scripts/build-windows.sh (Windows)"
        echo ""
        echo "Or specify a platform:"
        echo "  bash scripts/build.sh darwin/amd64"
        echo "  bash scripts/build.sh darwin/arm64"
        echo "  bash scripts/build.sh linux/amd64"
        echo "  bash scripts/build.sh windows/amd64"
        exit 1
        ;;
esac
