# Incident Response Runbook

Filled in now that Observability & SIEM (Epic 7) and Backup/DR (Epic 9) both exist -
this used to be a bare skeleton waiting on exactly those two epics.

## Escalation path

Every alert source in this project converges on one of two places:

- **Alertmanager** (Prometheus-based: node down, disk full, backup job failed, a
  Velero backup/restore failing, etc.) -> Slack `#cerberus-alerts`, via
  `infra/k8s/monitoring/alertmanager-config.yaml`. This channel is the real paging
  mechanism right now - there's no PagerDuty/phone escalation configured, so
  response time is bounded by someone actually watching that channel. That's a real
  limitation for anything Sev1 overnight, not a solved problem - revisit if/when
  this needs genuine 24/7 on-call.
- **Wazuh** (security events: Falco alerts via Falcosidekick's syslog bridge,
  host-level detections, the GitHub audit log forwarder) -> the Wazuh dashboard
  first. Wazuh has its own native Slack/email integration support
  (`ossec.conf` `<integration>` blocks) that could point at the same
  `#cerberus-alerts` channel or a dedicated one - **not configured here**, since it
  requires tuning against a live manager instance to avoid alert-fatigue-inducing
  noise. Documented as a manual follow-up step, not automated blindly.

Whoever sees the alert first (in Slack or by checking the Wazuh dashboard) is the
de facto first responder until a real on-call rotation exists - there's no handoff
schedule, which is itself worth revisiting once this is more than a solo/small-team
project.

## Severity levels

- **Sev1** - active compromise, data exposure, or total outage of something
  customer-facing (once real services exist - currently nothing is, so this baseline
  itself being broken, e.g. CI entirely down or Vault unrecoverable, counts as Sev1).
  Respond immediately, interrupt whatever else is happening.
- **Sev2** - a security control has failed or is bypassable (a Kyverno policy not
  enforcing, a secret-scan gap, TLS cert expired) but nothing confirmed exploited
  yet. Respond same day.
- **Sev3** - everything else worth a ticket but not urgent: a failed (not missed)
  backup job, a Lynis score regression, a Dependency-Track finding on a non-critical
  dependency. Respond within the week.

## During an incident

- Whoever responds first is incident commander (IC) until they explicitly hand it
  off - say so out loud/in-channel, don't let it be ambiguous.
- Use the Slack channel the alert arrived in as the shared incident channel - don't
  spin up a separate one by default, that just fragments context during the parts
  of an incident where speed matters most.
- Loop in anyone whose area is affected as soon as it's clear it's affected, not
  after the fact.

If the incident involves Vault being sealed, unreachable, or needing recovery, see
[`vault-unseal-procedure.md`](vault-unseal-procedure.md) rather than improvising -
that document exists specifically so this isn't designed from scratch under
pressure.

If the incident involves Keycloak or oauth2-proxy being down and nobody being
able to log into Vault's or ArgoCD's UI as a result (ArgoCD's local admin
account is disabled on purpose - there's no password fallback sitting next to
the Keycloak one), see [`sso-break-glass.md`](sso-break-glass.md).

If the incident involves restoring from a backup, see
[`restore-drills.md`](restore-drills.md) for the mechanics of each backup type - and
if this is the first time actually doing it for real rather than as a drill, that's
itself worth a line in the post-mortem below.

## After an incident

- Write a post-mortem: what happened, timeline, root cause, what's changing so it
  doesn't happen the same way twice. Blameless - the point is the system, not the
  person who happened to be looking at it.
- If the incident revealed a gap the threat model didn't anticipate, update
  [`threat-model.md`](threat-model.md) as part of the post-mortem follow-up, not
  as a separate forgotten task.
