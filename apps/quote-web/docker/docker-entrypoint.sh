#!/bin/sh
# Writes the runtime config the SPA reads (window.__ENV__) from API_URL, so
# the same image is promoted dev -> staging -> prod without a rebuild.
set -eu

API_URL="${API_URL:-}"
case "$API_URL" in
  *'"'* | *'\'* | *'<'* | *'
'*)
    echo "API_URL contains characters that are not allowed in a URL here" >&2
    exit 1
    ;;
esac

cat > /run/quote-web/env-config.js <<CONFIG
window.__ENV__ = { API_URL: "${API_URL}" };
CONFIG

exec nginx -g 'daemon off;'
