#!/usr/bin/env bash
# Bootstraps the GitHub Issues/Projects backlog for Cerberus from the baseline
# plan, so the epics/stories exist as real, trackable issues instead of only
# living in a planning doc.
#
# Run once, after `gh auth login` has been completed with a token that has
# repo + project scopes. Safe to re-run: label creation is idempotent (gh
# just reports "already exists"); issue creation is NOT deduplicated, so
# don't re-run the issues/backlog steps blindly - use the mode flags below
# if you need to redo just one part.
#
# Usage: ./tools/bootstrap-project.sh [all|--labels-only|--project-only|--issues-only]

set -euo pipefail

OWNER="RaiseOfEASV"
REPO="Cerberus"
REPO_SLUG="${OWNER}/${REPO}"
MODE="${1:-all}"

command -v gh >/dev/null || { echo "gh CLI is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
gh auth status >/dev/null || { echo "Run 'gh auth login' first" >&2; exit 1; }

# --- labels -------------------------------------------------------------

create_labels() {
    echo "Creating labels..."
    declare -A labels=(
        ["epic"]="5319e7:Major area of work from the baseline plan"
        ["story"]="0e8a16:A single PR-sized piece of work under an epic"
        ["security"]="d93f0b:Security finding or hardening work"
        ["dependencies"]="fbca04:Dependency version bump (Renovate)"
        ["area:ci-cd"]="1d76db:CI/CD pipelines"
        ["area:infra"]="1d76db:Kubernetes / Terraform / infrastructure"
        ["area:secrets"]="1d76db:Vault / secrets management"
        ["area:observability"]="1d76db:Monitoring, logging, SIEM"
        ["area:network"]="1d76db:Network, VPN, admin access"
        ["area:governance"]="1d76db:Repo governance, CODEOWNERS, policy"
        ["area:dependency-management"]="1d76db:Renovate, base images, package proxy"
    )
    for name in "${!labels[@]}"; do
        color="${labels[$name]%%:*}"
        desc="${labels[$name]#*:}"
        gh label create "$name" --repo "$REPO_SLUG" --color "$color" --description "$desc" --force
    done
}

# --- project board --------------------------------------------------------

create_project() {
    echo "Creating Projects (v2) board..."
    project_url=$(gh project create --owner "$OWNER" --title "Cerberus Baseline" --format json | jq -r '.url')
    project_number=$(echo "$project_url" | grep -oE '[0-9]+$')
    echo "Project created: $project_url (number $project_number)"
    echo "$project_number" > .project-number

    # gh project field-create only ADDS new fields - it can't rewrite the
    # options of the default "Status" field (which starts as Todo/In
    # Progress/Done). Add "Backlog" and "In Review" to Status by hand, once,
    # in the project UI (Settings > Fields > Status > add option) - it's a
    # one-time fix, not worth scripting via raw GraphQL for a single run.
    echo "NOTE: add 'Backlog' and 'In Review' options to the Status field"
    echo "      manually in the project UI - gh CLI can't edit existing"
    echo "      single-select options, only add new fields."

    gh project field-create "$project_number" --owner "$OWNER" \
        --name "Area" --data-type SINGLE_SELECT \
        --single-select-options "ci-cd,infra,secrets,observability,network,governance,dependency-management"

    gh project field-create "$project_number" --owner "$OWNER" \
        --name "Priority" --data-type SINGLE_SELECT \
        --single-select-options "P0,P1,P2"
}

# --- epic issues ----------------------------------------------------------
# Titles/bodies seeded from the baseline plan's "Work Breakdown (Epics)" section.

declare -a EPICS=(
"Repo Foundation & Governance|area:governance|Repo scaffold, Repository Rulesets, CODEOWNERS, PR/issue templates (incl. threat-model-update and linked-issue checklist items), commit conventions, pre-commit hooks, SECURITY.md, threat model skeleton, Projects board bootstrap, org-level GitHub hardening (2FA/PAT policy)."
"Dependency, Container & Package Management|area:dependency-management|Renovate config, centrally-defined base images, Nexus Repository OSS deployment, GHCR wiring for built images."
"CI/CD Pipelines|area:ci-cd|pr-validate/build/release/deploy workflows, actions-runner-controller (ARC) deployment and isolation, CodeQL/Semgrep/Trivy/GitGuardian wired into pr-validate."
"Kubernetes & Infrastructure Baseline|area:infra|Terraform cluster module, encrypted/locked state backend, MetalLB/Traefik, ArgoCD, Kyverno policies, NetworkPolicies (ingress+egress), kube-bench."
"VM / Host Hardening|area:infra|Packer/Ansible golden VM image, CIS hardening, Lynis gate in the image-build pipeline."
"Secrets Management|area:secrets|Vault deployment, External Secrets Operator, break-glass unseal procedure documented."
"Observability & SIEM|area:observability|kube-prometheus-stack + Alertmanager, Elastic, Wazuh (manager + agents), Falco + Falcosidekick, Trivy Operator, Dependency-Track."
"Network & Admin Access Security|area:network|Keycloak deployment, VPN (Tailscale/headscale), oauth2-proxy in front of every admin UI."
"Backup, DR & Incident Readiness|area:governance|Velero, scheduled restore drills, incident response runbook."
"Application Security Testing|area:ci-cd|OWASP ZAP DAST wiring, license compliance reporting from Trivy - picked up once the first real service exists."
)

create_epics() {
    echo "Creating epic issues..."
    for entry in "${EPICS[@]}"; do
        IFS='|' read -r title area body <<< "$entry"
        gh issue create --repo "$REPO_SLUG" \
            --title "[Epic] $title" \
            --label "epic,$area" \
            --body "$body"$'\n\n'"See the baseline plan for full context."
    done
}

# --- backlog (explicitly deferred) issues ----------------------------------

declare -a BACKLOG=(
"Canary tokens|area:secrets|Decoy Vault secret path, decoy cloud credential in a repo, decoy /admin endpoint - all wired to fire into Wazuh. Pick up once Vault and Wazuh are both live."
"Service mesh / mTLS (Linkerd)|area:network|East-west traffic encryption between services. Deferred - NetworkPolicy alone covers the baseline."
"Argo Rollouts (canary + auto-rollback)|area:infra|Automated rollback on failed health checks post-deploy."
"DefectDojo or Kubescape|area:observability|Consolidated findings triage across scanners, if juggling separate dashboards becomes a real problem."
"GDPR data retention/erasure implementation|area:governance|Keycloak holds real user data - needs a conscious decision on retention/erasure, not just infra."
"Chaos engineering|area:infra|Deferred until the core platform is stable enough to be worth stress-testing."
)

create_backlog() {
    echo "Creating backlog issues..."
    for entry in "${BACKLOG[@]}"; do
        IFS='|' read -r title area body <<< "$entry"
        gh issue create --repo "$REPO_SLUG" \
            --title "$title" \
            --label "story,$area" \
            --body "$body"$'\n\n'"Explicitly deferred - not scheduled into an epic yet."
    done
}

case "$MODE" in
    --labels-only) create_labels ;;
    --project-only) create_project ;;
    --issues-only) create_epics; create_backlog ;;
    all) create_labels; create_project; create_epics; create_backlog ;;
    *) echo "Unknown mode: $MODE" >&2; exit 1 ;;
esac

echo
echo "Done. gh issue create doesn't auto-add issues to a project board - add"
echo "the epic/backlog issues to the 'Cerberus Baseline' project now with:"
echo "  gh project item-add \$(cat .project-number) --owner $OWNER --url <issue-url>"
echo "or do it in bulk from the project UI (Add item > search by label)."
