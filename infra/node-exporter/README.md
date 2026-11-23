# node_exporter on the VMs

Host metrics (CPU, memory, disk, filesystems, network) as a host service on
both VMs, since 2026-11-23:

| VM | Listens on | Allowed scrapers |
|---|---|---|
| cerberus-bootstrap-vm | 10.0.1.4:9100 | 10.0.2.4 (the k3s node; pod traffic leaving the cluster is SNATed to it) |
| cerberus-k3s-vm | 10.0.2.4:9100 | 10.42.0.0/16 (pods; a pod reaching its own node keeps its pod IP) |

```sh
scp infra/node-exporter/install-node-exporter.sh cerberus-admin@<vm>:/tmp/
ssh cerberus-admin@10.0.1.4 'sudo bash /tmp/install-node-exporter.sh 10.0.1.4 10.0.2.4'
ssh cerberus-admin@10.0.2.4 'sudo bash /tmp/install-node-exporter.sh 10.0.2.4 10.42.0.0/16'
```

Scraped by Elastic Agent (bootstrap VM only, Fleet Prometheus integration)
and by Grafana Alloy (both VMs, see infra/helm/values/grafana-alloy-values.yaml).

- Upstream 1.13.1, checksum-verified, runs as a systemd DynamicUser with a
  64 MB memory cap (about 16 MB in use).
- Listens on the VM's private IP only. A NODE_EXPORTER iptables chain
  accepts the allowed scrapers and drops the rest. Checked: reachable from a
  pod, not from a VPN client.
- Scraped every 30s by the cluster's Elastic Agent through the Fleet
  package policy `node-exporter-bootstrap-vm` (Prometheus integration,
  collector stream only; `remote_write`, which would open a listener, is
  off). Data: `metrics-prometheus.collector-*`, label
  `prometheus.labels.instance: 10.0.1.4:9100`.

On the k3s node it runs as a host service, not a pod: no host /proc, /sys
or / mounts in Kubernetes (Trivy KSV-0121).
