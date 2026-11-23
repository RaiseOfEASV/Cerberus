# Elastic Cloud: pentest dashboard

`pentest-queries.json` holds the ES|QL queries behind the **Pentest – App
Security** dashboard. `pentest-dashboard.ndjson` is the Kibana saved-objects
export generated from them (8 saved searches and the dashboard). It also
references three saved searches created by hand in Kibana: Successful
Logins, Failed Logins and Successful Logins after Failure.

Object ids are fixed (UUIDv5), so importing again overwrites only these
objects:

```sh
curl -X POST "$KIBANA_URL/api/saved_objects/_import?overwrite=true" \
  -H "Authorization: ApiKey $KIBANA_API_KEY" -H "kbn-xsrf: true" \
  -F file=@infra/elastic/pentest-dashboard.ndjson
```

The queries read the `json.*` fields that the Kubernetes integration's
`decode_json_fields` processor adds for the app namespaces (Fleet policy),
and rely on the `logs-kubernetes.container_logs@custom` component template
that types those fields. `json.ip` is `ip` in the first backing index and
`keyword` in later ones, so the queries use `TO_STRING(json.ip)`.

## Viewer role

`pentest-viewer-role.json` is the custom role `pentest-dashboard-viewer`:
read on the container-log and audit-log data streams only, and Dashboard and
Discover in read mode. No other Kibana features, no cluster privileges, no
host logs, metrics or Fleet data. Kibana can't limit a user to a single
dashboard, so a viewer can still query those two data streams in Discover.

```sh
curl -X PUT "$KIBANA_URL/api/security/role/pentest-dashboard-viewer" \
  -H "Authorization: ApiKey $KIBANA_API_KEY" -H "kbn-xsrf: true" \
  -H "Content-Type: application/json" -d @infra/elastic/pentest-viewer-role.json
```

People are added in the Elastic Cloud console (Organization > Members >
Invite), with this custom role on the project. Remove them when the pentest
ends. The logs contain client IPs and usernames (personal data).

## Fleet policy (Agent policy 1)

Configured in Kibana, not in this repo. Current settings worth knowing:

- **Kubernetes integration:** kubelet streams (container, node, pod, system,
  volume) on, host `https://${env.NODE_IP}:10250` (`NODE_IP` comes from the
  DaemonSet). kube-state-metrics, kube-proxy, scheduler and
  controller-manager are off: none of them is reachable without
  kube-state-metrics or hostNetwork. Container logs have `decode_json_fields`
  for the app namespaces (matched on `log.file.path`) and a `fingerprint`
  processor (`@metadata._id`, so re-read lines are dropped as duplicates).
  Audit logs have the same fingerprint.
- **System integration:** host metrics and auth/syslog.
- **Prometheus integration** `node-exporter-bootstrap-vm`: see
  `infra/node-exporter/`.
