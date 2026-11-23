# node_exporter on the VMs

Host metrics (CPU, memory, disk, filesystems, network) for VMs that don't
run Elastic Agent. Installed on **cerberus-bootstrap-vm** (10.0.1.4, 1 GiB
RAM: too small for Elastic Agent) since 2026-11-23:

```sh
scp infra/node-exporter/install-node-exporter.sh cerberus-admin@10.0.1.4:/tmp/
ssh cerberus-admin@10.0.1.4 'sudo bash /tmp/install-node-exporter.sh 10.0.1.4 10.0.2.4'
```

- Upstream 1.13.1, checksum-verified, runs as a systemd DynamicUser with a
  64 MB memory cap (about 16 MB in use).
- Listens on 10.0.1.4:9100 only. iptables drops 9100 from anything but
  10.0.2.4 (the k3s node: pod traffic leaving the cluster is SNATed to it).
  Checked: reachable from the node and from the agent pod, not from a VPN
  client.
- Scraped every 30s by the cluster's Elastic Agent through the Fleet
  package policy `node-exporter-bootstrap-vm` (Prometheus integration,
  collector stream only; `remote_write`, which would open a listener, is
  off). Data: `metrics-prometheus.collector-*`, label
  `prometheus.labels.instance: 10.0.1.4:9100`.

The k3s node itself doesn't run node_exporter: Elastic Agent's System and
Kubernetes (kubelet) integrations already cover it, without the host
/proc, /sys and / mounts a node_exporter pod would need.
