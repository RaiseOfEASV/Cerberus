#!/usr/bin/env bash
# Render tests for the generic app chart. Run from anywhere:
#   infra/helm/charts/app/tests/render-test.sh
# Needs helm and python3 (with PyYAML).
set -euo pipefail
cd "$(dirname "$0")/.."
REPO=../../../..
fail=0
check() { if eval "$2"; then echo "ok   - $1"; else echo "FAIL - $1"; fail=1; fi; }

# 1. Existing apps render byte-identical to before the optional features existed.
if [ -f "$REPO/infra/helm/quote-api/values-prod.yaml" ]; then
  check "quote-api prod unchanged (golden)" \
    'diff -u tests/quote-api-prod.golden.yaml <(helm template quote-api . -n cerberoast -f $REPO/infra/helm/quote-api/values-prod.yaml)'
fi

# 2. Every optional feature renders as specified.
helm template vuln-api . -n dev -f tests/full-values.yaml > /tmp/render-full.$$.yaml
check "full fixture assertions" 'python3 - /tmp/render-full.$$.yaml <<"EOF"
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
kinds = {d["kind"]: d for d in docs}
sa = kinds["ServiceAccount"]
assert sa["metadata"]["name"] == "vuln-api", sa
assert sa.get("automountServiceAccountToken") is False
pod = kinds["Deployment"]["spec"]["template"]["spec"]
assert pod["serviceAccountName"] == "vuln-api"
assert pod["automountServiceAccountToken"] is False
app = pod["containers"][0]
assert [e["secretRef"]["name"] for e in app["envFrom"]] == ["vuln-api-oidc", "vuln-api-db-app"], app["envFrom"]
vols = {v["name"]: v for v in pod["volumes"]}
src = vols["vault-token"]["projected"]["sources"][0]["serviceAccountToken"]
assert src == {"audience": "vault", "expirationSeconds": 600, "path": "token"}, src
assert vols["cerberus-ca"]["configMap"]["name"] == "cerberus-internal-ca"
mounts = {m["name"]: m for m in app["volumeMounts"]}
assert mounts["vault-token"]["mountPath"] == "/var/run/secrets/vault" and mounts["vault-token"]["readOnly"]
assert mounts["cerberus-ca"]["mountPath"] == "/etc/cerberus" and mounts["cerberus-ca"]["readOnly"]
init = pod["initContainers"][0]
assert init["image"] == app["image"]
assert init["args"] == ["migrate"]
assert [e["secretRef"]["name"] for e in init["envFrom"]] == ["vuln-api-db-owner"]
assert init["securityContext"] == app["securityContext"]
assert init["resources"] == app["resources"]
assert "envFrom" not in init or all(e["secretRef"]["name"] != "vuln-api-oidc" for e in init["envFrom"]), "app secrets leaked into init"
EOF'
rm -f /tmp/render-full.$$.yaml

# 3. initContainer without args is a render error with a clear message.
out=$(helm template x . -f tests/full-values.yaml --set 'initContainer.args=null' 2>&1 || true)
check "initContainer.args required" 'grep -q "initContainer.args is required" <<<"$out"'

exit $fail
