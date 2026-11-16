# Redeploying Cerberus into a different Azure subscription

Written when the original Azure for Students subscription ran out of credit, and
updated with what actually happened when we did it (2026-11-16, recover path, into
a teammate's Student subscription in `belgiumcentral`). Credit can't be shared
between subscriptions: adding people to a subscription through IAM gives them
access, but billing stays with that subscription. The only way to run on someone
else's credit is to run **in their subscription**. This is the full move, in the
order that worked. Steps point at the README that owns the details; the
"Lesson" notes are the things that broke or surprised us.

## 0. Decide: recover or greenfield

When a subscription runs out of credit it is **disabled, not deleted**. VMs are
deallocated, disks are kept, and nothing can be started, resized or moved. After
roughly 90 days Microsoft deletes it. Everything stateful lives on those disks:

| Lives on | What | Recoverable from |
|---|---|---|
| Bootstrap VM | Terraform state Postgres, headscale DB, backup MinIO | the VM disk |
| Cluster VM | Vault raft data, Keycloak Postgres, Kubernetes Secrets (internal CA!) | backup MinIO (Vault snapshots only), otherwise the VM disk |

- **Recover:** needs the old subscription writable again (section 0a), and your
  **old Vault unseal shares**. Without the shares the Vault data is useless and
  greenfield is cheaper.
- **Greenfield:** Terraform state, Vault secrets, Keycloak users, headscale devices
  and the internal CA all start empty. Git holds everything else.

Either way the old resources are never *moved*: Azure's resource move needs both
subscriptions active, and in practice it's simpler to redeploy and copy data.

### 0a. Getting data off the old subscription

1. **Reactivate it:** there is no top-up for Azure for Students. Upgrade it to
   Pay-As-You-Go (portal -> Subscriptions -> the subscription -> Upgrade; needs a
   card). Immediately add a budget alert (Cost Management -> Budgets, e.g. EUR 5):
   Pay-As-You-Go has no spending cap.
   - **Lesson:** after the upgrade the subscription reports `Enabled` (and
     `quotaId: PayAsYouGo_2014-09-01`) within minutes, but writes still fail with
     `ReadOnlyDisabledSubscription` for another ~10-15 minutes. Retry.
2. **Start the old VMs.**
   - **Lesson:** `swedencentral` returned `AllocationFailed` (no capacity) for the
     bootstrap VM's `Standard_B2ats_v2` *and* for `Standard_D2as_v4`. The VM is
     deallocated, so it can be resized freely: `Standard_D2s_v4` started. Stick to
     older SCSI sizes (v3/v4); v6/v7 sizes need NVMe, which the old image may not
     boot. The cluster VM started at its own size (`D4s_v4`).
   - Kali's packaged `az` crashes on `az vm`/`az provider`; use `az rest` against
     `.../virtualMachines/<vm>/start`, `/deallocate`, and `PATCH` with
     `{"properties":{"hardwareProfile":{"vmSize":"..."}}}`. `start` returns 202
     immediately; check the real result in `/instanceView` (a fresh
     `ProvisioningState/failed/AllocationFailed` timestamp means it failed again).
3. **Copy, to `~/cerberus-old-sub/` on your laptop** (outside the repo):
   - The whole `cerberus-backups` bucket: on the old bootstrap VM, read the MinIO
     root credentials from `docker inspect` of the `minio-backups` container
     (never printed), `mc mirror old/cerberus-backups`, tar it back over SSH.
   - **A fresh `pg_dumpall` of the Terraform state DB.** Lesson: the daily
     `terraform-state-pg` backups had silently stopped after 2026-10-25, so the
     bucket copy was weeks stale. Investigate that sidecar.
   - From the old cluster (reachable without the VPN by jumping through the old
     bootstrap VM's public IP:
     `ssh -o ProxyCommand='ssh -W %h:%p cerberus-admin@<old-bootstrap-ip>' cerberus-admin@10.0.2.4`,
     **with a separate `-o UserKnownHostsFile=~/cerberus-old-sub/known_hosts`**,
     see lesson below):
     - `pg_dumpall` of Keycloak's Postgres (`kubectl -n keycloak exec keycloak-postgres-0`).
     - Raw tar of the persistent volumes under `/var/lib/rancher/k3s/storage`
       (`sudo tar -C <dir>`, not `cd`; the dir is root-only). There were only two
       PVs: Keycloak's Postgres and Vault's raft data.
     - `kubectl get secrets -A -o yaml` into a `0600` file. This holds the
       **internal CA** (`cert-manager/cerberus-internal-ca-secret`), the Let's
       Encrypt account key, and Helm's release records, which tell you the exact
       chart **versions** and values every release ran (they aren't pinned in the
       repo).
   - **Lesson: there were no Velero backups at all.** Velero was never deployed,
     so Keycloak's users and the CA existed only on the cluster disk.
   - **Lesson:** the old cluster and the new one both live at `10.0.2.4`. An
     `accept-new` SSH to the old one writes its key into `~/.ssh/known_hosts`,
     and the new cluster's Terraform kubeconfig fetch then fails with "host key
     has changed". Keep the old cluster's key in its own known_hosts file.
4. Deallocate both old VMs. Delete `cerberus-rg` there once the new environment
   works (section 10).

## 1. Get access to the new subscription

The subscription owner (the teammate with credit): Subscriptions -> *their
subscription* -> Access control (IAM) -> Add role assignment -> **Owner** -> your
account. The role must be on the subscription, not a resource group.

- **Lesson:** two students at the same school share the school's Entra tenant, so
  no guest invite is needed. The role just has to propagate (a few minutes).
- `az account list --refresh --all -o table` to see it. If refreshing fails with
  `AADSTS50078`/`AADSTS50076`, your MFA session expired: `az login` again
  (interactive, run it yourself).

A brand-new subscription has Compute/Network/Storage unregistered. Register them
up front so the checks below return data (a few minutes):

```
for p in Microsoft.Compute Microsoft.Network Microsoft.Storage; do
  az rest --method post --url "https://management.azure.com/subscriptions/<id>/providers/$p/register?api-version=2021-04-01"
done
```

Then check what the subscription allows **before** applying anything.

1. **Allowed regions.** Student subscriptions carry an "Allowed resource
   deployment regions" policy, different per subscription (original:
   `swedencentral` allowed; teammate's: only `belgiumcentral`, `denmarkeast`,
   `switzerlandnorth`, `italynorth`, `germanywestcentral`). Applying elsewhere
   fails with `RequestDisallowedByAzure`.
   ```
   az rest --url "https://management.azure.com/subscriptions/<id>/providers/Microsoft.Authorization/policyAssignments?api-version=2022-06-01" \
     --query "value[].properties.parameters.listOfAllowedLocations.value" -o json
   ```
