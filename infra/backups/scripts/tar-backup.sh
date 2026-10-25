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

# http, not https - the MinIO server in infra/backups/docker-compose.yml
# has no TLS cert configured, same "no TLS on standalone hosts yet" gap
# as headscale's server_url.
export MC_HOST_backup="http://${BACKUP_MINIO_ACCESS_KEY}:${BACKUP_MINIO_SECRET_KEY}@${BACKUP_MINIO_ENDPOINT}"

apk add --no-cache curl tar gzip >/dev/null 2>&1 || true
# dl.min.io now returns 410 Gone (MinIO stopped publishing client
# binaries there, same abandonment as its Docker Hub images - see
# infra/backups/docker-compose.yml's header). GitHub release assets
# survived it; pinned by release tag + checksum-verified, not `latest`.
MC_VERSION=RELEASE.2025-08-13T08-35-41Z
MC_URL="https://github.com/minio/mc/releases/download/${MC_VERSION}/mc.linux-amd64.${MC_VERSION}"
if [ ! -x ./mc ]; then
  curl -sL -o mc "$MC_URL"
  expected="$(curl -sL "${MC_URL}.sha256sum" | awk '{print $1}')"
  actual="$(sha256sum mc | awk '{print $1}')"
  [ "$expected" = "$actual" ] || { echo "mc checksum mismatch - aborting" >&2; exit 1; }
  chmod +x mc
fi

while true; do
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  file="/tmp/${PREFIX}-${ts}.tar.gz"
  tar -czf "$file" -C "$SRC_DIR" .
  ./mc cp "$file" "backup/${BUCKET}/${PREFIX}/$(basename "$file")"
  rm -f "$file"
  sleep 86400
done
