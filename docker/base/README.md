# Central base images

Every app's Dockerfile builds its final runtime stage `FROM` one of these,
instead of pulling an upstream base directly. One place to fix a CVE or bump
a runtime version instead of touching every app.

- `go/Dockerfile` → `ghcr.io/raiseofeasv/base-go`
- `node/Dockerfile` → `ghcr.io/raiseofeasv/base-node`

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

## Adding a new base (e.g. Python)

1. New directory here with a minimal `Dockerfile` (distroless runtime stage,
   non-root).
2. Add it to the `matrix.image` list in `base-images.yml`.
3. Nothing else changes - Renovate and the workflow already generalize.