2. **Per allowed region: quota and SKU restrictions** (`az rest` equivalents,
   since `az vm` may be broken):
   ```
   .../providers/Microsoft.Compute/locations/<region>/usages?api-version=2023-09-01
   .../providers/Microsoft.Compute/skus?api-version=2021-07-01     # restrictions[] must be empty
   ```
   - **Lesson: quota isn't availability.** DSv4 showed quota 4 everywhere, but
     `Standard_D4s_v4` was `NotAvailableForSubscription` in every allowed region
     except Belgium Central, and three regions blocked every size. Only
     `belgiumcentral` offered both `Standard_D4s_v4` and `Standard_B2ats_v2`.
   - Total regional vCPU quota was 6: cluster (4) + bootstrap (2) uses all of it.
   - Fallback if `D4s_v4` is blocked everywhere: `Standard_B4as_v2` (same spec,
     burstable, quota 10).
3. Set `location` (and `vm_size` if needed) in **both** roots; the cluster reads
   the bootstrap's resource group and VNet. The private IPs (10.0.1.4 / 10.0.2.4 /
   .50 / .51) don't depend on the region.

## 2. Pin the subscription

Both Azure roots require an explicit `subscription_id`:
`subscription_id = "..."` in `infra/terraform/bootstrap/terraform.tfvars`, and
`export TF_VAR_subscription_id=<id>` for the cluster root.

## 3. Bootstrap VM (local state)

The bootstrap root's local `terraform.tfstate` describes the **old** subscription.
Move it aside (`~/cerberus-old-sub/bootstrap/`), then `terraform init && terraform apply`.

- **Lesson:** the first apply failed creating the subnet with a 404 for the VNet
  it had just created (Azure eventual consistency). Re-running `apply` finished it.
- Wait for cloud-init (`cloud-init status --wait`), then copy `infra/headscale/`,
  `infra/terraform-state/` and `infra/backups/` to `~/cerberus/infra/` on the VM
  (keep the layout; the compose files reference each other by relative path).
- Generate new secrets (`TF_STATE_PG_PASSWORD`, `BACKUP_MINIO_ROOT_USER/_PASSWORD`,
  `BACKUP_MINIO_ACCESS_KEY/_SECRET_KEY`, `BACKUP_MINIO_ENDPOINT=10.0.1.4:9000`)
  into a `0600` file outside the repo, move them to your password manager, then
  `shred -u` the file. Pass them to the VM by piping `export ...` lines plus the
  commands into `ssh ... 'bash -s'`, so they never hit the VM's disk.
