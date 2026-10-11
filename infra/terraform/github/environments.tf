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

data "github_user" "default_reviewer" {
  # Same single-owner placeholder as CODEOWNERS - replace with a real team
  # once one exists; a lone reviewer gating production doesn't scale past
  # one person any better here than it does there.
  username = var.github_owner
}

resource "github_repository_environment" "staging" {
  repository  = var.github_repo
  environment = "staging"
  # No required reviewers - fast iteration is the point of staging.
}

resource "github_repository_environment" "production" {
  repository  = var.github_repo
  environment = "production"

  reviewers {
    users = [data.github_user.default_reviewer.id]
  }

  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}
