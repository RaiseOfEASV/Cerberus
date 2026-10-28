output "kubeconfig" {
  description = "Raw kubeconfig for this cluster, server URL rewritten from 127.0.0.1 to the node's private IP (reachable over the VPN)"
  value       = data.local_file.kubeconfig.content
  sensitive   = true
}

output "cluster_endpoint" {
  description = "API server URL"
  value       = "https://${local.node_ip}:6443"
}

output "cluster_public_ip" {
  description = "The second, dedicated public IP for the public-facing Traefik instance (cerberoast.site and future public apps) - NOT the VM's original outbound-only public IP. Point the site's DNS A record here."
  value       = azurerm_public_ip.cluster_public.ip_address
}
