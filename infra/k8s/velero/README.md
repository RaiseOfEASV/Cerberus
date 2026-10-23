# Velero (cluster backup)

## Order of operations

1. Epic 4's cluster, `infra/backups/` deployed (dedicated MinIO), Epic 6's Vault/ESO.
2. Create a dedicated, backup-only MinIO user+policy (not root) -
   `infra/backups/README.md`. `vault kv put secret/backup/minio access_key=...
   secret_key=... endpoint=...`
3. `kubectl apply -f ../eso/external-secret-velero-minio.yaml`
4. Fix the placeholder `s3Url` in `velero-values.yaml`, install the `velero` chart.
5. `kubectl apply -f schedule.yaml`

## Why this isn't a complete DR story by itself

Velero backs up Kubernetes object definitions plus, with File System Backup enabled
(`deployNodeAgent: true`, `defaultVolumesToFsBackup: true` in `velero-values.yaml` -
both on, deliberately, see that file's header), the actual contents of
PersistentVolumes too. Vault's own data is backed up separately
(`infra/k8s/vault/backup-cronjob.yaml`) via its native raft snapshot mechanism
instead of being swept up by Velero's generic FSB - raft storage has stronger
opinions about how it wants to be snapshotted than a filesystem-level copy gives it.

## Restore drills

See `docs/security/restore-drills.md` - an untested backup isn't a backup. Don't
treat this epic as "done" based on the schedule existing; it's done once a drill has
actually restored something.

## Not yet deployable

Needs everything in step 1.
