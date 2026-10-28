# Public-domain ingress (cerberoast.site)

A second, fully isolated Traefik instance for public-facing sites - completely
separate from `infra/helm/values/traefik-values.yaml`'s admin instance (Vault,
ArgoCD, Keycloak), which stays exactly as VPN-only as it was. See
`infra/helm/values/traefik-public-values.yaml`'s own header comment for why
isolation is required, not optional, and how it's enforced
(`providers.kubernetesCRD.labelSelector: "traefik-instance=public"` - every
public-app IngressRoute must carry that label, or this instance won't serve it).

## Network path

`infra/terraform/modules/k8s-cluster/azure-k3s/main.tf` gives this its own
secondary IP configuration (`10.0.2.51`, `metallb_public_pool_ip`) backed by a
**second, dedicated Azure Public IP** - not the VM's original one (which stays
outbound-only). The NSG rule allowing 80/443 from the internet is scoped to
`destination_address_prefix = 10.0.2.51` only, so nothing admin-related (the
VM's primary IP, the admin MetalLB VIP `10.0.2.50`) becomes reachable from the
internet as a side effect.

**Real gotcha hit doing this**: a new `ip_configuration` added to an
already-booted VM's NIC does NOT get live-reconfigured into the guest OS
automatically on this image - confirmed by restarting `walinuxagent` with no
effect. Fixed with a persistent netplan drop-in added directly on the VM
(`/etc/netplan/60-metallb-public-pool.yaml`, NOT managed by Terraform/
cloud-init - there's no reboot-free mechanism for this in either). If this
module ever needs a third post-boot IP, this manual step is needed again.

## Order of operations

1. `kubectl apply -f ../metallb/ip-address-pool.yaml` - adds the
   `cerberus-public-pool` IPAddressPool/L2Advertisement (`autoAssign: false`,
   claimed explicitly by this release's Service annotation, never by accident).
2. `kubectl apply -f ../kyverno/policies/enforce-namespace-baseline.yaml` -
   **must happen before step 3**, or the `traefik-public` namespace gets the
   normal restricted-PSA + default-deny-all treatment applied before the
   exclude-list update takes effect (hit this for real: created the namespace
   first, got exactly that).

   **Real, confirmed-the-hard-way gotcha**: Kyverno rejects a plain
   `kubectl apply` that changes a generate rule's `match`/`exclude` on an
   EXISTING ClusterPolicy ("changes of immutable fields of a rule spec in a
   generate rule is disallowed") - the only fix is deleting and recreating
   the whole ClusterPolicy. Deleting it does NOT cascade-delete
   already-generated resources via a Kubernetes ownerReference (verified -
   none exists), but it DOES wipe Kyverno's OWN internal tracking for every
   resource that policy ever generated, cluster-wide - confirmed for real:
   after delete+recreate, `default-deny-all` disappeared from EVERY
   non-excluded namespace that already existed (`vault`, `keycloak`,
   `oauth2-proxy`, `cerberoast`), not just the one namespace being fixed.
   Restarting the background-controller did NOT trigger a retroactive
   reconciliation for already-existing namespaces either (no new
   UpdateRequest objects appeared) - generate rules only fire on an
   admission EVENT. The actual fix: force one by touching each affected
   namespace, e.g. `kubectl annotate namespace vault kyverno.io/retrigger="$(date +%s)" --overwrite`
   for every non-excluded namespace, which immediately regenerated the
   missing NetworkPolicy in each. Vault itself was never actually
   unprotected in practice during the gap - its own specific `allow-*`
   NetworkPolicies already cover both Ingress and Egress for its pods, and
   Kubernetes' own NetworkPolicy semantics default-deny a direction as soon
   as ANY policy selects a pod for it, with or without a separate blanket
   deny rule. `cerberoast` was the one real (minor) exposure: its only
   policy was Ingress-only, so its placeholder pod briefly had unrestricted
   egress - fixed within the same remediation pass. If this policy's
   `match`/`exclude` is ever edited again, repeat this whole procedure, not
   just the `kubectl apply`.
3. `kubectl apply -f namespace.yaml`
4. `helm install traefik-public traefik/traefik -n traefik-public -f ../../helm/values/traefik-public-values.yaml`
5. `kubectl apply -f ../cert-manager/cluster-issuers.yaml` - adds
   `letsencrypt-public` (ACME DNS-01 via Cloudflare), separate from the
   internal-CA issuer every admin UI uses.
6. Cloudflare + Namecheap, one-time, external (can't be scripted from here):
   - Create a free Cloudflare account, add `cerberoast.site` as a site, note
     the two assigned nameservers.
   - Namecheap -> Domain List -> `cerberoast.site` -> Nameservers -> Custom DNS
     -> enter those two nameservers. Propagation is usually under an hour.
   - In Cloudflare, add an A record: `cerberoast.site` -> the Terraform output
     `cluster_public_ip` (NOT the VM's original public IP).
   - Create a Cloudflare API token scoped to **Zone:DNS:Edit, limited to the
     `cerberoast.site` zone only** - never a global/account-wide token.
7. `vault kv put secret/cloudflare/api-token api-token=<the token from step 6>`
   then `kubectl apply -f ../eso/external-secret-cloudflare-api-token.yaml` -
   needs `infra/terraform/vault/`'s `eso-read` policy to already include
   `secret/data/cloudflare/*` (added alongside this feature).
8. `kubectl apply -f ../cerberoast/` (namespace, network-policy, the
   placeholder ConfigMap/Deployment/Service, IngressRoute+Certificate) - a
   trivial "coming soon" page proving DNS -> TLS -> NSG -> Traefik all work
   end to end. Swap the Deployment/ConfigMap for a real app later; nothing
   else here needs to change for that.

## What's deliberately NOT done here

- `dev`/`staging` app environments stay VPN-only, same as the admin tools -
  only `prod` (or, for now, this placeholder) gets a public hostname. See
  `BACKLOG.local.md`'s public-domain entry for the original reasoning.
- The admin Traefik instance's own `providers.kubernetesCRD` is untouched -
  it has no `labelSelector`, so it will also harmlessly attempt to serve
  `cerberoast`'s IngressRoute, but only on its own private VIP, never
  reachable from the internet. Cosmetic duplication, not fixed, to avoid
  touching already-stable admin config for a non-issue.
