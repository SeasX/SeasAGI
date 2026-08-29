#!/usr/bin/env bash
# test-all.sh — 一键运行全部测试（Client + Server）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "######################################"
echo "#   SeasAGI Full Test Suite          #"
echo "######################################"

OVERALL_FAIL=0

# ── Client ──
echo ""
echo ">>> Running test-client.sh ..."
if bash "$SCRIPT_DIR/test-client.sh"; then
  echo ">>> test-client.sh ✅"
else
  echo ">>> test-client.sh ❌"
  OVERALL_FAIL=1
fi

# ── Server ──
echo ""
echo ">>> Running test-server.sh ..."
if bash "$SCRIPT_DIR/test-server.sh"; then
  echo ">>> test-server.sh ✅"
else
  echo ">>> test-server.sh ❌"
  OVERALL_FAIL=1
fi

echo ""
echo "######################################"
if [ "$OVERALL_FAIL" -eq 0 ]; then
  echo "#   All tests passed ✅               #"
else
  echo "#   Some tests FAILED ❌              #"
fi
echo "######################################"
exit $OVERALL_FAIL
