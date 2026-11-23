#!/usr/bin/env bash
# Installs and enrolls a Wazuh agent on an Ubuntu/Debian VM (amd64), pointed at
# the Wazuh Cloud environment. The enrollment password goes in on stdin so it
# never shows up in argv, the process list or shell history:
#
#   scp infra/wazuh/install-agent.sh cerberus-admin@<vm>:/tmp/
#   printf '%s\n' "$PASSWORD" | ssh cerberus-admin@<vm> \
#     'sudo bash /tmp/install-agent.sh htwuxp6amhh2.cloud.wazuh.com; rm /tmp/install-agent.sh'
#
# The password is the WAZUH_REGISTRATION_PASSWORD from the Wazuh dashboard's
# "Deploy new agent" wizard, kept in Vault at wazuh/cloud.
#
# - Installs from Wazuh's signed apt repository (signature checked), not a raw .deb.
# - Pins and holds WAZUH_VERSION: an agent must never be newer than the manager,
#   and unattended-upgrades would otherwise move it.
# - Agent name = hostname.
# - Adds the host logs worth having during the pentest: auditd, the k3s API
#   audit log (k3s node only) and Docker container logs (bootstrap VM only:
#   headscale, terraform-state, MinIO).
set -euo pipefail

MANAGER="${1:?usage: install-agent.sh <manager-fqdn>  (enrollment password on stdin)}"
WAZUH_VERSION="${WAZUH_VERSION:-4.14.9-1}"
GROUP="${WAZUH_AGENT_GROUP:-default}"
IFS= read -r PASSWORD
[ -n "$PASSWORD" ] || { echo "no enrollment password on stdin" >&2; exit 1; }

if [ "$(id -u)" -ne 0 ]; then echo "run as root (sudo)" >&2; exit 1; fi

install -d -m 755 /usr/share/keyrings
curl -fsSL https://packages.wazuh.com/key/GPG-KEY-WAZUH | gpg --dearmor --yes -o /usr/share/keyrings/wazuh.gpg
echo "deb [signed-by=/usr/share/keyrings/wazuh.gpg] https://packages.wazuh.com/4.x/apt/ stable main" \
  > /etc/apt/sources.list.d/wazuh.list
apt-get update -qq

WAZUH_MANAGER="$MANAGER" WAZUH_REGISTRATION_PASSWORD="$PASSWORD" \
  WAZUH_AGENT_GROUP="$GROUP" WAZUH_AGENT_NAME="$(hostname)" \
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "wazuh-agent=${WAZUH_VERSION}"
apt-mark hold wazuh-agent >/dev/null

CONF=/var/ossec/etc/ossec.conf
add_localfile() { # $1 = format, $2 = location
  grep -qF "<location>$2</location>" "$CONF" && return 0
  sed -i "0,/<\/ossec_config>/s||  <localfile>\n    <log_format>$1</log_format>\n    <location>$2</location>\n  </localfile>\n</ossec_config>|" "$CONF"
}
add_localfile audit /var/log/audit/audit.log
[ -d /var/log/kubernetes/audit ] && add_localfile json /var/log/kubernetes/audit/audit.log
[ -d /var/lib/docker/containers ] && add_localfile json '/var/lib/docker/containers/*/*-json.log'

systemctl daemon-reload
systemctl enable --now wazuh-agent >/dev/null
systemctl restart wazuh-agent

for _ in $(seq 1 30); do
  grep -q "Connected to the server" /var/ossec/logs/ossec.log && break
  sleep 2
done
grep -E "Connected to the server|Valid key received|ERROR" /var/ossec/logs/ossec.log | tail -n 3
systemctl is-active wazuh-agent
