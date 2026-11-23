#!/usr/bin/env bash
# Imports the Cerberus dashboards from grafana.com (pinned revisions) into the
# Grafana Cloud stack's "Cerberus" folder, with the stack's Prometheus and our
# label values preselected. The data comes from Grafana Alloy's local
# pipelines (infra/helm/values/grafana-alloy-values.yaml).
#
#   GF_URL=https://tenderraccoon360.grafana.net GF_TOKEN=<service account token> \
#     infra/grafana/import-dashboards.sh
#
# GF_TOKEN: Vault secret/grafana/cloud api_token (service account "cerberus",
# Editor). Re-running overwrites the same dashboards (same uids).
set -euo pipefail
: "${GF_URL:?}" "${GF_TOKEN:?}"
DS_UID="${DS_UID:-grafanacloud-prom}"
auth=(-H "Authorization: Bearer ${GF_TOKEN}" -H "Content-Type: application/json")

# id:revision  title
DASHBOARDS=(
  "1860:46"   # Node Exporter Full: both VMs (job integrations/node_exporter)
  "15757:43"  # Kubernetes / Views / Global: cluster-wide CPU, memory, pods, restarts
  "15760:41"  # Kubernetes / Views / Pods: per-pod CPU, memory, network, restarts, OOM
  "17346:9"   # Traefik Official Standalone: requests, status codes, latency per entrypoint/service
)

curl -fsS "${auth[@]}" "${GF_URL}/api/folders/cerberus" >/dev/null 2>&1 ||
  curl -fsS -X POST "${auth[@]}" "${GF_URL}/api/folders" -d '{"title":"Cerberus","uid":"cerberus"}' >/dev/null

for entry in "${DASHBOARDS[@]}"; do
  id="${entry%%:*}"; rev="${entry##*:}"
  curl -fsS "https://grafana.com/api/dashboards/${id}/revisions/${rev}/download" |
    python3 -c '
import json, sys
ds_uid = sys.argv[1]
d = json.load(sys.stdin)
d["id"] = None
defaults = {
    # datasource variables (names differ per dashboard)
    "ds_prometheus": ds_uid, "datasource": ds_uid, "DS_PROMETHEUS": ds_uid,
    "cluster": "cerberus-k3s",
}
for t in d.get("templating", {}).get("list", []):
    if t["name"] in defaults:
        t["current"] = {"text": defaults[t["name"]], "value": defaults[t["name"]]}
    if t["name"] == "job" and "node_" in json.dumps(t.get("query", "")):
        t["current"] = {"text": "integrations/node_exporter", "value": "integrations/node_exporter"}
# Dashboards exported with __inputs reference ${DS_PROMETHEUS} directly.
raw = json.dumps(d).replace("${DS_PROMETHEUS}", ds_uid)
# The Traefik dashboard assumes a single Traefik; there are two (admin and
# public, same service names), which breaks "Top slow services" with a
# duplicate labelset. Add an instance selector and filter every query on it.
if d.get("title", "").startswith("Traefik"):
    import re
    flt = "instance=~\\\"$instance\\\""
    raw = re.sub(r"(traefik_[a-z_]+)\{", lambda m: m.group(1) + "{" + flt + ",", raw)
    raw = re.sub(r"(traefik_[a-z_]+)(?![a-z_{])", lambda m: m.group(1) + "{" + flt + "}", raw)
    # Its label_replace keeps only the first two dash-separated parts of the
    # service name, written for another naming scheme. Traefik Kubernetes
    # CRD services are <namespace>-<name>-<hash>@kubernetescrd, so two routes
    # of one app collapse into one name (duplicate labelset again). Strip
    # only the @provider suffix instead.
    raw = raw.replace("([^-]+-[^-]+).*", "([^@]+)@.*")
    d = json.loads(raw)
    d["templating"]["list"].insert(1, {
        "name": "instance", "label": "Traefik", "type": "query",
        "datasource": {"type": "prometheus", "uid": ds_uid},
        "query": {"query": "label_values(traefik_open_connections, instance)", "refId": "instance"},
        "definition": "label_values(traefik_open_connections, instance)",
        "current": {"text": "traefik-public", "value": "traefik-public"},
        "refresh": 1, "includeAll": False, "multi": False, "sort": 1,
    })
else:
    d = json.loads(raw)
d.pop("__inputs", None)
d["tags"] = sorted(set(d.get("tags", [])) | {"cerberus"})
print(json.dumps({"dashboard": d, "folderUid": "cerberus", "overwrite": True}))
' "${DS_UID}" |
    curl -fsS -X POST "${auth[@]}" "${GF_URL}/api/dashboards/db" -d @- |
    python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], d["uid"], d["url"])'
done
