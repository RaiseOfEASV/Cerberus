# k8s-cluster module interface

Terraform doesn't have a formal "interface" construct for modules - this file IS the
interface contract, by convention. Every implementation directory under here
(`kind/` today; `eks/`, `gke/`, `aks/` later, if/when a cloud is chosen) MUST accept
the same input variables and produce the same outputs, so `infra/terraform/cluster/`
can swap `source = "../modules/k8s-cluster/kind"` for a different implementation
without changing anything that consumes the module.

## Required inputs (every implementation)

| Name                 | Type   | Notes                                              |
|-----------------------|--------|-----------------------------------------------------|
| `cluster_name`        | string | |
| `kubernetes_version`  | string | Implementation decides how to interpret this (e.g. a kind node image tag vs. a cloud provider's version string) - the variable name and meaning ("which k8s version") stays the same. |
| `node_count`          | number | Total nodes including control plane. |

## Required outputs (every implementation)

| Name               | Type           | Notes |
|--------------------|----------------|-------|
| `kubeconfig`       | string, sensitive | Raw kubeconfig content, usable directly by the `kubernetes`/`helm` Terraform providers or written to disk for `kubectl`. |
| `cluster_endpoint` | string         | The API server URL. |

## Only one implementation exists today

`kind/` - local, single-host, for the "local kind/k3d now" phase of the plan. Nothing
here is cloud-specific; adding `eks/`/`gke/`/`aks/` later means a new directory
honoring the same contract, not a rewrite of `infra/terraform/cluster/`.