- Start `backups` first, then create the bucket, backup-only user and retention
  (`infra/backups/README.md` steps 2-5). **Lesson:** `minio/mc` is no longer
  pullable from Docker Hub (or quay.io); `cgr.dev/chainguard/minio-client` works.
  In current `mc` it's `mc ilm rule add ... --expire-days 90`.
- Then `terraform-state` and `headscale`. Both backup sidecars upload a first
  backup on start; check their logs.

## 4. State, VPN

- **headscale:** set `server_url` in `infra/headscale/config/config.yaml` to the
  new public IP *before* copying it to the VM. Run
  `scripts/setup-subnet-router.sh` (route `10.0.0.0/16` is auto-approved), then
  re-join each admin device (`infra/headscale/README.md`). Lesson: if the join
  one-liner fails at the `sudo` prompt inside the pipe, `tailscale status` shows
  `NoState`; split it into "create key file" and `sudo tailscale up ... --force-reauth`.
- **Host keys:** `10.0.1.4` is now a different machine. Before removing the old
  `known_hosts` entry, check the new key's fingerprint matches the one you
  accepted on the VM's public IP.
- **Terraform state:** load only the `terraform_state` database section of the
  `pg_dumpall` (everything after its `\connect terraform_state` line) into the new
  Postgres over the SSH tunnel, then empty the cluster root's state, whose Azure
  IDs all point at the old subscription:
  `DELETE FROM terraform_remote_state.states;`. The `github` and `vault` schemas
  stay valid as restored.

## 5. Cluster

`infra/terraform/cluster/README.md`: tunnel to the new Postgres, `PG_CONN_STR`,
`TF_VAR_subscription_id`, on the VPN, `terraform init -reconfigure`, `apply`,
`terraform output -raw kubeconfig > kubeconfig.yaml` (`umask 077`). If the
kubeconfig fetch fails on a changed host key for `10.0.2.4`, remove that entry
and `apply` again; it only re-runs the fetch.

## 6. Platform, in the order that worked

Use the **old chart versions** (from the Helm release records, section 0a) and the
repo's values files. Every old release's values were identical to the repo's.
Versions as of 2026-11: metallb 0.16.1, traefik 42.1.3 (also traefik-public),
kyverno 3.9.2, argo-cd 10.11.1, cert-manager v1.21.3, vault 0.34.1,
external-secrets 2.14.0, keycloakx 7.4.0, oauth2-proxy 10.7.1.

1. **MetalLB** + `kubectl apply -f infra/k8s/metallb/`, **Traefik** (gets 10.0.2.50),
   **Kyverno** + `kyverno/policies/`.
2. **cert-manager.** On the recover path, apply the old
   `cerberus-internal-ca-secret` and `letsencrypt-public-account-key` (strip
   `resourceVersion`/`uid`/`creationTimestamp`) **before**
   `cert-manager/cluster-issuers.yaml`. cert-manager then adopts the old CA instead
   of minting a new one, so admin devices keep trusting it. Compare the SHA-256
   fingerprints old vs new to confirm.
3. **Vault:** `vault/namespace.yaml`, `network-policy.yaml`, `certificate.yaml`,
   then the chart. Section 7 for the data.
4. **ESO:** chart, `eso/cluster-secret-store.yaml`, create the `keycloak`,
   `oauth2-proxy` and `argocd` namespaces, then apply only the ExternalSecrets that
   were in use (the old cluster had: argocd-oidc-client, cloudflare-api-token,
   keycloak, oauth2-proxy-client-for-keycloak, oauth2-proxy, vault-oidc-client,
   vault-backup-minio). The ArgoCD one reports `SecretMissing` until ArgoCD exists;
   force a refresh afterwards (`force-sync` annotation).
5. **Keycloak:** `keycloak/network-policy.yaml` + `postgres.yaml`, restore the
   Keycloak dump's `keycloak` database section into the empty database **before**
   installing the chart, then the chart and `keycloak/ingressroute.yaml`.
   **Do not run the realm-config Job** on the recover path: the realm is in the
   restored DB, and that Job resets other clients' secrets (Keycloak README).
6. **CoreDNS:** `kubectl apply -f infra/k8s/coredns/custom-hosts.yaml` and restart
   CoreDNS. **Lesson: it's in no install order, and it hardcodes Traefik's
   ClusterIP**, which is different on every new cluster. Without the right IP,
   oauth2-proxy crash-loops on `lookup keycloak.admin.cerberus.internal ... no such host`.
   Put in the current IP (`kubectl -n traefik get svc traefik -o jsonpath='{.spec.clusterIP}'`)
   until this file stops hardcoding it.
