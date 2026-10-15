variable "cluster_name" {
  description = "kind cluster name"
  type        = string
}

variable "kubernetes_version" {
  description = "kind node image tag, e.g. v1.31.0 - check https://github.com/kubernetes-sigs/kind/releases for node images matching your kind version"
  type        = string
  default     = "v1.31.0"
}

variable "node_count" {
  description = "Total nodes including the control-plane node (node 0)"
  type        = number
  default     = 3

  validation {
    condition     = var.node_count >= 1
    error_message = "Need at least 1 node for the control plane."
  }
}
