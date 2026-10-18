variable "vault_address" {
  description = "Vault API address, reachable from wherever this Terraform runs. https since Epic 8 flipped Vault's TLS on - if applying from outside the cluster, VAULT_CACERT must also point at the internal CA (infra/k8s/cert-manager/) for the provider to trust this."
  type        = string
  default     = "https://vault.vault.svc:8200"
}

variable "kubernetes_host" {
  description = "API server URL for the Kubernetes auth method to validate service account tokens against"
  type        = string
  default     = "https://kubernetes.default.svc"
}
