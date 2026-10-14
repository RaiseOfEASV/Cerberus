# Terraform state backend

Closes the "Terraform state is local" gap flagged since Epic 1 (`infra/terraform/github/`)
and Epic 2. Every Terraform root in this repo now uses `backend "pg"` pointing at the
Postgres instance defined in `docker-compose.yml`.

## Why Postgres, not S3/MinIO

- Built-in state locking via Postgres advisory locks, supported since Terraform 0.9 -
  no DynamoDB-style side-table needed.
- The newer S3 backend native locking (`use_lockfile`) needs Terraform >= 1.10; this
  environment has 1.6.3 installed. Postgres sidesteps that version requirement.
- Self-hostable, consistent with the project's general self-hosted-where-reasonable
  posture (Nexus, ARC runners, Wazuh).

## Setup

1. `export TF_STATE_PG_PASSWORD=<a real secret>` - never commit it.
2. `docker compose -f docker-compose.yml up -d` on a persistent host (see the
   docker-compose.yml header for the "not yet on a hardened VM" caveat).
3. Each Terraform root does partial backend configuration (`backend "pg" {}` with no
   `conn_str` in the committed `.tf` file) and supplies the connection string at
   `init` time:

   ```
   terraform init -backend-config="conn_str=postgres://terraform:${TF_STATE_PG_PASSWORD}@<host>:5432/terraform_state?sslmode=require"
   ```

   Never put the connection string (it embeds the password) in a committed file.

## What's NOT solved here

- **Encryption at rest**: Postgres doesn't encrypt table contents by itself - this
  relies on disk/volume-level encryption on whatever host runs the container.
  Confirm that's true of wherever this actually gets deployed; this compose file
  doesn't verify it.
- **Credential lifecycle**: `TF_STATE_PG_PASSWORD` is a bare env var for now. Once
  Epic 6 ships Vault, this should become a Vault-issued credential - but Vault's own
  Terraform-provisioned infra would need this backend to exist first, so there's an
  unavoidable bootstrap step here that stays manual/out-of-band.
- **Backup**: this is now the single source of truth for what Terraform thinks it
  manages. Losing it doesn't destroy the real infra, but it does mean falling back to
  `terraform import` for everything. **Closed in Epic 9**: `docker-compose.yml`'s
  `backup` service runs a daily `pg_dump` to the dedicated backup MinIO
  (`infra/backups/`) - see that directory's README for restore instructions and the
  encryption-at-rest caveat that still applies to backups too.
