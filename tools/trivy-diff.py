#!/usr/bin/env python3
"""Compare Trivy reports of an image before and after a version bump.

    tools/trivy-diff.py --asset NAME --new NEW.json [--old OLD.json]
                        --new-ref REF [--old-ref REF]

Prints a markdown section for the PR report and exits 1 when the bump
introduces a CRITICAL vulnerability with a fix available (for a new catalog
entry: any such CRITICAL). Fixable CRITICALs the old image already had are
listed as a warning but don't block - blocking them would keep the worse
image. Used by .github/workflows/images-scan.yml.
"""
from __future__ import annotations

import argparse
import json
import sys

SEVERITIES = ["CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"]
MAX_ROWS = 15


def load(path: str | None) -> dict[tuple[str, str], dict]:
    """(vulnerability id, package) -> vulnerability, for one Trivy JSON report."""
    if not path:
        return {}
    with open(path) as f:
        rep = json.load(f)
    out = {}
    for res in rep.get("Results") or []:
        for v in res.get("Vulnerabilities") or []:
            key = (v.get("VulnerabilityID", ""), v.get("PkgName", ""))
            out.setdefault(key, v)
    return out


def counts(vulns: dict) -> dict[str, int]:
    c = {s: 0 for s in SEVERITIES}
    for v in vulns.values():
        c[v.get("Severity", "UNKNOWN") if v.get("Severity") in c else "UNKNOWN"] += 1
    return c


def rows(vulns: list[dict]) -> list[str]:
    order = {s: i for i, s in enumerate(SEVERITIES)}
    vulns = sorted(vulns, key=lambda v: (order.get(v.get("Severity"), 9), v.get("VulnerabilityID", "")))
    out = []
    for v in vulns[:MAX_ROWS]:
        vid = v.get("VulnerabilityID", "")
        url = v.get("PrimaryURL")
        out.append(f"| {v.get('Severity', 'UNKNOWN')} | {f'[{vid}]({url})' if url else vid} | `{v.get('PkgName', '')}` "
                   f"{v.get('InstalledVersion', '')} | {v.get('FixedVersion') or 'no fix yet'} |")
    if len(vulns) > MAX_ROWS:
        out.append(f"| | ...and {len(vulns) - MAX_ROWS} more | | |")
    return out


def report(asset: str, new_ref: str, old_ref: str | None, new: dict, old: dict) -> tuple[str, bool]:
    fixable_crit = {k: v for k, v in new.items() if v.get("Severity") == "CRITICAL" and v.get("FixedVersion")}
    gate = [v for k, v in fixable_crit.items() if k not in old]
    inherited = [v for k, v in fixable_crit.items() if k in old]
    fixed = [old[k] for k in old.keys() - new.keys()]
    introduced = [new[k] for k in new.keys() - old.keys()]
    nc, oc = counts(new), counts(old)

    md = [f"#### `{asset}` {'❌' if gate else ('⚠️' if inherited else '✅')}", ""]
    md.append(f"`{old_ref}` → `{new_ref}`" if old_ref else f"New image: `{new_ref}`")
    md.append("")
    if old_ref:
        md += ["| Severity | Before | After | Change |", "|---|---:|---:|---:|"]
        for s in SEVERITIES:
            d = nc[s] - oc[s]
            md.append(f"| {s.title()} | {oc[s]} | {nc[s]} | {('+' if d > 0 else '') + str(d) if d else '-'} |")
    else:
        md += ["| Severity | Count |", "|---|---:|"]
        md += [f"| {s.title()} | {nc[s]} |" for s in SEVERITIES]
    md.append("")
    if old_ref:
        md.append(f"**Fixed by this bump: {len(fixed)}**")
        if fixed:
            md += ["", "| Severity | Vulnerability | Package | Fixed in |", "|---|---|---|---|", *rows(fixed)]
        md.append("")
        md.append(f"**Introduced by this bump: {len(introduced)}**")
        if introduced:
            md += ["", "| Severity | Vulnerability | Package | Fixed in |", "|---|---|---|---|", *rows(introduced)]
        md.append("")
    if gate:
        md += [f"**Blocked:** this bump introduces {len(gate)} CRITICAL with a fix available.", "",
               "| Severity | Vulnerability | Package | Fixed in |", "|---|---|---|---|", *rows(gate), ""]
    if inherited:
        md += [f"**Still present (not blocking):** {len(inherited)} CRITICAL with a fix available that the old image "
               "already had - fixed upstream, waiting for a newer image.", "",
               "| Severity | Vulnerability | Package | Fixed in |", "|---|---|---|---|", *rows(inherited), ""]
    return "\n".join(md), bool(gate)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--asset", required=True)
    ap.add_argument("--new", required=True)
    ap.add_argument("--old")
    ap.add_argument("--new-ref", required=True)
    ap.add_argument("--old-ref")
    a = ap.parse_args()
    md, blocked = report(a.asset, a.new_ref, a.old_ref, load(a.new), load(a.old))
    print(md)
    return 1 if blocked else 0


if __name__ == "__main__":
    sys.exit(main())
