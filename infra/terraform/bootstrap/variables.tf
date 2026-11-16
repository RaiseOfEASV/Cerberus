variable "subscription_id" {
  description = <<-EOT
    Azure subscription every resource in this root is created in. Required,
    no default, on purpose: the provider used to fall back to whatever
    `az login` had active, which was fine with exactly one subscription but
    silently picks the wrong one as soon as the operator can see several
    (e.g. a teammate's subscription added after the original Azure for
    Students credit ran out). Set it via terraform.tfvars or
    TF_VAR_subscription_id - see docs/runbooks/redeploy-new-subscription.md.
    `az login` is still the auth mechanism; this only pins the target.
  EOT
  type        = string

  validation {
    condition     = can(regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$", var.subscription_id))
    error_message = "subscription_id must be a subscription GUID (az account list -o table)."
  }
}

variable "location" {
  description = <<-EOT
    Azure region. West Europe (what the cost comparison was priced
    against) and North Europe are both blocked for this subscription by
    an Azure-side policy restricting Students/free subscriptions to a
    specific allowed-region set - "RequestDisallowedByAzure" on apply.
    The original subscription allowed Sweden Central. The current one
    (a teammate's, since 2026-11) has an "Allowed resource deployment
    regions" policy of belgiumcentral, denmarkeast, switzerlandnorth,
    italynorth, germanywestcentral - and of those only Belgium Central
    offers both this VM's size and the cluster's Standard_D4s_v4
    (the rest return NotAvailableForSubscription). The allowed list is
    per subscription: read it from the policy assignment before moving
    again (docs/runbooks/redeploy-new-subscription.md).
  EOT
  type        = string
  default     = "belgiumcentral"
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

variable "admin_source_ips" {
  description = <<-EOT
    CIDRs allowed to reach SSH (22). No safe default - must be set
    explicitly. A list, not a single value, because the admin's actual
    source IP isn't singular: two homes, each with its own dynamic ISP
    IP. Caught for real when a plain SSH to the public IP started
    silently failing with no error on this side, while the VPN path
    (unaffected - that's a WireGuard connection to the VM's own tailnet
    identity, not this NSG rule) kept working and made the mismatch easy
    to spot. Update and re-apply whenever either IP changes.
  EOT
  type        = list(string)
}
