#!/bin/bash
# ============================================================
# SeasAGI Server (Community Edition) — Cross-Platform Build
#
# Builds platform-api + relay-gateway for macOS, Linux and Windows
# (amd64/arm64) using CGO_ENABLED=0 (pure-Go SQLite).
#
# 版本号来源（优先级从高到低）：
#   SEASAGI_VERSION > scripts/version.txt > git describe > 1.0.0
#
# Usage:
#   ./scripts/build-cross.sh              # all targets
#   ./scripts/build-cross.sh linux/amd64  # single target
#   ./scripts/build-cross.sh darwin/arm64 linux/amd64 windows/amd64
#   CGO=1 ./scripts/build-cross.sh        # native build with CGO
#   SKIP_FRONTEND=1 ./scripts/build-cross.sh   # reuse embedded admin dist
# ============================================================
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS_DIR="${PROJECT_ROOT}/scripts"
BUILD_DIR="${PROJECT_ROOT}/build"

SRC_ADMIN="${PROJECT_ROOT}/src-admin"
API_DIR="${PROJECT_ROOT}/platform-api"
RELAY_DIR="${PROJECT_ROOT}/relay-gateway"

VERSION_DEFAULT="1.0.0"

resolve_version() {
    if [[ -n "${SEASAGI_VERSION:-}" ]]; then
        echo "${SEASAGI_VERSION#v}"
        return
    fi
    if [[ -f "${SCRIPTS_DIR}/version.txt" ]]; then
        local v
        v="$(tr -d '[:space:]' < "${SCRIPTS_DIR}/version.txt")"
        if [[ -n "${v}" ]]; then
            echo "${v#v}"
            return
        fi
    fi
    if git -C "${PROJECT_ROOT}" describe --tags --always >/dev/null 2>&1; then
        git -C "${PROJECT_ROOT}" describe --tags --always | sed 's/^v//'
        return
    fi
    echo "${VERSION_DEFAULT}"
}

VERSION="$(resolve_version)"
LDFLAGS="-s -w -X main.Version=${VERSION}"

# --- Targets ---------------------------------------------------
ALL_TARGETS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

if [[ $# -gt 0 ]]; then
    TARGETS=("$@")
else
    TARGETS=("${ALL_TARGETS[@]}")
fi

# --- Helpers ---------------------------------------------------
log() { echo "  -> $*"; }

build_admin_frontend() {
    if [[ "${SKIP_FRONTEND:-0}" == "1" ]]; then
        echo "--- Skipping admin frontend (SKIP_FRONTEND=1) ---"
        if [[ ! -d "${API_DIR}/cmd/admin_dist" ]]; then
            echo "Error: ${API_DIR}/cmd/admin_dist not found; unset SKIP_FRONTEND to build it." >&2
            exit 1
        fi
        return
    fi
    echo "--- Building admin frontend ---"
    cd "${SRC_ADMIN}"
    if [[ -f package-lock.json ]]; then
        npm ci --quiet
    else
        npm install --quiet
    fi
    npm run build
    rm -rf "${API_DIR}/cmd/admin_dist"
    cp -r "${SRC_ADMIN}/dist" "${API_DIR}/cmd/admin_dist"
    log "admin frontend embedded"
}

goos_arch_from_target() {
    local t="$1"
    printf '%s %s\n' "${t%%/*}" "${t#*/}"
}

binary_name() {
    local base="$1"
    local target="$2"
    local ext=""
    [[ "${target%%/*}" == "windows" ]] && ext=".exe"
    echo "${base}-${target//\//-}${ext}"
}

build_binary() {
    local src_dir="$1"
    local module_name="$2"
    local out_name="$3"
    local target="$4"
    local goos goarch cgo

    read -r goos goarch <<<"$(goos_arch_from_target "$target")"
    cgo=0

    # Optionally use CGO=1 for native builds
    if [[ "${CGO:-0}" == "1" && "$(go env GOOS)" == "$goos" && "$(go env GOARCH)" == "$goarch" ]]; then
        cgo=1
    fi

    local out="${BUILD_DIR}/${out_name}"
    echo "--- Building ${module_name} ${VERSION} for ${target} (CGO=${cgo}) ---"
    cd "${src_dir}"
    CGO_ENABLED=${cgo} GOOS="${goos}" GOARCH="${goarch}" \
        go build -trimpath -ldflags="${LDFLAGS}" -o "${out}" .
    log "${out} ($(du -h "${out}" | cut -f1))"
}

# --- Main ------------------------------------------------------
echo "=== SeasAGI Server (Community) Cross-Platform Build ==="
echo "  Project:  ${PROJECT_ROOT}"
echo "  Version:  ${VERSION}"
echo "  Targets:  ${TARGETS[*]}"
echo ""

for cmd in go node; do
    if ! command -v "${cmd}" >/dev/null 2>&1; then
        echo "Error: missing required command '${cmd}'" >&2
        exit 1
    fi
done
if [[ "${SKIP_FRONTEND:-0}" != "1" ]] && ! command -v npm >/dev/null 2>&1; then
    echo "Error: missing required command 'npm'" >&2
    exit 1
fi

rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# Build frontend once (same for all platforms)
build_admin_frontend
echo ""

# Build Go binaries for each target
for t in "${TARGETS[@]}"; do
    api_out=$(binary_name "platform-api" "$t")
    relay_out=$(binary_name "relay-gateway" "$t")
    build_binary "${API_DIR}" "platform-api" "${api_out}" "$t"
    build_binary "${RELAY_DIR}" "relay-gateway" "${relay_out}" "$t"
    echo ""
done

echo "=== Build complete (version ${VERSION}) ==="
ls -lh "${BUILD_DIR}/"
