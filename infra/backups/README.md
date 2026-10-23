# Backups - index

What backs up what, and how. Written because by Epic 9 there are enough moving
pieces (in-cluster + 3 standalone hosts) that "where's the backup for X" needed a
single answer instead of being scattered across every component's own README.

| Source | Mechanism | Destination | Schedule |
|--------|-----------|--------------|----------|
| Kubernetes object definitions + PV file contents | Velero (FSB via node-agent) | `cerberus-backups/velero/` | Daily (`infra/k8s/velero/schedule.yaml`) |
| Vault raft data | Native `vault operator raft snapshot save`, via a CronJob | `cerberus-backups/vault/` | Daily (`infra/k8s/vault/backup-cronjob.yaml`) |
| Terraform state Postgres | `pg_dump` sidecar | `cerberus-backups/terraform-state-pg/` | Daily (`infra/terraform-state/docker-compose.yml`) |
| headscale sqlite + config | tar sidecar | `cerberus-backups/headscale/` | Daily (`infra/headscale/docker-compose.yml`) |
| Wazuh indexer + manager data | tar sidecar (not point-in-time consistent, see caveat) | `cerberus-backups/wazuh/` | Daily (`infra/wazuh/docker-compose.yml`) |

## What's explicitly NOT covered here (open gaps, not oversights)

- **Nexus's blob store** (Epic 2) - not yet deployed, so not yet backed up either.
  Revisit once it's live; it's mostly a re-fetchable cache (go/npm/docker proxies) so
  losing it is more an inconvenience than data loss, but its hosted repos (if any
  end up hosted rather than proxy-only) would need real backup.
- **Elasticsearch data** (Epic 7) - ECK supports native snapshot repositories
  (S3-compatible, so the same backup MinIO could serve this), but wiring that up
  isn't done here. General logs are lower-stakes than security logs (Wazuh, which
  IS covered above) or secrets (Vault, which IS covered above), so this was
  deprioritized rather than silently forgotten.
- **GHCR images** - GitHub's own infrastructure, outside this project's backup
  surface entirely. cosign signatures + Rekor's transparency log mean a re-pull is
  verifiable even if GHCR itself had an outage, which is a different kind of
  resilience than a backup.

## Setup

1. Stand up `docker-compose.yml` on its own dedicated host (same "plain VM now,
   Epic 5 golden image later" caveat as every other standalone host in this repo).
2. Create a bucket: `mc mb backup/cerberus-backups`
3. Create a **dedicated, backup-only** user and policy - not the root user used to
   bootstrap this:
   ```
   mc admin user add backup <access-key> <secret-key>
   mc admin policy create backup backup-only-policy.json   # scope to cerberus-backups/* only
   mc admin policy attach backup backup-only-policy --user backup
   ```
4. `vault kv put secret/backup/minio access_key=<backup-user-key> secret_key=<backup-user-secret> endpoint=<host>:9000`
5. Set a lifecycle/retention policy so backups don't grow forever:
   `mc ilm add backup/cerberus-backups --expiry-days 90` (adjust per source if some
   need longer retention than others).
6. Export `BACKUP_MINIO_ACCESS_KEY`/`BACKUP_MINIO_SECRET_KEY`/`BACKUP_MINIO_ENDPOINT`
   on each standalone host before `docker compose up` there.

## Restore drills

See `docs/security/restore-drills.md` - the schedule existing proves nothing by
itself.

## Not yet running

Needs a persistent host. Nothing in this table is actually backing anything up until
that host exists and every consumer above is pointed at its real address instead of
a placeholder.
