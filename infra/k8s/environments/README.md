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
and digest in those values files are what promotion changes. `master` is
protected (PR + review + signed commits), so every change lands through a
reviewed PR:

- **dev**: edit `values-dev.yaml` (tag + digest) in a normal PR.
- **staging/prod**: run `.github/workflows/deploy.yml`
  (`gh workflow run deploy.yml -f app=<app> -f version=<tag> -f environment=<staging|prod>`).
  It resolves the digest, commits the bump on a `deploy/*` branch (signed by
  GitHub) and opens a PR. prod first waits for the `production` environment
  approval. A teammate merges the PR; staging then auto-syncs, prod needs a
  manual `argocd app sync <app>-prod`.

## Syncing prod

Prod Applications never sync on their own. After a prod bump PR merges:

- **Image change only** (the usual case): sync the changed app -
  `argocd app sync quote-web-prod`.
- **Changes under `infra/k8s/environments/prod/` or `infra/k8s/cerberoast/`**
  (new app, routing, certificate): sync `root-prod`.

Without the `argocd` CLI the same is done by setting the Application's
`operation` (what the CLI does under the hood):

```bash
K="--kubeconfig infra/terraform/cluster/kubeconfig.yaml"
kubectl $K -n argocd patch application quote-web-prod --type merge \
  -p '{"operation":{"initiatedBy":{"username":"<you>"},"sync":{"revision":"master"}}}'
```

**When a change adds an app *and* routes traffic to it**, sync in this order so
the site never points at something that isn't running (this is how the first
cutover was done):

1. root-prod, only the new Application(s) - add
   `"resources":[{"group":"argoproj.io","kind":"Application","name":"<app>-prod","namespace":"argocd"}]`
   to the `sync` object above;
2. the new `<app>-prod` Application(s); wait until the pods are Ready;
3. root-prod in full (IngressRoute, Certificate, namespace, NetworkPolicy).

`infra/k8s/cerberoast`'s namespace is annotated `Prune=false,Delete=false`, so
no root-prod sync can delete it (or the Let's Encrypt certificate in it).

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
