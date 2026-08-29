#!/bin/bash
# SeasAGI Server (Community Edition) — Independent Build Script
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build"
VERSION="${SEASAGI_VERSION:-1.0.0}"
LDFLAGS="-s -w -X main.Version=${VERSION}"
SRC_ADMIN_DIR="${PROJECT_ROOT}/src-admin"
PLATFORM_API_DIR="${PROJECT_ROOT}/platform-api"
RELAY_GATEWAY_DIR="${PROJECT_ROOT}/relay-gateway"

echo "=== SeasAGI Server (Community Edition) Build ==="
echo "Project root: ${PROJECT_ROOT}"
echo "Build dir:    ${BUILD_DIR}"
echo "Version:      ${VERSION}"

rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

echo ""
echo "--- Building admin frontend ---"
cd "${SRC_ADMIN_DIR}"
npm install --quiet
npm run build
rm -rf "${PLATFORM_API_DIR}/cmd/admin_dist"
cp -r "${SRC_ADMIN_DIR}/dist" "${PLATFORM_API_DIR}/cmd/admin_dist"
echo "  -> ${PLATFORM_API_DIR}/cmd/admin_dist"

echo ""
echo "--- Building platform-api ---"
cd "${PLATFORM_API_DIR}"
CGO_ENABLED=1 go build -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/platform-api" .
echo "  -> ${BUILD_DIR}/platform-api ($(du -h "${BUILD_DIR}/platform-api" | cut -f1))"

echo ""
echo "--- Building relay-gateway ---"
cd "${RELAY_GATEWAY_DIR}"
CGO_ENABLED=1 go build -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/relay-gateway" .
echo "  -> ${BUILD_DIR}/relay-gateway ($(du -h "${BUILD_DIR}/relay-gateway" | cut -f1))"

echo ""
echo "=== Build complete ==="
ls -la "${BUILD_DIR}/"