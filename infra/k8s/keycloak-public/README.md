# keycloak-public (VulnMan's IdP)

A second Keycloak, public at `auth.cerberoast.site`, for VulnMan's users only
(design decision D3 in `docs/superpowers/plans/2026-11-16-vulnman.md`). The admin
Keycloak in `infra/k8s/keycloak/` guards Vault, ArgoCD and oauth2-proxy and stays
VPN-only; keeping them apart means a public-facing bug or brute-force campaign
can't reach the admin realm.

Only `/realms/vulnman`, `/realms/vulnman-dev`, `/realms/vulnman-staging` and
`/resources/` are routed publicly (`ingressroute.yaml`). The admin console is
reached with `kubectl -n keycloak-public port-forward svc/keycloak-public-http 8080:80`.

## Realms

| Realm | App URL | MFA |
|---|---|---|
| `vulnman-dev` | `https://vulns-dev.admin.cerberus.internal` (VPN) | optional |
| `vulnman-staging` | `https://vulns-staging.admin.cerberus.internal` (VPN) | optional |
| `vulnman` | `https://cerberoast.site` | TOTP required on first login (AUTH-05) |

Each has the four app roles (`analyst`, `developer`, `pm`, `auditor`), a
confidential `vuln-api` login client (code flow + PKCE S256 only) and a
`vuln-api-admin` service-account client limited to managing this realm's users.

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
6. Create users by hand in each realm (admin console via port-forward), one
   per role for testing, with temporary passwords. Never commit users.

## Checks

```
curl -s https://auth.cerberoast.site/realms/vulnman/.well-known/openid-configuration | jq .issuer   # "https://auth.cerberoast.site/realms/vulnman"
curl -s -o /dev/null -w '%{http_code}\n' https://auth.cerberoast.site/admin/         # 404
curl -s -o /dev/null -w '%{http_code}\n' https://auth.cerberoast.site/realms/master/ # 404
```
