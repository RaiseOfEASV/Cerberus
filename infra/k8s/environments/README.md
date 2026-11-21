# GitOps environments

Each directory is synced by one ArgoCD app-of-apps Application from
`infra/k8s/argocd/root-apps.yaml`:

| Directory | Root app | Sync | Namespace(s) |
|---|---|---|---|
| `dev/` | `root-dev` | automated | `dev` |
| `staging/` | `root-staging` | automated | `staging` |
| `prod/` | `root-prod` | apps automated (no prune), **root manual** | `cerberoast` (cerberoast.site) |

Each app is a multi-source child Application: the generic chart
`infra/helm/charts/app` plus `infra/helm/<app>/values-<env>.yaml`. Image tag
and digest in those values files are what promotion changes. `master` is
protected (PR + review + signed commits), so every change lands through a
reviewed PR:

- **dev**: edit `values-dev.yaml` (tag + digest) in a normal PR. After the
  merge, `deploy-dev.yml` records the release in GitHub's Deployments list
  (`dev` environment); ArgoCD does the deploying.
- **staging/prod**: run `.github/workflows/deploy.yml`. One run deploys the
  whole release (backend and frontend together) as one PR:

  ```
  # 1. staging: every app at the commit CI built
  gh workflow run deploy.yml -f environment=staging -f source=tag -f version=<commit-sha>
  # 2. after testing staging: prod gets exactly what staging runs
  gh workflow run deploy.yml -f environment=prod -f source=staging
  ```

  `apps` defaults to `all`; pass `-f apps=vuln-web` to deploy one app. The
  workflow resolves each digest (or copies staging's), commits the bump on a
  `deploy/*` branch (signed by GitHub) and opens a PR. prod first waits for the `production` environment
  approval. The PR is merged (by a reviewing teammate, or by an admin via
  the ruleset bypass - so one admin *can* promote to prod alone; see
  deploy.yml's header); ArgoCD then deploys it - staging and the prod apps
  both sync automatically.

## Syncing prod

- **Image change** (the usual release): nothing to do - the prod apps sync
  automatically when the bump PR merges. They never prune, so removing a
  resource from a prod app still needs a manual sync with prune.
- **Changes under `infra/k8s/environments/prod/` or `infra/k8s/cerberoast/`**
  (new app, routing, certificate, policies): `root-prod` is manual - sync it.

Without the `argocd` CLI the same is done by setting the Application's
`operation` (what the CLI does under the hood):

```bash
K="--kubeconfig infra/terraform/cluster/kubeconfig.yaml"
kubectl $K -n argocd patch application vuln-web-prod --type merge \
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

`dev/vulnman.yaml` and `staging/vulnman.yaml` expose VulnMan to the team at:

- https://vulns-dev.admin.cerberus.internal
- https://vulns-staging.admin.cerberus.internal

served by the internal Traefik (10.0.2.50, VPN only) with an internal-CA
certificate. There's no oauth2-proxy gate: VulnMan has its own login (the
`vulnman-dev` / `vulnman-staging` realms of `keycloak-public`). `/api` and
`/auth` go to vuln-api on the same origin.

One-time steps outside git:

1. **Name resolution on each admin device**: VPN clients run with
   `--accept-dns=false`, so add to the hosts file:
   `10.0.2.50 vulns-dev.admin.cerberus.internal vulns-staging.admin.cerberus.internal`.
   The headscale `extra_records` list both for when MagicDNS is enabled.
2. The device must trust the internal CA (same as for Vault/ArgoCD).
3. Test users per role are created by hand in the realm (see
   `infra/k8s/keycloak-public/README.md`).
