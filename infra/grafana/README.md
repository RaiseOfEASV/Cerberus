# Grafana Cloud dashboards

Stack `tenderraccoon360`, folder **Cerberus**. Imported from grafana.com at
pinned revisions by `import-dashboards.sh` (re-running overwrites them).

| Dashboard | grafana.com | Data (Alloy job) |
|---|---|---|
| Node Exporter Full | 1860 rev 46 | `integrations/node_exporter` (both VMs, infra/node-exporter/) |
| Kubernetes / Views / Global | 15757 rev 43 | kube-state-metrics, kubelet cAdvisor, node_exporter |
| Kubernetes / Views / Pods | 15760 rev 41 | kube-state-metrics, kubelet cAdvisor |
| Traefik Official Standalone | 17346 rev 9 | `integrations/traefik` (admin and public instances) |

Changes made at import:
- Data source, `cluster = cerberus-k3s` and the node_exporter `job` are
  preselected.
- Traefik: a **Traefik** selector (`instance`: traefik / traefik-public,
  default traefik-public) filters every query. The dashboard assumes one
  Traefik, and two with the same service names gave "vector cannot contain
  metrics with the same labelset". Its service-name `label_replace` regex is
  changed to strip only `@provider`: the original kept two dash-separated
  parts, which merged different routes of the same app.

Known empty panels: the two Secret/ConfigMap counts in "Global"
(kube-state-metrics runs without those collectors, so it needs no Secret
access), and the panels that only show problems (OOM kills, restarts,
throttling, unscheduled pods, 5xx, SLO breaches) while there are none.

Series budget: Alloy keeps only what these dashboards query. About 8,200
active series in total (node_exporter about 3,400, kube-state-metrics about
1,600, Traefik about 620, cAdvisor about 130).
