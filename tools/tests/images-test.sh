#!/usr/bin/env bash
# Tests for tools/images.py against a throwaway git repo. Run from anywhere.
set -uo pipefail
tool="$(cd "$(dirname "$0")/.." && pwd)/images.py"
fail=0
check() { if eval "$2"; then echo "ok   - $1"; else echo "FAIL - $1"; fail=1; fi; }
D1=sha256:$(printf 'a%.0s' {1..64}); D2=sha256:$(printf 'b%.0s' {1..64})

new_repo() {
  R=$(mktemp -d)
  mkdir -p "$R/docker" "$R/infra/k8s/db" "$R/infra/helm/values" "$R/infra/helm/vuln-scanner" "$R/apps/web" "$R/scripts"
  cat > "$R/docker/images.yaml" <<YAML
images:
  postgres: { image: postgres, tag: 16-alpine, digest: $D1, source: direct, status: active }
  vault: { image: hashicorp/vault, tag: "1.17", digest: $D1, source: direct, status: active }
  nginx: { image: nginxinc/nginx-unprivileged, tag: 1.30-alpine, digest: $D1, source: build, status: active }
  agent: { image: example/agent, tag: 4.9.0, digest: "", source: direct, status: inactive }
  keycloak: { image: quay.io/keycloak/keycloak, tag: 26.8.1, digest: $D1, source: direct, status: active }
charts:
  kyverno: { chart: kyverno/kyverno, version: 3.9.2, images: [] }
YAML
  printf 'spec:\n  containers:\n    - name: db\n      image: postgres:16-alpine\n    - name: agent\n      image: example/agent:4.9.0\n    - name: ours\n      image: ghcr.io/raiseofeasv/vuln-api:abc\n' > "$R/infra/k8s/db/db.yaml"
  printf 'server:\n  image:\n    repository: hashicorp/vault\n    tag: "1.17"\n' > "$R/infra/helm/values/vault.yaml"
  # Chart with its own digest field (keycloakx): tag must stay a plain version,
  # it ends up in a label.
  printf 'image:\n  repository: quay.io/keycloak/keycloak\n  tag: "26.8.0"\n  digest: ""\n' > "$R/infra/helm/values/kc.yaml"
  printf 'FROM nginxinc/nginx-unprivileged:1.30-alpine AS base\nFROM base\n' > "$R/apps/web/Dockerfile"
  printf 'docker run --rm docker.io/library/postgres:16-alpine true\n' > "$R/scripts/db.sh"
  printf '1. `helm install kyverno kyverno/kyverno -n kyverno`\n' > "$R/README.md"
  (cd "$R" && git init -q && git add -A && git -c user.email=t@t -c user.name=t commit -qm init)
  echo "$R"
}

R=$(new_repo)
out=$(IMAGES_ROOT=$R python3 "$tool" check 2>&1); rc=$?
check "check fails on a fresh repo" '[ $rc = 1 ]'
check "loose tag in a manifest" 'grep -q "db.yaml:4: postgres:16-alpine -> catalog says postgres:16-alpine@$D1" <<<"$out"'
check "split repository/tag in Helm values" 'grep -q "vault.yaml:4: hashicorp/vault tag" <<<"$out"'
check "split tag + digest: tag is checked against the plain version" 'grep -qE "kc.yaml:3: quay.io/keycloak/keycloak tag .26.8.0. -> catalog says .26.8.1.$" <<<"$out"'
check "split tag + digest: digest is checked separately" 'grep -q "kc.yaml:4: quay.io/keycloak/keycloak digest" <<<"$out"'
check "Dockerfile FROM, not the stage alias" '[ "$(grep -c "apps/web/Dockerfile" <<<"$out")" = 1 ]'
check "shell script, docker.io/library/ treated as the same image" 'grep -q "scripts/db.sh:1: docker.io/library/postgres:16-alpine" <<<"$out"'
check "helm install without --version" 'grep -q "README.md:1: helm install kyverno/kyverno should use --version 3.9.2" <<<"$out"'
check "inactive image without digest matches its tag" '! grep -q "example/agent" <<<"$out"'
check "our own images are ignored" '! grep -q raiseofeasv <<<"$out"'
check "stale scan list reported" 'grep -q "scan-images.yaml: out of date" <<<"$out"'

IMAGES_ROOT=$R python3 "$tool" sync >/dev/null 2>&1
out=$(IMAGES_ROOT=$R python3 "$tool" check 2>&1); rc=$?
check "check passes after sync" '[ $rc = 0 ]'
check "sync pins by digest" 'grep -q "image: postgres:16-alpine@$D1" $R/infra/k8s/db/db.yaml'
check "sync keeps the repository as written" 'grep -q "docker.io/library/postgres:16-alpine@$D1" $R/scripts/db.sh'
check "sync writes tag@digest into split Helm values" 'grep -q "tag: \"1.17@$D1\"" $R/infra/helm/values/vault.yaml'
check "sync keeps a plain tag where the values have a digest key" 'grep -q "^  tag: \"26.8.1\"$" $R/infra/helm/values/kc.yaml && grep -q "^  digest: \"$D1\"$" $R/infra/helm/values/kc.yaml'
check "sync adds --version to helm install" 'grep -q "kyverno/kyverno --version 3.9.2 -n kyverno" $R/README.md'
check "scan list skips digest-less entries" '! grep -q example/agent $R/infra/helm/vuln-scanner/scan-images.yaml && grep -q "^    postgres postgres:16-alpine@$D1" $R/infra/helm/vuln-scanner/scan-images.yaml'

(cd "$R" && git add -A && git -c user.email=t@t -c user.name=t commit -qm synced)
sed -i "s/vault, tag: \"1.17\", digest: $D1/vault, tag: \"1.18\", digest: $D2/" "$R/docker/images.yaml"
out=$(IMAGES_ROOT=$R python3 "$tool" changed --base HEAD 2>&1)
check "changed lists only the bumped entry" '[ "$out" = "hashicorp/vault hashicorp/vault:1.18@$D2" ]'
out=$(IMAGES_ROOT=$R python3 "$tool" changed --base HEAD --with-old 2>&1)
check "changed --with-old adds the previous ref" '[ "$out" = "hashicorp/vault hashicorp/vault:1.18@$D2 hashicorp/vault:1.17@$D1" ]'
out=$(IMAGES_ROOT=$R python3 "$tool" check 2>&1); rc=$?
check "a catalog bump makes check fail until synced" '[ $rc = 1 ] && grep -q "catalog says .1.18@$D2" <<<"$out"'

printf '      image: busybox:1.36\n' >> "$R/infra/k8s/db/db.yaml"
out=$(IMAGES_ROOT=$R python3 "$tool" sync 2>&1); rc=$?
check "sync refuses images missing from the catalog" '[ $rc = 1 ] && grep -q "busybox:1.36 is not in docker/images.yaml" <<<"$out"'

rm -rf "$R"
exit $fail
