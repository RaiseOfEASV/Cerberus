# The 3 interface-required variables (see ../README.md) - every
# implementation must accept these with the same meaning.

variable "cluster_name" {
  description = "Cluster name"
  type        = string
}

variable "kubernetes_version" {
  description = "k3s version string, e.g. v1.31.0+k3s1 - check https://github.com/k3s-io/k3s/releases. Not the same format as kind's node image tag, but the same meaning (which k8s version)."
  type        = string
  default     = "v1.31.0+k3s1"
}

variable "node_count" {
  description = "Total nodes including control plane."
  type        = number
  default     = 1

  validation {
    condition     = var.node_count == 1
    error_message = "Only single-node is implemented so far - adding workers means fetching a join token from the first node and feeding it into additional VMs' cloud-init, which isn't built yet. Revisit when a second node is actually needed."
  }
}

# Azure-specific, not part of the module interface - mirrors
# infra/terraform/bootstrap/variables.tf's pattern.

variable "location" {
  description = "Azure region. Must match the region infra/terraform/bootstrap/ used (Sweden Central at the time of writing) - that's what the referenced resource group/VNet actually live in."
  type        = string
  default     = "swedencentral"
}

variable "resource_group_name" {
  description = "Existing resource group created by infra/terraform/bootstrap/ - looked up via data source, not created here."
  type        = string
  default     = "cerberus-rg"
}

variable "vm_size" {
  description = "VM size for the single k3s node. Not defaulted to the same free-tier B-series as the bootstrap VM - running k3s plus ArgoCD/Kyverno/MetalLB/Traefik/Vault/ARC runners needs more than 4GB RAM. Standard_B4ms (BS family, same 4 vCPU/16GB spec) hit a real-time capacity stockout; Standard_D4s_v5 (DSv5 family) hit a hard 0-vCPU quota on this Student subscription - this subscription only has nonzero quota on older v4-and-earlier D-series families (DSv4/DASv4/Dv4/DDv4, all limit 4 vCPUs) plus the B-series. D4s_v4 fits that quota exactly."
  type        = string
  default     = "Standard_D4s_v4"
}

variable "admin_username" {
  description = "Linux admin user created on the VM."
  type        = string
  default     = "cerberus-admin"
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key granted access. ~ is expanded. Defaults to the same dedicated key generated for infra/terraform/bootstrap/."
  type        = string
  default     = "~/.ssh/cerberus_azure.pub"
}

variable "ssh_private_key_path" {
  description = "Path to the matching private key - used by this module's own provisioners to fetch the kubeconfig over SSH after k3s comes up. Never passed as a literal value, only a path."
  type        = string
  default     = "~/.ssh/cerberus_azure"
}

variable "admin_source_ip" {
  description = "CIDR allowed to reach SSH (22) and the Kubernetes API (6443). No safe default - must be set explicitly, same reasoning as the bootstrap root's own admin_source_ip."
  type        = string
}
