#!/usr/bin/env bash
# Runs "$@" with VULN_TEST_DATABASE_URL pointing at a throwaway Postgres 16
# (superuser). testdb creates one fresh database per test from it.
# Reuses an existing VULN_TEST_DATABASE_URL if one is already set (CI service).
set -euo pipefail
if [ -n "${VULN_TEST_DATABASE_URL:-}" ]; then exec "$@"; fi
name="vuln-test-pg-$$"
docker run -d --rm --name "$name" -e POSTGRES_PASSWORD=test -p 127.0.0.1::5432 postgres:16-alpine@sha256:1d5b02d3a9941fbd52437db4107f8c52d6297dac553ca1e5c299ebb3756b4857 >/dev/null
trap 'docker stop "$name" >/dev/null 2>&1 || true' EXIT
port=$(docker port "$name" 5432/tcp | head -1 | sed 's/.*://')
for _ in $(seq 1 60); do
  docker exec "$name" pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1 && break
  sleep 1
done
export VULN_TEST_DATABASE_URL="postgres://postgres:test@127.0.0.1:${port}/postgres?sslmode=disable"
"$@"
