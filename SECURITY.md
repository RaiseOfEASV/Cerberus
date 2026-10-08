# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for a suspected vulnerability.

Use GitHub's private vulnerability reporting instead:
https://github.com/RaiseOfEASV/Cerberus/security/advisories/new

Include what you found, how to reproduce it, and the potential impact. We'll
acknowledge reports and follow up as the project's disclosure process matures -
this section will be filled in with concrete response-time commitments once
there's a team/on-call rotation behind it (tracked as part of the Backup, DR &
Incident Readiness epic).

## Scope

This repository contains infrastructure-as-code and application services still
being built out. Until the baseline is complete, treat any finding in the platform
layer (CI/CD, secrets management, Kubernetes config, network policy) as in scope.

## Supported versions

Pre-1.0 - `main` only.

## Secret scanning

CI scans every PR with GitGuardian (`.github/workflows/pr-validate.yml`), and
contributors run a local `ggshield` pre-commit hook so a secret is caught
before it's committed at all - see [`CONTRIBUTING.md`](CONTRIBUTING.md) for
setup.
