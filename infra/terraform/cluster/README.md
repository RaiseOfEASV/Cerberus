# Cluster provisioning

Provisions the actual cluster via the swappable `modules/k8s-cluster/kind` interface
(see `../modules/k8s-cluster/README.md`). This is the thing every other Epic 4+ piece
(ArgoCD, Kyverno, MetalLB, the ARC runner pool from Epic 3) needs to exist before it
can be deployed.

## Before applying

1. `infra/terraform-state/` must be running (see its README) - this uses the shared
   `pg` backend.
2. `terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=require"`
3. `terraform plan` / `terraform apply`.

## After applying

`terraform output -raw kubeconfig > kubeconfig.yaml` (don't commit this file -
`.gitignore` already covers `kubeconfig*.yaml` patterns... verify that, it's a
credential). Then everything else in Epic 4 (MetalLB, Traefik, ArgoCD, Kyverno,
kube-bench) and Epic 3's ARC runner pool get installed against this cluster, in the
order documented in `infra/k8s/README.md`.

## Known limitation carried over from the module interface

`node_count` doesn't distinguish control-plane vs. worker counts - node 0 is always
control-plane, everything else is a worker. Fine for a single-control-plane local
dev cluster; revisit if HA control plane ever matters here.
