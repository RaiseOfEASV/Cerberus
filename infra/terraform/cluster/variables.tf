variable "cluster_name" {
  description = "Cluster name"
  type        = string
  default     = "cerberus"
}

variable "kubernetes_version" {
  description = "Passed straight through to the module implementation - see modules/k8s-cluster/kind for what this means concretely"
  type        = string
  default     = "v1.31.0"
}

variable "node_count" {
  description = "Total nodes including control plane"
  type        = number
  default     = 3
}
