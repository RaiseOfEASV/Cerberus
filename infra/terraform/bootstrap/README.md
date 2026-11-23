# Bootstrap VM

Stands up the one Azure VM that `infra/headscale/`, `infra/terraform-state/`,
and `infra/backups/` run on - the three standalone docker-compose stacks
light enough to share a host.

## Why this root uses local state, not the `pg` backend

Every other Terraform root in this repo (`infra/terraform/github`,
`infra/terraform/cluster`, `infra/terraform/vault`) backs onto the Postgres
instance `infra/terraform-state/` runs. That Postgres instance runs on the
VM *this* root creates. A root can't depend on a backend that depends on
the resource the root itself is creating - so this one deliberately stays
on local state, same resolution `infra/terraform-state/README.md` already
documents for why its own Postgres isn't managed by the Terraform it backs.

Local state here means exactly that: `terraform.tfstate` lives only on
whoever runs `apply`, is gitignored (`*.tfstate` in the root `.gitignore`),
and isn't shared. That's an accepted tradeoff for a one-time, rarely-changed
bootstrap resource, not an oversight.

## Auth

Authenticates with an already-authenticated `az login` session, but the
target subscription is an explicit, required variable (`subscription_id` in
`terraform.tfvars`, or `TF_VAR_subscription_id`) rather than whatever `az`
happens to have active - that guess breaks as soon as you can see more than
one subscription. Moving the project to a different subscription is
`docs/runbooks/redeploy-new-subscription.md`.

## Setup

1. Copy `terraform.tfvars.example` to `terraform.tfvars`, set
   `subscription_id`, and set `admin_source_ips` to your current public IP(s)
   (`curl -s https://ifconfig.me`), in `x.x.x.x/32` form. This is gitignored -
   never commit your real IP.
2. SSH key: defaults to `~/.ssh/cerberus_azure.pub`, a dedicated key
   generated for this project (not a reused one).
3. `terraform init && terraform apply`

## What this does NOT do

Does not install or start headscale, terraform-state's Postgres, or the
backup MinIO. It only provisions the VM, networking, and Docker. Deploying
the actual stacks (and setting `TF_STATE_PG_PASSWORD`,
`BACKUP_MINIO_ROOT_USER`/`_PASSWORD`, headscale's `server_url`, etc.) is a
manual follow-up: copy `infra/headscale/`, `infra/terraform-state/`, and
`infra/backups/` onto the VM (preserving their relative layout - their
compose files reference each other's scripts by relative path, e.g.
`../backups/scripts/tar-backup.sh`), set the required env vars, and
`docker compose up -d` each stack. None of those secrets belong in
cloud-init or Terraform state.

## Network exposure

- SSH (22): restricted to `admin_source_ip` only. Not VPN-gated, because
  this host **is** the VPN - there's nothing to gate behind until headscale
  is actually running and the admin's own device has joined it.
- `8080/tcp` and `41641/udp`: headscale's coordination ports, open to the
  internet. Inherent to how a VPN control plane works - see
  `infra/headscale/docker-compose.yml`'s own header.
- Everything else (Postgres 5432, MinIO's S3 API 9000) is NOT opened in the
  NSG. Azure denies all other inbound traffic by default. Once a client
  joins the headscale VPN, that traffic reaches this host over its tun
  interface, not through the NSG's public-NIC filtering - so those become
  reachable over the VPN without any additional rule.

## If your IP changes

If `admin_source_ip` goes stale (dynamic ISP IP), SSH will simply stop
working with no obvious error from this side. Update `terraform.tfvars`
and `terraform apply` again.
