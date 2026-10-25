# Restore Drills

"An untested backup isn't a backup" - the plan's own words for this epic. A schedule
existing in `infra/k8s/velero/schedule.yaml` proves nothing by itself; this document
is what actually proves recoverability, and the log at the bottom is what proves it's
still being checked, not just set up once and forgotten.

## Cadence

Quarterly, minimum. Add an entry to the log below every time, pass or fail - a failed
drill that gets logged and fixed is far more valuable than a skipped one.

## Drill 1: Velero cluster restore

1. `velero backup create drill-$(date +%Y%m%d) --include-namespaces <a real namespace>`
2. Wait for `velero backup describe drill-... ` to show `Completed`.
3. Restore into a **different, scratch namespace** - never restore over the live one
   during a drill:
   ```
   velero restore create --from-backup drill-... --namespace-mappings <real>:<real>-drill-restore
   ```
4. Confirm the restored resources AND their actual file data (not just object
   definitions - see `infra/k8s/velero/README.md`'s File System Backup note) match
   what's expected.
5. Delete the scratch namespace and the drill backup when done.

## Drill 2: Vault raft snapshot restore

Don't improvise this during a drill - follow
[`vault-unseal-procedure.md`](vault-unseal-procedure.md)'s existing
`vault operator raft snapshot restore` section verbatim, onto a throwaway Vault
instance, not the live one. A drill that restores onto production Vault isn't a
drill.

## Drill 3: Postgres restore (terraform-state, and any app DB backed up the same way)

1. Download the latest `.sql.gz` for the target DB from the backup MinIO
   (`infra/backups/`).
2. `gunzip` and `psql` it into a fresh, throwaway Postgres instance - never the live
   one.
3. Spot-check row counts / a few known records against what's expected.

## Drill 4: headscale / Wazuh tar restore

Same shape as Drill 3: pull the latest tarball, extract into a scratch directory, and
confirm the expected files are there and readable. These are file-level, not
point-in-time-consistent backups (see `infra/backups/scripts/tar-backup.sh`'s header)
- a drill here is really confirming "the tar isn't corrupt," not "this is a perfectly
consistent snapshot." That limitation is real; don't let a passing drill imply more
confidence than that.

## Drill log

| Date | What was drilled | Result | Notes / follow-ups |
|------|-------------------|--------|---------------------|
| _none yet_ | | | |
