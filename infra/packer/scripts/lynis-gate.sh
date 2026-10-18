#!/usr/bin/env bash
# Build-time quality gate: fails the Packer build if the image's Lynis
# hardening index is below threshold. Runs after the Ansible provisioner,
# which installs lynis as its last step (see ansible/playbook.yml).
set -euo pipefail

threshold="${LYNIS_MIN_SCORE:-70}"

sudo lynis audit system --quick --no-colors

score="$(sudo awk -F'=' '/hardening_index=/ {print $2}' /var/log/lynis-report.dat)"

echo "Lynis hardening index: ${score} (threshold: ${threshold})"

if [ -z "${score}" ]; then
  echo "FAIL: could not read hardening_index from /var/log/lynis-report.dat" >&2
  exit 1
fi

if [ "${score}" -lt "${threshold}" ]; then
  echo "FAIL: hardening index ${score} is below threshold ${threshold}" >&2
  exit 1
fi

echo "PASS"
