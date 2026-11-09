#!/usr/bin/env bash
# Render checks for the generic app chart. Run: infra/helm/charts/app/tests/render-test.sh
set -uo pipefail
chart="$(cd "$(dirname "$0")/.." && pwd)"
D=sha256:3c61afc2ce5deb3d6b831f455ecaa1a90583d6461b11fb32bb5d21c515b08ed2
fail=0
img() { helm template t "$chart" --set image.repository=ghcr.io/raiseofeasv/x "$@" 2>&1 | grep -m1 'image:' | sed 's/.*image: //; s/"//g'; }
expect() { # name expected actual
  if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want [$2] got [$3]"; fail=1; fi
}
rejects() { # name args...
  local n=$1; shift
  if helm template t "$chart" --set image.repository=ghcr.io/raiseofeasv/x "$@" >/dev/null 2>&1; then
    echo "FAIL $n: rendered, want error"; fail=1; else echo "PASS $n"; fi
}
expect "tag only"          "ghcr.io/raiseofeasv/x:v1"      "$(img --set image.tag=v1)"
expect "tag plus digest"   "ghcr.io/raiseofeasv/x:v1@$D"   "$(img --set image.tag=v1 --set image.digest=$D)"
rejects "malformed digest" --set image.tag=v1 --set image.digest=sha256:nothex
rejects "latest tag"       --set image.tag=latest
rejects "missing tag"
exit $fail
