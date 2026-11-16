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

variable "cluster_name" {
  description = "Cluster name"
  type        = string
  default     = "cerberus"
}

variable "kubernetes_version" {
  description = "Passed straight through to the module implementation - see modules/k8s-cluster/azure-k3s for what this means concretely (a k3s release tag, not a bare k8s version)"
  type        = string
  default     = "v1.31.0+k3s1"
}

variable "node_count" {
  description = "Total nodes including control plane"
  type        = number
  default     = 1
}

# Azure-specific passthrough to modules/k8s-cluster/azure-k3s - see that
# module's variables.tf for what each one means.

variable "location" {
  type    = string
  default = "belgiumcentral"
}

variable "resource_group_name" {
  type    = string
  default = "cerberus-rg"
}

variable "vm_size" {
  type    = string
  default = "Standard_D4s_v4"
}

variable "admin_username" {
  type    = string
  default = "cerberus-admin"
}

variable "ssh_public_key_path" {
  type    = string
  default = "~/.ssh/cerberus_azure.pub"
}

variable "ssh_private_key_path" {
  type    = string
  default = "~/.ssh/cerberus_azure"
}

variable "metallb_pool_ip" {
  type    = string
  default = "10.0.2.50"
}

variable "metallb_public_pool_ip" {
  type    = string
  default = "10.0.2.51"
}
