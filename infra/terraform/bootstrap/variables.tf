variable "location" {
  description = <<-EOT
    Azure region. West Europe (what the cost comparison was priced
    against) and North Europe are both blocked for this subscription by
    an Azure-side policy restricting Students/free subscriptions to a
    specific allowed-region set - "RequestDisallowedByAzure" on apply.
    Confirmed empirically (tried northeurope, swedencentral,
    germanywestcentral, uksouth, francecentral in that order) that
    Sweden Central is allowed. Pricing may differ slightly from the
    West Europe figures used in the earlier cost comparison, but not
    meaningfully for B-series/Spot sizes.
  EOT
  type        = string
  default     = "swedencentral"
}

variable "resource_group_name" {
  description = "Single resource group for the whole project for now - a solo-student-project simplification, not a multi-team convention."
  type        = string
  default     = "cerberus-rg"
}

variable "vm_size" {
  description = "Standard_B2ats_v2 - one of Azure for Students' free-tier-eligible B-series sizes (750 free hours/month for 12 months). 2 vCPU / 4GB - enough for headscale + terraform-state's Postgres + backup MinIO, all light."
  type        = string
  default     = "Standard_B2ats_v2"
}

variable "admin_username" {
  description = "Linux admin user created on the VM."
  type        = string
  default     = "cerberus-admin"
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key granted access. ~ is expanded. Defaults to the dedicated key generated for this project, not a shared/reused one."
  type        = string
  default     = "~/.ssh/cerberus_azure.pub"
}

variable "admin_source_ip" {
  description = <<-EOT
    CIDR allowed to reach SSH (22). No safe default - must be set
    explicitly. Detected as 91.101.59.222/32 at the time this root was
    written; if that IP is dynamic, update this and re-apply when it
    changes, or SSH will start failing with no obvious error from this
    side.
  EOT
  type        = string
}
