# Contributing

## One-time dev environment setup

### Commit signing (required)

`master` only accepts **signed, verified** commits (the `main-protection`
ruleset, `infra/terraform/github/`). SSH signing reuses a key you already have:

```bash
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global commit.gpgsign true
```

Then add the same public key on GitHub → Settings → SSH and GPG keys → New
SSH key, **Key type: Signing Key**. Squash-merges done through GitHub are
signed by GitHub itself, so PR merges always pass this rule.

### Secret scanning (required)

Every PR is gated by a GitGuardian scan in CI
(`.github/workflows/pr-validate.yml`), but that only catches a leaked secret
*after* it's already in your branch's history. Install the local pre-commit
hook so it's caught before you ever commit:

```bash
pip install pre-commit ggshield
ggshield auth login          # or: export GITGUARDIAN_API_KEY=<your personal token>
pre-commit install
```

From then on, `git commit` runs `ggshield` against your staged changes
automatically. If it flags something, fix it before committing - don't reach
for `--no-verify` to skip past a real finding. If it's a false positive,
raise it instead of silently bypassing the hook; `ggshield` supports an
ignore list (`.gitguardian.yaml`) for recurring ones.

This hook is a courtesy layer, not the enforcement point - the CI scan in
`pr-validate.yml` is what actually blocks a PR. Having the hook just means
you find out locally instead of waiting on CI.

See `.pre-commit-config.yaml` for the hook config and `SECURITY.md` for how
to report a vulnerability you find in the codebase itself.

## Working on master

`master` is protected - nobody can push to it directly:

- **Every change is a pull request.** It needs one approval from a code owner
  other than the author (`.github/CODEOWNERS`: Ju5tAmir, andriiVIt,
  grosuionutandrei).
- **Repo admins can bypass the review when merging** ("Merge without waiting
  for requirements to be met"). Use it deliberately - it's recorded in the
  repo's rule insights.
- **Merge with squash** (linear history is required).
- **Credit co-authors** with trailers in the commit/PR message - all three
  team members are credited on every commit in this repo:

  ```
  Co-authored-by: Andrii Savchenko <109040480+andriiVIt@users.noreply.github.com>
  Co-authored-by: Ionut Andrei Grosu <80783004+grosuionutandrei@users.noreply.github.com>
  ```

- **CI on every PR** (`pr-validate.yml`): `lint` (`just lint-all` + the
  generic chart's render tests), `sast` (CodeQL + Semgrep), `sca` (Trivy
  vulnerabilities, IaC misconfiguration, licenses - accepted risks live in
  `.trivyignore.yaml` with a reason each), `secret-scan` (GitGuardian).

Deploying is also a PR: see `infra/k8s/environments/README.md`.
