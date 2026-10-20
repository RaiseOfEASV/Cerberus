# Falco + Falcosidekick

`falco` chart installs Falco + the `falcosidekick` sub-chart together (see
`falco-values.yaml`'s `falcosidekick.enabled: true`) - `falcosidekick-values.yaml`
only configures the sub-chart's output, it isn't a separate install.

## Not yet deployable

Needs Epic 4's cluster and `infra/wazuh/` deployed with a real address (both
`falco-values.yaml`'s eBPF driver and `falcosidekick-values.yaml`'s syslog target are
placeholders/unverified until then - see each file's own header).
