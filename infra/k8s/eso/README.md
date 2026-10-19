# External Secrets Operator

Syncs Vault secrets into native K8s `Secret` objects. This is what finally resolves
two gaps that were flagged as open since Epic 2 and Epic 3:

- `infra/nexus/provision.sh`'s Nexus admin credentials -> `external-secret-nexus-admin.yaml`
- the ARC runner pool's GitHub App key (`arc-runners-values.yaml`'s `githubConfigSecret`) -> `external-secret-arc-github-app.yaml`

## Order of operations

1. Epic 4's cluster, Epic 6's Vault deployed + initialized/unsealed + `infra/terraform/vault/` applied.
2. `helm install external-secrets external-secrets/external-secrets -n external-secrets --create-namespace -f ../helm/values/eso-values.yaml`
3. `kubectl apply -f cluster-secret-store.yaml`
4. Write the real secret values into Vault by hand (never via Terraform or this
   repo - see `infra/terraform/vault/README.md` step 3).
5. `kubectl apply -f external-secret-nexus-admin.yaml -f external-secret-arc-github-app.yaml`
   (the target namespaces, `nexus` and `arc-runners`, must already exist).
6. `kubectl get externalsecret -A` - `SYNCED` means the real K8s Secret now exists;
   until step 4 is done for a given path, expect sync errors, not silence.

## Not yet deployable

Needs everything in step 1. Nothing here is guessable ahead of that - there was
deliberately no attempt to pre-populate fake secret values anywhere.
