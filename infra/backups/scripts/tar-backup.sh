#!/bin/sh
# Generic directory backup loop, shared across every standalone host that
# needs file-based (not database) backup: headscale (sqlite + config),
# Wazuh (indexer/manager data volumes). Not point-in-time consistent -
# it's a live tar of whatever's on disk, which is a real limitation for a
# running database-like store (Wazuh's indexer especially). Acceptable
# for now, flagged - see infra/wazuh/README.md and infra/backups/README.md
# for the "move to native snapshot APIs later" note.
set -eu

: "${SRC_DIR:?}"
: "${BACKUP_MINIO_ACCESS_KEY:?}" "${BACKUP_MINIO_SECRET_KEY:?}" "${BACKUP_MINIO_ENDPOINT:?}"
: "${BUCKET:?}" "${PREFIX:?}"

export MC_HOST_backup="https://${BACKUP_MINIO_ACCESS_KEY}:${BACKUP_MINIO_SECRET_KEY}@${BACKUP_MINIO_ENDPOINT}"

apk add --no-cache curl tar gzip >/dev/null 2>&1 || true
[ -x ./mc ] || { curl -sO https://dl.min.io/client/mc/release/linux-amd64/mc && chmod +x mc; }

while true; do
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  file="/tmp/${PREFIX}-${ts}.tar.gz"
  tar -czf "$file" -C "$SRC_DIR" .
  ./mc cp "$file" "backup/${BUCKET}/${PREFIX}/$(basename "$file")"
  rm -f "$file"
  sleep 86400
done
