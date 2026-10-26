# SSO break-glass: Vault and ArgoCD

Keycloak + oauth2-proxy are the only intended way into Vault's and ArgoCD's
UIs (2026-10-26). ArgoCD's local `admin` account is disabled outright
(`admin.enabled: "false"`, `infra/helm/values/argocd-values.yaml`) - there is
no parallel password-based login sitting next to the Keycloak one. If
Keycloak or oauth2-proxy is down, broken, or misconfigured, the UI is simply
unreachable until it's fixed. This is deliberate, not an oversight: a
password login that bypasses the identity layer defeats the point of having
one.

The break-glass path for both is the same: **direct cluster access
(`kubectl`), not a parallel UI login.** If you can reach the cluster, you can
always recover access to either app - this matches the pattern this project
already uses for Vault's own unseal (`vault-unseal-procedure.md`) and the
VPN (`infra/headscale/README.md`): the deepest layer of access always comes
back to someone with real infrastructure access, not another password.

## ArgoCD

Admin is disabled, not deleted - re-enabling it doesn't need a reinstall.

1. Re-enable the account:
   ```
   kubectl patch configmap argocd-cm -n argocd --type merge \
     -p '{"data":{"admin.enabled":"true"}}'
   kubectl rollout restart deployment argocd-server -n argocd
   ```
2. Get (or reset) the admin password:
   - If `argocd-initial-admin-secret` still exists (it's deleted after first
     login per ArgoCD's own setup guide - it may not):
     ```
     kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
     ```
   - If it doesn't, set a new one directly. Generate a bcrypt hash (the
     `argocd-server` pod already has the CLI) and patch `argocd-secret`:
     ```
     kubectl exec -n argocd deploy/argocd-server -- argocd account bcrypt --password '<new password>'
     ```
     Take that hash and patch it in (never put the plaintext password in a
     `kubectl patch` command - it'd be visible via `ps` and shell history):
     ```
     kubectl patch secret argocd-secret -n argocd --type merge \
       -p "{\"stringData\":{\"admin.password\":\"<bcrypt hash>\",\"admin.passwordMtime\":\"$(date +%FT%T%Z)\"}}"
     ```
3. Log in as `admin` with that password, fix whatever broke in the
   Keycloak/oauth2-proxy chain, then **disable admin again**
   (`admin.enabled: "false"`, same patch as step 1 with `false`, or re-apply
   `argocd-values.yaml` via `helm upgrade`) once it's confirmed working.
   Don't leave the local account enabled "just in case" - that's exactly the
   parallel-login path this setup is meant to remove.

## Vault

Vault was never SSO-only to begin with - its own `token`-based access (the
scoped `admin` policy token from `vault-unseal-procedure.md`, or a fresh one
minted from an unseal-key holder's root token) has always been the real
break-glass path, independent of whatever the UI shows. The `kubernetes` auth
method's `listing_visibility: hidden` tune
(`infra/terraform/vault/main.tf`) only hides it from the UI's own
method-discovery list - it has no effect on API/CLI authentication, and
neither does anything about `oidc`'s visibility.

If Keycloak/oauth2-proxy breaks and OIDC login stops working:
1. Use the CLI directly against the UI hostname's carved-out `/v1/*` API path
   (reachable without going through oauth2-proxy at all, see
   `infra/k8s/vault/ingressroute.yaml`): `vault login` with a real token, or
   `VAULT_ADDR=https://vault.admin.cerberus.internal vault <command>`.
2. Or, from inside the cluster, `kubectl exec -n vault vault-0 -- vault ...`
   directly - this never goes through Traefik/oauth2-proxy/Keycloak at all.
3. Vault's own UI will still show a "Token" tab regardless of any `oidc`
   configuration or outage - paste a valid token there for a UI session
   without touching Keycloak.

Nothing to "restore" afterward on Vault's side - no account gets disabled,
there's no equivalent toggle to flip back.

## What this does and doesn't protect against

**Does**: stops a casual browser visit to either UI from reaching real
content without authenticating through Keycloak first, and removes ArgoCD's
local password login as an alternative, lower-friction path in.

**Doesn't**: this is not a defense against someone who already has a valid
Vault token or cluster (`kubectl`) access - by design, those remain
sufficient on their own, the same way they always have been. SSO-only means
"no second front door for a human browsing the UI," not "no other way to
administer the system at all." Removing *that* would also remove the actual
break-glass path this doc describes.
