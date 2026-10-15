output "kubeconfig" {
  description = "Raw kubeconfig for this cluster"
  value       = kind_cluster.this.kubeconfig
  sensitive   = true
}

output "cluster_endpoint" {
  description = "API server URL"
  value       = kind_cluster.this.endpoint
}