7. **oauth2-proxy:** `oauth2-proxy/ca-configmap.yaml`, `network-policy.yaml`,
   `forwardauth-middleware.yaml`, chart, `ingressroutes.yaml` (its monitoring,
   elastic and dependency-track routes fail: those apps weren't deployed),
   `vault/ingressroute.yaml`.
8. **ArgoCD:** `kubectl apply -f argocd/ca-configmap.yaml` **before** the chart.
   Lesson: otherwise `argocd-server` hangs in `ContainerCreating` and the Helm
   install times out.
9. **traefik-public:** `traefik-public/namespace.yaml`, chart (gets 10.0.2.51).
10. `kubectl apply -f argocd/root-apps.yaml`: dev and staging sync on their own.

Not deployed before, not deployed now: ARC, Velero, monitoring, Falco, Elastic,
Dependency-Track, Nexus, Wazuh agent. kube-bench's CronJob is rejected by
Kyverno's `require-resource-limits` policy, so it isn't running either.

## 7. Vault data

- **Recover, simplest:** if the raw Vault PV was copied while the old Vault was
  **sealed** (it is after any reboot), nothing was writing, so the copy is
  consistent. Install the chart, `kubectl -n vault scale sts vault --replicas=0`,
  replace the new PV directory's contents with the old ones (`tar -xp
  --same-owner`, `cp -a`), scale back to 1. `vault status` should say
  `Initialized: true` with the old share count. Then **you** unseal with the old
  shares, in your own terminal (`docs/security/vault-unseal-procedure.md`).
  Raft snapshots are the fallback; restoring one needs a freshly initialized
  Vault plus the old shares.
- `infra/terraform/vault/` needs **no** re-apply: the Kubernetes auth config
  stores no CA, so Vault uses its own pod's service account and the new cluster's
  CA. `ClusterSecretStore` turning `Valid` proves it.
- **Greenfield:** initialize with new shares, `terraform apply` in
  `infra/terraform/vault/`, then re-enter every secret the ExternalSecrets read
  (Keycloak, oauth2-proxy, OIDC clients, Cloudflare token, backup MinIO, ...).
- **Backups:** `kubectl apply -f vault/backup-cronjob.yaml` (CronJob
  `vault-raft-snapshot`) and test it once with
  `kubectl -n vault create job vault-backup-now --from=cronjob/vault-raft-snapshot`.
  Lesson: Vault's `secret/backup/minio` still holds the *old* MinIO backup user.
  Either update it in Vault, or (what we did) add that same access key to the new
  MinIO as a second user with `backup-only-policy`.

## 8. Prod

Prod is manual sync (`infra/k8s/environments/README.md`, "Syncing prod").
**Lesson:** on a fresh cluster the `cerberoast` namespace doesn't exist yet. It
comes with the *full* root-prod sync, so the documented cutover order (apps
first, root-prod last) fails with `namespaces "cerberoast" not found`. On a fresh
cluster nothing is serving yet, so sync root-prod in full first, then
`quote-api-prod` and `quote-web-prod`.

## 9. Public site

Cloudflare -> A records for `cerberoast.site`, `www`, `api` -> new
`terraform output cluster_public_ip`. The Let's Encrypt certificate is issued
through DNS-01 before the DNS switch, so you can verify the new IP beforehand:
`curl --resolve cerberoast.site:443:<new-ip> https://cerberoast.site/` (200), and
`https://api.cerberoast.site/quote` / `/healthz` (200; `/` is 404 by design).

## 10. Clean up

- Copy the old backup history into the new `cerberus-backups` bucket (`mc mirror`
  from an uploaded tarball; `chmod -R a+rX` it first, the minio-client container
  runs as non-root).
- Old subscription: delete `cerberus-rg`, then **cancel the subscription**
  (Subscriptions -> Cancel) so Pay-As-You-Go can't bill again. It shows as
  `Warned` afterwards. The card can be removed under Cost Management + Billing ->
  Payment methods once the final invoice is paid.
- Keep `~/cerberus-old-sub/` (old Secrets, CA key, dumps) somewhere safe and
  encrypted. It's the only copy outside the cluster.
- Update the region/size comments in `bootstrap/variables.tf` and
  `modules/k8s-cluster/azure-k3s/variables.tf` if they changed.
- Log the move in `docs/security/restore-drills.md`'s drill log.

## When this subscription runs out too

Same procedure again. To make it cheaper next time:

- keep a copy of the backup bucket **outside** Azure (periodic `mc mirror` to a
  laptop or another provider), since today the backups die with the subscription
  that holds them;
- actually deploy Velero, or at least back up the internal CA and the Keycloak DB;
- fix the stalled `terraform-state-pg` backup sidecar;
- pin chart versions in the repo;
- stop hardcoding Traefik's ClusterIP in `coredns/custom-hosts.yaml`.
