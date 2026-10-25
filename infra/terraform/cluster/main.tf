# Root config that actually provisions the cluster - the only file in this
# tree that should ever change `source` to swap implementations (see
# ../modules/k8s-cluster/README.md for the interface contract).
#
# Needs infra/terraform-state/ running first (backend below is partial
# config, conn_str supplied at init time - see that directory's README).

terraform {
  required_version = ">= 1.6"

  backend "pg" {}
}

provider "azurerm" {
  features {}
}

module "cluster" {
  source = "../modules/k8s-cluster/azure-k3s"

  cluster_name       = var.cluster_name
  kubernetes_version = var.kubernetes_version
  node_count         = var.node_count

  location             = var.location
  resource_group_name  = var.resource_group_name
  vm_size              = var.vm_size
  admin_username       = var.admin_username
  ssh_public_key_path  = var.ssh_public_key_path
  ssh_private_key_path = var.ssh_private_key_path
  admin_source_ip      = var.admin_source_ip
  metallb_pool_ip      = var.metallb_pool_ip
}

output "kubeconfig" {
  value     = module.cluster.kubeconfig
  sensitive = true
}

output "cluster_endpoint" {
  value = module.cluster.cluster_endpoint
}
