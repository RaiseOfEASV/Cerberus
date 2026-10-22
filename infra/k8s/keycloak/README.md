# Keycloak

OIDC provider for every admin UI's identity-layer check.

## Order of operations

1. Epic 4's cluster, Epic 6's Vault/ESO, Epic 8's cert-manager.
2. `vault kv put secret/keycloak/admin username=admin password=<...>`
   `vault kv put secret/keycloak/db password=<...>`
3. `kubectl apply -f ../eso/external-secret-keycloak.yaml`
4. Install the `keycloak` chart (`../../helm/values/keycloak-values.yaml`).
5. `vault kv put secret/oauth2-proxy/client secret=<generate one>` and apply
   `infra/k8s/eso/external-secret-oauth2-proxy.yaml` (Epic 8's oauth2-proxy piece) -
   `apply-realm-config.yaml` needs that secret to already be synced.
6. `kubectl create configmap keycloak-realm-config --from-file=../../keycloak/realm-config.yaml -n keycloak`
   then `kubectl apply -f ../../keycloak/apply-realm-config.yaml`.
7. Create at least one real admin user in the `cerberus-admin` realm and add them to
   the `admins` group - `realm-config.yaml` defines the group, not any users (no
   real person's credentials belong in a committed file).

## Not yet deployable

Needs everything in step 1. `realm-config.yaml`'s schema is unverified against a live
instance - see its own header comment.
