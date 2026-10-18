# Vault initialization, unseal, and break-glass recovery

Vault (`infra/helm/values/vault-values.yaml`) starts **uninitialized and sealed**
after install. Nothing in this repo automates initialization or unsealing - doing
so would mean either committing unseal material to git (defeats the entire point of
Vault) or having a single human/script hold enough key shares to unseal alone
(defeats the point of Shamir splitting). This is the one piece of Epic 6 that is
inherently a manual, out-of-band human process, and it has to be written down before
anyone needs it under pressure.

## Before running any vault CLI command

Since Epic 8 flipped Vault's TLS on, `kubectl exec` into the pod still works exactly
as below regardless (it talks to the container directly, not through the TLS
listener). Running the `vault` CLI from **outside** the pod against
`vault.admin.cerberus.internal` instead requires `VAULT_ADDR=https://...` and either
`VAULT_CACERT` pointed at the internal CA cert or that CA already trusted by the OS
(see `infra/k8s/cert-manager/README.md`) - plain `http://` will simply fail to
connect, not silently work insecurely.

## One-time initialization

```
kubectl exec -n vault vault-0 -- vault operator init -key-shares=5 -key-threshold=3
```

This prints **5 unseal key shares** and **1 initial root token**, once, ever. There is
no way to retrieve them again later.

### Distributing the shares

- Give each of the 5 shares to a **different** trusted person, in a **different**
  storage location (e.g. each person's own password manager). Never store more than
  one share in the same place - the entire reason for splitting is that no single
  compromised location should be enough to unseal Vault alone.
- The root token is not a key share - store it separately from all 5 shares, and
  treat it as even more sensitive (it's a usable credential by itself, not 1/5 of
  one). After initial setup, create a scoped admin token for day-to-day use
  (`infra/terraform/vault/` applies with whatever token you export as `VAULT_TOKEN` -
  use a scoped one, not the root token, once one exists) and consider revoking the
  root token.
- **Update the list below as the actual team changes.** A stale holder list defeats
  the point of this document - if someone listed here has left, their share is
  presumed compromised/lost and Vault should be re-keyed (`vault operator rekey`)
  using the remaining valid shares before it matters.

| Share # | Holder | Storage location |
|---------|--------|-------------------|
| 1       | _TODO: name_ | _TODO_ |
| 2       | _TODO: name_ | _TODO_ |
| 3       | _TODO: name_ | _TODO_ |
| 4       | _TODO: name_ | _TODO_ |
| 5       | _TODO: name_ | _TODO_ |

## Routine unseal (after every pod restart)

Vault is **always sealed on process start** - a restart (pod reschedule, node
reboot, upgrade) reseals it even though the raft data on disk is untouched. Three
different share-holders each run, interactively, with their own share (never collect
shares into one script or one person's hands for this - that's exactly the scenario
splitting was meant to prevent):

```
kubectl exec -n vault vault-0 -it -- vault operator unseal
```

Each run prompts for one key share; after the 3rd distinct, valid share, Vault
unseals.

## Why there's no auto-unseal yet

Production Vault deployments usually use auto-unseal (cloud KMS, or a second Vault's
Transit engine) specifically to avoid needing 3 humans every restart. Not done here:
cloud KMS ties to a cloud provider that hasn't been chosen (the project stays
provider-agnostic), and a self-hosted Transit auto-unseal needs a *second* Vault,
which just moves the bootstrapping problem rather than solving it. Manual Shamir
unseal is the deliberate interim choice, not an oversight - revisit if/when a cloud
provider is actually picked.

## Break-glass recovery scenarios

**Lost fewer than `5 - 3 + 1 = 3` shares:** no problem, the remaining shares still
reach the threshold of 3. Re-key (`vault operator rekey`) to invalidate the lost
share(s) and issue a fresh set once convenient.

**Lost 3 or more shares (can no longer reach the threshold):** Vault's data is
**permanently inaccessible by design** - this is Shamir secret sharing working
correctly, not a bug to work around. The only recovery is a raft snapshot taken
*before* the loss:

```
vault operator raft snapshot save backup.snap   # while still unsealed, BEFORE any loss
vault operator raft snapshot restore backup.snap # onto a fresh, newly-initialized Vault
```

**No snapshot schedule exists yet.** This is a real gap, not resolved by this epic -
tracked for Epic 9 (Backup, DR & Incident Readiness). Until a schedule exists, take a
manual snapshot after any significant change to what Vault holds (new secrets
engines, new roles, bulk secret writes).

**Root token lost or suspected compromised:** revoke it with a different valid token
(`vault token revoke <accessor>`), or if no other token exists, regenerate a new root
token via `vault operator generate-root` - this also requires the key-share holders
to participate, by design.

## After unsealing, before anything else

Run `infra/terraform/vault/` to configure the KV engine, Kubernetes auth method, and
policies/roles - see that directory's README for the exact order. Nothing in that
Terraform can run against a sealed or uninitialized Vault.
