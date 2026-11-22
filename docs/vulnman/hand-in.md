# VulnMan - SSD exam hand-in (fall 2026)

The exam spec this answers is vendored at
[`docs/superpowers/specs/2026-11-16-vulnman-exam-spec.md`](../superpowers/specs/2026-11-16-vulnman-exam-spec.md)
(upstream commit `519d59a` - re-check upstream before submitting). The design and
every decision (D1-D18) are in
[`docs/superpowers/plans/2026-11-16-vulnman.md`](../superpowers/plans/2026-11-16-vulnman.md).

- **Code:** `apps/vuln-api` (Go), `apps/vuln-web` (React), deployment in `infra/`.
  Public repository: https://github.com/RaiseOfEASV/Cerberus
- **Live:** https://cerberoast.site (report form: `/report`) - once the cutover in
  plan Task 22 is done.
- **Threat model:** [`docs/security/threat-model.md`](../security/threat-model.md), section "VulnMan".

## Requirements

Evidence names the automated test(s) that prove the behaviour (Go tests in
`apps/vuln-api`, Vitest in `apps/vuln-web`).

| ID | Priority | Status | How / evidence |
|---|---|---|---|
| AUTH-01 | MUST | Done | Keycloak (`keycloak-public`) as the IdP. VulnMan's own login form posts to vuln-api, which uses Keycloak's password grant as a confidential client; the OIDC code flow with PKCE stays as the single sign-on fallback. Either way the browser only gets an HttpOnly session cookie. `TestPasswordLoginCreatesSession`, `TestPasswordLoginRateLimited`, `TestLoginSetsStateAndPKCE`, `TestCallbackCreatesSession` |
| AUTH-02 | MUST | Done | Permission matrix enforced server-side on every route. `TestCanMatrix`, `TestDeveloperOwnership`, `TestTransitions`, `TestAuditorReadOnly`, `TestDeveloperEditOthersFinding403NoAudit`, `TestCreateRequiresAnalystOrPM` |
| AUTH-03 | MUST | Done | `/report` + `POST /api/public/reports`, acknowledgement with a `VM-` reference. `TestPublicCreatesNewExternalFinding`, `Report.test.tsx` |
| AUTH-04 | SHOULD | Done | PM changes roles through the Keycloak admin API; sessions of the target end. `TestSetRoleReplacesOthers`, `TestSetRoleRevokesSessions`, `TestPMCannotChangeOwnRole` |
| AUTH-05 | STRETCH | Not done (switched off) | TOTP was enforced in the prod realm, then switched off on 2026-11-21 to simplify sign-in for now. It can be turned back on in `infra/keycloak-public/realm-prod.yaml` (`CONFIGURE_TOTP` defaultAction). The password-grant login would then need an OTP field. |
| FIND-01 | MUST | Done | `POST /api/findings` (analyst, PM). `TestCreateRequiresAnalystOrPM`, `FindingNew.test.tsx` |
| FIND-02 | MUST | Done | See AUTH-03; reporter e-mail stored encrypted |
| FIND-03 | SHOULD | Done | Attachments, type sniffed from content, 10 MiB, encrypted. `TestAttachmentTypeSniffed`, `TestAttachmentTooLarge413`, `TestAttachmentRoundTripEncrypted`, `TestAttachmentDownloadHeaders` |
| FIND-04 | MUST | Done | Assign + due date; SLA due date set at triage from PM targets. `TestAssignValidatesAssigneeAndDueDate`, `TestTriageSetsSLADue` |
| FIND-05 | MUST | Done | Filters: status, severity, assignee, asset, text, overdue. `TestFilterCombos`, `TestFilterQueryParsing`, `Findings.test.tsx` |
| FIND-06 | SHOULD | Done | Every change audited in the same transaction. `TestUpdateAuditsChangedFieldsOnly`, `TestAuditRecordsActorAndTime` |
| FIND-07 | SHOULD | Done | Links: duplicate / related / same root cause. `TestLinkSymmetricAndNoSelf`, `TestLinksAPI` |
| FIND-08 | STRETCH | Partial | CSV import (all-or-nothing) and export (formula-safe); no Excel. `TestCSVImportBadSeverity400`, `TestCSVExportRoundTrip`, `TestCSVFormulaEscaped` |
| FIND-09 | STRETCH | Done | Trivy image and SBOM scan JSON, de-duplicated. Fixtures are real trivy 0.58.1 output. Also automatic: a nightly in-cluster scan (`apps/vuln-scanner`) scans the images running in the environment and syncs them through vuln-api's internal endpoint, opening findings for new vulnerabilities and closing (Remediated, audited) the ones no longer reported. `TestParseTrivyImage`, `TestParseTrivySBOM`, `TestTrivyImportAndReimportSkipsDuplicates`, `TestSyncScanCreatesSkipsAndCloses`, `TestScanEndpointRefusals` |
| FIND-10 | STRETCH | Not done | Out of scope (plan D18 / optional Task 24) |
| FIND-11 | SHOULD | Done | Envelope encryption with Vault Transit. `TestCreateEncryptsDescription`, `TestSwappedCiphertextFails`, `TestOpenWrongAAD` |
| AUDIT-01 | SHOULD | Done | Who, what, which finding, when - for every mutation, login, import, export and role change |
| AUDIT-02 | SHOULD | Done | PM and auditor only. `TestAuditReadableByPMAndAuditorOnly` |
| AUDIT-03 | STRETCH | Done | Hash chain + append-only grants + trigger + verify endpoint. `TestAuditTamperDetected`, `TestConcurrentMutationsKeepChain`, `TestAppRoleCannotUpdateAudit`, `TestAuditTriggerBlocksOwner` |
| FR-RPT-01 | MUST | Done | Dashboard by severity and status. `TestDashboardCounts`, `Pages.test.tsx` |
| FR-RPT-02 | SHOULD | Done | JSON report per period / asset. `TestReportContents`, `TestReportSignedAndVerifies` |
| FR-RPT-03 | STRETCH | Done | Ed25519 signature (Vault Transit) + offline verifier. `TestReportTamperDetected`, `TestVerifyTampered` |
| FR-RPT-04 | SHOULD | Done | SLA compliance and overdue list. `TestSLACompliance` |
| FR-RPT-05 | STRETCH | Done | Opened vs closed per week. `TestTrendWeeks` |

