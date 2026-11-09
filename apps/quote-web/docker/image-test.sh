#!/usr/bin/env bash
# Container-level checks for the quote-web image (nginx config + entrypoint).
# Usage: apps/quote-web/docker/image-test.sh [image]   (default: builds quote-web:image-test)
# Runs the container the way Kubernetes does: read-only root FS, no
# capabilities, emptyDir-like tmpfs for the writable paths.
set -uo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
image="${1:-}"
if [ -z "$image" ]; then
  image=quote-web:image-test
  docker build -q -t "$image" "$here" >/dev/null || { echo "FAIL build"; exit 1; }
fi
name="quote-web-image-test-$$"
port=$((20000 + RANDOM % 20000))
docker run -d --name "$name" --read-only --cap-drop ALL \
  --tmpfs /tmp --tmpfs /var/cache/nginx --tmpfs /run/quote-web:uid=101,gid=101 \
  -e API_URL=https://api.example.test -p "$port:8080" "$image" >/dev/null || { echo "FAIL run"; exit 1; }
trap 'docker rm -f "$name" >/dev/null 2>&1' EXIT
for _ in $(seq 1 40); do curl -sf "localhost:$port/healthz" >/dev/null && break; sleep 0.25; done

fail=0
hdr() { curl -s -D - -o /dev/null "localhost:$port$1" | tr -d '\r' | grep -i "^$2:" | head -1 | cut -d' ' -f2-; }
code() { curl -s -o /dev/null -w '%{http_code}' "localhost:$port$1"; }
expect() { # name want got
  if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want [$2] got [$3]"; fail=1; fi
}
contains() { # name needle haystack
  case "$3" in *"$2"*) echo "PASS $1";; *) echo "FAIL $1: [$3] lacks [$2]"; fail=1;; esac
}

asset=$(curl -s "localhost:$port/" | grep -oE '/assets/[^"]+\.js' | head -1)

# A cached index.html pointing at bundles from a previous deploy is what
# produces a blank page - so the HTML must always be revalidated.
expect   "index.html is no-cache"            "no-cache" "$(hdr / Cache-Control)"
expect   "SPA route is no-cache"             "no-cache" "$(hdr /some/route Cache-Control)"
expect   "SPA route still serves the app"    "200"      "$(code /some/route)"
# A missing hashed asset must be a 404, never index.html with a 200.
expect   "missing asset is 404"              "404"      "$(code /assets/index-doesnotexist.js)"
contains "hashed asset is immutable"         "immutable" "$(hdr "$asset" Cache-Control)"
expect   "env-config.js is no-store"         "no-store" "$(hdr /env-config.js Cache-Control)"
contains "env-config.js has runtime API_URL" "https://api.example.test" "$(curl -s "localhost:$port/env-config.js")"
# Location-level add_header replaces server-level headers - check they survive.
for p in / /some/route "$asset" /env-config.js; do
  expect "nosniff on $p"         "nosniff"                         "$(hdr "$p" X-Content-Type-Options)"
  expect "referrer-policy on $p" "strict-origin-when-cross-origin" "$(hdr "$p" Referrer-Policy)"
done
exit $fail
