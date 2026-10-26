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
   `terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=disable"` -
   `disable`, not `require`: the Postgres container has no SSL cert configured
   (confirmed against the real server - `require` fails with
   `pq: SSL is not enabled on the server`). `<host>` should be `localhost` via an
   SSH tunnel to the bootstrap VM, not its public IP - see
   `infra/terraform/cluster/README.md`'s "Reaching the backend" section.
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

**From outside the cluster** (routine case - a laptop, not a pod), `var.vault_address`
needs an explicit override: its default (`https://vault.vault.svc:8200`) is
cluster-internal DNS, unresolvable from here. Use the real external hostname
instead, with `VAULT_CACERT` pointed at the internal CA (same cert
`infra/k8s/oauth2-proxy/ca-configmap.yaml` has) so the provider actually
validates the connection rather than skipping TLS verification:
```
export TF_VAR_vault_address="https://vault.admin.cerberus.internal"
export VAULT_CACERT="/path/to/cerberus-internal-ca.crt"
```
That hostname also sits behind the oauth2-proxy ForwardAuth gate for its UI -
Vault's own `/v1/*` API (what Terraform's provider actually calls) is carved out
of that gate in `infra/k8s/vault/ingressroute.yaml`, so this works without an
oauth2-proxy session.

## Native SSO: logging into Vault itself with Keycloak

Separate from the oauth2-proxy gate (which only checks "is this request allowed
through to Traefik's backend at all" - Vault itself has no idea who you are once
past it). `vault_auth_backend.oidc` + `vault_jwt_auth_backend_role.oidc_default`
here manage the auth method's *mount* and *role* (both non-secret) - the
`client_secret` is never a Terraform argument, same "secrets never in state"
pattern as everything else in this file. After `terraform apply`, configure the
secret-bearing part directly against Vault:
```
vault write auth/oidc/config \
  oidc_discovery_url="https://keycloak.admin.cerberus.internal/realms/cerberus-admin" \
  oidc_discovery_ca_pem=@/path/to/cerberus-internal-ca.crt \
  oidc_client_id="vault" \
  oidc_client_secret="<the vault client's secret - vault kv get -mount=secret oidc/vault-client>" \
  default_role="default"
```
`oidc_discovery_ca_pem` is not optional in practice - without it, this fails with
a real "error checking oidc discovery URL" that doesn't distinguish "can't trust
this cert" from "can't reach this host at all" (confirmed both ways: it's the
former here, Vault's own HTTP client doesn't trust cert-manager's internal CA by
default for its *outbound* calls, a separate concern from Vault's own listener
cert). The `admins` Keycloak group is what actually gates this - enforced via
`bound_claims` on the role, not by Keycloak alone.

**Recurring hazard, not a one-off**: re-running `infra/keycloak/apply-realm-config.yaml`
for ANY reason resets the `oauth2-proxy`/`vault`/`argocd` clients' secrets in
Keycloak, even ones that didn't change - see that Job manifest's own header. If
Vault's native OIDC login ever starts failing with "Invalid client or Invalid
client credentials" after that Job ran, re-run the `vault write auth/oidc/config`
command above with the freshly-regenerated secret, the same remediation as any
other affected client.

## What's NOT here

- No path under `secret/ci/*` has anything written to it, and no workflow reads it
  yet - the `arc-runners` Kubernetes auth role exists ahead of any consumer,
  deliberately, so wiring a future workflow to read a CI secret doesn't require
  touching this Terraform again.
- No secret rotation policy. Token TTLs are short (300s) by design - roles re-
  authenticate per-use rather than holding a long-lived Vault token - but nothing
  here rotates the underlying secret *values* (Nexus admin password, GitHub App
  key) on a schedule.
