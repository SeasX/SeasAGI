#!/bin/bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build"
INSTALL_DIR="/opt/seasagi"
DATA_DIR="/var/lib/seasagi"
CONFIG_DIR="/etc/seasagi"
LOG_DIR="/var/log/seasagi"

echo "=== SeasAGI Install ==="

if [[ $EUID -ne 0 ]]; then
    echo "Error: This script must be run as root (use sudo)"
    exit 1
fi

echo "--- Creating user ---"
if ! id -u seasagi &>/dev/null; then
    useradd -r -s /bin/false -d "${DATA_DIR}" seasagi
    echo "  User 'seasagi' created"
else
    echo "  User 'seasagi' already exists"
fi

echo "--- Creating directories ---"
mkdir -p "${INSTALL_DIR}/platform-api"
mkdir -p "${INSTALL_DIR}/relay-gateway"
mkdir -p "${DATA_DIR}"
mkdir -p "${CONFIG_DIR}"
mkdir -p "${LOG_DIR}"

echo "--- Installing binaries ---"
if [[ ! -f "${BUILD_DIR}/platform-api" ]]; then
    echo "Error: ${BUILD_DIR}/platform-api not found. Run build.sh first."
    exit 1
fi
cp "${BUILD_DIR}/platform-api" "${INSTALL_DIR}/platform-api/platform-api"
cp "${BUILD_DIR}/relay-gateway" "${INSTALL_DIR}/relay-gateway/relay-gateway"
chmod 755 "${INSTALL_DIR}/platform-api/platform-api"
chmod 755 "${INSTALL_DIR}/relay-gateway/relay-gateway"

echo "--- Installing config ---"
if [[ ! -f "${CONFIG_DIR}/platform-api.env" ]]; then
    cp "${PROJECT_ROOT}/deploy/.env.example" "${CONFIG_DIR}/platform-api.env"
    echo "  Created ${CONFIG_DIR}/platform-api.env (please edit!)"
else
    echo "  ${CONFIG_DIR}/platform-api.env already exists, skipping"
fi

if [[ ! -f "${CONFIG_DIR}/relay-gateway.env" ]]; then
    cp "${PROJECT_ROOT}/deploy/.env.example" "${CONFIG_DIR}/relay-gateway.env"
    echo "  Created ${CONFIG_DIR}/relay-gateway.env (please edit!)"
else
    echo "  ${CONFIG_DIR}/relay-gateway.env already exists, skipping"
fi

echo "--- Installing systemd services ---"
cp "${PROJECT_ROOT}/deploy/systemd/seasagi-platform-api.service" /etc/systemd/system/
cp "${PROJECT_ROOT}/deploy/systemd/seasagi-relay-gateway.service" /etc/systemd/system/
systemctl daemon-reload

echo "--- Setting permissions ---"
chown -R seasagi:seasagi "${DATA_DIR}"
chown -R seasagi:seasagi "${LOG_DIR}"
chown root:root "${CONFIG_DIR}"/*.env
chmod 600 "${CONFIG_DIR}"/*.env

echo ""
echo "=== Install complete ==="
echo ""
echo "Next steps:"
echo "  1. Edit ${CONFIG_DIR}/platform-api.env — set JWT_SECRET, JWT_REFRESH_SECRET"
echo "  2. Edit ${CONFIG_DIR}/relay-gateway.env — set JWT_SECRET (same as above)"
echo "  3. Start services:"
echo "       sudo systemctl enable --now seasagi-platform-api"
echo "       sudo systemctl enable --now seasagi-relay-gateway"
echo "  4. Verify:"
echo "       curl http://localhost:9318/healthz"
echo "       curl http://localhost:8318/ops/overview"
echo "  5. (Optional) Install nginx:"
echo "       sudo cp deploy/nginx.conf /etc/nginx/nginx.conf"
echo "       sudo systemctl reload nginx"