### Changed or interpreted requirements

- **Developer scope (D5):** the spec says developers "update status and details";
  we limit that to findings assigned to them, and they can't set Accepted Risk.
- **Status transitions (D6):** the spec lists statuses but not transitions; we
  defined a state machine (e.g. no New -> Remediated, Remediated can be reopened).
- **Disclosure approval:** the spec says no role may approve public disclosures,
  so we built no disclosure workflow (D18).
- **Reports as JSON, not PDF** (the spec allows either); signed instead.
- **Name (2026-11-21):** the app is presented as **Cerberoast** (themed on
  Cerberus, the guard dog of the underworld); "VulnMan" stays the exam
  brief's name and the internal name (repos, realms, `VM-` references).
- **Own login page (2026-11-21):** sign-in happens on VulnMan's own page,
  with vuln-api checking the password against Keycloak (password grant). This
  is a deliberate trade-off against OAuth guidance, which prefers the redirect
  flow; mitigations are listed in the threat model.

### Added requirements

Public form abuse controls (rate limit, honeypot, size limits - D12); session
expiry and revocation on role change; CSV-injection-safe export; attachment
content sniffing; scanner de-duplication by fingerprint; a separate public IdP
so the admin Keycloak is never exposed (D3).

## How we use cryptography

| Where | What | Why |
|---|---|---|
| Transport | TLS everywhere: Let's Encrypt for public names, the internal CA for admin names and for vuln-api -> Vault | Confidentiality and server authentication |
| Login | Own login form: password checked by vuln-api through Keycloak's password grant (confidential client, server-side). Fallback: OAuth 2.0 code flow with PKCE (S256), `state` and `nonce`. Tokens are JWTs verified against Keycloak's keys and never reach the browser | Credentials go only to our own origin; tokens stay server-side; the fallback blocks code interception, login CSRF and token replay |
| Sessions | 256-bit random IDs; only SHA-256(ID) stored | A database leak doesn't hand out live sessions |
| Finding data at rest (FIND-11) | Envelope encryption: a fresh AES-256 data key per finding from Vault Transit (`datakey/plaintext`), fields sealed with AES-256-GCM, AAD = `"<finding id>:<field>"`; only the wrapped key is stored | The master key never leaves Vault; ciphertexts can't be moved between rows or fields; keys can be rotated with Transit `rewrap` |
| Audit integrity (AUDIT-03) | Each row's hash = SHA-256(previous hash ‖ canonical JSON of the row) | Editing or deleting history is detectable |
| Reports (FR-RPT-03) | Ed25519 signature by Vault Transit over the exact report bytes; public key published; `vuln-verify-report` verifies offline | Tampering after export is detectable |
| Supply chain | Images signed with cosign (keyless) in CI; Kyverno refuses unsigned images | Only CI-built images run |

## Use of LLMs and agents

**Co-authored by an LLM (Claude, via Claude Code):** the implementation plan
(`docs/superpowers/plans/2026-11-16-vulnman.md`), the Go and React code in
`apps/vuln-api` and `apps/vuln-web`, the Helm/Kubernetes/Terraform changes, the
threat-model section and this document. Unless noted otherwise, assume
every commit on the VulnMan branch was co-written this way.

**Workflow:** the spec was turned into a written plan with explicit design
decisions, then implemented task by task, test-first: each task's tests were
written and seen failing before the code, then the full suite was run before the
task was marked done. Deviations from the plan were recorded as rulings with
their cost. A separate reviewer agent reviewed the whole branch at the end. The
team reviews every change in a pull request before it reaches `master`.

**How quality and security are ensured:** the plan names the failure modes most
likely to hurt users (its "Review Focus") and each has a test; authorization,
encryption, audit integrity and input limits all have tests (see the matrix
above); CI gates every PR (lint, tests with a real Postgres, Helm rendering,
container checks, Trivy, secret scanning) and Kyverno enforces the runtime
baseline; `master` requires a PR, review by a teammate and signed commits. We
read the generated code - an LLM's output is a proposal, not a decision.
