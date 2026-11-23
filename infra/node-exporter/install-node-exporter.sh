#!/usr/bin/env bash
# Installs Prometheus node_exporter on a VM (Ubuntu, amd64) for host metrics.
# The cluster's Elastic Agent scrapes it (Fleet: Prometheus integration).
#
#   scp infra/node-exporter/install-node-exporter.sh cerberus-admin@10.0.1.4:/tmp/
#   ssh cerberus-admin@10.0.1.4 'sudo bash /tmp/install-node-exporter.sh 10.0.1.4 10.0.2.4'
#
# Args: <listen-ip> <only allowed scraper ip>
#
# - Upstream release, SHA-256 checked against the release's sha256sums.txt
#   (Ubuntu 22.04's package is 1.3.1 from 2021, built with an old Go).
# - Listens on the private IP only, never 0.0.0.0.
# - iptables drops port 9100 from anyone but the scraper (the k3s node; pod
#   traffic leaving the cluster is SNATed to the node IP). Re-applied on
#   every service start, because this VM has no persistent firewall (ufw
#   is inactive).
# - Runs as a transient DynamicUser with a read-only system and a 64 MB cap.
set -euo pipefail

LISTEN_IP="${1:?usage: install-node-exporter.sh <listen-ip> <allowed-scraper-ip>}"
SCRAPER_IP="${2:?usage: install-node-exporter.sh <listen-ip> <allowed-scraper-ip>}"
VERSION="${NODE_EXPORTER_VERSION:-1.13.1}"
PORT=9100

[ "$(id -u)" -eq 0 ] || { echo "run as root (sudo)" >&2; exit 1; }

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
base="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}"
tarball="node_exporter-${VERSION}.linux-amd64.tar.gz"
curl -fsSL -o "$tmp/$tarball" "$base/$tarball"
curl -fsSL -o "$tmp/sha256sums.txt" "$base/sha256sums.txt"
(cd "$tmp" && grep " ${tarball}\$" sha256sums.txt | sha256sum -c -)
tar -xzf "$tmp/$tarball" -C "$tmp"
install -m 0755 -o root -g root "$tmp/node_exporter-${VERSION}.linux-amd64/node_exporter" /usr/local/bin/node_exporter

rule="-p tcp --dport ${PORT} ! -s ${SCRAPER_IP} -j DROP"
cat > /etc/systemd/system/node_exporter.service <<EOF
[Unit]
Description=Prometheus node_exporter (host metrics, scraped by Elastic Agent)
After=network-online.target
Wants=network-online.target

[Service]
# "+" runs the firewall step as root; the exporter itself runs unprivileged.
ExecStartPre=+/bin/sh -c 'iptables -C INPUT ${rule} 2>/dev/null || iptables -I INPUT 1 ${rule}'
ExecStart=/usr/local/bin/node_exporter --web.listen-address=${LISTEN_IP}:${PORT}
DynamicUser=yes
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
RestrictNamespaces=yes
LockPersonality=yes
MemoryMax=64M
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable node_exporter >/dev/null
systemctl restart node_exporter
sleep 2
systemctl is-active node_exporter
/usr/local/bin/node_exporter --version 2>&1 | head -1
ss -ltn "sport = :${PORT}"
iptables -S INPUT | grep -- "--dport ${PORT}"
