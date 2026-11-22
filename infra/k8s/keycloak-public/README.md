# keycloak-public (Cerberoast's IdP)

A second Keycloak, public at `auth.cerberoast.site`, for Cerberoast's users only
(design decision D3 in `docs/superpowers/plans/2026-11-16-vulnman.md`). The admin
Keycloak in `infra/k8s/keycloak/` guards Vault, ArgoCD and oauth2-proxy and stays
VPN-only; keeping them apart means a public-facing bug or brute-force campaign
can't reach the admin realm.

Only `/realms/vulnman`, `/realms/vulnman-dev`, `/realms/vulnman-staging` and
`/resources/` are routed publicly (`ingressroute.yaml`). The admin console is
reached with `kubectl -n keycloak-public port-forward svc/keycloak-public-http 8080:80`.

Runs with a read-only root filesystem and no Kyverno exception: an init
container seeds `/opt/keycloak/lib/quarkus` into an emptyDir, which Keycloak's
boot-time build then writes to (see `keycloak-public-values.yaml`).

## Realms

| Realm | App URL | MFA |
|---|---|---|
| `vulnman-dev` | `https://vulns-dev.admin.cerberus.internal` (VPN) | optional |
| `vulnman-staging` | `https://vulns-staging.admin.cerberus.internal` (VPN) | optional |
| `vulnman` | `https://cerberoast.site` | off (switched off 2026-11-21; `CONFIGURE_TOTP` kept with defaultAction false) |

Display names are "Cerberoast (dev)", "Cerberoast (staging)" and "Cerberoast";
the realm IDs keep their `vulnman*` names.

Each has the four app roles (`analyst`, `developer`, `pm`, `auditor`), a
confidential `vuln-api` login client (code flow + PKCE S256, plus the
password grant used by the app's own sign-in page) and a `vuln-api-admin`
service-account client limited to managing this realm's users.

The prod user profile requires an email, and the password grant can't ask for
missing profile fields, so every account needs one (placeholders like
`manager@example.invalid`, marked verified, are fine).

## Order of operations

1. Vault (by hand, never in git or Terraform):
   ```
   vault kv put secret/keycloak-public/admin username=admin password=<generate>
   vault kv put secret/keycloak-public/db password=<generate>
   for e in dev staging prod; do
     vault kv put secret/vulnman/$e/oidc client_secret=<generate>
     vault kv put secret/vulnman/$e/kc-admin client_secret=<generate>
   done
   ```
   `infra/terraform/vault/` must include `secret/data/keycloak-public/*` and
   `secret/data/vulnman/*` in the `eso-read` policy.
2. `kubectl apply -f namespace.yaml -f network-policy.yaml -f external-secrets.yaml -f postgres.yaml`
3. `helm install keycloak-public codecentric/keycloakx --version 7.4.0 -n keycloak-public -f ../../helm/values/keycloak-public-values.yaml --wait`
4. `kubectl apply -f ingressroute.yaml`, and in Cloudflare an A record
   `auth.cerberoast.site` -> the cluster's public IP (`terraform output cluster_public_ip`).
5. Realms - see the header of `../../keycloak-public/apply-realm-config.yaml`.
   ArgoCD doesn't apply realm files: re-run that Job after every change. The
   files only define realm settings and service accounts, so re-running it
   never touches people's accounts.
6. Create users by hand in each realm (admin console or `kcadm.sh`), never in
   git. Current accounts, named after their role: `manager` (pm), `analyst`,
   `developer`, `auditor` in `vulnman` and `vulnman-dev`, with permanent
   passwords and placeholder emails. `vulnman-staging` has none yet.

## Checks

```
curl -s https://auth.cerberoast.site/realms/vulnman/.well-known/openid-configuration | jq .issuer   # "https://auth.cerberoast.site/realms/vulnman"
curl -s -o /dev/null -w '%{http_code}\n' https://auth.cerberoast.site/admin/         # 404
curl -s -o /dev/null -w '%{http_code}\n' https://auth.cerberoast.site/realms/master/ # 404
```
