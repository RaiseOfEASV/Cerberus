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
kubectl exec -n vault vault-0 -- vault operator init -key-shares=3 -key-threshold=2
```

This prints **3 unseal key shares** and **1 initial root token**, once, ever. There is
no way to retrieve them again later.

**Run this yourself, in your own terminal - not through an AI assistant or any
shared session.** The output is the one thing this entire procedure exists to
protect; it must never be pasted into a chat, a shared terminal, or anywhere a
single other party (human or otherwise) would end up holding all of it at once -
that defeats Shamir splitting just as completely as writing all 3 shares on one
sticky note would.

### Distributing the shares (solo-operator reality, not the 5-person original)

This is currently a solo project - there's no team to hand separate shares to, so
the "different person" half of the usual advice doesn't apply here. 2-of-3 (not the
textbook 3-of-5) was chosen deliberately for that reason: it still gets the real
benefit that matters for one person - **no single storage location leak is enough
to unseal alone**, and **losing access to any one location doesn't lock you out** -
without pretending there are 5 distinct humans involved.

- Store each of the 3 shares in a **different storage location you control** (e.g.
  password manager, a hardware key/offline note, a separate cloud secrets vault
  outside this project entirely). Never put two shares in the same place.
- The root token is not a key share - store it separately from all 3 shares, and
  treat it as even more sensitive (it's a usable credential by itself, not 2/3 of
  one). After initial setup, create a scoped admin token for day-to-day use
  (`infra/terraform/vault/` applies with whatever token you export as `VAULT_TOKEN` -
  use a scoped one, not the root token, once one exists) and consider revoking the
  root token.
- **If this ever becomes a real team project**, re-key (`vault operator rekey`) to a
  higher share count and hand shares to actual separate people at that point - this
  section's reasoning only holds while it's genuinely one operator.

| Share # | Storage location |
|---------|-------------------|
| 1       | _TODO_ |
| 2       | _TODO_ |
| 3       | _TODO_ |

## Routine unseal (after every pod restart)

Vault is **always sealed on process start** - a restart (pod reschedule, node
reboot, upgrade) reseals it even though the raft data on disk is untouched. Run this
yourself, entering shares from (at least) 2 of your 3 storage locations - still
don't collect them into one script or paste them anywhere:

```
kubectl exec -n vault vault-0 -it -- vault operator unseal
```

Each run prompts for one key share; after the 2nd distinct, valid share, Vault
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

**Lost 1 share:** no problem, the remaining 2 still reach the threshold. Re-key
(`vault operator rekey`) to invalidate the lost share and issue a fresh set of 3
once convenient.

**Lost 2 or more shares (can no longer reach the threshold):** Vault's data is
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
