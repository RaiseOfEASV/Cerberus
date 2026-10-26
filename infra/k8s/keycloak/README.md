# Keycloak

OIDC provider for the admin-UI identity layer, at two separate levels:

1. **The oauth2-proxy ForwardAuth gate** (`infra/k8s/oauth2-proxy/`) - checks
   "is this request allowed through to Traefik's backend at all," before Vault/
   ArgoCD ever see it. One shared Keycloak client (`oauth2-proxy`).
2. **Native SSO inside each app** - Vault's own `oidc` auth method
   (`infra/terraform/vault/`) and ArgoCD's own `oidc.config`
   (`infra/helm/values/argocd-values.yaml`). Each app has its own dedicated
   Keycloak client (`vault`, `argocd`), separate from oauth2-proxy's, since
   each has its own callback URL(s). Both layers gate on the same `admins`
   Keycloak group, mapped to real in-app permissions (Vault's `admin` policy,
   ArgoCD's `role:admin`) - neither layer alone, both together.

**Deployed and verified working** (2026-10-26): all three clients, the gate,
and both native SSO logins have been confirmed end-to-end against a real
browser session, not just config review.

**The gate deliberately does NOT auto-redirect to Keycloak login.** Visiting
`https://vault.admin.cerberus.internal/` or `https://argocd.admin.cerberus.internal/`
unauthenticated gets a real `401 Unauthorized` with oauth2-proxy's own styled
sign-in page as the body (a "Sign in with Keycloak OIDC" button) - no
`Location` header, nothing navigates until that button is actually clicked.
This was tried two other ways first: a real `302` via Traefik's `errors`
middleware + `statusRewrites` (reverted - an instant redirect confirms to
anyone who can merely reach the hostname, authenticated or not, that it's
live and exactly which Keycloak realm fronts it), and a completely bare `401`
with no body at all (reverted too - no way for a legitimate user to know how
to log in). Querying `/oauth2/sign_in` instead of `/oauth2/start` in
`infra/k8s/oauth2-proxy/forwardauth-middleware.yaml`'s `oauth2-proxy-errors`
middleware is what gets this real page without the disclosure a redirect
would cause.

## Order of operations

1. Epic 4's cluster, Epic 6's Vault/ESO, Epic 8's cert-manager + headscale.
2. `vault kv put secret/keycloak/admin username=admin password=<...>`
   `vault kv put secret/keycloak/db password=<...>`
3. `kubectl apply -f ../eso/external-secret-keycloak.yaml`
4. Install the `keycloak` chart (`../../helm/values/keycloak-values.yaml`).
5. For each of the three clients, generate a secret and store it in Vault,
   then apply the matching ExternalSecret(s):
   - `oauth2-proxy/client`: `vault kv put secret/oauth2-proxy/client secret=<...> cookie-secret=<...>`,
     then `../eso/external-secret-oauth2-proxy.yaml` and
     `../eso/external-secret-oauth2-proxy-client-for-keycloak.yaml`.
   - `oidc/vault-client`: `vault kv put secret/oidc/vault-client secret=<...>`,
     then `../eso/external-secret-vault-oidc-client.yaml`.
   - `oidc/argocd-client`: `vault kv put secret/oidc/argocd-client secret=<...>`,
     then `../eso/external-secret-argocd-oidc-client.yaml`.
   `apply-realm-config.yaml` needs all three already synced.
6. `kubectl create configmap keycloak-realm-config --from-file=../../keycloak/realm-config.yaml -n keycloak`
   then `kubectl apply -f ../../keycloak/apply-realm-config.yaml`.
7. Create at least one real admin user in the `cerberus-admin` realm, **with
   Email verified toggled on** (no SMTP is configured for this realm, so a new
   user's email starts unverified by default - oauth2-proxy rejects an
   unverified email outright, confirmed the hard way with a real "email in
   id_token isn't verified" error), and add them to the `admins` group -
   `realm-config.yaml` defines the group, not any users (no real person's
   credentials belong in a committed file).
8. Configure Vault's own OIDC auth method - see
   `infra/terraform/vault/README.md`'s "Native SSO" section. ArgoCD's native
   OIDC needs no equivalent manual step; its secret comes entirely through
   `argocd-secret` via ESO (`../eso/external-secret-argocd-oidc-client.yaml`).
9. Both apps' Traefik IngressRoutes need their own OIDC callback path carved
   out of the oauth2-proxy gate (`PathPrefix(/v1)` for Vault,
   `PathPrefix(/auth/callback)` for ArgoCD) - already done in
   `infra/k8s/vault/ingressroute.yaml` / `infra/k8s/argocd/ingressroute.yaml`,
   but if either app's callback path ever changes, this needs updating too.
   Without it, the native login loops forever (the callback itself gets
   gated, finds no session yet, and gets redirected into a brand-new login
   instead of completing the one in progress).
10. ArgoCD's and oauth2-proxy's own pods need to trust the internal CA for
    their own outbound calls to Keycloak's discovery endpoint - already wired
    via `infra/k8s/argocd/ca-configmap.yaml` / `infra/k8s/oauth2-proxy/ca-configmap.yaml`.
    Vault's side of this is the `oidc_discovery_ca_pem` argument in step 8.

## Recurring hazard: re-running the realm-config Job resets other clients' secrets

**Confirmed real, hit three times in one session.** Re-running
`apply-realm-config.yaml` for any reason - even just adding a brand-new
client - silently regenerates the `secret` of clients that were NOT
intentionally changed too, breaking their already-working logins. Root cause
not pinned down (keycloak-config-cli's own docs/issues didn't give a clear
answer). Until/unless that's fixed:

After every run of this Job, for each of the three clients
(`oauth2-proxy`/`vault`/`argocd`), check whether its secret still matches
what Keycloak has - don't assume only the client you meant to change was
affected. The reliable test (via Keycloak's own token endpoint, with a
deliberately fake authorization code - this exercises real client
authentication without needing a valid session):
```
curl -d grant_type=authorization_code -d code=fake -d redirect_uri=<the client's real redirect_uri> \
  -d client_id=<client> --data-urlencode client_secret=<its current value> \
  https://keycloak.admin.cerberus.internal/realms/cerberus-admin/protocol/openid-connect/token
```
`{"error":"invalid_grant","error_description":"Code not valid"}` means the
secret is fine (it got past client auth to reject the fake code).
`{"error":"unauthorized_client","error_description":"Invalid client or Invalid client credentials"}`
means it's broken - regenerate via Keycloak's own admin API
(`POST /admin/realms/cerberus-admin/clients/<uuid>/client-secret`), write the
result into Vault at that client's path, force an ESO resync, restart
whichever pod consumes it (oauth2-proxy's own pod / `vault write
auth/oidc/config` again / `argocd-server`).

## Not yet done

- Grafana/Wazuh/Kibana/Dependency-Track aren't deployed yet - once they are,
  each needs the same two-layer treatment (oauth2-proxy gate + possibly their
  own native OIDC, if they support it) following this same pattern.
- ArgoCD's `/api/*` (used by the `argocd` CLI with a bearer token, not a
  browser cookie) is still fully behind the oauth2-proxy gate - only
  `/auth/callback` got a carve-out, covering the browser login flow but not
  CLI/automation access through the gated hostname.
