#!/usr/bin/env bash
# Imports "Node Exporter Full" (grafana.com dashboard 1860, pinned revision)
# into the Grafana Cloud stack's "Cerberus" folder, defaulting to the stack's
# Prometheus and the integrations/node_exporter job that Alloy scrapes
# (infra/helm/values/grafana-alloy-values.yaml, infra/node-exporter/).
#
#   GF_URL=https://tenderraccoon360.grafana.net GF_TOKEN=<service account token> \
#     infra/grafana/import-node-exporter-dashboard.sh
#
# GF_TOKEN: Vault secret/grafana/cloud api_token (service account "cerberus",
# Editor). Re-running overwrites the same dashboard (uid rYdddlPWk).
set -euo pipefail
: "${GF_URL:?}" "${GF_TOKEN:?}"
REV="${REV:-46}"
DS_UID="${DS_UID:-grafanacloud-prom}"
auth=(-H "Authorization: Bearer ${GF_TOKEN}" -H "Content-Type: application/json")

curl -fsS "${auth[@]}" "${GF_URL}/api/folders/cerberus" >/dev/null 2>&1 ||
  curl -fsS -X POST "${auth[@]}" "${GF_URL}/api/folders" -d '{"title":"Cerberus","uid":"cerberus"}' >/dev/null

curl -fsS "https://grafana.com/api/dashboards/1860/revisions/${REV}/download" |
  python3 -c '
import json, sys
ds_uid = sys.argv[1]
d = json.load(sys.stdin)
d["id"] = None
for t in d["templating"]["list"]:
    if t["name"] == "ds_prometheus":
        t["current"] = {"text": ds_uid, "value": ds_uid}
    if t["name"] == "job":
        t["current"] = {"text": "integrations/node_exporter", "value": "integrations/node_exporter"}
d["tags"] = sorted(set(d.get("tags", [])) | {"cerberus", "node-exporter"})
print(json.dumps({"dashboard": d, "folderUid": "cerberus", "overwrite": True}))
' "${DS_UID}" |
  curl -fsS -X POST "${auth[@]}" "${GF_URL}/api/dashboards/db" -d @- |
  python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], d["url"])'
