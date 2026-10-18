# Vault configuration as code

Configures Vault's secrets engine, Kubernetes auth method, and policies/roles.
Does NOT initialize/unseal Vault (see `docs/security/vault-unseal-procedure.md`,
inherently manual) and does NOT write any actual secret *values* (step 3 below -
those can never be Terraform resources, since the value would end up in state).

## Order of operations

1. Vault deployed (Epic 4 cluster + `infra/helm/values/vault-values.yaml`),
   initialized and unsealed per `docs/security/vault-unseal-procedure.md`.
2. `export VAULT_TOKEN=<a real, sufficiently privileged token>` (not the raw root
   token for routine use - see the unseal doc on creating a scoped admin token).
   `terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=require"`,
   then `terraform apply`. This creates the `secret/` KV mount, the `kubernetes`
   auth method, and the `eso-read`/`ci-read` policies + roles - but no actual
   secrets exist at any path yet.
3. Write the real secret values by hand, directly against Vault, never through
   Terraform:
   ```
   vault kv put secret/nexus/admin username=<...> password=<...>
   vault kv put secret/arc/github-app app_id=<...> installation_id=<...> private_key=@/path/to/key.pem
   ```
4. Deploy External Secrets Operator and apply `infra/k8s/eso/` - see that
   directory's README for how these paths map to the ExternalSecrets that Epic
   2's Nexus provisioning and Epic 3's ARC runner pool were both waiting on.

## What's NOT here

- No path under `secret/ci/*` has anything written to it, and no workflow reads it
  yet - the `arc-runners` Kubernetes auth role exists ahead of any consumer,
  deliberately, so wiring a future workflow to read a CI secret doesn't require
  touching this Terraform again.
- No secret rotation policy. Token TTLs are short (300s) by design - roles re-
  authenticate per-use rather than holding a long-lived Vault token - but nothing
  here rotates the underlying secret *values* (Nexus admin password, GitHub App
  key) on a schedule.
