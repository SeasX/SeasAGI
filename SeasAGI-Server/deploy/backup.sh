#!/bin/bash
set -euo pipefail

DATA_DIR="/var/lib/seasagi"
BACKUP_DIR="/var/lib/seasagi/backups"
RETENTION_DAYS=7
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${BACKUP_DIR}"

echo "=== SeasAGI Backup ${TIMESTAMP} ==="

for db in "${DATA_DIR}"/*.db; do
    [[ -f "${db}" ]] || continue
    dbname=$(basename "${db}")
    backup_file="${BACKUP_DIR}/${dbname}.${TIMESTAMP}"
    echo "  Backing up ${dbname}..."
    sqlite3 "${db}" ".backup '${backup_file}'"
    gzip "${backup_file}"
    echo "  -> ${backup_file}.gz ($(du -h "${backup_file}.gz" | cut -f1))"
done

echo ""
echo "Cleaning backups older than ${RETENTION_DAYS} days..."
find "${BACKUP_DIR}" -name "*.gz" -mtime +${RETENTION_DAYS} -delete

echo "=== Backup complete ==="
