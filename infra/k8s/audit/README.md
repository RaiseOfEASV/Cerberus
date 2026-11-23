# Kubernetes API audit log (k3s)

k3s leaves API server auditing off. Without it there is no record of who
created, changed, deleted or exec'd into what in the cluster.

These files are applied by hand on the node instead of through the cloud-init
in `infra/terraform/modules/k8s-cluster/azure-k3s/main.tf`: changing
`custom_data` makes Terraform replace the VM. Same kind of manual step as the
netplan drop-in in `infra/k8s/traefik-public/README.md`. If the VM is ever
rebuilt, add these to the cloud-init at the same time.

## Turn it on

From a machine on the VPN (the node is `10.0.2.4`):

```sh
scp infra/k8s/audit/audit-policy.yaml infra/k8s/audit/k3s-audit.yaml <admin>@10.0.2.4:/tmp/
ssh <admin>@10.0.2.4 '
  sudo install -m 600 /tmp/audit-policy.yaml /etc/rancher/k3s/audit-policy.yaml &&
  sudo install -d -m 755 /etc/rancher/k3s/config.yaml.d &&
  sudo install -m 600 /tmp/k3s-audit.yaml /etc/rancher/k3s/config.yaml.d/audit.yaml &&
  sudo install -d -m 700 /var/log/kubernetes/audit &&
  sudo systemctl restart k3s'
```

Restarting k3s restarts the API server only; running pods keep running.
The API is unavailable for about 30 seconds.

## Check

```sh
ssh <admin>@10.0.2.4 'sudo tail -n 2 /var/log/kubernetes/audit/audit.log'
kubectl auth can-i --list >/dev/null   # should show up as a new entry
```

## Shipping

The log is a file on the node, `/var/log/kubernetes/audit/audit.log` (JSON,
one event per line). The log agents (Grafana Alloy, Elastic Agent, Wazuh
agent) pick it up from that path, next to auditd's `/var/log/audit/audit.log`.
