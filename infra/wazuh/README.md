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

## Not yet running

Needs a persistent host (same "plain VM for now, Epic 5's golden image later" caveat
as `infra/terraform-state/`). Nothing here has actually been deployed - both
consumers (VM agents, in-cluster DaemonSet) are still pointed at placeholder
addresses, by design, not oversight.
