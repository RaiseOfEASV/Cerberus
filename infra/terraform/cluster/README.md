# Cluster provisioning

Provisions the actual cluster via the swappable `modules/k8s-cluster/azure-k3s`
implementation (see `../modules/k8s-cluster/README.md` for the interface contract;
`kind/` is still there for local dev). This is the thing every other Epic 4+ piece
(ArgoCD, Kyverno, MetalLB, the ARC runner pool from Epic 3) needs to exist before it
can be deployed.

Self-hosted k3s on a single Azure VM was chosen over AKS - see
`../modules/k8s-cluster/README.md` for why. The VM lands in the same resource
group/VNet `infra/terraform/bootstrap/` already created (new subnet, 10.0.2.0/24),
so it can reach terraform-state's Postgres and the backups MinIO (both on
10.0.1.4) over private IPs once something in-cluster needs to.

## Before applying

1. `infra/terraform/bootstrap/` must already be applied (resource group + VNet this
   depends on).
2. `infra/terraform-state/` must be running (see its README) - this uses the shared
   `pg` backend.
3. Copy `terraform.tfvars.example` to `terraform.tfvars` and fill in `admin_source_ip`
   (gitignored, never commit it).
4. `terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=disable"`
   (`sslmode=disable`, not `require` - the Postgres container has no SSL cert configured,
   confirmed by hitting `pq: SSL is not enabled on the server` on the real first attempt.
   Safe here because `<host>` should be `localhost`, reached through an SSH tunnel to the
   bootstrap VM - see "Reaching the backend" below - not a plaintext connection over the
   open internet.)

### Reaching the backend

Postgres is bound to `127.0.0.1:5432` on the bootstrap VM only (see
`infra/terraform-state/docker-compose.yml`'s own comment on why) - it is NOT reachable
at the VM's public IP, by design. Until this VM has joined headscale's own tailnet
(Epic 8), `terraform init`/`plan`/`apply` need an SSH tunnel first:

```
ssh -f -N -L 5432:127.0.0.1:5432 -i ~/.ssh/cerberus_azure cerberus-admin@<bootstrap-public-ip>
```

Then use `localhost:5432` as `<host>` in the `conn_str` above. Kill the tunnel
(`pkill -f "5432:127.0.0.1:5432"`) when done.
5. `terraform plan` / `terraform apply`. The apply takes longer than the bootstrap
   VM's did - it waits (via a `null_resource` provisioner, polling over SSH) for
   k3s to finish installing before it can fetch the kubeconfig.

## After applying

`terraform output -raw kubeconfig > kubeconfig.yaml` (don't commit this file -
`.gitignore` already covers `kubeconfig*.yaml` patterns). Then everything else in
Epic 4 (MetalLB, Traefik, ArgoCD, Kyverno, kube-bench) and Epic 3's ARC runner pool
get installed against this cluster, in the order documented in `infra/k8s/README.md`.

## Known limitations

- Single node only (control-plane + worker combined) - `azure-k3s`'s `node_count`
  variable rejects anything but `1`. Adding workers means fetching k3s's join token
  from this node and feeding it into additional VMs' cloud-init - not built yet.
- The Kubernetes API (6443) is restricted to `admin_source_ip`, not VPN-gated - same
  reasoning as the bootstrap VM's SSH rule. Revisit once this VM has joined
  headscale's tailnet as a client.
- VM size is `Standard_D4s_v4` (4 vCPU/16GB) on-demand, not Spot - chosen so the
  cluster doesn't go down mid-session from an eviction. `Standard_B4ms` hit a real
  `SkuNotAvailable` capacity stockout; `Standard_D4s_v5` hit a hard 0-vCPU quota on
  this Student subscription (only pre-v5 D-series families have nonzero quota here).
  Costs real money if left running continuously; this project's usage pattern is
  session-based, not 24/7.
