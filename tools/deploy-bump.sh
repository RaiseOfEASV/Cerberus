#!/usr/bin/env bash
# Pins apps' image tag + digest in their per-environment Helm values - the
# GitOps change behind a deploy. Used by .github/workflows/deploy.yml.
#
#   deploy-bump.sh --env dev     --apps all --version <tag>
#   deploy-bump.sh --env staging --apps all --version <tag>
#   deploy-bump.sh --env prod    --apps all --from staging
#
# --apps: "all" (every app with a Dockerfile and a values file for --env)
#         or a comma-separated list, e.g. vuln-api,vuln-web
# --version: image tag to deploy; its digest is resolved from the registry
# --from staging: (prod only) copy the exact tag + digest staging runs, so
#         prod gets byte-for-byte what was tested - no registry lookup
#
# All or nothing: every app is validated and resolved before any file is
# written. Prints the changed values files, one per line (nothing if all
# were already pinned). DEPLOY_SUMMARY=<file> also writes a markdown list.
# DEPLOY_ROOT overrides the repo root; DEPLOY_RESOLVE_DIGEST overrides the
# registry lookup (tests) - a shell snippet given the image ref as $1.
set -euo pipefail
root=${DEPLOY_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
REGISTRY_PREFIX=${REGISTRY_PREFIX:-ghcr.io/raiseofeasv}
die() { echo "error: $*" >&2; exit 1; }

env= apps= version= from=
while [ $# -gt 0 ]; do
  case "$1" in
    --env) env=${2:-}; shift 2 ;;
    --apps) apps=${2:-}; shift 2 ;;
    --version) version=${2:-}; shift 2 ;;
    --from) from=${2:-}; shift 2 ;;
    *) die "unknown argument $1" ;;
  esac
done
case "$env" in dev|staging|prod) ;; *) die "--env must be dev, staging or prod" ;; esac
[ -n "$apps" ] || die "--apps is required"
if [ -n "$from" ]; then
  [ "$from" = staging ] || die "--from only supports staging"
  [ "$env" = prod ] || die "--from staging only promotes to prod"
  [ -z "$version" ] || die "use either --version or --from, not both"
else
  [ -n "$version" ] || die "--version or --from is required"
  printf '%s' "$version" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$' || die "invalid image tag: $version"
  [ "$version" != latest ] || die "the latest tag is never deployed"
fi

if [ "$apps" = all ]; then
  list=()
  for d in "$root"/apps/*/; do
    a=$(basename "$d")
    [ -f "$d/Dockerfile" ] && [ -f "$root/infra/helm/$a/values-$env.yaml" ] && list+=("$a")
  done
  [ ${#list[@]} -gt 0 ] || die "no deployable apps found for $env"
else
  IFS=, read -ra list <<<"$apps"
fi

resolve() {
  if [ -n "${DEPLOY_RESOLVE_DIGEST:-}" ]; then
    bash -c "$DEPLOY_RESOLVE_DIGEST" _ "$1"
  else
    docker buildx imagetools inspect "$1" --format '{{json .Manifest}}' | jq -r .digest
  fi
}

# Pass 1: validate and resolve everything; nothing is written yet.
declare -A tags digests
for a in "${list[@]}"; do
  printf '%s' "$a" | grep -Eq '^[a-z0-9][a-z0-9-]*$' || die "invalid app name: $a"
  target="$root/infra/helm/$a/values-$env.yaml"
  [ -f "$target" ] || die "$a isn't wired into $env (no infra/helm/$a/values-$env.yaml)"
  if [ -n "$from" ]; then
    src="$root/infra/helm/$a/values-$from.yaml"
    [ -f "$src" ] || die "$a has no values-$from.yaml to promote from"
    t=$(yq '.image.tag // ""' "$src"); d=$(yq '.image.digest // ""' "$src")
    printf '%s' "$t" | grep -Eq '^0+$' && die "$a in $from still has the placeholder tag - deploy it to $from first"
  else
    t=$version
    d=$(resolve "$REGISTRY_PREFIX/$a:$version") || die "couldn't resolve $REGISTRY_PREFIX/$a:$version"
  fi
  [ -n "$t" ] || die "$a: no image tag found"
  printf '%s' "$d" | grep -Eq '^sha256:[a-f0-9]{64}$' || die "$a: no valid digest for tag $t (got '$d')"
  tags[$a]=$t; digests[$a]=$d
done

# Pass 2: write.
[ -z "${DEPLOY_SUMMARY:-}" ] || : > "$DEPLOY_SUMMARY"
for a in "${list[@]}"; do
  target="$root/infra/helm/$a/values-$env.yaml"
  before=$(cat "$target")
  TAG=${tags[$a]} DIGEST=${digests[$a]} yq -i '.image.tag = strenv(TAG) | .image.digest = strenv(DIGEST)' "$target"
  if [ "$(cat "$target")" != "$before" ]; then
    echo "infra/helm/$a/values-$env.yaml"
    [ -z "${DEPLOY_SUMMARY:-}" ] || echo "- **$a**: tag \`${tags[$a]}\`, digest \`${digests[$a]}\`" >> "$DEPLOY_SUMMARY"
  fi
done
