# SSD fall 2026 exam spec (vendored)

Copied verbatim from https://github.com/rpede/SSDf26-Vulnerability-Management at commit `519d59a` (2026-11-09). The upstream repo "will continuously be updated throughout the semester" - re-check it before hand-in.

---


# Exam project specifications

## 1. Overview

- Intake channel:
  - Internal report
  - External report
  - Vulnerability scanning (Trivy etc)
- Triage:
  - Categorize:
    - Severity
    - Project/feature
  - Assign:
    - Developer
    - SLA
  - Remediation
  - Maybe disclosure
- Stretch goals:
  - Project management integration (Jira, Scrumwise, GitHub Issues, Kanban etc)
  - PR management
  - KPI

## 2. User roles

| Role | Description |
|---|---|
| **Analyst** | Can view, create, update findings, and assign findings; cannot manage users, approve public disclosures, |
| Developer | Can view, update status and details on findings, cannot manage users, approve public disclosures |
| **PM** | Everything an Analyst can do, plus: set SLA targets, manage user roles |
| **Auditor** | Read-only access to all findings, reports, and audit logs; cannot modify anything |
| **External Reporter** *(unauthenticated)* | Can submit a new finding via a public disclosure form; cannot view anything |

## 3. Data

Finding information:

- title
- description
- affected asset/component
- severity (e.g., Critical/High/Medium/Low, or numeric CVSS, or EPSS).
- discovery date
- status (New, Triaged, In Progress, Remediated, Accepted Risk)

Contact email should be recorded for external reporter.

## 4. Functional requirements

Requirement priority/importance levels:

- MUST = Minimum requirements to the solution
- SHOULD = Additional requirements most of which should be implemented (depending on group size)
- STRETCH = Nice-to-have features

### 4.1 Authentication & authorization

| ID | Requirement | Priority |
|---|---|---|
| AUTH-01 | Authenticate via an external identity provider using OIDC (e.g., Keycloak) | MUST |
| AUTH-02 | Implement proper access control (see "User roles") | MUST |
| AUTH-03 | Public accessible vulnerability reporting form with acknowledgement | MUST |
| AUTH-04 | PM being able to change another user's role. | SHOULD |
| AUTH-05 | Multi-factor authentication, delegated via the IdP. | STRETCH |

### 4.2 Findings management

| ID | Requirement | Priority |
|---|---|---|
| FIND-01 | Allow Analyst and PM to create a finding manually | MUST |
| FIND-02 | Allow external reporter to submit new finding via form | MUST |
| FIND-03 | Allow file attachments to findings (e.g., evidence, screenshots, scan output). | SHOULD |
| FIND-04 | Analyst/PM can assign a finding to a specific user and set a due date / SLA target. | MUST |
| FIND-05 | Ability to filter and search findings by status, severity, assigned owner, and affected asset. | MUST |
| FIND-06 | Every state change on a finding (status change, reassignment, edits) is recorded in an audit trail (see 4.3). | SHOULD |
| FIND-07 | Support linking related findings to each other (e.g., duplicates, same root cause). | SHOULD |
| FIND-08 | Bulk import/export of findings (CSV, Excel). | STRETCH |
| FIND-09 | Support importing findings from vulnerability scan of SBOM | STRETCH |
| FIND-10 | Project management integration (Sync with issues on Jira, Scrumwise etc) | STRETCH |
| FIND-11 | Encrypt finding details to protect confidentiality | SHOULD |

### 4.3 Audit logging

| ID | Requirement | Priority |
|---|---|---|
| AUDIT-01 | Log: who did what, to which finding, and when. | SHOULD |
| AUDIT-02 | PMs and Auditors can read audit trail. | SHOULD |
| AUDIT-03 | Take appropriate measures to protect integrity of audit trail. | STRETCH |

### 4.7 Reporting & dashboard

| ID | Requirement | Priority |
|---|---|---|
| FR-RPT-01 | Dashboard summarizing open findings by severity and by status. | MUST |
| FR-RPT-02 | Export report (e.g., PDF or JSON) summarizing current findings for a given time period or asset. | SHOULD |
| FR-RPT-04 | The dashboard SHOULD show SLA compliance (findings overdue against their due date). | SHOULD |
| FR-RPT-03 | Exported reports MUST be integrity-protected (e.g., digitally signed) so tampering after export is detectable. | STRETCH |
| FR-RPT-05 | Trend charts over time (e.g., findings opened vs. closed per week). | STRETCH |


---

# Hand-in requirements (hand-in.md)

# Requirements for exam hand-in

- Threat model
- Link to public code repository
- Requirements:
  - Which of the requirements you've implemented
  - Have you made changes to some of the requirements?
  - Any additional requirements you've added
- How you use cryptography in your solution
- Disclose your use of LLM and agents
  - Which parts are co-authored by LLM
  - How did you use it / What was your workflow ?
  - How do you ensure quality and security of LLM contributions?
  - See our guidelines on LLM use
