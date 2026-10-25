# Platform baseline (Epic 4)

Order of operations once `infra/terraform/cluster/` has provisioned a cluster:

1. **MetalLB** - `helm install metallb metallb/metallb -n metallb-system --create-namespace -f ../helm/values/metallb-values.yaml`,
   then `kubectl apply -f metallb/ip-address-pool.yaml`. **Done** - the pool is a
   real IP now (`10.0.2.50`), not the kind-era placeholder. On Azure specifically,
   this IP also has to be registered as a secondary IP configuration on the
   cluster VM's NIC (see `infra/terraform/modules/k8s-cluster/azure-k3s/main.tf`) -
   MetalLB's default L2/ARP mode doesn't work unmodified on Azure's SDN otherwise.
2. **Traefik** - `helm install traefik traefik/traefik -n traefik --create-namespace -f ../helm/values/traefik-values.yaml`.
   Gets its external IP from MetalLB. **Done** - confirmed serving traffic on
   `10.0.2.50` (301 on :80, redirecting to :443 per `traefik-values.yaml`; 404 on
   :443, expected with no IngressRoutes registered yet). Also had to fix the
   values file's `ports.web/websecure` keys for the chart's current schema (v42,
   Traefik v3.7) - the originally-scaffolded `redirectTo`/`tls` keys are from an
   older chart major version and were rejected outright on install.
3. **Kyverno** - `helm install kyverno kyverno/kyverno -n kyverno --create-namespace -f ../helm/values/kyverno-values.yaml`,
   then `kubectl apply -f kyverno/policies/`. Do this early - the namespace-baseline
   policies (PSA labels + default-deny NetworkPolicy generation) only apply to
   namespaces created AFTER Kyverno is up, so later steps benefit from them, earlier
   ones (MetalLB/Traefik namespaces above) don't and are explicitly excluded anyway.
   **Done** - chart v3.9.2 (Kyverno v1.19.2) split the old single controller into
   four (admission/background/cleanup/reports controllers), so the originally
   scaffolded top-level `replicaCount`/`backgroundScan`/`resources` keys are stale
   for this schema - unlike Traefik's chart, this one has no strict schema
   validation, so they were silently ignored rather than erroring (installed
   "successfully" while doing nothing until checked against the real schema).
   `enforce-namespace-baseline.yaml` was also missing `traefik` from its namespace
   exclude list - caught before applying it, since it would have generated a
   default-deny-all NetworkPolicy onto Traefik's own namespace. `verify-image-
   signatures.yaml`'s keyless attestor needed an explicit `rekor.url` added -
   Kyverno's admission webhook now rejects a keyless entry without one outright.
   After all six policies were applied, a background scan turned up real
   `require-resource-limits` violations on the live MetalLB/Traefik pods (both
   values files set `limits.memory` but not `limits.cpu`) - fixed in
   `metallb-values.yaml`/`traefik-values.yaml`, which also disabled MetalLB's
   unused `frrk8s` (BGP) sidecar entirely, since L2Advertisement is the only mode
   this design uses and that sidecar was failing `require-readonly-rootfs` for a
   component nothing here needs. Re-verified clean (0 FAIL) on every live pod
   after both `helm upgrade`s rolled out.
4. **ArgoCD** - see `argocd/README.md`.
5. **kube-bench** - `kubectl apply -f kube-bench/cronjob.yaml`. See that file's header
   for why it's an intentional exception to the policies from step 3.
6. **ARC runner pool** (Epic 3, not this epic, but depends on the same cluster) -
   see `arc/README.md`.

## What this epic deliberately leaves open

- **Kyverno policy API**: all six policies use the legacy `kyverno.io/v1 ClusterPolicy`
  kind, which the chart itself now warns is deprecated in favor of
  `policies.kyverno.io` (ValidatingPolicy/ImageValidatingPolicy/etc., CEL-based).
  Still fully supported in v1.19.2, not urgent, but a real future migration -
  tracked in the local backlog rather than done speculatively here.
- **Traefik TLS**: no cert-manager/ACME wired up - internal CA vs. public ACME isn't
  decided, depends on Epic 8's VPN/admin-access design.
- **ARC's dind vs. rootless BuildKit** (Epic 3) and **kube-bench's privileged
  exception** are both named, flagged tradeoffs against the `restricted`-everywhere
  posture, not oversights.
- **NetworkPolicy egress to github.com/ghcr.io** (Epic 3's `arc/network-policy.yaml`)
  is coarse (`0.0.0.0/0:443`) because vanilla Kubernetes NetworkPolicy can't match by
  hostname - tightening it needs a CNI with FQDN-aware policies (e.g. Cilium), which
  this epic doesn't pick. Whatever CNI the cluster module ends up using (currently
  kind's default, not explicitly chosen) should be revisited with that in mind.
- **RBAC least-privilege per service account** and **namespace-per-app-per-
  environment** are conventions this epic enables (via the Kyverno baseline and the
  `environments/{dev,staging,prod}` layout) but doesn't enforce yet - there's no app
  to apply them to.
