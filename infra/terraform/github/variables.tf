variable "github_owner" {
  description = "GitHub org/user that owns the repo"
  type        = string
  default     = "RaiseOfEASV"
}

variable "github_repo" {
  description = "Repository name (not the full owner/repo slug)"
  type        = string
  default     = "Cerberus"
}

variable "github_token" {
  description = "Fine-grained PAT with Administration: write on the repo. Pass via TF_VAR_github_token - never commit this."
  type        = string
  sensitive   = true
}
