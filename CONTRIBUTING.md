# Contributing

## One-time dev environment setup

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
