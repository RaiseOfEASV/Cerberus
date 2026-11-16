#!/usr/bin/env python3
"""Static checks for VulnMan's deployment config - each guards a defect the
final review found (plan docs/superpowers/plans/2026-11-16-vulnman.md).
Run: python3 tools/check-vulnman-config.py   (needs PyYAML and kubectl)"""
import glob, os, re, shutil, subprocess, sys, tempfile, yaml

fail = 0
def check(name, ok):
    global fail
    print(("ok   - " if ok else "FAIL - ") + name)
    fail |= not ok

# C1: keycloak-config-cli substitutes $(env:VAR), never ${VAR}.
for f in sorted(glob.glob("infra/keycloak-public/realm-*.yaml")):
    text = open(f).read()
    check(f"{f}: no literal ${{...}} secrets", "${" not in text)
    check(f"{f}: secrets use $(env:...)", len(re.findall(r"secret: \"\$\(env:VULN_[A-Z]+_(OIDC|ADMIN)_SECRET\)\"", text)) == 2)
    realm = yaml.safe_load(text)
    login = next(c for c in realm["clients"] if c["clientId"] == "vuln-api")
    scopes = login.get("defaultClientScopes", [])
    # C2: 'basic' carries sub (Keycloak >= 25); 'openid' is not a client scope.
    check(f"{f}: login client has basic+roles scopes, no 'openid'", "basic" in scopes and "roles" in scopes and "openid" not in scopes)
    # I3: role lookups (GET /roles/{r}, /roles/{r}/users) need view-realm.
    sa = next(u for u in realm["users"] if u.get("serviceAccountClientId") == "vuln-api-admin")
    check(f"{f}: admin service account can view realm roles", "view-realm" in sa["clientRoles"]["realm-management"])

# C3: every vulnman.yaml must build with kustomize, as ArgoCD will.
for f in ["infra/k8s/environments/dev/vulnman.yaml", "infra/k8s/environments/staging/vulnman.yaml", "infra/k8s/cerberoast/vulnman.yaml"]:
    with tempfile.TemporaryDirectory() as d:
        shutil.copy(f, os.path.join(d, "vulnman.yaml"))  # kustomize refuses symlinks leaving its root
        open(os.path.join(d, "kustomization.yaml"), "w").write("resources: [vulnman.yaml]\n")
        r = subprocess.run(["kubectl", "kustomize", d], capture_output=True, text=True)
    check(f"{f}: kustomize builds", r.returncode == 0)

# I2: public Traefik may reach app ports in cerberoast, never Postgres.
for doc in yaml.safe_load_all(open("infra/k8s/cerberoast/network-policy.yaml")):
    if doc and doc["metadata"]["name"] == "allow-traefik-public-ingress":
        ports = [p.get("port") for rule in doc["spec"]["ingress"] for p in rule.get("ports", [])]
        check("cerberoast: traefik-public ingress limited to 8080", ports == [8080])

# I1: behind Cloudflare, vuln-api must trust Cloudflare's ranges to find the client IP.
prod = open("infra/helm/vuln-api/values-prod.yaml").read()
check("vuln-api prod trusts Cloudflare ranges for X-Forwarded-For", "173.245.48.0/20" in prod and "10.42.0.0/16" in prod)
tp = yaml.safe_load(open("infra/helm/values/traefik-public-values.yaml"))
check("traefik-public keeps client IPs (externalTrafficPolicy Local)", tp.get("service", {}).get("spec", {}).get("externalTrafficPolicy") == "Local")
trusted = tp.get("ports", {}).get("websecure", {}).get("forwardedHeaders", {}).get("trustedIPs", [])
check("traefik-public trusts Cloudflare's X-Forwarded-For", "173.245.48.0/20" in trusted)

sys.exit(fail)
