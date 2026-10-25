output "kubeconfig" {
  description = "Raw kubeconfig for this cluster, server URL rewritten from 127.0.0.1 to the node's private IP (reachable over the VPN)"
  value       = data.local_file.kubeconfig.content
  sensitive   = true
}

output "cluster_endpoint" {
  description = "API server URL"
  value       = "https://${local.node_ip}:6443"
}
