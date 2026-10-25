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
  default = "swedencentral"
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

variable "admin_source_ip" {
  description = "CIDR allowed to reach SSH (22) and the Kubernetes API (6443). No safe default - set in terraform.tfvars."
  type        = string
}
