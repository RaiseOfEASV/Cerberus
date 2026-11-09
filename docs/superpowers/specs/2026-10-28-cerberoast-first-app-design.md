# First real app: quote-api + quote-web

## Intent and why

Cerberus was built infra-first: the `apps/` convention (per-app `Dockerfile` +
`Justfile`, auto-discovered by `build.yml`/`release.yml`), the Go/Node base
images, and the full GitOps path to `cerberoast.site` all exist, but have
never been exercised by a real application - `cerberoast.site` still serves
a static nginx placeholder, and `infra/k8s/environments/{dev,staging,prod}/`
are empty stubs. This project ships the first real app (a Go backend that
returns a random quote, and a React frontend that displays it) and, in doing
so, nails down the concrete convention every future app under `apps/` will
copy: how an app is structured, built, charted, and promoted dev → staging →
prod. It also stands up Nexus (Epic 2) as a decoupled milestone, at the
user's explicit request.

## Goals

- Establish the reusable pattern for apps under `apps/`: directory layout,
  `Dockerfile`, `Justfile` contract, Helm chart shape, GitOps wiring.
- Ship `apps/quote-api` (Go) and `apps/quote-web` (React+TypeScript), two
  independently built/deployed apps.
- Exercise the full dev → staging → prod ArgoCD promotion path
  (`infra/k8s/argocd/root-apps.yaml`) for the first time, for real.
- Replace the placeholder on `cerberoast.site` (prod) with `quote-web`;
  `quote-api` becomes reachable at `api.cerberoast.site`.
- Deploy Nexus (Epic 2) for real: namespace, ExternalSecret, Helm release,
  `provision.sh` run as a one-off Job.

## Non-goals

- Deploying the ARC self-hosted runner pool (Epic 3) - explicitly deferred.
  `pr-validate.yml`/`build.yml`/`release.yml` run on GitHub-hosted runners for
  now, documented as a temporary exception (same pattern `base-images.yml`
  and `golden-image.yml` already use).
- Wiring up DAST (`docs/security/dast-targets.yaml` stays `targets: []`) -
  that file's own convention assumes a staging URL reachable from the ARC
  runner network, which doesn't exist yet.
- Persistence for quotes - a static in-memory list is the whole point of
  "simple."
- Public internet exposure for dev/staging - consistent with
  `dast-targets.yaml`'s own existing assumption that staging is reached via
  `*.svc.cluster.local`, not a public hostname.

## Anti-goals

Things that would make this a failure even if it "technically works":

- Any shortcut against the Kyverno baseline (non-root, read-only rootfs,
  resource limits, no `:latest`, signed+attested images) - the user was
  explicit: no shortcuts.
- Baking an environment-specific API URL into the frontend's build output.
  `release.yml` never rebuilds; the same image digest is promoted across
  dev/staging/prod. A build-time constant would silently break on promotion.
- Quietly expanding scope into ARC or DAST instead of naming the gap (both
  are named above, not solved here).

## Constraints

- No self-hosted runner pool exists yet (confirmed live: no `arc-*`
  namespace in the cluster). CI must run on `ubuntu-latest` for now.
- Nexus is cluster-internal only, no ingress, by its own design
  (`nexus-values.yaml`) - deploying it does not speed up this app's CI on
  GitHub-hosted runners. It's Epic 2 groundwork, decoupled from this app.
- `cerberoast` namespace, `traefik-public`, and the `letsencrypt-public`
  `ClusterIssuer` already exist and serve the live placeholder - prod cutover
  replaces that deployment, it doesn't duplicate the namespace/ingress setup.
- Nexus admin credentials are a manual Vault step by design
  (`vault kv put secret/nexus/admin ...`) - the user runs this, same pattern
  as every other Vault secret in this repo.

## Decided

- **Layout**: `apps/quote-api` (Go, own `go.mod`) and `apps/quote-web`
  (React + TypeScript + Vite), two separate app directories, each with its
  own `Dockerfile` and `Justfile` (`build`/`test`/`lint`) - matches the
  existing `apps/*/Justfile` fan-out in the root `Justfile` and the
  discover-by-`Dockerfile` matrix in `build.yml`/`release.yml`.
- **quote-api**: `GET /quote` → `{"quote": "..."}` picked from a small
  in-memory slice; `GET /healthz` for k8s probes. CORS allowing exactly the
  frontend's origin (different subdomain now). Dockerfile final stage
  `FROM ghcr.io/raiseofeasv/base-go`.
- **quote-web**: Vite+React SPA. Served at runtime by
  `nginxinc/nginx-unprivileged` (same image the current placeholder already
  uses). The build stage uses plain `node:22-alpine` (public, not
  `base-node`) - `docker/base/README.md`'s central-base rule only applies to
  an app's *final runtime stage*, and `base-node` is distroless/no-shell
  anyway, so it cannot run `npm run build` at all. The API base URL is
  injected at container start (small entrypoint script writing a runtime
  config, not a build-time constant) - required by the constraint above.
