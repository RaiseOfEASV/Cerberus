#!/usr/bin/env bash
# Joins THIS host (the bootstrap VM, which also runs headscale) to the
# tailnet as the subnet router for the whole VNet. Run on the bootstrap VM:
#   ssh cerberus-admin@<bootstrap-ip> 'bash -s' < infra/headscale/scripts/setup-subnet-router.sh
# Idempotent: safe to re-run; skips steps already done.
set -euo pipefail

# The compose service label, not `docker compose ps` - compose refuses to
# run anything here without the stack's backup env vars set.
C=$(docker ps -q --filter label=com.docker.compose.service=headscale)
[ -n "$C" ] || { echo "headscale container not running"; exit 1; }
hs() { docker exec "$C" headscale "$@" 2>&1 | grep -v "updated version" || true; }

docker exec "$C" headscale users list -o json 2>/dev/null | grep -q '"name": *"cerberus"' \
  || hs users create cerberus

if ! command -v tailscale >/dev/null; then
  curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg | sudo tee /usr/share/keyrings/tailscale-archive-keyring.gpg >/dev/null
  curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/jammy.tailscale-keyring.list | sudo tee /etc/apt/sources.list.d/tailscale.list >/dev/null
  sudo apt-get update -qq >/dev/null
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq tailscale >/dev/null
fi

# Docker already sets this at runtime, but a subnet router shouldn't depend
# on another daemon having started first.
echo "net.ipv4.ip_forward = 1" | sudo tee /etc/sysctl.d/99-tailscale-subnet-router.conf >/dev/null
sudo sysctl -q -p /etc/sysctl.d/99-tailscale-subnet-router.conf

if tailscale status >/dev/null 2>&1; then
  echo "already joined"
else
  # Short-lived, single-use key, passed via file - never printed, never on
  # a command line. tag:router comes from the key itself (forced tag).
  umask 077
  KEYF=$(mktemp)
  trap 'shred -u "$KEYF" 2>/dev/null || true' EXIT
  docker exec "$C" headscale preauthkeys create --user cerberus --expiration 10m --tags tag:router -o json 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["key"])' > "$KEYF"
  # 127.0.0.1, not the public IP: headscale runs on this same host.
  sudo tailscale up \
    --login-server=http://127.0.0.1:8080 \
    --auth-key="file:$KEYF" \
    --advertise-routes=10.0.0.0/16 \
    --advertise-tags=tag:router \
    --accept-dns=false \
    --hostname=cerberus-router
fi

tailscale status
hs routes list   # 10.0.0.0/16 should show Enabled=true (autoApprovers in acl-policy.hujson)
