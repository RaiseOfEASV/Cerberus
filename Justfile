# Root task runner. Keeps CI language-agnostic: pr-validate.yml calls `just`
# targets, not go/npm/etc directly, so adding a new app in a new language never
# requires touching the pipeline definitions.
#
# Convention: every directory under apps/ has its own Justfile exposing at
# least `build`, `test`, and `lint`. These root recipes fan out to each app
# that has changed (path-filtered in CI) or to all apps (locally).

default:
    @just --list

# --- fan-out across apps ---

build-all:
    #!/usr/bin/env bash
    set -euo pipefail
    for app in apps/*/; do
        [ -f "${app}Justfile" ] && (cd "$app" && just build)
    done

test-all:
    #!/usr/bin/env bash
    set -euo pipefail
    for app in apps/*/; do
        [ -f "${app}Justfile" ] && (cd "$app" && just test)
    done

lint-all:
    #!/usr/bin/env bash
    set -euo pipefail
    for app in apps/*/; do
        [ -f "${app}Justfile" ] && (cd "$app" && just lint)
    done

# --- infra ---

tf-fmt-check:
    terraform fmt -check -recursive infra/terraform

tf-validate:
    #!/usr/bin/env bash
    set -euo pipefail
    for dir in infra/terraform/*/; do
        [ -f "${dir}main.tf" ] && (cd "$dir" && terraform init -backend=false && terraform validate)
    done

helm-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    # Charts live under infra/helm/charts/ (the old infra/helm/*/ glob only
    # matched per-app values directories, so it linted nothing).
    for chart in infra/helm/charts/*/; do
        if [ -f "${chart}tests/values-test.yaml" ]; then helm lint "$chart" -f "${chart}tests/values-test.yaml"; else helm lint "$chart"; fi
        if [ -x "${chart}tests/render-test.sh" ]; then "${chart}tests/render-test.sh"; fi
    done
