#!/bin/sh
# Generic Postgres backup loop, shared across every standalone-host
# Postgres in this repo (currently just infra/terraform-state/) - one
# script instead of duplicating the same dump+upload logic per host.
# Mounted read-only into a sidecar `backup` service in that host's own
# docker-compose.yml rather than running as a separate deployment, to
# keep "what backs up what" obvious from reading one compose file.
# pipefail matters here: `pg_dump | gzip` is a pipeline, and without it
# a failed pg_dump (e.g. the DB not accepting connections yet - hit this
# for real on first deploy) would still "succeed" since gzip exits 0 on
# empty stdin, silently uploading a useless empty "backup" instead of
# failing loudly. Confirmed BusyBox ash (this image's /bin/sh) supports
# pipefail before relying on it.
set -eu -o pipefail

: "${PGHOST:?}" "${PGDATABASE:?}" "${PGUSER:?}" "${PGPASSWORD:?}"
: "${BACKUP_MINIO_ACCESS_KEY:?}" "${BACKUP_MINIO_SECRET_KEY:?}" "${BACKUP_MINIO_ENDPOINT:?}"
: "${BUCKET:?}" "${PREFIX:?}"

# http, not https - the MinIO server in infra/backups/docker-compose.yml
# has no TLS cert configured, same "no TLS on standalone hosts yet" gap
# as headscale's server_url.
export MC_HOST_backup="http://${BACKUP_MINIO_ACCESS_KEY}:${BACKUP_MINIO_SECRET_KEY}@${BACKUP_MINIO_ENDPOINT}"

apk add --no-cache curl >/dev/null 2>&1 || true
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
  file="/tmp/${PREFIX}-${ts}.sql.gz"
  pg_dump --no-owner | gzip >"$file"
  ./mc cp "$file" "backup/${BUCKET}/${PREFIX}/$(basename "$file")"
  rm -f "$file"
  sleep 86400
done
