#!/usr/bin/env bash
# Tests for tools/trivy-diff.py with small Trivy-shaped fixtures.
set -uo pipefail
tool="$(cd "$(dirname "$0")/.." && pwd)/trivy-diff.py"
fail=0
check() { if eval "$2"; then echo "ok   - $1"; else echo "FAIL - $1"; fail=1; fi; }
T=$(mktemp -d)
v() { printf '{"VulnerabilityID":"%s","PkgName":"%s","InstalledVersion":"1.0","FixedVersion":"%s","Severity":"%s"}' "$@"; }
printf '{"SchemaVersion":2,"Results":[{"Vulnerabilities":[%s,%s,%s]}]}' \
  "$(v CVE-2026-0001 openssl 3.0.9 HIGH)" "$(v CVE-2026-0002 zlib '' LOW)" "$(v CVE-2026-0003 tzdata 2026c MEDIUM)" > "$T/old.json"
printf '{"SchemaVersion":2,"Results":[{"Vulnerabilities":[%s,%s]}]}' \
  "$(v CVE-2026-0002 zlib '' LOW)" "$(v CVE-2026-0009 libxml2 2.13.9 MEDIUM)" > "$T/new.json"
printf '{"SchemaVersion":2,"Results":[{"Vulnerabilities":[%s]}]}' "$(v CVE-2026-0666 glibc 2.41-7 CRITICAL)" > "$T/crit.json"
printf '{"SchemaVersion":2,"Results":[{"Vulnerabilities":[%s]}]}' "$(v CVE-2026-0777 glibc '' CRITICAL)" > "$T/crit-nofix.json"

out=$(python3 "$tool" --asset postgres --old "$T/old.json" --new "$T/new.json" --old-ref postgres:16-alpine@sha256:a --new-ref postgres:16-alpine@sha256:b); rc=$?
check "a bump without fixable CRITICAL passes" '[ $rc = 0 ] && grep -q "#### \`postgres\` ✅" <<<"$out"'
check "before/after counts" 'grep -q "| High | 1 | 0 | -1 |" <<<"$out" && grep -q "| Medium | 1 | 1 | - |" <<<"$out"'
check "fixed vulnerabilities listed" 'grep -q "Fixed by this bump: 2" <<<"$out" && grep -q "CVE-2026-0001" <<<"$out"'
check "introduced vulnerabilities listed" 'grep -q "Introduced by this bump: 1" <<<"$out" && grep -q "CVE-2026-0009" <<<"$out"'
check "unchanged ones are in neither list" '! grep -q "CVE-2026-0002" <<<"$out"'

out=$(python3 "$tool" --asset curl --old "$T/old.json" --new "$T/crit.json" --old-ref a --new-ref b); rc=$?
check "a fixable CRITICAL the bump introduces blocks" '[ $rc = 1 ] && grep -q "Blocked:\*\* this bump introduces 1" <<<"$out" && grep -q "❌" <<<"$out"'
out=$(python3 "$tool" --asset nginx --old "$T/crit.json" --new "$T/crit.json" --old-ref a --new-ref b); rc=$?
check "a fixable CRITICAL the old image already had warns but passes" '[ $rc = 0 ] && grep -q "Still present (not blocking):\*\* 1" <<<"$out" && grep -q "⚠️" <<<"$out"'
python3 "$tool" --asset brand-new --new "$T/crit.json" --new-ref b >/dev/null; rc=$?
check "a new catalog entry with a fixable CRITICAL blocks" '[ $rc = 1 ]'
python3 "$tool" --asset curl --new "$T/crit-nofix.json" --new-ref b >/dev/null; rc=$?
check "CRITICAL without a fix does not block" '[ $rc = 0 ]'
out=$(python3 "$tool" --asset newimg --new "$T/new.json" --new-ref img@sha256:b)
check "a new catalog entry gets plain counts" 'grep -q "New image:" <<<"$out" && ! grep -q "Before" <<<"$out"'
rm -rf "$T"
exit $fail
