#!/usr/bin/env bash
# test-client.sh — SeasAGI-Client 单元测试 + 编译验证
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT_DIR="$SCRIPT_DIR/SeasAGI-Client/src-app"

echo "=========================================="
echo "  SeasAGI-Client Test Suite"
echo "=========================================="

# ── Go 编译 ──
echo ""
echo "[1/4] Go build ..."
cd "$CLIENT_DIR"
if ! go build ./... 2>&1; then
  echo "❌ Go build FAILED"
  exit 1
fi
echo "✅ Go build passed"

# ── Go 单元测试 ──
echo ""
echo "[2/4] Go unit tests ..."
if ! go test ./internal/... -count=1 -timeout 120s 2>&1; then
  echo "❌ Go unit tests FAILED"
  exit 1
fi
echo "✅ Go unit tests passed"

# ── Go vet ──
echo ""
echo "[3/4] Go vet ..."
if ! go vet ./... 2>&1; then
  echo "❌ Go vet FAILED"
  exit 1
fi
echo "✅ Go vet passed"

# ── 前端 TypeScript 编译检查 ──
echo ""
echo "[4/4] TypeScript type check ..."
cd "$CLIENT_DIR/frontend"
if ! npx tsc --noEmit 2>&1; then
  echo "❌ TypeScript type check FAILED"
  exit 1
fi
echo "✅ TypeScript type check passed"

echo ""
echo "=========================================="
echo "  All client tests passed ✅"
echo "=========================================="
