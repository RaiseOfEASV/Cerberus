# GitHub Rulesets and Environments as code

Branch protection and deployment environments managed via Terraform instead
of clicking through repo settings. **Everything here is applied** (2026-11-09).

## Current state

| Resource | State |
|---|---|
| `main_protection` ruleset | **active** on the default branch and `release/**`: PR required, 1 approval, code-owner review, signed commits, linear history, no force-push/deletion. Bypass: repo **Admin** role, PR merges only |
| `required_checks` ruleset | `evaluate` (logs only) - see below |
| `staging` environment | no reviewers |
| `production` environment | required reviewer(s) = `var.production_reviewers` (default `Ju5tAmir`); deployments only from `master` |

What this means day to day is in `CONTRIBUTING.md` (signed commits, PRs,
merging, promotions).

## Applying changes

1. Fine-grained PAT, resource owner **RaiseOfEASV**, only this repo,
   **Administration: Read and write** (the org may need to approve it; a
   classic PAT with `repo` also works). `export TF_VAR_github_token=<token>`.
2. Tunnel to the state backend and export its connection string - see
   `infra/terraform/cluster/README.md` ("Reaching the backend"):
   `export PG_CONN_STR=postgres://terraform:${TF_STATE_PG_PASSWORD}@localhost:5432/terraform_state?sslmode=disable`
3. `terraform init && terraform plan` - read the plan; changes to
   `main_protection` affect everyone's ability to merge.

Not managed here: the org/repo Actions setting "Allow GitHub Actions to create
and approve pull requests" (enabled by hand - deploy.yml needs it).

## required_checks - not active yet

`github_repository_ruleset.required_checks` is applied with
`enforcement = "evaluate"`: it logs which merges *would* be blocked, but blocks
nothing. Its contexts (`lint`, `sast`, `sca`, `secret-scan`) match
`pr-validate.yml`'s job names, and all four have run and passed on every PR
since 2026-11-09 - so the original preconditions are met.

**Before flipping it to `"active"`**, fix one thing: `deploy.yml` opens its
bump PRs with `GITHUB_TOKEN`, and PRs opened that way don't trigger other
workflows - those PRs would never get the four checks and could only be merged
via the admin bypass. Have `deploy.yml` use a GitHub App token first, then
flip it.

## GitHub Environments

`environments.tf` creates the `staging` and `production` GitHub Environments that
`.github/workflows/deploy.yml` targets (its `prod` input maps to `production`).
Only `production` requires a reviewer and only allows deployments from `master`
(by name - `master` has no classic branch protection, so a "protected branches
only" policy would refuse every deploy). `staging` stays unrestricted. Deliberately does NOT manage any
environment *secret values* - those go in directly via `gh secret set --env production ...`
or the repo UI, never through Terraform (the value would land in state).

## State

Uses the shared `pg` backend from `infra/terraform-state/` (Epic 4) - this was local
state until then, flagged as a known gap; now resolved for every Terraform root,
including this one. Run that instance first, then:

```
terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=disable"
```

`sslmode=disable`, not `require` - the Postgres container has no SSL cert configured
(confirmed against the real server - `require` fails with
`pq: SSL is not enabled on the server`). `<host>` should be `localhost` via an SSH
tunnel to the bootstrap VM, not its public IP - see
`infra/terraform/cluster/README.md`'s "Reaching the backend" section.

If you already ran `terraform init` against local state before this changed, you'll
need `terraform init -migrate-state` to move the existing state into Postgres rather
than starting fresh and losing track of what's already applied.
