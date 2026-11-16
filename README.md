# Cerberus

Cerberus is a security-focused monorepo containing VulnMan, a vulnerability
management application, together with the platform it runs on. It was built
for the Secure Software Development course (fall 2026) at EASV.

Live at [cerberoast.site](https://cerberoast.site).

## Application

VulnMan tracks security findings from intake to remediation.

- **Intake** from manual entry, a public reporting form (`/report`), Trivy
  image and SBOM scans, and CSV import.
- **Triage** by severity, asset and status, with assignment, due dates and
  SLA targets per severity.
- **Roles:** analyst, developer, project manager and auditor, enforced
  server-side.
- **Reporting:** a dashboard with SLA compliance and trends, and signed JSON
  reports that can be verified offline.
- **Security controls:**
  - OIDC login with PKCE, handled by the backend;
  - per-finding encryption of sensitive fields through Vault Transit;
  - a hash-chained, append-only audit trail.

| Component | Technology |
|---|---|
| `apps/vuln-api` | Go, PostgreSQL |
| `apps/vuln-web` | React, TypeScript, served by nginx |
| Identity | Keycloak (`auth.cerberoast.site`) |

Requirements coverage, cryptography and LLM use are documented in
[`docs/vulnman/hand-in.md`](docs/vulnman/hand-in.md).

## Infrastructure

| Area | Technology |
|---|---|
| Hosting | Azure virtual machines provisioned with Terraform |
| Cluster | k3s, MetalLB, Traefik |
| Delivery | GitHub Actions builds, scans and signs images; ArgoCD deploys them (dev, staging, prod) |
| Policy | Kyverno: signed images only, non-root, read-only filesystems, resource limits, default-deny networking |
| Secrets | HashiCorp Vault and External Secrets Operator |
| Certificates | cert-manager: Let's Encrypt for public names, an internal CA for admin names |
| Admin access | headscale VPN, with Keycloak and oauth2-proxy in front of every admin interface |
| Backups | MinIO, with Vault snapshots and database dumps on a schedule |

## Repository layout

```
apps/               Applications (vuln-api, vuln-web)
infra/terraform/    Azure VMs and cluster, Vault configuration, GitHub settings
infra/k8s/          Platform manifests and per-environment ArgoCD definitions
infra/helm/         Helm charts and per-environment values
infra/keycloak*/    Identity realm configuration
docker/base/        Shared base images
docs/               Threat model, runbooks, design documents
```

## Documentation

- [Contributing](CONTRIBUTING.md)
- [Threat model](docs/security/threat-model.md)
- [Adding or changing an app](apps/README.md)
- [Deploying and promoting](infra/k8s/environments/README.md)
- [Redeploying to a new Azure subscription](docs/runbooks/redeploy-new-subscription.md)

Run `just` from the repository root to list available commands.
