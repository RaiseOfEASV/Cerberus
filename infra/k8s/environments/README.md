# GitOps environments

Each directory is synced by one ArgoCD app-of-apps Application from
`infra/k8s/argocd/root-apps.yaml`:

| Directory | Root app | Sync | Namespace(s) |
|---|---|---|---|
| `dev/` | `root-dev` | automated | `dev` |
| `staging/` | `root-staging` | automated | `staging` |
| `prod/` | `root-prod` | **manual** | `cerberoast` (cerberoast.site) |

Each app is a multi-source child Application: the generic chart
`infra/helm/charts/app` plus `infra/helm/<app>/values-<env>.yaml`. Image tag
and digest in those values files are what promotion changes - by hand for
dev, via `.github/workflows/deploy.yml` for staging/prod.

## Team previews of dev/staging (admin side)

`dev/admin-preview.yaml` and `staging/admin-preview.yaml` expose the quote
apps to the team at:

- https://quote-dev.admin.cerberus.internal
- https://quote-staging.admin.cerberus.internal

served by the internal Traefik (10.0.2.50, VPN only), behind the same
Keycloak/oauth2-proxy login as Vault and ArgoCD (group `admins`), with an
internal-CA certificate. `/api/*` goes to quote-api on the same origin.

One-time steps outside git:

1. **Keycloak**: in the `cerberus-admin` realm, client `oauth2-proxy` →
   Valid redirect URIs, add
   `https://quote-dev.admin.cerberus.internal/oauth2/callback` and
   `https://quote-staging.admin.cerberus.internal/oauth2/callback`
   (already recorded in `infra/keycloak/realm-config.yaml`). Do this in the
   admin console - don't re-run the realm-config Job, it resets every
   client's secret.
2. **Name resolution on each admin device**: VPN clients run with
   `--accept-dns=false`, so add to the hosts file (as for the other admin
   UIs): `10.0.2.50 quote-dev.admin.cerberus.internal quote-staging.admin.cerberus.internal`.
   The headscale `extra_records` already list both for when MagicDNS is enabled.
3. The device must trust the internal CA (same as for Vault/ArgoCD).
