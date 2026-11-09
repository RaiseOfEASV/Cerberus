# Central base images

Every app's Dockerfile builds its final runtime stage `FROM` one of these,
instead of pulling an upstream base directly. One place to fix a CVE or bump
a runtime version instead of touching every app.

- `go/Dockerfile` → `ghcr.io/raiseofeasv/base-go` (distroless `static-debian12`) -
  used by `apps/quote-api`
- `node/Dockerfile` → `ghcr.io/raiseofeasv/base-node` (distroless
  `nodejs26-debian13`, Node 26 LTS) - no consumer yet (`apps/quote-web` is a
  static SPA served by nginx, so it doesn't need a Node runtime)

Both are published and **public** on GHCR (first published 2026-11-09). The
cluster has no registry pull secret by design - every `ghcr.io/raiseofeasv/*`
package has to be public; flip a new package's visibility in its GitHub
package settings after its first push.

Both are distroless (no shell, no package manager, minimal attack surface)
and run as a non-root user already.

## Lifecycle

- Built, scanned (Trivy), SBOM'd (Syft), and signed (cosign, keyless) by
  `.github/workflows/base-images.yml` on every change under `docker/base/**`,
  plus weekly on a schedule so a Renovate-driven digest bump gets rebuilt and
  rescanned even if nothing else in the Dockerfile changed.
- Renovate tracks the upstream `FROM` digest in each Dockerfile here
  (`renovate.json`'s `dockerfile` manager, `pinDigests: true`) and opens a PR
  whenever a new digest is published upstream.
- App Dockerfiles must pin the tag they consume from GHCR by digest too -
  Renovate tracks that the same way, one ecosystem, one mechanism.
  **Not done yet:** `apps/quote-api` uses `base-go:latest` (no digest existed
  when it was written) - pin it.
- Trivy's gate fails on any *fixed* CRITICAL. When an upstream distroless tag
  lags behind a Debian fix (as `nodejs22-debian12` did with CVE-2026-31789),
  move to a newer upstream tag rather than ignoring the finding.

## Adding a new base (e.g. Python)

1. New directory here with a minimal `Dockerfile` (distroless runtime stage,
   non-root).
2. Add it to the `matrix.image` list in `base-images.yml`.
3. Nothing else changes - Renovate and the workflow already generalize.
