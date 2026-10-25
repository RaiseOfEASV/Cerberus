output "bootstrap_vm_public_ip" {
  description = "Public IP of the bootstrap VM. SSH here as var.admin_username; headscale/terraform-state/backups get deployed manually after - see README.md."
  value       = azurerm_public_ip.bootstrap.ip_address
}
