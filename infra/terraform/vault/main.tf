# Configures Vault as code - everything EXCEPT the actual secret values
# (those are written manually post-apply, see README) and the
# initialization/unseal itself (see docs/security/vault-unseal-procedure.md
# - inherently a manual, human process, never Terraform).
#
# NOT YET APPLIABLE: needs Vault deployed (Epic 4's cluster +
# infra/helm/values/vault-values.yaml) AND initialized/unsealed per the
# break-glass doc above. VAULT_TOKEN must be a real, sufficiently
# privileged token - never the plan's placeholder, and never passed as a
# Terraform variable (that would put it in state).

terraform {
  required_providers {
    vault = {
      source  = "hashicorp/vault"
      version = "~> 4.0"
    }
  }

  # Own schema - every root shares one Postgres database and the pg backend
  # keys state only by schema + workspace (see infra/terraform/cluster/
  # main.tf's backend comment). Without this, this root would load the
  # cluster root's state.
  backend "pg" {
    schema_name = "vault"
  }
}

provider "vault" {
  address = var.vault_address
  # token read from VAULT_TOKEN env var by the provider directly - never a
  # Terraform variable, never in state.
}

resource "vault_mount" "kv" {
  path = "secret"
  type = "kv-v2"
}

resource "vault_auth_backend" "kubernetes" {
  type = "kubernetes"
}

resource "vault_kubernetes_auth_backend_config" "k8s" {
  backend         = vault_auth_backend.kubernetes.path
  kubernetes_host = var.kubernetes_host
  # kubernetes_ca_cert / token_reviewer_jwt deliberately omitted - when
  # applied from a pod inside the cluster (the normal case), the provider/
  # Vault can use the in-cluster service account token and CA automatically.
  # Supply them explicitly if ever applying this from outside the cluster.
}

# --- External Secrets Operator ---
# Reads Nexus admin creds (Epic 2), the ARC runner pool's GitHub App key
# (Epic 3), and Grafana/Wazuh-agent/Dependency-Track-DB creds (Epic 7) -
# see infra/k8s/eso/ for the ExternalSecret resources that consume these
# paths. Add a new path block here whenever a new ExternalSecret needs a
# new prefix - don't widen this to a blanket "secret/data/*" just to save
# a line, that's exactly the kind of over-broad grant this is meant to
# avoid.

resource "vault_policy" "eso_read" {
  name = "eso-read"

  policy = <<-EOT
    path "secret/data/nexus/*" {
      capabilities = ["read"]
    }
    path "secret/data/arc/*" {
      capabilities = ["read"]
    }
    path "secret/data/grafana/*" {
      capabilities = ["read"]
    }
    path "secret/data/wazuh/*" {
      capabilities = ["read"]
    }
    path "secret/data/dependency-track/*" {
      capabilities = ["read"]
    }
    path "secret/data/keycloak/*" {
      capabilities = ["read"]
    }
    path "secret/data/oauth2-proxy/*" {
      capabilities = ["read"]
    }
    path "secret/data/backup/*" {
      capabilities = ["read"]
    }
    path "secret/data/alertmanager/*" {
      capabilities = ["read"]
    }
  EOT
}

resource "vault_kubernetes_auth_backend_role" "eso" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "external-secrets"
  bound_service_account_names      = ["external-secrets"]
  bound_service_account_namespaces = ["external-secrets"]
  token_policies                   = [vault_policy.eso_read.name]
  token_ttl                        = 300
}

# --- CI (ARC runner pool) ---
# Not consumed by any workflow yet - no job currently calls Vault directly
# (pr-validate/build/release/deploy all use GitHub-native secrets). This
# exists so a future workflow CAN read CI-scoped secrets via the pod's own
# ServiceAccount, without a separate Epic to wire it in later.

resource "vault_policy" "ci_read" {
  name = "ci-read"

  policy = <<-EOT
    path "secret/data/ci/*" {
      capabilities = ["read"]
    }
  EOT
}

resource "vault_kubernetes_auth_backend_role" "ci" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "arc-runners"
  bound_service_account_names      = ["*"]
  bound_service_account_namespaces = ["arc-runners"]
  token_policies                   = [vault_policy.ci_read.name]
  token_ttl                        = 300
}

# --- Raft snapshot backup (Epic 9) ---
# Closes the gap docs/security/vault-unseal-procedure.md named explicitly:
# "No snapshot schedule exists yet... tracked for Epic 9." See
# infra/k8s/vault/backup-cronjob.yaml for the CronJob this role backs.
# Read-only on the snapshot endpoint specifically - this role can take a
# snapshot, it cannot restore one or read any actual secret data.

resource "vault_policy" "backup" {
  name = "backup"

  policy = <<-EOT
    path "sys/storage/raft/snapshot" {
      capabilities = ["read"]
    }
  EOT
}

resource "vault_kubernetes_auth_backend_role" "backup" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "vault-backup"
  bound_service_account_names      = ["vault-backup"]
  bound_service_account_namespaces = ["vault"]
  token_policies                   = [vault_policy.backup.name]
  token_ttl                        = 300
}
