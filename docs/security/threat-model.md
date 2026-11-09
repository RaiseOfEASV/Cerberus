# Threat Model

This is a living document, not a one-time exercise. Update the relevant component's
section as part of any PR that adds a new data flow, trust boundary, or external
integration (the PR template has a checklist item for this). Re-read the whole
document quarterly regardless of whether a PR triggered a change.

Each component is analyzed against STRIDE:
**S**poofing, **T**ampering, **R**epudiation, **I**nformation disclosure,
**D**enial of service, **E**levation of privilege.

## How to add a component

Copy this skeleton into a new `##` section below:

```
## <Component name>

**What it is / does:**

**Trust boundaries it sits on:** (e.g. public internet <-> ingress, cluster <-> VM,
service <-> Vault)

**Data flows in/out:**

| Flow | From | To | Data | Sensitivity |
|------|------|----|----|--------------|

**STRIDE:**

| Threat | Applicable? | Mitigation | Residual risk |
|--------|-------------|------------|----------------|
| Spoofing | | | |
| Tampering | | | |
| Repudiation | | | |
| Information disclosure | | | |
| Denial of service | | | |
| Elevation of privilege | | | |
```

## Review log

| Date | Trigger | Reviewer | Notes |
|------|---------|----------|-------|
| _pending first review_ | Baseline creation | | Components will be added as each epic ships infrastructure, not written speculatively ahead of the code that implements them. |
| 2026-11-09 | First public app (quote-api/quote-web on cerberoast.site) | Ju5tAmir + Claude | Added the component below. Other running infrastructure (Vault, Keycloak/oauth2-proxy, headscale, CI) still has no section - add them. |

---

<!-- Component sections go below this line, added epic by epic. -->

## quote-api / quote-web (cerberoast.site)

**What it is / does:** The first application. `quote-web` (React SPA, nginx-unprivileged) shows a
random quote fetched from `quote-api` (Go, `GET /quote`, `GET /healthz`). No database, no user
accounts, no secrets - quotes are a static in-memory list.

**Trust boundaries it sits on:**
- Public internet ↔ Cloudflare (proxied DNS) ↔ `traefik-public` (dedicated public IP 4.225.171.137) ↔ `cerberoast` namespace (prod)
- VPN + Keycloak login ↔ admin Traefik (10.0.2.50) ↔ `dev` / `staging` namespaces (team previews)
- CI (GitHub Actions) ↔ GHCR ↔ cluster admission (Kyverno) - supply chain

**Data flows in/out:**

| Flow | From | To | Data | Sensitivity |
|------|------|----|----|--------------|
| Page load | Browser | quote-web (cerberoast.site, www) | static assets, `env-config.js` | public |
| Quote fetch | Browser | quote-api (api.cerberoast.site) | quote text | public |
| Preview access | Team browser | quote-{dev,staging}.admin.cerberus.internal | same, behind SSO | internal |
| Image pull + signature check | Cluster (kubelet, Kyverno) | GHCR (public packages) | images, cosign signatures | public, integrity-critical |
| Promotion | deploy.yml | GitHub (deploy/* PR) → ArgoCD | image tag + digest | integrity-critical |

**STRIDE:**

| Threat | Applicable? | Mitigation | Residual risk |
|--------|-------------|------------|----------------|
| Spoofing | Yes - fake site/API, swapped image | Let's Encrypt TLS via Cloudflare (Full strict); Kyverno admits only images keyless-signed by `build.yml`/`base-images.yml`; images deployed by digest | Cloudflare terminates TLS and sees all traffic |
| Tampering | Yes - image, config or env-config.js altered | Signed + digest-pinned images; read-only root FS; `API_URL` validated by the entrypoint (no `"`, `\`, `<`, newline); GitOps changes only via PRs (signed commits, `production` env approval) | **No two-person control:** one repo admin can dispatch, approve the `production` environment and merge the bump PR via the ruleset bypass (deliberate for a 3-person team; see deploy.yml header for how to tighten) |
| Repudiation | Low - no user actions | Git history (signed), ArgoCD sync history, GitHub environment approvals | No app-level request logging shipped to a SIEM yet (Epic 7 not deployed) |
| Information disclosure | Low - all content is public | No secrets in images or env; CORS limited to the two prod origins; `server_tokens off`; dev/staging only behind VPN + SSO; admin hostnames not served on the public IP (verified 404) | - |
| Denial of service | Yes - public endpoint | Cloudflare proxy in front; resource limits on every container; stateless, so restarts are cheap | Single replica each, single-node cluster; no rate limiting at Traefik yet |
| Elevation of privilege | Yes - container breakout | Non-root numeric UID, all capabilities dropped, no privilege escalation, seccomp RuntimeDefault, no service-account token mounted, restricted PSA; default-deny NetworkPolicy with ingress only from the matching Traefik and no egress | NetworkPolicy peer selectors are unreliable on k3s's kube-router (BACKLOG) |
