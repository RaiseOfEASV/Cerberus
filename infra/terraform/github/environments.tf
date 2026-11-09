# Closes the gap .github/workflows/deploy.yml (Epic 3) flagged in its own
# header comment: "a candidate for infra/terraform/github once that's
# worth the Terraform github_repository_environment resource." It's the
# `environment: ${{ inputs.environment }}` gate in that workflow - this is
# what actually configures what gating means for each environment.
#
# Deliberately does NOT create or reference any environment SECRETS -
# github_actions_environment_secret requires the value in the Terraform
# config, which would put it in state. Secret values are set directly via
# `gh secret set --env production ...` or the repo UI, never here.

data "github_user" "production_reviewers" {
  # Real user accounts - an organization (var.github_owner) can't be an
  # environment reviewer. A lone reviewer doesn't scale past one person;
  # swap for a team (reviewers.teams) once one exists.
  for_each = toset(var.production_reviewers)
  username = each.value
}

resource "github_repository_environment" "staging" {
  repository  = var.github_repo
  environment = "staging"
  # No required reviewers - fast iteration is the point of staging.
}

resource "github_repository_environment" "production" {
  repository  = var.github_repo
  environment = "production"

  # deploy.yml maps its `prod` input to this environment; the job waits
  # here until a reviewer approves it.
  reviewers {
    users = [for u in data.github_user.production_reviewers : u.id]
  }

  # Named branches, not "protected branches only": that policy only counts
  # *classic* branch protection, which master doesn't have (it's protected
  # by the main_protection ruleset instead), so it would refuse every deploy
  # from master.
  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "production_master" {
  repository     = var.github_repo
  environment    = github_repository_environment.production.environment
  branch_pattern = "master"
}
