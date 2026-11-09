# ArgoCD (app-of-apps)

GitOps controller. `.github/workflows/deploy.yml` never runs kubectl/helm - it
opens a PR bumping an image tag + digest in `infra/helm/<app>/values-<env>.yaml`;
once merged, ArgoCD reconciles that onto the cluster from here, under its own
in-cluster credentials. See `infra/k8s/environments/README.md` for the full flow.

## Order of operations (after Epic 4's cluster exists)

1. `helm install argocd argo/argo-cd -n argocd --create-namespace -f ../../helm/values/argocd-values.yaml`
2. `kubectl apply -f root-apps.yaml`
3. Reach the UI at `https://argocd.admin.cerberus.internal` over headscale, once
   Epic 8's IngressRoute/oauth2-proxy/Keycloak are applied
   (`infra/k8s/oauth2-proxy/ingressroutes.yaml`). Before that's done, `kubectl
   port-forward svc/argocd-server -n argocd 8080:80` still works as a fallback -
   never expose this via a public ingress either way.

## Logging in - two separate options, deployed and verified

- **The default `admin` user** - password from `argocd-initial-admin-secret`
  initially, same as any fresh install. Still works as a break-glass path.
- **"Log in via Keycloak"** - real native SSO (`configs.cm.oidc.config` in
  `../../helm/values/argocd-values.yaml`), separate from the oauth2-proxy gate
  in front of this UI - that gate only checks "is this request allowed through
  at all," this is ArgoCD's own login, mapping the Keycloak `admins` group to
  `role:admin` via `configs.rbac`. See `infra/k8s/keycloak/README.md` for the
  full setup (dedicated `argocd` Keycloak client, the CA-trust fix this needed
  for its own outbound discovery call, and a real recurring hazard around
  re-running the realm-config Job that's worth reading before touching any of
  this again).

## Why prod doesn't auto-sync

`root-prod` and its children (`quote-api-prod`, `quote-web-prod`) have no
automated sync on purpose. A prod change already passes the `production` GitHub
Environment approval and a PR review; auto-sync on top would make the final step
implicit. Sync prod deliberately: `argocd app sync root-prod`, then the changed
app (e.g. `argocd app sync quote-web-prod`).

## What's deployed through it (2026-11-09)

| Root app | Children | Sync |
|---|---|---|
| `root-dev` | `quote-api-dev`, `quote-web-dev` (+ admin preview ingress) | automated |
| `root-staging` | `quote-api-staging`, `quote-web-staging` (+ admin preview ingress) | automated |
| `root-prod` | `quote-api-prod`, `quote-web-prod`, and `infra/k8s/cerberoast/` | manual |

Children are multi-source: the generic chart `infra/helm/charts/app` plus a
`ref: values` source; `$values` resolves to the repo root, so value files are
referenced by full path (`$values/infra/helm/<app>/values-<env>.yaml`).

## Known gotchas

- **Kyverno pins images to a digest at admission** (`verify-image-signatures`).
  Values files therefore carry `image.digest` alongside `image.tag`, so git
  matches what runs; without it the Deployment shows OutOfSync forever.
- **A sync stuck retrying an old revision** (e.g. one Kyverno rejected) blocks
  newer automated syncs until it gives up. Terminate it
  (`argocd app terminate-op <app>`, or patch
  `status.operationState.phase=Terminating`) and the next sync picks up HEAD.
