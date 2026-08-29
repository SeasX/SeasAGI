#!/bin/bash
set -euo pipefail

DATA_DIR="${HOME}/.seasagi"
RETENTION_DAYS=30

echo "=== SeasAGI Trace Cleanup ==="

for db in "${DATA_DIR}"/relay-gateway.db "${DATA_DIR}"/platform-api.db; do
    [[ -f "${db}" ]] || continue
    echo "  Cleaning ${db} trace_records older than ${RETENTION_DAYS} days..."
    sqlite3 "${db}" "DELETE FROM trace_records WHERE created_at < datetime('now', '-${RETENTION_DAYS} days');"
    sqlite3 "${db}" "VACUUM;"
    echo "  Done."
done

echo "=== Trace cleanup complete ==="
