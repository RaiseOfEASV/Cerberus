# headscale (VPN)

Self-hosted Tailscale-protocol control plane - chosen over Tailscale's own SaaS
specifically to avoid adding a SaaS dependency, consistent with this project's
general self-hosted-first posture. Standard Tailscale clients connect to this
instead of `controlplane.tailscale.com`.

## Why this exists before oauth2-proxy/Keycloak

The plan requires BOTH network-layer (must be on the VPN) and identity-layer (must be
an authenticated Keycloak user) restriction on every admin UI - either alone is the
usual failure mode (a VPN alone means "anyone who got a device on the VPN has full
access"; identity alone means an admin UI is one leaked credential away from being
reachable by anyone on the internet). headscale is the network-layer half.

## Setup

1. Fill in the real host address in `config/config.yaml`'s `server_url` (placeholder
   right now) and `config/acl-policy.hujson`'s `tagOwners` username.
2. `docker compose up -d`
3. Create a user and tag a device as admin:
   ```
   docker compose exec headscale headscale users create admin
   docker compose exec headscale headscale preauthkeys create --user admin --tags tag:admin
   ```
4. On each admin's machine: install the standard Tailscale client, then
   `tailscale up --login-server https://<headscale-host>:8080 --authkey <preauthkey>`.
5. Once Traefik (Epic 4) has a real LoadBalancer IP, replace every
   `CHANGE_ME_TRAEFIK_LB_IP` in `config/config.yaml`'s `extra_records` with it, and
   restart headscale.

## Not yet running

Needs a persistent host - same caveat as `infra/terraform-state/` and `infra/wazuh/`.
DERP relay uses Tailscale's own public fleet for now (not self-hosted) - a smaller,
still-real SaaS dependency than running the full control plane there; revisit if that
matters more than the setup cost of self-hosting DERP too.
