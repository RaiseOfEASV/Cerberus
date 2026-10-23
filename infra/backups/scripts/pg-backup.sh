#!/bin/sh
# Generic Postgres backup loop, shared across every standalone-host
# Postgres in this repo (currently just infra/terraform-state/) - one
# script instead of duplicating the same dump+upload logic per host.
# Mounted read-only into a sidecar `backup` service in that host's own
# docker-compose.yml rather than running as a separate deployment, to
# keep "what backs up what" obvious from reading one compose file.
set -eu

: "${PGHOST:?}" "${PGDATABASE:?}" "${PGUSER:?}" "${PGPASSWORD:?}"
: "${BACKUP_MINIO_ACCESS_KEY:?}" "${BACKUP_MINIO_SECRET_KEY:?}" "${BACKUP_MINIO_ENDPOINT:?}"
: "${BUCKET:?}" "${PREFIX:?}"

export MC_HOST_backup="https://${BACKUP_MINIO_ACCESS_KEY}:${BACKUP_MINIO_SECRET_KEY}@${BACKUP_MINIO_ENDPOINT}"

apk add --no-cache curl >/dev/null 2>&1 || true
[ -x ./mc ] || { curl -sO https://dl.min.io/client/mc/release/linux-amd64/mc && chmod +x mc; }

while true; do
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  file="/tmp/${PREFIX}-${ts}.sql.gz"
  pg_dump --no-owner | gzip >"$file"
  ./mc cp "$file" "backup/${BUCKET}/${PREFIX}/$(basename "$file")"
  rm -f "$file"
  sleep 86400
done
