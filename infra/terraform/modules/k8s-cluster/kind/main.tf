# Local kind implementation of the k8s-cluster module interface (see
# ../README.md). Uses the tehcyx/kind provider rather than shelling out to
# the kind CLI via null_resource/local-exec, so cluster lifecycle is a real
# Terraform resource with real state, not a side effect of a script.

terraform {
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "~> 0.9"
    }
  }
}

resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = "kindest/node:${var.kubernetes_version}"
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    dynamic "node" {
      for_each = range(var.node_count)
      content {
        role = node.key == 0 ? "control-plane" : "worker"
      }
    }

    networking {
      # MetalLB (Epic 4) needs an IP range inside kind's docker network -
      # this doesn't configure that range itself, just leaves the default
      # pod/service CIDRs alone. See infra/k8s/metallb/ip-address-pool.yaml
      # for the TODO on finding kind's actual docker network subnet.
      disable_default_cni = false
    }
  }
}
