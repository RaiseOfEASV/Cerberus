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

---

<!-- Component sections go below this line, added epic by epic. -->
