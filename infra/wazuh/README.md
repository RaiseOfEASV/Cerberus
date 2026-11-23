# Wazuh manager (dedicated host)

Deliberately NOT in the app cluster and NOT sharing infrastructure with the Elastic
observability stack (`infra/k8s/elastic/`) - a compromised workload shouldn't be able
to reach or tamper with the security logging that would have caught it.

## Setup

1. Generate certs with Wazuh's own tool (never commit the output):
   ```
   curl -so wazuh-certs-generator.sh https://packages.wazuh.com/4.9/wazuh-certs-generator.sh
   # follow Wazuh's own single-node cert-generation instructions, output into ./certs/
   ```
2. `export INDEXER_PASSWORD=<...> DASHBOARD_PASSWORD=<...>` - never hardcode or commit.
3. `docker compose up -d`

## After it's up

- Update the `WAZUH_MANAGER` value used by:
  - Epic 5's Ansible `wazuh_agent` role (`infra/packer/variables.pkr.hcl`'s
    `wazuh_manager_address` default) - still `CHANGE_ME_NO_WAZUH_MANAGER_YET` until
    this exists somewhere with a real, reachable address.
  - `infra/k8s/wazuh-agent/daemonset.yaml` (in-cluster agents) - same placeholder
    problem, same fix.
- Open ports 1514 (agent events), 1515 (agent enrollment), 514/udp (syslog, for
  Falcosidekick) to whatever network the cluster and VM fleet actually live on - not
  just loopback, unlike the dashboard.
- Write a real agent enrollment password into Vault
  (`vault kv put secret/wazuh/agent password=...`) - the in-cluster DaemonSet reads
  it via ESO (`infra/k8s/eso/external-secret-wazuh-agent.yaml`).

## Wazuh Cloud (in use since 2026-11-23)

For the external pentest the self-hosted manager below was skipped in favour of
a Wazuh Cloud trial: environment `htwuxp6amhh2`, agents connect to
`htwuxp6amhh2.cloud.wazuh.com` on 1514/1515. Vault `wazuh/cloud` holds the
environment id, dashboard login and Cloud API key.

Agents (4.14.9, held at that version) run on both VMs, installed with
`install-agent.sh` in this directory:

| Agent | Host | Extra log sources |
|---|---|---|
| `cerberus-k3s-vm` | 10.0.2.4 | auditd, k3s API audit log (`infra/k8s/audit/`) |
| `cerberus-bootstrap-vm` | 10.0.1.4 | auditd, Docker container logs (headscale, terraform-state, MinIO) |

journald (SSH logins, sudo) and dpkg are in the agent's default config. The
in-cluster DaemonSet (`infra/k8s/wazuh-agent/`) is not deployed: with a single
node it would only duplicate the k3s node's agent. Container logs from the
cluster go to Grafana Cloud / Elastic instead.

## Self-hosted manager: not yet running

Needs a persistent host (same "plain VM for now, Epic 5's golden image later" caveat
as `infra/terraform-state/`). Nothing here has actually been deployed - both
consumers (VM agents, in-cluster DaemonSet) are still pointed at placeholder
addresses, by design, not oversight.

**Decision (2026-11-22): not deploying it for now.** The only place it fits
without a quota increase is the bootstrap VM, which would have to grow from
`B2ats_v2` (1 GiB, about $8/month, free-tier eligible) to `B2as_v2` (8 GiB,
about $63/month); running it inside the cluster would break the isolation
above. The images stay in `docker/images.yaml` as `inactive` and are still
scanned nightly. When it is deployed: install 4.14.x (the agent image
`wazuh/wazuh-agent` only exists from 4.13 on, and agents must not be newer
than the manager).