- **Helm**: one generic, reusable chart (`infra/helm/charts/app/`) -
  Deployment, Service, Traefik `IngressRoute`, `NetworkPolicy`, with
  non-root/read-only-rootfs/resource-limits baked in once instead of
  duplicated per app. `quote-api` and `quote-web` each get
  `infra/helm/quote-api/values-{dev,staging,prod}.yaml` (and the `-web`
  equivalent) instantiating that chart.
- **GitOps wiring**: `infra/k8s/environments/{dev,staging,prod}/` each get a
  real `kustomization.yaml` listing ArgoCD `Application` children for both
  apps. The chart (`infra/helm/charts/app/`) and the per-env values file
  (`infra/helm/quote-api/values-<env>.yaml`) live in different directories,
  so each child `Application` uses ArgoCD's **multi-source** form: one
  source for the chart, a second (`ref: values`) for
  `infra/helm/quote-api/`, with `helm.valueFiles: ['$values/values-<env>.yaml']`.
  This is the standard ArgoCD pattern for "one chart, values elsewhere in
  the same repo" - the plain Kustomize-path `Application`s already in
  `root-apps.yaml` have no precedent for it, so it's called out explicitly.
  `root-apps.yaml`'s three `Application`s also get
  `syncOptions: [CreateNamespace=true]` added, since no `dev`/`staging`/
  `prod` `namespace.yaml` exists anywhere today - Kyverno's existing
  `enforce-namespace-baseline.yaml` (`generate: synchronize: true`) applies
  the restricted-PSA + default-deny-NetworkPolicy baseline automatically to
  whatever namespace appears, the same as every namespace created so far.
- **dev/staging**: cluster-internal only (`ClusterIP`, `*.svc.cluster.local`)
  - no new public DNS/TLS. Matches `dast-targets.yaml`'s existing assumption.
- **prod**: public, and deploys into the existing `cerberoast` namespace,
  not the generic `prod` namespace `root-prod` creates. The two are
  deliberately different: `prod` (via `CreateNamespace=true`) is there for
  future prod apps that aren't `cerberoast.site` itself; it stays unused by
  this project. `quote-api`'s and `quote-web`'s prod `Application`s set
  `destination.namespace: cerberoast` explicitly, overriding `root-prod`'s
  default, so they land where the live Certificate/NetworkPolicy already
  are instead of duplicating that setup. The existing
  `allow-traefik-public-ingress` `NetworkPolicy` (`podSelector: {}`, i.e.
  namespace-wide) already covers any new pod here with no edits needed.
  `infra/k8s/cerberoast/{namespace,network-policy}.yaml` move from
  hand-applied to ArgoCD-managed (listed in prod's `kustomization.yaml`) so
  nothing in this shared namespace drifts between a hand-applied resource
  and a chart-managed one. `quote-web` replaces the placeholder
  Deployment/Service/ConfigMap on `cerberoast.site`/`www.cerberoast.site`
  (the placeholder's three resources are deleted, not left orphaned).
  `quote-api` gets `api.cerberoast.site`: `infra/k8s/cerberoast/
  ingressroute.yaml` gets a new rule (same `cerberoast-site-tls`
  `Certificate`, whose `dnsNames` gains `api.cerberoast.site`; same
  `letsencrypt-public` issuer and public IP; labeled
  `traefik-instance: public` same as today). **Needs one new Cloudflare DNS
  A record (`api.cerberoast.site` → the existing public IP) - a user
  action**, same as the original `cerberoast.site` cutover.
- **CORS**: `quote-api`'s allowed origin is a per-environment chart value,
  not hardcoded. dev/staging are cluster-internal only (non-goal above) and
  not security-sensitive, so they allow any origin, which also keeps
  `kubectl port-forward`-based manual testing (`http://localhost:<port>`
  origin) working without extra config. prod locks to the exact
  `https://cerberoast.site`/`https://www.cerberoast.site` origins only.
- **CI**: `pr-validate.yml`/`build.yml`/`release.yml`/`deploy.yml` - `runs-on`
  switched from `arc-runner-set` to `ubuntu-latest`, with a header comment
  explaining this is temporary pending Epic 3. `deploy.yml`'s tag-bump step
  (currently a literal `TODO(epic-4)` stub) gets implemented for real: it
  edits the target environment's `values-<env>.yaml` `image.tag` field
  (`yq`) and commits/pushes - this is what Goal 3 ("exercise the full
  promotion path... for real") actually requires, and the manifest
  structure it was blocked on is exactly what this project creates.
- **Nexus**: `nexus` namespace + the existing
  `infra/k8s/eso/external-secret-nexus-admin.yaml` + `helm install` with
  `nexus-values.yaml` + `provision.sh` run as a one-off Job, once the user
  has put admin credentials into Vault. Not wired into this app's CI.

## What's left to the builder

- Exact Go/Node toolchain versions (pin per existing Renovate conventions).
- Exact quote list content/count.
- Whether the generic chart includes an HPA (default: no, not needed at this
  scale) and the exact per-app `NetworkPolicy` egress rules.
- `docs/security/dast-targets.yaml` stays untouched (explicit non-goal).
