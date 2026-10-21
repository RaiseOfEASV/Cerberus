# Dependency-Track

Ingests the SBOMs `build.yml` (Epic 3) already generates with Syft, tracking
vulnerability status per service continuously instead of only at build time.

## Order of operations

1. Epic 4's cluster, Epic 6's Vault/ESO.
2. `vault kv put secret/dependency-track/db username=dtrack password=<...>`
3. `kubectl apply -f ../eso/external-secret-dependency-track-db.yaml -f deployment.yaml`
4. Log in with Dependency-Track's own default `admin`/`admin` and change it
   immediately - a manual day-1 step, same pattern as Vault's root token.
5. Create an API key in the UI, store it as a **GitHub Actions secret**
   (`DEPENDENCY_TRACK_API_KEY`) - NOT via Vault/ESO. This is a CI-side secret
   consumed by `build.yml`, a different trust boundary from the in-cluster
   runtime secrets ESO manages; see that workflow's header for why the two
   mechanisms stay separate.

## Not yet deployable

Needs everything in step 1.
