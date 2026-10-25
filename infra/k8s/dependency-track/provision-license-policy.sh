#!/usr/bin/env bash
# Defines a license compliance policy in Dependency-Track via its REST
# API - the other half of Epic 10's license compliance item. Trivy
# (trivy.yaml, pr-validate.yml) gates NEW code at PR time; this gates
# everything already shipped, continuously, using the SBOM data
# Dependency-Track already ingests from every build (Epic 7) - one
# license list, enforced at two different points in time, not two
# separate reporting paths.
#
# Same forbidden-license list as /trivy.yaml, deliberately kept in sync -
# if one changes, change the other, or the two systems start disagreeing
# about what's actually allowed.
#
# CANNOT RUN YET: needs a live Dependency-Track instance (Epic 7) and an
# API key (infra/k8s/dependency-track/README.md step 5 - reuses that
# credential, doesn't invent a new one).
#
# UNVERIFIED against a live instance - the exact policy/condition REST
# schema should be double-checked against <DT_URL>/api/swagger.json for
# whatever version is actually deployed before relying on this. Same
# caliber of caveat as Epic 2's Nexus provision.sh.

set -euo pipefail

: "${DT_URL:?set DT_URL, e.g. http://dt-apiserver.dependency-track.svc:8080}"
: "${DT_API_KEY:?set DT_API_KEY}"

auth=(-H "X-Api-Key: ${DT_API_KEY}")

echo "Creating policy: forbidden-copyleft-licenses"
policy_uuid=$(curl -sf "${auth[@]}" -X PUT "${DT_URL}/api/v1/policy" \
  -H "Content-Type: application/json" \
  -d '{"name": "forbidden-copyleft-licenses", "operator": "ANY", "violationState": "FAIL"}' \
  | jq -r '.uuid')

echo "Policy UUID: ${policy_uuid}"

for license_id in GPL-2.0-only GPL-2.0-or-later GPL-3.0-only GPL-3.0-or-later AGPL-3.0-only AGPL-3.0-or-later; do
  echo "Adding condition: ${license_id}"
  curl -sf "${auth[@]}" -X PUT "${DT_URL}/api/v1/policy/${policy_uuid}/condition" \
    -H "Content-Type: application/json" \
    -d "{\"subject\": \"LICENSE\", \"operator\": \"IS\", \"value\": \"${license_id}\"}" \
    >/dev/null
done

echo "Done. The license list above (GPL/AGPL) is a common default starting"
echo "point - NOT a legal determination for this project. Review it with"
echo "whoever actually owns licensing decisions before treating a"
echo "violation as authoritative."
