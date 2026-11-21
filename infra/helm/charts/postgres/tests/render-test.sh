#!/usr/bin/env bash
# Render tests for the postgres chart: infra/helm/charts/postgres/tests/render-test.sh
set -euo pipefail
cd "$(dirname "$0")/.."
f=$(mktemp); trap 'rm -f $f' EXIT
helm template vuln-db . -n dev -f tests/values-test.yaml > "$f"
python3 - "$f" <<'EOF'
import sys, yaml
docs=[d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
by={(d["kind"],d["metadata"]["name"]):d for d in docs}
ok=True
def check(name, cond):
    global ok
    print(("ok   - " if cond else "FAIL - ")+name); ok = ok and cond
sts=by.get(("StatefulSet","vuln-db"))
check("statefulset exists", sts is not None)
if sts:
    pod=sts["spec"]["template"]["spec"]; c=pod["containers"][0]
    check("runs as uid 70", pod["securityContext"]["runAsUser"]==70 and pod["securityContext"]["runAsNonRoot"])
    check("read-only rootfs", c["securityContext"]["readOnlyRootFilesystem"] is True)
    mounts={m["mountPath"] for m in c["volumeMounts"]}
    check("writable run+tmp+data", {"/var/run/postgresql","/tmp","/var/lib/postgresql/data"} <= mounts)
    check("PGDATA under PVC", {"name":"PGDATA","value":"/var/lib/postgresql/data/pgdata"} in c["env"])
    check("limits set", all(c["resources"][k].get(r) for k in ("requests","limits") for r in ("cpu","memory")))
    check("initdb roles script mounted", "/docker-entrypoint-initdb.d" in mounts)
cm=by.get(("ConfigMap","vuln-db-initdb"))
check("initdb creates owner+app roles", cm is not None and "vulnman_owner" in str(cm["data"]) and "vulnman_app" in str(cm["data"]))
check("initdb revokes PUBLIC connect on postgres db", cm is not None and "REVOKE CONNECT ON DATABASE postgres FROM PUBLIC" in str(cm["data"]))
np=by.get(("NetworkPolicy","vuln-db"))
if np:
    peers=np["spec"]["ingress"][0]["from"]
    check("netpol: exactly 2 ingress peers", len(peers)==2)
    check("netpol: client selector", {"podSelector":{"matchLabels":{"app.kubernetes.io/name":"vuln-api"}}} in peers)
else: check("networkpolicy exists", False)
es={d["metadata"]["name"]:d for d in docs if d["kind"]=="ExternalSecret"}
check("credentials ExternalSecret from vault path", es.get("vuln-db-credentials",{}).get("spec",{}).get("data",[{}])[0].get("remoteRef",{}).get("key")=="vulnman/dev/db")
appurl=es.get("vuln-db-app-url",{}).get("spec",{}).get("target",{}).get("template",{}).get("data",{}).get("DATABASE_URL","")
check("app url uses vulnman_app, literal ESO template", appurl.startswith("postgres://vulnman_app:{{ .app_password }}@vuln-db:5432/vulnman"))
ownurl=es.get("vuln-db-owner-url",{}).get("spec",{}).get("target",{}).get("template",{}).get("data",{}).get("MIGRATE_DATABASE_URL","")
check("owner url uses vulnman_owner", ownurl.startswith("postgres://vulnman_owner:{{ .owner_password }}@vuln-db:5432/vulnman"))
cj=by.get(("CronJob","vuln-db-backup"))
check("backup cronjob", cj is not None and cj["spec"]["schedule"]=="30 2 * * *")
if cj:
    js=cj["spec"]["jobTemplate"]["spec"]["template"]["spec"]
    imgs=[c["image"] for c in js.get("initContainers",[])+js["containers"]]
    check("backup uses chainguard minio-client, not minio/mc", any("cgr.dev/chainguard/minio-client" in i for i in imgs) and not any(i.startswith("minio/mc") for i in imgs))
    dump=[c for c in js.get("initContainers",[]) if c["name"]=="dump"][0]
    check("backup waits for the database before pg_dump (kube-router admits new pods late)", "pg_isready" in " ".join(dump["command"]))
    check("backup containers have limits", all(c["resources"]["limits"].get("cpu") and c["resources"]["limits"].get("memory") for c in js.get("initContainers",[])+js["containers"]))
sys.exit(0 if ok else 1)
EOF
