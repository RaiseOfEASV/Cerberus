#!/usr/bin/env bash
# Tests for tools/deploy-bump.sh against a throwaway repo layout, with the
# registry lookup stubbed (DEPLOY_RESOLVE_DIGEST). Run: tools/tests/deploy-bump-test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")/.." && pwd); bump="$here/deploy-bump.sh"
fail=0
ok()   { echo "ok   - $1"; }
bad()  { echo "FAIL - $1"; fail=1; }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

new_repo() {
  R=$(mktemp -d)
  for a in vuln-api vuln-web; do mkdir -p "$R/apps/$a"; touch "$R/apps/$a/Dockerfile"; done
  for a in vuln-api vuln-web; do for e in staging prod; do
    mkdir -p "$R/infra/helm/$a"
    printf 'image:\n  repository: ghcr.io/raiseofeasv/%s\n  tag: "old"\n  digest: "sha256:%064d"\n' "$a" 0 > "$R/infra/helm/$a/values-$e.yaml"
  done; done
  mkdir -p "$R/infra/helm/vuln-db"; printf 'vaultPath: x\n' > "$R/infra/helm/vuln-db/values-staging.yaml"; cp "$R/infra/helm/vuln-db/values-staging.yaml" "$R/infra/helm/vuln-db/values-prod.yaml"
  echo "$R"
}
# Stub: digest = sha256 of "<app>:<tag>", so each app/tag pair is distinct and checkable.
export DEPLOY_RESOLVE_DIGEST='printf "sha256:%s" "$(printf "%s" "$1" | sha256sum | cut -c1-64)"'
digest_of() { printf "sha256:%s" "$(printf "%s" "ghcr.io/raiseofeasv/$1:$2" | sha256sum | cut -c1-64)"; }
tag()    { yq '.image.tag' "$1"; }
digest() { yq '.image.digest' "$1"; }

# 1. all apps to staging: both images bumped and pinned, the database untouched.
R=$(new_repo); out=$(DEPLOY_ROOT=$R "$bump" --env staging --apps all --version abc123 2>&1); rc=$?
check "all -> staging exits 0" '[ $rc = 0 ]'
check "all -> staging reports both files" '[ "$(printf "%s\n" "$out" | grep -c "values-staging.yaml")" = 2 ]'
check "vuln-api staging tag+digest" '[ "$(tag $R/infra/helm/vuln-api/values-staging.yaml)" = abc123 ] && [ "$(digest $R/infra/helm/vuln-api/values-staging.yaml)" = "$(digest_of vuln-api abc123)" ]'
check "vuln-web staging tag+digest" '[ "$(tag $R/infra/helm/vuln-web/values-staging.yaml)" = abc123 ] && [ "$(digest $R/infra/helm/vuln-web/values-staging.yaml)" = "$(digest_of vuln-web abc123)" ]'
check "prod untouched" '[ "$(tag $R/infra/helm/vuln-api/values-prod.yaml)" = old ]'
check "vuln-db (no image) untouched" '[ "$(cat $R/infra/helm/vuln-db/values-staging.yaml)" = "vaultPath: x" ]'

# 2. explicit list
R=$(new_repo); DEPLOY_ROOT=$R "$bump" --env staging --apps vuln-web --version v2 >/dev/null 2>&1
check "explicit list bumps only that app" '[ "$(tag $R/infra/helm/vuln-web/values-staging.yaml)" = v2 ] && [ "$(tag $R/infra/helm/vuln-api/values-staging.yaml)" = old ]'

# 3. promote staging -> prod copies exactly what staging runs
R=$(new_repo); DEPLOY_ROOT=$R "$bump" --env staging --apps all --version s1 >/dev/null 2>&1
out=$(DEPLOY_ROOT=$R DEPLOY_RESOLVE_DIGEST='echo SHOULD-NOT-BE-CALLED; exit 1' "$bump" --env prod --apps all --from staging 2>&1); rc=$?
check "promote exits 0 without a registry lookup" '[ $rc = 0 ]'
for a in vuln-api vuln-web; do
  check "promote copies $a tag+digest" '[ "$(tag $R/infra/helm/'$a'/values-prod.yaml)" = s1 ] && [ "$(digest $R/infra/helm/'$a'/values-prod.yaml)" = "$(digest $R/infra/helm/'$a'/values-staging.yaml)" ]'
done

# 4. refusals
R=$(new_repo); yq -i '.image.tag = "0000000000000000000000000000000000000000"' "$R/infra/helm/vuln-web/values-staging.yaml"
DEPLOY_ROOT=$R "$bump" --env prod --apps all --from staging >/dev/null 2>&1; rc=$?
check "promote refuses a placeholder staging tag" '[ $rc != 0 ] && [ "$(tag $R/infra/helm/vuln-api/values-prod.yaml)" = old ]'
R=$(new_repo)
for args in "--env staging --apps all --from staging" "--env staging --apps all --version latest" "--env staging --apps ../etc --version v1" \
            "--env staging --apps nope --version v1" "--env qa --apps all --version v1" "--env staging --apps all" "--env prod --apps all --version v1 --from staging"; do
  DEPLOY_ROOT=$R "$bump" $args >/dev/null 2>&1; rc=$?
  check "refuses: $args" '[ $rc != 0 ]'
done
check "refusals changed nothing" '[ "$(tag $R/infra/helm/vuln-api/values-staging.yaml)" = old ]'
R=$(new_repo); DEPLOY_RESOLVE_DIGEST='echo not-a-digest' DEPLOY_ROOT=$R "$bump" --env staging --apps all --version v1 >/dev/null 2>&1; rc=$?
check "refuses an unresolvable digest, all or nothing" '[ $rc != 0 ] && [ "$(tag $R/infra/helm/vuln-api/values-staging.yaml)" = old ]'

# 5. nothing to do
R=$(new_repo); DEPLOY_ROOT=$R "$bump" --env staging --apps all --version same >/dev/null 2>&1
out=$(DEPLOY_ROOT=$R "$bump" --env staging --apps all --version same 2>/dev/null)
check "no change prints no files" '[ -z "$out" ]'

# 6. summary for the PR body
R=$(new_repo); S=$(mktemp); DEPLOY_SUMMARY=$S DEPLOY_ROOT=$R "$bump" --env staging --apps all --version t9 >/dev/null 2>&1
check "summary lists each app with tag and digest" 'grep -q "vuln-api.*t9.*sha256:" $S && grep -q "vuln-web.*t9.*sha256:" $S'
exit $fail
