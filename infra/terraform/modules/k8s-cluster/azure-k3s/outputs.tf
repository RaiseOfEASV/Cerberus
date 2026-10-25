output "kubeconfig" {
  description = "Raw kubeconfig for this cluster, server URL rewritten from 127.0.0.1 to the VM's public IP"
  value       = data.local_file.kubeconfig.content
  sensitive   = true
}

output "cluster_endpoint" {
  description = "API server URL"
  value       = "https://${azurerm_public_ip.cluster.ip_address}:6443"
}
