# ArgoCD (app-of-apps)

GitOps controller. `.github/workflows/deploy.yml` (Epic 3) never runs kubectl/helm -
it only commits an image-tag change to `infra/k8s/environments/<env>/`; ArgoCD
reconciles that onto the cluster from here, under its own in-cluster credentials.

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

`root-prod`'s `syncPolicy.automated` is `null` on purpose. `deploy.yml`'s `production`
GitHub Environment already requires a reviewer before it'll commit a change into
`infra/k8s/environments/prod/` - letting ArgoCD auto-sync on top of that would make
the reviewer gate cosmetic (the moment the commit lands, it ships regardless). Sync
prod deliberately: `argocd app sync root-prod`.

## Not yet useful

`infra/k8s/environments/{dev,staging,prod}/` only has a `.gitkeep` in each - no
kustomization, no app manifests. These Applications will report "no resources found"
until that changes. Not a bug, just sequencing - same pattern as Epic 2/3's
not-yet-runnable pieces.
