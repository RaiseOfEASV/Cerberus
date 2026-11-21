#!/usr/bin/env bash
# Renders every app's real per-environment values through its chart (the
# generic app chart, or the one named in infra/helm/<app>/.chart)
# and builds every GitOps environment, so a values change that the chart's
# Kyverno guards (or plain template errors) reject fails in PR CI - not at
# ArgoCD sync time. Run from anywhere: infra/helm/render-all.sh
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
fail=0; n=0
for values in "$root"/infra/helm/*/values-*.yaml; do
  app=$(basename "$(dirname "$values")")
  case "$app" in charts|values) continue;; esac
  n=$((n+1))
  chart="$root/infra/helm/charts/$(cat "$(dirname "$values")/.chart" 2>/dev/null || echo app)"
  extra=()
  # prod's vuln-scanner also gets the image catalog's scan list (tools/images.py).
  overlay="$(dirname "$values")/scan-images.yaml"
  if [ "$(basename "$values")" = values-prod.yaml ] && [ -f "$overlay" ]; then extra=(-f "$overlay"); fi
  if out=$(helm template "$app" "$chart" -f "$values" "${extra[@]}" 2>&1); then
    echo "PASS render $app/$(basename "$values")"
  else
    echo "FAIL render $app/$(basename "$values"): $(printf '%s' "$out" | grep -m1 -iE 'error|fail')"; fail=1
  fi
done
for env in "$root"/infra/k8s/environments/*/; do
  [ -f "$env/kustomization.yaml" ] || continue
  if out=$(kubectl kustomize "$env" 2>&1); then
    echo "PASS kustomize $(basename "$env")"
  else
    echo "FAIL kustomize $(basename "$env"): $(printf '%s' "$out" | head -1)"; fail=1
  fi
done
[ "$n" -gt 0 ] || { echo "FAIL no values files found"; fail=1; }
exit $fail
