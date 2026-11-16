#!/usr/bin/env bash
# Container-level checks for the vuln-web image (nginx config).
# Usage: apps/vuln-web/docker/image-test.sh [image]   (default: builds vuln-web:image-test)
# Runs the container like Kubernetes does: read-only root, no capabilities.
set -uo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
image="${1:-}"
if [ -z "$image" ]; then
  image=vuln-web:image-test
  docker build -q -t "$image" "$here" >/dev/null || { echo "FAIL build"; exit 1; }
fi
name="vuln-web-image-test-$$"
port=$((20000 + RANDOM % 20000))
docker run -d --name "$name" --read-only --cap-drop ALL --tmpfs /tmp --tmpfs /var/cache/nginx \
  -p "$port:8080" "$image" >/dev/null || { echo "FAIL run"; exit 1; }
trap 'docker rm -f "$name" >/dev/null 2>&1' EXIT
for _ in $(seq 1 40); do curl -sf "localhost:$port/healthz" >/dev/null && break; sleep 0.25; done

fail=0
hdr() { curl -s -D - -o /dev/null "localhost:$port$1" | tr -d '\r' | grep -i "^$2:" | head -1 | cut -d' ' -f2-; }
code() { curl -s -o /dev/null -w '%{http_code}' "localhost:$port$1"; }
expect() { if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want [$2] got [$3]"; fail=1; fi; }
contains() { case "$3" in *"$2"*) echo "PASS $1";; *) echo "FAIL $1: [$3] lacks [$2]"; fail=1;; esac; }

asset=$(curl -s "localhost:$port/" | grep -oE '/assets/[^"]+\.js' | head -1)
expect   "index.html is no-cache"         "no-cache"  "$(hdr / Cache-Control)"
expect   "SPA route is no-cache"          "no-cache"  "$(hdr /findings/12 Cache-Control)"
expect   "SPA route serves the app"       "200"       "$(code /findings/12)"
expect   "missing asset is 404"           "404"       "$(code /assets/index-doesnotexist.js)"
contains "hashed asset is immutable"      "immutable" "$(hdr "$asset" Cache-Control)"
expect   "no env-config.js shipped"       "no-cache"  "$(hdr /env-config.js Cache-Control)" # falls back to index.html
for p in / /findings/12 "$asset"; do
  contains "CSP on $p"                     "default-src 'self'"     "$(hdr "$p" Content-Security-Policy)"
  contains "CSP forbids framing on $p"     "frame-ancestors 'none'" "$(hdr "$p" Content-Security-Policy)"
  expect   "nosniff on $p"                 "nosniff"                "$(hdr "$p" X-Content-Type-Options)"
  expect   "referrer-policy on $p"         "same-origin"            "$(hdr "$p" Referrer-Policy)"
done
if curl -s "localhost:$port/" | grep -q '<script>'; then echo "FAIL inline script in index.html"; fail=1; else echo "PASS no inline script"; fi
exit $fail
