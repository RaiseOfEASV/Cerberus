# ARC runner pool (actions-runner-controller v2)

The self-hosted GitHub Actions runners that `.github/workflows/*.yml` target via
`runs-on: arc-runner-set`. Chosen over GitHub-hosted runners because CI needs to
reach the private Nexus proxy (Epic 2); chosen over Jenkins to keep one pipeline
engine with native GitHub Environments/Rulesets/secret-scanning integration.

## Pieces

- `infra/helm/values/arc-controller-values.yaml` - the cluster-wide controller
  (`gha-runner-scale-set-controller` chart), installed once into `arc-systems`.
- `infra/helm/values/arc-runners-values.yaml` - the actual runner pool
  (`gha-runner-scale-set` chart), installed into `arc-runners` (this directory's
  `namespace.yaml`). Its Helm release name **is** the `runs-on` value the
  workflows use - install it as `arc-runner-set`, not anything else.
- `namespace.yaml`, `network-policy.yaml` - namespace + default-deny-then-allow
  NetworkPolicies for the runner pool.

## Not yet deployable

- Needs Epic 4's cluster (and its CNI choice - see the gap noted in
  `network-policy.yaml`).
- `arc-runners-values.yaml`'s `githubConfigSecret` needs a GitHub App registered
  against Cerberus, with its private key stored as a k8s Secret created
  out-of-band (not in this repo). Nowhere secure to source that from until Epic
  6's Vault + External Secrets Operator exist.
- `containerMode: dind` (privileged sidecar for Docker builds) conflicts with the
  `restricted` Pod Security Admission label on the namespace - see the comment in
  `arc-runners-values.yaml`. Needs a real decision (dind vs. rootless BuildKit via
  buildx's `kubernetes` driver) once there's a cluster to test against.

## Order of operations once Epic 4 lands a cluster

1. Pick and install a CNI that supports FQDN-based egress policies if tightening
   `allow-internet-egress` matters before go-live; otherwise the coarse `0.0.0.0/0:443`
   rule is the starting point.
2. `kubectl apply -f namespace.yaml`
3. Install the controller chart with `arc-controller-values.yaml`.
4. Create the GitHub App, store its key as `arc-runner-github-app` in `arc-runners`.
5. Install the runner chart with `arc-runners-values.yaml`, release name `arc-runner-set`.
6. `kubectl apply -f network-policy.yaml`
7. Open a throwaway PR and confirm `pr-validate.yml`'s jobs actually pick up a runner.
