# Cerberus

Security-first monorepo: Go backend services, JS/TS frontend app(s), and all
the platform infrastructure (CI/CD, secrets, Kubernetes, observability, SIEM)
they run on.

Built baseline-first: the platform came before application code. The first
real app, **quote-api + quote-web**, runs on it since 2026-11-09 and serves
[cerberoast.site](https://cerberoast.site) (API at `api.cerberoast.site`).
See [`docs/security/threat-model.md`](docs/security/threat-model.md) for the
living threat model.

## Current state (2026-11-09)

| Area | Status |
|---|---|
| Cluster (k3s on Azure), MetalLB, Traefik (admin + public), ArgoCD, Kyverno | running |
| Vault + External Secrets, cert-manager (internal CA + Let's Encrypt) | running |
| headscale VPN, Keycloak + oauth2-proxy in front of every admin UI | running |
| Backups: backup MinIO host, Vault raft-snapshot CronJob | running |
| CI: `pr-validate`, `build`, `base-images` on GitHub-hosted runners | running, green |
| `master` protection (PR, review, signed commits) + `production` environment | applied |
| quote-api / quote-web | live in dev, staging and **prod** (cerberoast.site, api.cerberoast.site) |
| ARC self-hosted runners, Nexus, observability/SIEM stack, Velero, kube-bench, golden VM image | written as code, not deployed |

## Layout

```
apps/                 Apps - one directory each, see apps/README.md
pkg/                  Shared Go libraries
docker/base/          Central distroless base images (base-go, base-node)
infra/helm/charts/app Generic chart every app is deployed with
infra/helm/<app>/     Per-app, per-environment values (image tag + digest)
infra/helm/values/    Values for third-party platform charts
infra/k8s/            Platform manifests; environments/ = ArgoCD app-of-apps per env
infra/terraform/      Azure VMs + cluster, Vault config, GitHub rulesets/environments
infra/packer/         Golden VM image definitions + Ansible roles
infra/{headscale,terraform-state,backups,wazuh}/  Standalone hosts (docker compose)
docs/security/        Threat model, incident response, Vault unseal, restore drills
docs/superpowers/     Design specs and implementation plans
tools/                Shared scripts
```

## Where to start

- **Contributing** (signed commits, PRs, how merging works): [`CONTRIBUTING.md`](CONTRIBUTING.md)
- **Adding or changing an app**: [`apps/README.md`](apps/README.md)
- **Deploying / promoting** dev → staging → prod: [`infra/k8s/environments/README.md`](infra/k8s/environments/README.md)
- **Cluster platform**: [`infra/k8s/README.md`](infra/k8s/README.md), [`infra/k8s/argocd/README.md`](infra/k8s/argocd/README.md)
- **Admin access** (VPN + Keycloak): [`infra/headscale/README.md`](infra/headscale/README.md), [`infra/k8s/keycloak/README.md`](infra/k8s/keycloak/README.md)

Run `just` from the repo root to see available commands.
