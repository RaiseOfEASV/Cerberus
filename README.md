# Cerberus

Security-first monorepo baseline: Go backend services, JS/TS frontend app(s), and all
the platform infrastructure (CI/CD, secrets, Kubernetes, observability, SIEM) that
those services will run on.

This repo is being built baseline-first, before application code — see
[`docs/security/threat-model.md`](docs/security/threat-model.md) for the living threat
model and the project's GitHub Issues/Projects board for the epic-by-epic build-out.

## Layout

```
apps/            Go backend services and frontend app(s)
pkg/             Shared Go libraries
infra/terraform/ Cluster + platform provisioning, GitHub-as-code
infra/helm/      Per-app charts + platform umbrella chart
infra/k8s/       ArgoCD Application manifests (app-of-apps), per environment
infra/packer/    Golden VM image definitions + Ansible roles
docker/base/     Centrally-defined, minimal base images
tools/           Shared scripts (see tools/bootstrap-project.sh)
docs/security/   Threat model, incident response runbook
```

Run `just` from the repo root to see available commands.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for one-time dev environment setup
(including the required local secret-scanning hook) before your first commit.
