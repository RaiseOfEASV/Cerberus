# GitHub Rulesets as code - the Epic 1 gap flagged in BACKLOG.local.md.
#
# APPLIED 2026-11-09 (everything in this root). terraform apply needs a
# GitHub token with admin rights on RaiseOfEASV/Cerberus (fine-grained PAT,
# Administration: read and write), passed via TF_VAR_github_token. State lives
# in the pg backend (schema "github") - see README.md for how to reach it.
#
# Two rulesets:
#   - main_protection: ACTIVE. PR + 1 approval + code-owner review + signed
#     commits + linear history, no force-push/deletion. Repo admins can
#     bypass when merging a PR (bypass_mode = "pull_request"); nobody can
#     push to master directly.
#   - required_checks: enforcement = "evaluate" - logs, doesn't block. The
#     checks (lint/sast/sca/secret-scan) run and pass on every PR since
#     2026-11-09, so flipping to "active" is now possible - but deploy.yml's
#     bump PRs are opened with GITHUB_TOKEN, which doesn't trigger
#     pr-validate, so they'd never get the checks. Switch deploy.yml to a
#     GitHub App token first.

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

  # Repo admins may merge a PR without the required review/checks - a
  # small team's escape hatch, chosen deliberately (2026-11-09). Mode
  # "pull_request" means the bypass only applies when merging a PR: direct
  # pushes and force-pushes to master stay blocked for everyone, so every
  # change still lands as a PR (history, CI, GitHub-signed merge commit).
  # Bypassed merges are recorded in the repo's rule insights.
  bypass_actors {
    actor_id    = 5 # built-in repository role: Admin
    actor_type  = "RepositoryRole"
    bypass_mode = "pull_request"
  }

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
