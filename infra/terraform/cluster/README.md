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
3. **Be on the VPN** (`infra/headscale/README.md`). The cluster VM has no inbound
   internet rules at all, so the kubeconfig fetch (SSH to the node's private IP) only
   works over the tailnet.
4. Export the backend connection string as `PG_CONN_STR` - the `pg` backend reads it
   from the environment, which keeps the password off the command line (a
   `-backend-config` argument shows up in `ps`):
   ```
   export PG_CONN_STR="postgres://terraform:${TF_STATE_PG_PASSWORD}@localhost:5432/terraform_state?sslmode=disable"
   terraform init
   ```
   `sslmode=disable`, not `require` - the Postgres container has no SSL cert
   configured. Safe because `localhost` is an SSH tunnel to the bootstrap VM (below),
   not a plaintext connection over the open internet.

### Reaching the backend

Postgres is bound to `127.0.0.1:5432` on the bootstrap VM only (see
`infra/terraform-state/docker-compose.yml`'s own comment on why) - it is NOT reachable
at the VM's public IP, by design. Tunnel to it over SSH first:

```
ssh -f -N -o ServerAliveInterval=30 -L 5432:127.0.0.1:5432 -i ~/.ssh/cerberus_azure cerberus-admin@<bootstrap-ip>
```

`<bootstrap-ip>` can be its public IP (SSH is still open there, admin-IP-restricted)
or `10.0.1.4` over the VPN. `ServerAliveInterval` matters: a tunnel that sat idle for
hours went stale once and hung `terraform init` indefinitely while still accepting
local TCP connections, so a plain port check didn't catch it.

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
- No inbound internet access at all: SSH and the Kubernetes API are reachable only
  over the VPN, at the node's private IP (`10.0.2.4`). Break-glass if the VPN is
  down: `az vm run-command invoke -g cerberus-rg -n cerberus-k3s-vm --command-id
  RunShellScript --scripts '...'` - goes through the Azure VM agent, no network path
  needed.
- The plan shows a perpetual in-place diff on the VM
  (`vm_agent_platform_updates_enabled = true -> false`) - an azurerm provider quirk,
  Azure sets it itself. Don't apply it casually: an in-place VM update risks a reboot,
  which reseals Vault (needs 2 unseal shares). Target other changes with `-target`
  until the provider is upgraded.
- VM size is `Standard_D4s_v4` (4 vCPU/16GB) on-demand, not Spot - chosen so the
  cluster doesn't go down mid-session from an eviction. `Standard_B4ms` hit a real
  `SkuNotAvailable` capacity stockout; `Standard_D4s_v5` hit a hard 0-vCPU quota on
  this Student subscription (only pre-v5 D-series families have nonzero quota here).
  Costs real money if left running continuously; this project's usage pattern is
  session-based, not 24/7.
