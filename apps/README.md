# Apps

One directory per app. The root `Justfile`, `build.yml` and `release.yml`
discover apps automatically - nothing in CI needs editing to add one.
`quote-api` (Go) and `quote-web` (React) are the reference implementation
of this convention.

## What an app directory needs

| File | Why |
|---|---|
| `Dockerfile` | Its presence makes `build.yml` build, scan, sign and push the app |
| `Justfile` with `build`, `test`, `lint` | `just {build,test,lint}-all` fan out over every app; `pr-validate.yml` runs `just lint-all` |
| `.dockerignore` | Keep `node_modules`, build output, etc. out of the build context |

## Image rules (enforced, not advice)

The cluster's Kyverno policies reject a pod that breaks any of these, so the
image has to make them possible:

- **Final stage `FROM` a central base** (`ghcr.io/raiseofeasv/base-*`, see
  `docker/base/`) where one fits. Build stages can use any public image.
  Exception: a static frontend is served by `nginxinc/nginx-unprivileged`
  (stable line) instead.
- **Numeric non-root `USER`** (e.g. `65532:65532` for distroless, `101` for
  nginx-unprivileged) - Kubernetes can't verify `runAsNonRoot` for a named user.
- **Works with a read-only root filesystem.** List every path it must write
  in the Helm values' `writablePaths` (each becomes an emptyDir).
- **No CRITICAL vulnerability with a fix available** - `build.yml`'s Trivy
  gate fails the build. Keep runtimes current (Node 26 LTS, Go 1.27) and
  `apk upgrade` in Alpine runtime stages.
- **Listens on 8080 and answers `GET /healthz`** - the chart's defaults for
  ports and probes.
- **No environment-specific values baked in.** The same image digest is
  promoted dev → staging → prod; read config from environment variables at
  start-up (quote-web writes `env-config.js` from `API_URL` in its entrypoint).
- **Static frontends: cache headers matter.** Serve `index.html` with
  `no-cache`, hashed assets as `immutable`, and a missing asset as a real 404
  (never the SPA fallback) - otherwise returning visitors get a blank page
  after a deploy. Copy `apps/quote-web/docker/nginx.conf`; its behaviour is
  tested by `apps/quote-web/docker/image-test.sh`.

## Deploying a new app

1. Merge the app directory; `build.yml` publishes
   `ghcr.io/raiseofeasv/<app>:<commit-sha>`, signed.
2. Make the GHCR package **public** (package settings) - the cluster has no
   pull secret.
3. Add `infra/helm/<app>/values-{dev,staging,prod}.yaml` for the generic chart
   `infra/helm/charts/app` (image repository/tag/digest, env, resources,
   `podSecurityContext.runAsUser`, `writablePaths`).
4. Add a child Application per environment under
   `infra/k8s/environments/<env>/` and list it in that directory's
   `kustomization.yaml` (copy the quote-* ones).
5. Promote: see `infra/k8s/environments/README.md`.

Local toolchains: Go 1.27, Node 26, `just`.
