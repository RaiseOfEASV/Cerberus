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

## How it's laid out

- headscale itself runs on the bootstrap VM (`docker compose`, this directory),
  control API on `:8080`, public.
- The **bootstrap VM is also the subnet router**: its own tailscaled (tagged
  `tag:router`) advertises the whole VNet, `10.0.0.0/16` - the bootstrap subnet and
  the cluster subnet. Nothing VPN-related runs on the cluster VM, so nothing touches
  k3s's networking, and the cluster VM can be deallocated without cutting VPN access.
  Route approval is in `config/acl-policy.hujson`'s `autoApprovers`, not a manual step.
- **Only `tag:admin` devices can reach anything** (`config/acl-policy.hujson`). A
  device that joins without that tag gets nothing - checked: the router itself
  (`tag:router`) can't reach an admin laptop, while the laptop reaches everything.

## Setup (already done on the current bootstrap VM)

1. `docker compose up -d` with the backup env vars set (see `infra/backups/`).
2. Subnet router: `ssh cerberus-admin@<bootstrap-ip> 'bash -s' < scripts/setup-subnet-router.sh`
   - creates the `cerberus` user, installs tailscale from Tailscale's signed apt repo,
   joins this host as `tag:router` advertising `10.0.0.0/16`. Idempotent.

Run headscale CLI commands via the container's compose label - `docker compose
exec` fails here unless the backup env vars are set:
```
C=$(docker ps -q --filter label=com.docker.compose.service=headscale)
docker exec $C headscale nodes list --tags
```

## Adding an admin device

1. Install the standard Tailscale client (Kali/Debian: Tailscale's `sid` apt repo).
2. Join with a 10-minute single-use key tagged `tag:admin`, passed via a temp file so
   it never hits the screen or shell history:
   ```
   K=$(mktemp) && ssh -i ~/.ssh/cerberus_azure cerberus-admin@<bootstrap-ip> 'docker exec $(docker ps -q --filter label=com.docker.compose.service=headscale) headscale preauthkeys create --user cerberus --expiration 10m --tags tag:admin -o json 2>/dev/null' | python3 -c 'import json,sys; print(json.load(sys.stdin)["key"])' > "$K" && sudo tailscale up --login-server=http://<bootstrap-ip>:8080 --auth-key="file:$K" --accept-routes --accept-dns=false; shred -u "$K"
   ```
   `--accept-routes` is required on Linux, or the client ignores subnet routes.
3. Then use private IPs directly: `ssh cerberus-admin@10.0.2.4` (cluster) /
   `10.0.1.4` (bootstrap), `kubectl --server https://10.0.2.4:6443`, Traefik at
   `10.0.2.50`.

Remove a device: `docker exec $C headscale nodes delete -i <id>`. A different person
gets their own headscale user (`headscale users create <name>`), not `cerberus`.

## Known gaps

- **`server_url` is plain HTTP.** Tailscale's Noise protocol still encrypts and
  authenticates the control channel, but a client's first contact trusts whatever
  server key it gets (TOFU). Fix: a real TLS cert on this host.
- **MagicDNS isn't usable yet.** `config/config.yaml` sets no upstream nameservers, so
  a client with `--accept-dns` would lose normal DNS. Clients use `--accept-dns=false`
  and IPs until that's configured.
- **Public SSH/API ports are still open** on both VMs' NSGs (admin-IP-restricted).
  Now that the VPN path works, they can be closed - a deliberate, separate step.
- DERP relay uses Tailscale's own public fleet (not self-hosted). Only matters when a
  direct connection can't be made - the admin laptop -> router path is direct
  (UDP 41641 is open on the bootstrap NSG).
