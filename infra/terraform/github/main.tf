# GitHub Rulesets as code - the Epic 1 gap flagged in BACKLOG.local.md.
#
# NOT YET APPLIED. terraform apply requires a GitHub token with admin rights
# on RaiseOfEASV/Cerberus (fine-grained PAT, repo administration: write) -
# nowhere secure to source that from until Vault exists (Epic 6), so for now
# pass it by hand via TF_VAR_github_token and don't commit any tfstate
# (already gitignored). State backend is local for now - revisit once Epic 4
# picks an encrypted/locked remote backend; don't treat local state here as
# the final answer.
#
# Split into two rulesets deliberately:
#   - main_protection: safe to apply today, no dependency on anything that
#     doesn't exist yet.
#   - required_checks: enforcement = "evaluate", NOT "active" - it logs what
#     would be blocked without actually blocking merges. Flip to "active"
#     only after Epic 3 ships pr-validate.yml and its job names below are
#     confirmed to match and report successfully at least once. Applying
#     this as "active" before Epic 3 exists would make every PR unmergeable
#     forever, since the required check could never succeed.

terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }

  # Partial config deliberately - conn_str supplied at `terraform init` time
  # via -backend-config, never committed (it embeds a password). See
  # infra/terraform-state/README.md for the instance this points at and why
  # pg over S3/MinIO.
  # Own schema - every root shares one Postgres database and the pg backend
  # keys state only by schema + workspace (see infra/terraform/cluster/
  # main.tf's backend comment).
  backend "pg" {
    schema_name = "github"
  }
}

provider "github" {
  owner = var.github_owner
  token = var.github_token
}

resource "github_repository_ruleset" "main_protection" {
  name        = "main-protection"
  repository  = var.github_repo
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      # ~DEFAULT_BRANCH tracks whatever the default branch actually is
      # (master today, main if it's ever renamed) - avoids hardcoding a
      # name that might not match.
      include = ["~DEFAULT_BRANCH", "refs/heads/release/**"]
      exclude = []
    }
  }

  rules {
    creation                = false
    deletion                = true # blocks branch deletion
    non_fast_forward        = true # blocks force-push
    required_linear_history = true # squash/rebase merges only, no merge commits
    required_signatures     = true # unsigned/unverified commits rejected

    pull_request {
      required_approving_review_count   = 1
      require_code_owner_review         = true
      dismiss_stale_reviews_on_push     = true
      required_review_thread_resolution = true
    }
  }
}

resource "github_repository_ruleset" "required_checks" {
  name        = "required-status-checks"
  repository  = var.github_repo
  target      = "branch"
  enforcement = "evaluate" # NOT active - see header comment. Flip once Epic 3 ships.

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH", "refs/heads/release/**"]
      exclude = []
    }
  }

  rules {
    required_status_checks {
      # TODO(epic-3): these context strings must exactly match the `jobs.<id>.name`
      # (or job id, if no `name` is set) values in .github/workflows/pr-validate.yml
      # once it exists. Placeholders below describe intent, not confirmed names.
      required_check {
        context = "lint"
      }
      required_check {
        context = "sast"
      }
      required_check {
        context = "sca"
      }
      required_check {
        context = "secret-scan"
      }
      strict_required_status_checks_policy = true # branch must be up to date before merge
    }
  }
}
