# GitHub Rulesets as code

Closes the Epic 1 gap: branch protection managed via Terraform instead of
clicking through repo settings.

## Before applying

1. Create a fine-grained PAT scoped to just this repo, with
   **Administration: write** permission.
2. `export TF_VAR_github_token=<the token>` (don't put it in a `.tfvars`
   file that could get committed - it's gitignored, but don't tempt fate).
3. `terraform init && terraform plan` from this directory.

## What applies safely today

`github_repository_ruleset.main_protection` - PR required, no force-push or
branch deletion, signed commits required, CODEOWNERS review required,
linear history required. None of this depends on anything that doesn't
exist yet. Safe to `terraform apply` now.

## What's intentionally NOT active yet

`github_repository_ruleset.required_checks` is written but set to
`enforcement = "evaluate"`, not `"active"`. It references status-check
context names (`lint`, `sast`, `sca`, `secret-scan`) that don't exist until
Epic 3 ships `.github/workflows/pr-validate.yml`. "evaluate" mode logs what
*would* be blocked without actually blocking merges - safe to apply as-is.

**Do not flip it to `"active"` until:**
1. Epic 3's `pr-validate.yml` exists and its job names match the
   `required_check` contexts here exactly (fix whichever side is wrong -
   don't just assume these placeholders are correct).
2. At least one real PR has shown those checks reporting success.

Flipping it to `"active"` before both of those are true will make every PR
permanently unmergeable, since a required check that can never run can
never succeed.

## GitHub Environments (added in Epic 6)

`environments.tf` creates the `staging` and `production` GitHub Environments that
`.github/workflows/deploy.yml` (Epic 3) targets via `environment: ${{ inputs.environment }}`.
Only `production` requires a reviewer and restricts deploys to protected branches -
`staging` stays unrestricted for fast iteration. Deliberately does NOT manage any
environment *secret values* - those go in directly via `gh secret set --env production ...`
or the repo UI, never through Terraform (the value would land in state).

## State

Uses the shared `pg` backend from `infra/terraform-state/` (Epic 4) - this was local
state until then, flagged as a known gap; now resolved for every Terraform root,
including this one. Run that instance first, then:

```
terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=require"
```

If you already ran `terraform init` against local state before this changed, you'll
need `terraform init -migrate-state` to move the existing state into Postgres rather
than starting fresh and losing track of what's already applied.
