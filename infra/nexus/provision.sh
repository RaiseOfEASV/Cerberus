#!/usr/bin/env bash
# Provisions Nexus's proxy repositories via its REST API - "code-defined, not
# click-ops" for repo config, without betting on a community Terraform
# provider's exact resource schema (datadrivers/nexus3 exists but its schema
# moves between versions; the REST API below is the stable, documented
# contract Sonatype keeps backwards-compatible).
#
# CANNOT RUN YET: needs a live Nexus instance (Epic 4 ships the cluster that
# runs infra/helm/values/nexus-values.yaml first). Admin credential sourcing
# is SOLVED IN CODE as of Epic 6: infra/k8s/eso/external-secret-nexus-admin.yaml
# syncs NEXUS_USER/NEXUS_PASS from Vault into a Secret named
# nexus-admin-credentials in the nexus namespace - run this script as a
# one-off Job with `envFrom: secretRef: {name: nexus-admin-credentials}`
# instead of a human exporting the vars by hand. Still blocked on an
# actual deployed cluster+Vault+Nexus, not on a missing design.
#
# NOTE: the exact JSON fields for the helm/proxy endpoint in particular
# should be double-checked against your Nexus version's own API docs
# (<NEXUS_URL>/service/rest/swagger.json) before relying on this - written
# without a live instance to verify against.
#
# Idempotent: checks if a repo already exists before creating it.

set -euo pipefail

: "${NEXUS_URL:?set NEXUS_URL, e.g. http://nexus.nexus.svc.cluster.local:8081}"
: "${NEXUS_USER:?set NEXUS_USER}"
: "${NEXUS_PASS:?set NEXUS_PASS}"

auth=(-u "${NEXUS_USER}:${NEXUS_PASS}")

repo_exists() {
    curl -sf "${auth[@]}" "${NEXUS_URL}/service/rest/v1/repositories/$1" >/dev/null 2>&1
}

create_repo() {
    local format="$1" type="$2" name="$3" body="$4"
    if repo_exists "$name"; then
        echo "skip: $name already exists"
        return
    fi
    echo "creating: $name ($format/$type)"
    curl -sf "${auth[@]}" -X POST \
        -H "Content-Type: application/json" \
        -d "$body" \
        "${NEXUS_URL}/service/rest/v1/repositories/${format}/${type}"
}

create_repo go proxy go-proxy '{
  "name": "go-proxy",
  "online": true,
  "storage": {"blobStoreName": "default", "strictContentTypeValidation": true},
  "proxy": {"remoteUrl": "https://proxy.golang.org", "contentMaxAge": 1440, "metadataMaxAge": 1440},
  "negativeCache": {"enabled": true, "timeToLive": 1440},
  "httpClient": {"blocked": false, "autoBlock": true}
}'

create_repo npm proxy npm-proxy '{
  "name": "npm-proxy",
  "online": true,
  "storage": {"blobStoreName": "default", "strictContentTypeValidation": true},
  "proxy": {"remoteUrl": "https://registry.npmjs.org", "contentMaxAge": 1440, "metadataMaxAge": 1440},
  "negativeCache": {"enabled": true, "timeToLive": 1440},
  "httpClient": {"blocked": false, "autoBlock": true}
}'

# Pull-through cache for public upstream images our base images FROM
# (distroless, etc). This is NOT where our own built images live - those
# are GHCR, the registry of record. This just saves re-pulling the same
# public upstream layers from Docker Hub on every build.
create_repo docker proxy docker-hub-proxy '{
  "name": "docker-hub-proxy",
  "online": true,
  "storage": {"blobStoreName": "default", "strictContentTypeValidation": true},
  "proxy": {"remoteUrl": "https://registry-1.docker.io", "contentMaxAge": 1440, "metadataMaxAge": 1440},
  "negativeCache": {"enabled": true, "timeToLive": 1440},
  "httpClient": {"blocked": false, "autoBlock": true},
  "docker": {"v1Enabled": false, "forceBasicAuth": true, "httpPort": 8082},
  "dockerProxy": {"indexType": "HUB", "useTrustStoreForIndexAccess": false}
}'

create_repo helm proxy helm-proxy '{
  "name": "helm-proxy",
  "online": true,
  "storage": {"blobStoreName": "default", "strictContentTypeValidation": true},
  "proxy": {"remoteUrl": "https://charts.helm.sh/stable", "contentMaxAge": 1440, "metadataMaxAge": 1440},
  "negativeCache": {"enabled": true, "timeToLive": 1440},
  "httpClient": {"blocked": false, "autoBlock": true}
}'

echo "Done. NOTE: Nexus OSS has no first-class Terraform-provider-grade"
echo "mirroring for Terraform providers themselves - that gap is unresolved,"
echo "see the baseline plan."
