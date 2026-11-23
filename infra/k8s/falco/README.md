# Falco

The `falco` chart (values in `infra/helm/values/falco-values.yaml`). Falco
writes its alerts to stdout as JSON; Elastic Agent's container-log stream
(`infra/k8s/elastic-agent/`) ships them to Elastic like any other pod's logs.
Falcosidekick is disabled: there is no separate alert forwarder.

## Not yet deployed

`falco-values.yaml`'s eBPF driver is unverified against the k3s node's kernel.
Once deployed, add `falco` to the namespaces in the Fleet policy's
`decode_json_fields` condition so the alert fields are parsed, not just
searchable as text.
