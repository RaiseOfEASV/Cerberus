# Image catalog and third-party image scanning

## Why

About 40 third-party container images are referenced across the repo:
Dockerfiles, plain Kubernetes manifests, Helm values, docker-compose files on
the VMs, test scripts, and upstream Helm charts installed by hand. Each place
pins versions its own way, several loosely (`postgres:16-alpine`,
`hashicorp/vault:1.17`, `alpine:3.20`, an untagged `minio-client`), and
nothing scans them after they are first pulled. The owner wants one place to
manage those versions and wants every image scanned.

## Goals

- One file, `docker/images.yaml`, lists every third-party image (and the
  upstream Helm chart versions) with an exact tag and digest.
- Every reference in the repo matches the catalog; CI fails otherwise.
- Changing a version is one edit plus one command (`just images-sync`).
- A version bump can't merge if the new image has a fixable CRITICAL CVE.
- Every catalog image is scanned nightly and its vulnerabilities live in
  Cerberoast as findings, opened and closed automatically.

## Non-goals

- Our own images (`ghcr.io/raiseofeasv/*`): `build.yml`/`deploy.yml` already
  pin them per environment and the nightly pod scan already covers them.
- Rendering references from the catalog at build/deploy time (option 2 in the
  discussion): every file keeps a literal image reference.
- Installing the Renovate app (it isn't installed today).

## Anti-goals

- Changing what runs. Images already running are pinned to the digest the
  cluster is running now, so the bits stay the same (pods whose spec changes
  restart once).
- A scan failure closing findings (same rule as the pod scan).

## Approaches considered

1. **Catalog as the record, CI keeps files in sync** (chosen). Literal
   references stay where they are; a checker compares them with the catalog
   and a sync command rewrites them. No change to how anything is built or
   deployed.
2. **Everything reads the catalog at build/deploy time** (build args,
   kustomize `images:`, generated Helm values and `.env` files). True single
   reference, but changes every component's build and deploy path and adds a
   generation step ArgoCD must handle. Rejected: more risk for the same
   control.

Scanning: both a **PR gate in CI** (changed entries only, fail on fixable
CRITICAL) and a **nightly in-cluster scan into Cerberoast** (all entries, prod
only). GitHub's Security tab was rejected: it would split vulnerabilities
across two systems.

## Design

### Catalog (`docker/images.yaml`)

```yaml
images:
  postgres:
    image: postgres
    tag: 16-alpine
    digest: sha256:...
    source: direct        # direct | chart | k3s | build | vm
    status: active        # active | inactive (in the repo, not deployed)
    note: vuln-db, keycloak DBs, backups, terraform-state VM, CI test DB
charts:
  argo-cd: { repo: argo/argo-cd, version: 10.11.1, images: [argocd, redis] }
k3s:
  version: v1.31.0+k3s1
```

- Keys are short names; `image` is the repository as written in the repo.
- `tag` may be empty (digest-only, e.g. Chainguard images that only publish
  `latest`).
- Chart- and k3s-provided images are listed for scanning only; the repo has
  no references to keep in sync for them (their version follows the chart or
  k3s version).
- Initial digests: what the cluster runs today (pod `imageID`); for images
  not running, the registry digest of the current tag.

### `tools/images.py`

Python 3 + PyYAML (already used by the chart render tests). Subcommands:

- `check`: finds every image reference in tracked files and fails on
  - a reference whose repository is in the catalog but whose tag/digest
    differs (`postgres:16-alpine` vs `postgres:16-alpine@sha256:...`);
  - a reference whose repository is not in the catalog;
  - a README `helm install <release> <repo/chart>` without the catalog's
    `--version`;
  - a stale generated scan list (below).
- `sync`: rewrites all of the above to match the catalog (and regenerates
  the scan list).
- `list`: prints `asset<TAB>ref` for every catalog image (scan input).
- `changed --base <git ref>`: the same, only for entries that differ from
  the base revision's catalog (PR gate input).

Reference forms recognised: `FROM <ref>` (Dockerfiles), `image: <ref>`
(manifests, compose, Helm values), Helm values `repository:` followed by
`tag:` (written as `tag: "<tag>@<digest>"`), and `<repo>:<tag>` tokens in
shell scripts for catalog repositories. Ignored: our own images, anything
templated (`{{`, `$`), Kyverno policy patterns, Markdown except the
`helm install` lines, and test fixtures. Repository names compare after
dropping `docker.io/` and `library/`.

### PR gate

- `pr-validate.yml` runs `tools/images.py check` on every PR.
- New `images-scan.yml` runs on PRs that touch `docker/images.yaml`: Trivy on
  each changed entry, `--severity CRITICAL --ignore-unfixed`, exit 1 on a
  hit. Same rule as `build.yml` for our images.
  The PR that introduces the catalog scans nothing: it records what already
  runs rather than bumping anything, and the nightly scan covers it.

### Nightly scan into Cerberoast (prod only)

- The app chart gains an optional `files:` map, rendered as a ConfigMap and
  mounted read-only at `/etc/app`.
- `tools/images.py sync` writes `infra/helm/vuln-scanner/scan-images.yaml`
  (`files: {images.txt: ...}`), added to the prod vuln-scanner Application's
  value files. `check` fails when it is stale.
- vuln-scanner reads `IMAGES_FILE` (`asset<TAB>ref` per line) and scans those
  images too, after the running pods. Asset = the repository name (e.g.
  `postgres`, `quay.io/keycloak/keycloak`), so findings keep the same asset
  across version bumps and auto-close when a bump fixes them.
- Prod's scanner deadline goes to 3 hours (about 40 images, some large).

### Renovate

The config gains a regex manager for `docker/images.yaml` and disables the
docker-datasource managers elsewhere, so a future Renovate install bumps the
catalog only. Its PRs would then need `just images-sync` before the check
passes (documented; no automation while Renovate isn't installed).

## Rollout

One PR. On merge: ArgoCD-managed manifests whose image string changes restart
once on the same digest (vuln-db in each environment, backup CronJobs pick it
up at their next run). Hand-applied components (Vault, Keycloak, VMs) change
only when someone re-applies them. The prod vuln-scanner picks up the catalog
list at its next run.

## Calls made by the builder (not discussed)

- Inactive images (kube-bench, Dependency-Track) are scanned
  like active ones: they are in the repo and could be deployed.
- Build-stage images (golang, node) are scanned too.
- Severity gate in CI is CRITICAL with a fix available, matching `build.yml`.
