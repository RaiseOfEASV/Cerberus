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
| 2026-11-16 | VulnMan (SSD exam app) replaces the quote app | Ju5tAmir + Claude | Added the VulnMan section; quote section removed when the quote app is retired (plan Task 22). |

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

## VulnMan (vuln-api / vuln-web / keycloak-public)

**What it is / does:** Vulnerability management for the SSD exam
(`docs/superpowers/specs/2026-11-16-vulnman-exam-spec.md`): intake (manual, public
form, Trivy/SBOM, CSV), triage, assignment with SLA, remediation, dashboard, signed
reports and an audit trail. `vuln-api` (Go) is also the OIDC client; `vuln-web` is a
static React SPA on the same origin. Users log in at a separate public Keycloak
(`auth.cerberoast.site`). Design decisions D1-D18 are in
`docs/superpowers/plans/2026-11-16-vulnman.md`.

**Trust boundaries it sits on:**
- Public internet <-> Cloudflare <-> `traefik-public` <-> `cerberoast` namespace (prod app) and `keycloak-public` (IdP)
- Anonymous reporter <-> `POST /api/public/reports` (the only unauthenticated write)
- Browser (session cookie) <-> vuln-api; vuln-api <-> Keycloak back-channel (in-cluster)
- vuln-api <-> Vault Transit (Kubernetes auth, internal CA TLS)
- vuln-api <-> its Postgres (`vulnman_app` role) ; migrations initContainer (`vulnman_owner`)
- VPN <-> admin Traefik <-> `dev` / `staging` previews

**Data flows in/out:**

| Flow | From | To | Data | Sensitivity |
|------|------|----|------|-------------|
| Login (code + PKCE) | Browser -> Keycloak -> vuln-api | vuln-api | Authorization code, ID/access tokens (never sent to the browser) | High |
| Session | Browser | vuln-api | Opaque `__Host-vm_session` cookie; DB stores only SHA-256 of it | High |
| Public report | Anonymous | vuln-api | Title, description, asset, reporter e-mail | High (unpatched vuln details, PII) |
| Findings CRUD | Staff browser | vuln-api -> Postgres | Description/e-mail/attachments encrypted per finding (AES-256-GCM, Transit-wrapped key) | High |
| Data keys / signatures | vuln-api | Vault Transit | Wrapped data keys, report bytes to sign | High |
| Scanner import | Analyst/PM | vuln-api | Trivy JSON (<= 5 MiB) | Medium |
| Exports | vuln-api | Staff browser | CSV (no encrypted fields), signed JSON report | Medium |
| Role changes | PM | vuln-api -> Keycloak admin API | App role of a user | High |
| Backups | vuln-db CronJob | backup MinIO | `pg_dump` (fields still encrypted) | High |

**STRIDE:**

| Threat | Applicable? | Mitigation | Residual risk |
|--------|-------------|------------|----------------|
| Spoofing | Yes - stolen or forged sessions, login CSRF, forged tokens | OIDC code flow with PKCE S256, state + nonce in a short-lived `__Host-` cookie, ID token checked for issuer/audience/nonce and access token for `azp`/`sub`; session ID 256-bit random, only its hash stored; `__Host-`/Secure/HttpOnly/SameSite=Lax; 30 min idle, 8 h absolute; MFA (TOTP) enforced in the prod realm; brute-force protection in Keycloak | A compromised staff device with a live session acts as that user until expiry or a role change |
| Tampering | Yes - finding data, audit trail, exported reports, CSRF | Server-side authorization on every route (permission matrix + developer ownership); `SameOrigin` check on every state-changing request; AES-GCM with AAD `"<id>:<field>"` so ciphertexts can't be moved between rows; audit rows SHA-256 hash-chained, app DB role has only SELECT/INSERT on `audit_log`, trigger blocks UPDATE/DELETE/TRUNCATE, `/api/audit/verify` finds the first bad row; reports signed with Ed25519 in Vault, verifiable offline (`vuln-verify-report`) | The owner role (migrations) could disable the trigger and rewrite the chain from the tampered row onward; anchoring the chain head outside the DB would close that |
| Repudiation | Yes | Every mutation, login, import, export and role change writes an audit row in the same transaction (actor sub + role, time, details); PMs/auditors read it | Read access to findings is not audited (only exports) |
| Information disclosure | Yes - vuln details are exploitable, reporter e-mails are PII | Description, reporter e-mail and attachments encrypted at rest with per-finding keys from Vault Transit (key never leaves Vault); list endpoints never decrypt; CSV exports and reports exclude encrypted fields; JSON-only API with nosniff; strict CSP (`default-src 'self'`, no inline script, no framing); attachments sniffed by content, served as `attachment` with a sandbox CSP; generic 500s, no internals; public Keycloak exposes only the vulnman realms, not the admin console | Title and asset stay plaintext (needed for search - D9); anyone with read access to findings sees decrypted descriptions by design |
| Denial of service | Yes - public form, uploads, imports | Public form: 5 submissions/hour per client (X-Forwarded-For trusted only from Traefik's pod range), 32 KiB body, honeypot; bounded bodies everywhere (64 KiB JSON, 10 MiB attachments, 5 MiB Trivy, 1 MiB CSV, 2000 rows); pagination caps; server timeouts | Rate limiter is in memory (restart resets it, one replica); single replica of every component |
| Elevation of privilege | Yes - role escalation, developer overreach, broad IdP admin rights | One app role per user from Keycloak, re-read at every login; role changes end the target's sessions; PM can't change their own role; kcadmin only ever touches the four app roles; developers act only on findings assigned to them and can't accept risk; containers non-root, read-only root, no capabilities (Kyverno); vuln-api's Vault role allows only its environment's Transit operations; NetworkPolicy egress limited to DNS, its DB, Vault and Keycloak | The `vuln-api-admin` service account holds `manage-users` in its realm (broader than role mapping); a vuln-api compromise could misuse it within that realm |
