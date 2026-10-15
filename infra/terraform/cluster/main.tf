# Root config that actually provisions the cluster - the only file in this
# tree that should ever change `source` to swap implementations (see
# ../modules/k8s-cluster/README.md for the interface contract).
#
# NOT YET APPLIED: needs infra/terraform-state/ running first (backend
# below is partial config, conn_str supplied at init time - see that
# directory's README).

terraform {
  required_version = ">= 1.6"

  backend "pg" {}
}

module "cluster" {
  source = "../modules/k8s-cluster/kind"

  cluster_name       = var.cluster_name
  kubernetes_version = var.kubernetes_version
  node_count         = var.node_count
}

output "kubeconfig" {
  value     = module.cluster.kubeconfig
  sensitive = true
}

output "cluster_endpoint" {
  value = module.cluster.cluster_endpoint
}
