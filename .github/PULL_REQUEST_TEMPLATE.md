## Summary

<!-- What does this change do, and why? -->

## Linked issue

Closes #

<!-- Every PR should reference the Story/Epic issue it's part of. If this PR
doesn't close the issue outright, use "Part of #<n>" instead. -->

## Threat model

- [ ] This change does **not** add a new data flow, trust boundary, or external
      integration - no threat model update needed.
- [ ] This change **does** add one of the above, and
      [`docs/security/threat-model.md`](../docs/security/threat-model.md) has been
      updated in this PR to reflect it.

## Security checklist

- [ ] No secrets, tokens, or credentials in this diff (push protection and
      GitGuardian are backstops, not a substitute for checking).
- [ ] New dependencies are pinned (no floating versions), and Renovate will track
      them going forward.
- [ ] If this touches `infra/k8s/` or `infra/helm/`: resource requests/limits,
      non-root, and read-only root filesystem are set (Kyverno will reject the
      deploy otherwise, but check before pushing).

## Testing

<!-- How was this verified? Include commands run / screenshots / pipeline links. -->
