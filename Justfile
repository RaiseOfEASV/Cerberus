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
    for chart in infra/helm/*/; do
        [ -f "${chart}Chart.yaml" ] && helm lint "$chart"
    done
