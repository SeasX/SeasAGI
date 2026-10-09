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
    VERSION="${SEASAGI_VERSION:-0.2.0}"
fi

# Platform override: $1 > $WAILS_PLATFORM > default
if [ -n "${1:-}" ]; then
    TARGET_PLATFORM="$1"
elif [ -n "${WAILS_PLATFORM:-}" ]; then
    TARGET_PLATFORM="$WAILS_PLATFORM"
else
    TARGET_PLATFORM="linux/amd64"
fi

APP_NAME="SeasAGI"
export VITE_APP_VERSION="$VERSION"

resolve_wails() {
    if command -v wails &> /dev/null; then
        echo "wails"
        return
    fi
    GOPATH="$(go env GOPATH 2>/dev/null || echo "$HOME/go")"
    if [ -x "$GOPATH/bin/wails" ]; then
        echo "$GOPATH/bin/wails"
        return
    fi
    echo ""
}

echo "=== SeasAGI Client Build (Linux) ==="
echo "Client root: $CLIENT_ROOT"
echo "Version:     $VERSION"
echo ""

echo "[1/6] Checking prerequisites..."
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

echo "[2/6] Checking system libraries..."
if command -v apt &> /dev/null; then
    echo "  Detected apt (Debian/Ubuntu). Required packages: libgtk-3-dev libwebkit2gtk-4.0-dev build-essential"
    for pkg in libgtk-3-dev libwebkit2gtk-4.0-dev; do
        if dpkg -s "$pkg" &>/dev/null; then
            echo "    $pkg: found"
        else
            echo "    $pkg: NOT found — install with: sudo apt install $pkg"
        fi
    done
elif command -v yum &> /dev/null || command -v dnf &> /dev/null; then
    PKG_MGR="$(command -v yum || command -v dnf)"
    echo "  Detected $PKG_MGR (RHEL/CentOS/Fedora). Required packages: gtk3-devel webkit2gtk3-devel"
    for pkg in gtk3-devel webkit2gtk3-devel; do
        if $PKG_MGR list installed "$pkg" &>/dev/null 2>&1; then
            echo "    $pkg: found"
        else
            echo "    $pkg: NOT found — install with: sudo $PKG_MGR install $pkg"
        fi
    done
else
    echo "  Warning: unknown package manager. Ensure GTK3 and WebKit2GTK development headers are installed."
fi
echo ""

echo "[3/6] Building frontend..."
cd "$FRONTEND_DIR"
npm install --quiet
npm run build
echo "  Frontend build complete"
echo ""

echo "[4/6] Verifying Go packages..."
cd "$SRC_APP_DIR"
go vet ./...
echo "  Go vet complete"
echo ""

echo "[5/6] Building Wails application..."
"$WAILS_BIN" build -s -platform "$TARGET_PLATFORM" -ldflags "-s -w" -o "$APP_NAME"
echo "  Wails build complete"
echo ""

echo "[6/6] Copying artifacts..."
mkdir -p "$BUILD_DIR"
if [ -f "$SRC_APP_DIR/build/bin/$APP_NAME" ]; then
    cp "$SRC_APP_DIR/build/bin/$APP_NAME" "$BUILD_DIR/"
    echo "  Binary: $BUILD_DIR/$APP_NAME"
fi
echo ""

echo "=== Linux Build Summary ==="
echo "  Output: $SRC_APP_DIR/build/bin/$APP_NAME"
if [ -f "$BUILD_DIR/$APP_NAME" ]; then
    echo "  Copied: $BUILD_DIR/$APP_NAME ($(du -sh "$BUILD_DIR/$APP_NAME" | cut -f1))"
fi
echo ""
echo "Linux build complete."