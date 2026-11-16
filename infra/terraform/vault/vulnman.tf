# --- VulnMan (docs/superpowers/plans/2026-11-16-vulnman.md, D9/D15) ---
# Transit keys per environment, and a Kubernetes auth role per environment
# that lets only the vuln-api ServiceAccount in that environment's
# namespace use only that environment's keys. Secret VALUES for VulnMan
# live in KV under secret/vulnman/<env>/ and are written by hand
# (infra/k8s/keycloak-public/README.md) - never here.

variable "vulnman_envs" {
  description = "VulnMan environment -> Kubernetes namespace its vuln-api runs in."
  type        = map(string)
  default = {
    dev     = "dev"
    staging = "staging"
    prod    = "cerberoast"
  }
}

resource "vault_mount" "transit" {
  path        = "transit"
  type        = "transit"
  description = "Encryption as a service - keys never leave Vault"
}

# Envelope encryption of finding fields (FIND-11): vuln-api asks for a data
# key per finding and stores only its wrapped form.
resource "vault_transit_secret_backend_key" "vulnman_data" {
  for_each         = var.vulnman_envs
  backend          = vault_mount.transit.path
  name             = "vulnman-${each.key}-data"
  type             = "aes256-gcm96"
  deletion_allowed = false
}

# Report signatures (FR-RPT-03). The public key is readable so anyone can
# verify an exported report offline.
resource "vault_transit_secret_backend_key" "vulnman_report" {
  for_each         = var.vulnman_envs
  backend          = vault_mount.transit.path
  name             = "vulnman-${each.key}-report"
  type             = "ed25519"
  deletion_allowed = false
}

resource "vault_policy" "vuln_api" {
  for_each = var.vulnman_envs
  name     = "vuln-api-${each.key}"

  policy = <<-EOT
    path "transit/datakey/plaintext/vulnman-${each.key}-data" {
      capabilities = ["update"]
    }
    path "transit/decrypt/vulnman-${each.key}-data" {
      capabilities = ["update"]
    }
    path "transit/sign/vulnman-${each.key}-report" {
      capabilities = ["update"]
    }
    path "transit/verify/vulnman-${each.key}-report" {
      capabilities = ["update"]
    }
    path "transit/keys/vulnman-${each.key}-report" {
      capabilities = ["read"]
    }
  EOT
}

# vuln-api logs in with a projected ServiceAccount token whose audience is
# "vault" (infra/helm/charts/app vaultToken), so a token minted for anything
# else can't be replayed here.
resource "vault_kubernetes_auth_backend_role" "vuln_api" {
  for_each                         = var.vulnman_envs
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "vuln-api-${each.key}"
  bound_service_account_names      = ["vuln-api"]
  bound_service_account_namespaces = [each.value]
  audience                         = "vault"
  token_policies                   = [vault_policy.vuln_api[each.key].name]
  token_ttl                        = 900
  token_max_ttl                    = 900
}
