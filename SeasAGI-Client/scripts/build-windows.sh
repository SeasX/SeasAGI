#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLIENT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FRONTEND_DIR="$CLIENT_ROOT/src-app/frontend"
SRC_APP_DIR="$CLIENT_ROOT/src-app"
BUILD_DIR="$CLIENT_ROOT/build"

# Read version from version.txt, allow env var override
VERSION_FILE="$SCRIPT_DIR/version.txt"
if [ -f "$VERSION_FILE" ]; then
    VERSION="${SEASAGI_VERSION:-$(cat "$VERSION_FILE")}"
else
    VERSION="${SEASAGI_VERSION:-0.1.5}"
fi

# Platform override: $1 > $WAILS_PLATFORM > default
if [ -n "${1:-}" ]; then
    TARGET_PLATFORM="$1"
elif [ -n "${WAILS_PLATFORM:-}" ]; then
    TARGET_PLATFORM="$WAILS_PLATFORM"
else
    TARGET_PLATFORM="windows/amd64"
fi

APP_NAME="SeasAGI"
export VITE_APP_VERSION="$VERSION"

resolve_wails() {
    if command -v wails &> /dev/null; then
        echo "wails"
        return
    fi
    GOPATH="$(go env GOPATH 2>/dev/null || echo "$HOME/go")"
    if [ -x "$GOPATH/bin/wails.exe" ]; then
        echo "$GOPATH/bin/wails.exe"
        return
    fi
    if [ -x "$GOPATH/bin/wails" ]; then
        echo "$GOPATH/bin/wails"
        return
    fi
    echo ""
}

echo "=== SeasAGI Client Build (Windows) ==="
echo "Client root: $CLIENT_ROOT"
echo "Version:     $VERSION"
echo ""

echo "[1/5] Checking prerequisites..."
WAILS_BIN="$(resolve_wails)"
if [ -z "$WAILS_BIN" ]; then
    echo "ERROR: wails CLI not found. Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi
if ! command -v go &> /dev/null; then
    echo "ERROR: go not found — install Go from https://go.dev/dl/"
    exit 1
fi
if ! command -v npm &> /dev/null; then
    echo "ERROR: npm not found — install Node.js from https://nodejs.org/"
    exit 1
fi
echo "  wails: $WAILS_BIN"
echo "  go:    $(go version)"
echo "  node:  $(node --version)"
echo ""

echo "[2/5] Building frontend..."
cd "$FRONTEND_DIR"
npm install --quiet
npm run build
echo "  Frontend build complete"
echo ""

echo "[3/5] Verifying Go packages..."
cd "$SRC_APP_DIR"
go vet ./...
echo "  Go vet complete"
echo ""

echo "[4/5] Building Wails application..."
"$WAILS_BIN" build -s -platform "$TARGET_PLATFORM" -ldflags "-s -w" -o "$APP_NAME"
echo "  Wails build complete"
echo ""

echo "[5/5] Copying artifacts..."
mkdir -p "$BUILD_DIR"
if [ -f "$SRC_APP_DIR/build/bin/${APP_NAME}.exe" ]; then
    cp "$SRC_APP_DIR/build/bin/${APP_NAME}.exe" "$BUILD_DIR/"
    echo "  Binary: $BUILD_DIR/${APP_NAME}.exe"
fi
echo ""

echo "=== Windows Build Summary ==="
echo "  Output: $SRC_APP_DIR/build/bin/${APP_NAME}.exe"
if [ -f "$BUILD_DIR/${APP_NAME}.exe" ]; then
    echo "  Copied: $BUILD_DIR/${APP_NAME}.exe ($(du -sh "$BUILD_DIR/${APP_NAME}.exe" | cut -f1))"
fi
echo ""
echo "Windows build complete."