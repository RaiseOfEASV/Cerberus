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
  # Hides this from Vault's own login UI's method list - ESO authenticates
  # via the API directly (TokenReview), never through the UI, so this is
  # purely about not showing a human a login option that was never meant
  # for them. Doesn't affect API/CLI access at all - listing_visibility is
  # a UI-discovery setting, not an authorization control.
  tune {
    listing_visibility = "hidden"
  }
}

# --- Native SSO: Keycloak login directly into Vault itself ---
# Different from oauth2-proxy's ForwardAuth gate (infra/k8s/oauth2-proxy/) -
# that only decides "is this request allowed through to Traefik's backend
# at all," Vault itself has no idea who you are once past it. This lets
# a user actually log into VAULT (UI or `vault login -method=oidc`) with
# their Keycloak identity and get a real Vault token.
#
# Only the MOUNT is managed here - `vault write auth/oidc/config` (which
# needs the Keycloak client's secret) is a manual step, same "secret
# values never through Terraform" pattern as everything else in this
# file. See docs/security/vault-unseal-procedure.md's sibling doc, or
# just: generate the secret, store it in Vault via the CLI, configure
# this auth method's client_secret via the CLI too - never as a
# Terraform argument, which would put it in state.
#
# That manual `vault write` also needs `oidc_discovery_ca_pem` pointed
# at the internal CA (infra/k8s/oauth2-proxy/ca-configmap.yaml has the
# current cert) - confirmed the hard way: without it, Vault's own HTTP
# client doesn't trust cert-manager's internal CA when validating the
# discovery URL's TLS cert (a separate concern from Vault's own listener
# cert), and the write fails with a generic "error checking oidc
# discovery URL" that doesn't distinguish this from a network problem.
resource "vault_auth_backend" "oidc" {
  type = "oidc"
  path = "oidc"
  # unauth here (not hidden - the opposite setting from kubernetes
  # above) is what makes Vault's login UI actually feature this as the
  # prominent/default option instead of a generic "Token" field with
  # OIDC buried in a method dropdown a human has to know to switch to.
  tune {
    listing_visibility = "unauth"
  }
}

# role_type/user_claim/bound_audiences/redirect URIs are all non-secret -
# safe to manage here. bound_claims is the actual gate: without it, ANY
# successfully authenticated cerberus-admin realm user (not just "admins"
# group members) would get a Vault token with the admin policy - Vault's
# OWN OIDC role has no idea about Keycloak's "admins" group unless told.
resource "vault_jwt_auth_backend_role" "oidc_default" {
  backend         = vault_auth_backend.oidc.path
  role_name       = "default"
  role_type       = "oidc"
  token_policies  = [vault_policy.admin.name]
  user_claim      = "sub"
  oidc_scopes     = ["groups"] # explicit, not relying on "groups" being a Keycloak default scope the plugin's own auth_url request (which only ever asks for "openid") would pick up anyway - bound_claims below needs this claim actually present
  bound_audiences = ["vault"]
  allowed_redirect_uris = [
    "https://vault.admin.cerberus.internal/ui/vault/auth/oidc/oidc/callback",
    "https://vault.admin.cerberus.internal/v1/auth/oidc/oidc/callback",
    "http://localhost:8250/oidc/callback",
  ]
  bound_claims      = { groups = "admins" }
  bound_claims_type = "glob"
  token_ttl         = 3600
}

resource "vault_kubernetes_auth_backend_config" "k8s" {
  backend         = vault_auth_backend.kubernetes.path
  kubernetes_host = var.kubernetes_host
  # kubernetes_ca_cert / token_reviewer_jwt deliberately omitted - when
  # applied from a pod inside the cluster (the normal case), the provider/
  # Vault can use the in-cluster service account token and CA automatically.
  # Supply them explicitly if ever applying this from outside the cluster.
}

# --- Day-to-day admin ---
# What a human operator (and this Terraform, after the initial root-token
# apply) uses instead of the root token. Covers everything this root
# manages plus writing secret values and managing tokens. Deliberately
# excludes root-only powers - sys/raw, generate-root, rekey, seal config.
# Create the token as an ORPHAN: it's created by the root token, and
# revoking root would otherwise revoke this child token along with it.
# See docs/security/vault-unseal-procedure.md.

resource "vault_policy" "admin" {
  name = "admin"

  policy = <<-EOT
    # Secret values (KV v2)
    path "secret/*" {
      capabilities = ["create", "read", "update", "delete", "list"]
    }

    # Mounts, auth methods, policies - what this Terraform manages
    path "sys/mounts" {
      capabilities = ["read", "list"]
    }
    path "sys/mounts/*" {
      capabilities = ["create", "read", "update", "delete", "list", "sudo"]
    }
    path "sys/auth" {
      capabilities = ["read", "list"]
    }
    path "sys/auth/*" {
      capabilities = ["create", "read", "update", "delete", "list", "sudo"]
    }
    path "auth/kubernetes/*" {
      capabilities = ["create", "read", "update", "delete", "list"]
    }
    path "auth/oidc/*" {
      capabilities = ["create", "read", "update", "delete", "list"]
    }
    path "sys/policies/acl" {
      capabilities = ["list"]
    }
    path "sys/policies/acl/*" {
      capabilities = ["create", "read", "update", "delete", "list"]
    }
    path "sys/policy/*" {
      capabilities = ["create", "read", "update", "delete", "list"]
    }

    # Tokens - the Terraform provider mints a child token per run
    path "auth/token/create" {
      capabilities = ["create", "update"]
    }
    path "auth/token/lookup-self" {
      capabilities = ["read"]
    }
    path "auth/token/renew-self" {
      capabilities = ["update"]
    }
    path "auth/token/revoke-self" {
      capabilities = ["update"]
    }
    path "auth/token/revoke-accessor" {
      capabilities = ["update"]
    }

    # Manual raft snapshots (see the unseal doc's break-glass section)
    path "sys/storage/raft/snapshot" {
      capabilities = ["read"]
    }
  EOT
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
    path "secret/data/oidc/*" {
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
