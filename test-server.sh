#!/usr/bin/env bash
# test-server.sh — SeasAGI-Server 单元测试 + 编译验证
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/SeasAGI-Server"
ENTERPRISE_DIR="$SCRIPT_DIR/SeasAGI-Server-Enterprise"

echo "=========================================="
echo "  SeasAGI-Server Test Suite"
echo "=========================================="

FAIL=0

# ── 平台 API ──
echo ""
echo "[1/4] Platform API — build + test ..."
cd "$SERVER_DIR/platform-api"
if ! go build ./... 2>&1; then
  echo "❌ Platform API build FAILED"
  FAIL=1
else
  echo "✅ Platform API build passed"
  if ! go test ./internal/... -count=1 -timeout 120s 2>&1; then
    echo "❌ Platform API tests FAILED"
    FAIL=1
  else
    echo "✅ Platform API tests passed"
  fi
fi

# ── Relay Gateway ──
echo ""
echo "[2/4] Relay Gateway — build + test ..."
cd "$SERVER_DIR/relay-gateway"
if ! go build ./... 2>&1; then
  echo "❌ Relay Gateway build FAILED"
  FAIL=1
else
  echo "✅ Relay Gateway build passed"
  if ! go test ./internal/... -count=1 -timeout 120s 2>&1; then
    echo "❌ Relay Gateway tests FAILED"
    FAIL=1
  else
    echo "✅ Relay Gateway tests passed"
  fi
fi

# ── 企业版（如果存在）──
echo ""
if [ -d "$ENTERPRISE_DIR" ]; then
  echo "[3/4] Enterprise Server — build + test ..."
  if [ -d "$ENTERPRISE_DIR/platform-api" ]; then
    cd "$ENTERPRISE_DIR/platform-api"
    if ! go build ./... 2>&1; then
      echo "❌ Enterprise Platform API build FAILED"
      FAIL=1
    else
      echo "✅ Enterprise Platform API build passed"
      if ! go test ./internal/... -count=1 -timeout 120s 2>&1; then
        echo "❌ Enterprise Platform API tests FAILED"
        FAIL=1
      else
        echo "✅ Enterprise Platform API tests passed"
      fi
    fi
  fi
  if [ -d "$ENTERPRISE_DIR/relay-gateway" ]; then
    cd "$ENTERPRISE_DIR/relay-gateway"
    if ! go build ./... 2>&1; then
      echo "❌ Enterprise Relay Gateway build FAILED"
      FAIL=1
    else
      echo "✅ Enterprise Relay Gateway build passed"
      if ! go test ./internal/... -count=1 -timeout 120s 2>&1; then
        echo "❌ Enterprise Relay Gateway tests FAILED"
        FAIL=1
      else
        echo "✅ Enterprise Relay Gateway tests passed"
      fi
    fi
  fi
else
  echo "[3/4] Enterprise Server — skipped (not found)"
fi

# ── Admin 前端 ──
echo ""
echo "[4/4] Admin frontend — TypeScript type check ..."
if [ -d "$SERVER_DIR/src-admin" ]; then
  cd "$SERVER_DIR/src-admin"
  if ! npx tsc --noEmit 2>&1; then
    echo "❌ Admin frontend type check FAILED"
    FAIL=1
  else
    echo "✅ Admin frontend type check passed"
  fi
else
  echo "  skipped (not found)"
fi

echo ""
echo "=========================================="
if [ "$FAIL" -eq 0 ]; then
  echo "  All server tests passed ✅"
else
  echo "  Some server tests FAILED ❌"
fi
echo "=========================================="
exit $FAIL
