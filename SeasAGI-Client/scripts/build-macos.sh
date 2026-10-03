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

# Platform override: $1 > $WAILS_PLATFORM > uname detection
if [ -n "${1:-}" ]; then
    TARGET_PLATFORM="$1"
elif [ -n "${WAILS_PLATFORM:-}" ]; then
    TARGET_PLATFORM="$WAILS_PLATFORM"
else
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64)  TARGET_PLATFORM="darwin/amd64" ;;
        arm64)   TARGET_PLATFORM="darwin/arm64" ;;
        *)       TARGET_PLATFORM="darwin/amd64" ;;
    esac
fi

APP_NAME="SeasAGI"
export VITE_APP_VERSION="$VERSION"

resolve_wails() {
    if command -v wails &> /dev/null; then
        echo "wails"
        return
    fi
    GOPATH="$(go env GOPATH)"
    if [ -x "$GOPATH/bin/darwin_amd64/wails" ]; then
        echo "$GOPATH/bin/darwin_amd64/wails"
        return
    fi
    if [ -x "$GOPATH/bin/wails" ]; then
        echo "$GOPATH/bin/wails"
        return
    fi
    echo ""
}

echo "=== SeasAGI Client Build ==="
echo "Client root: $CLIENT_ROOT"
echo "Version:     $VERSION"
echo "Platform:    $TARGET_PLATFORM"
echo ""

echo "[1/5] Checking prerequisites..."
WAILS_BIN="$(resolve_wails)"
if [ -z "$WAILS_BIN" ]; then
    echo "ERROR: wails CLI not found. Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi
if ! command -v go &> /dev/null; then
    echo "ERROR: go not found"
    exit 1
fi
if ! command -v npm &> /dev/null; then
    echo "ERROR: npm not found"
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
go test ./...
echo "  Go tests complete"
echo ""

echo "[4/5] Building Wails application..."
"$WAILS_BIN" build -s -platform "$TARGET_PLATFORM" -ldflags "-s -w" -o "$APP_NAME"
echo "  Wails build complete"
echo ""

echo "[5/5] Packaging macOS DMG (darwin only)..."
mkdir -p "$BUILD_DIR"
APP_BUNDLE="$SRC_APP_DIR/build/bin/$APP_NAME.app"
if [[ "$TARGET_PLATFORM" == darwin/* ]] && [ -d "$APP_BUNDLE" ]; then
    DMG_PATH="$BUILD_DIR/${APP_NAME}-${VERSION}.dmg"
    STAGING_DIR="/tmp/${APP_NAME}-dmg-staging"
    rm -f "$DMG_PATH"
    rm -rf "$STAGING_DIR"
    mkdir -p "$STAGING_DIR"
    cp -R "$APP_BUNDLE" "$STAGING_DIR/"
    ln -s /Applications "$STAGING_DIR/Applications"
    hdiutil create -volname "$APP_NAME $VERSION" -srcfolder "$STAGING_DIR" -ov -format UDZO "$DMG_PATH"
    rm -rf "$STAGING_DIR"
    echo "  DMG created: $DMG_PATH"
else
    echo "  Skipped DMG packaging because current target is $TARGET_PLATFORM"
fi
echo ""

echo "=== Client Build Summary ==="
echo "  App output:    $SRC_APP_DIR/build/bin"
if [ -d "$APP_BUNDLE" ]; then
    echo "  App bundle:    $APP_BUNDLE ($(du -sh "$APP_BUNDLE" | cut -f1))"
fi
if [ -f "$BUILD_DIR/${APP_NAME}-${VERSION}.dmg" ]; then
    echo "  DMG:           $BUILD_DIR/${APP_NAME}-${VERSION}.dmg ($(du -sh "$BUILD_DIR/${APP_NAME}-${VERSION}.dmg" | cut -f1))"
fi
echo ""
echo "Client build complete."
