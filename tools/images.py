#!/usr/bin/env python3
"""Keep every third-party image reference in the repo in line with docker/images.yaml.

    tools/images.py check            fail on references that disagree with the catalog
    tools/images.py sync             rewrite them (and the generated scan list) to match
    tools/images.py list             "asset ref" for every scannable catalog image
    tools/images.py changed --base R the same, only entries that differ from revision R

Design: docs/superpowers/specs/2026-11-21-image-catalog-design.md
"""
from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

import yaml

# IMAGES_ROOT points the tool at another checkout (tools/tests/images-test.sh).
ROOT = Path(os.environ.get("IMAGES_ROOT") or Path(__file__).resolve().parent.parent)
CATALOG = "docker/images.yaml"
SCAN_LIST = "infra/helm/vuln-scanner/scan-images.yaml"
OWN = "ghcr.io/raiseofeasv/"
SYNCED_SOURCES = {"direct", "vm", "build"}

# Files that mention images but aren't references to manage.
SKIP_PATHS = (
    CATALOG,
    SCAN_LIST,
    "infra/k8s/kyverno/policies/",  # admission patterns like "!*:latest"
    "infra/helm/charts/app/tests/",  # chart test fixtures
    ".github/workflows/",  # matrices like `image: [go, node]`
    "docs/",
)
REF_RE = r"[A-Za-z0-9][A-Za-z0-9._/-]*(?::[A-Za-z0-9._-]+)?(?:@sha256:[a-f0-9]{64})?"
DIGEST_RE = re.compile(r"^sha256:[a-f0-9]{64}$")


def normalize(repo: str) -> str:
    """Compare repositories the way registries do: docker.io/ and library/ are implicit."""
    if repo.startswith("docker.io/"):
        repo = repo[len("docker.io/"):]
    if repo.startswith("library/"):
        repo = repo[len("library/"):]
    return repo


def split_ref(ref: str) -> tuple[str, str, str]:
    """'repo:tag@sha256:x' -> (repo, tag, digest); tag/digest may be ''."""
    digest = ""
    if "@" in ref:
        ref, digest = ref.split("@", 1)
    tag = ""
    slash = ref.rfind("/")
    colon = ref.rfind(":")
    if colon > slash:
        ref, tag = ref[:colon], ref[colon + 1:]
    return ref, tag, digest


def join_ref(repo: str, tag: str, digest: str) -> str:
    out = repo + (f":{tag}" if tag else "")
    return out + (f"@{digest}" if digest else "")


@dataclass
class Entry:
    key: str
    image: str
    tag: str
    digest: str
    source: str
    status: str

    def ref(self, as_written: str | None = None) -> str:
        return join_ref(as_written or self.image, self.tag, self.digest)

    @property
    def pin(self) -> str:
        """The tag field for Helm values that split repository and tag."""
        return self.tag + (f"@{self.digest}" if self.digest else "")


def load_catalog(text: str | None = None) -> tuple[dict[str, Entry], dict]:
    data = yaml.safe_load(text if text is not None else (ROOT / CATALOG).read_text())
    entries: dict[str, Entry] = {}
    seen: dict[str, str] = {}
    problems = []
    for key, v in (data.get("images") or {}).items():
        e = Entry(key, str(v["image"]), str(v.get("tag") or ""), str(v.get("digest") or ""),
                  str(v.get("source", "direct")), str(v.get("status", "active")))
        if e.digest and not DIGEST_RE.match(e.digest):
            problems.append(f"{CATALOG}: {key}: digest must look like sha256:<64 hex>")
        if not e.tag and not e.digest:
            problems.append(f"{CATALOG}: {key}: needs a tag, a digest or both")
        n = normalize(e.image)
        if n in seen:
            problems.append(f"{CATALOG}: {key} and {seen[n]} are the same image")
        seen[n] = key
        entries[key] = e
    if problems:
        sys.exit("\n".join(problems))
    return entries, data


def by_repo(entries: dict[str, Entry]) -> dict[str, Entry]:
    return {normalize(e.image): e for e in entries.values()}


@dataclass
class Ref:
    path: str
    line: int  # 1-based
    start: int  # column span of the text to replace within the line
    end: int
    text: str
    kind: str  # image | pair-tag | helm


def tracked_files() -> list[str]:
    out = subprocess.run(["git", "ls-files"], cwd=ROOT, check=True, capture_output=True, text=True).stdout
    files = []
    for p in out.splitlines():
        if any(p == s or p.startswith(s) for s in SKIP_PATHS) or "node_modules/" in p:
            continue
        name = Path(p).name
        if name.startswith("Dockerfile") or p.endswith((".yaml", ".yml", ".sh", ".md")):
            files.append(p)
    return files


def find_refs(path: str, text: str, repos: dict[str, Entry], charts: dict) -> list[Ref]:
    lines = text.splitlines()
    refs: list[Ref] = []
    name = Path(path).name
    if name.startswith("Dockerfile"):
        stages: set[str] = set()
        for i, line in enumerate(lines, 1):
            m = re.match(r"^\s*FROM\s+(?:--platform=\S+\s+)?(" + REF_RE + r")(?:\s+AS\s+(\S+))?", line, re.I)
            if not m:
                continue
            if m.group(1) not in stages and m.group(1) != "scratch":
                refs.append(Ref(path, i, m.start(1), m.end(1), m.group(1), "image"))
            if m.group(2):
                stages.add(m.group(2))
    elif path.endswith((".yaml", ".yml")):
        for i, line in enumerate(lines, 1):
            m = re.match(r"""^(\s*(?:-\s+)?[A-Za-z]*[Ii]mage:\s*)(["']?)(""" + REF_RE + r""")\2\s*(?:#.*)?$""", line)
            if m and m.group(3):
                refs.append(Ref(path, i, m.start(3), m.end(3), m.group(3), "image"))
                continue
            # Helm values that split the reference: repository: X / tag: Y
            m = re.match(r"""^(\s*)repository:\s*["']?([^"'\s#]+)["']?""", line)
            if m and m.group(2):
                indent = m.group(1)
                for j in range(i, min(i + 4, len(lines))):
                    t = re.match(r"^" + re.escape(indent) + r"""tag:\s*(["']?)([^"'\s#]*)\1""", lines[j])
                    if t:
                        repo = m.group(2)
                        refs.append(Ref(path, j + 1, t.start(2), t.end(2), f"{repo}|{t.group(2)}", "pair-tag"))
                        break
    elif path.endswith(".sh"):
        # Any repo:tag token whose repository is in the catalog (shell has no
        # image: key to anchor on, so unknown tokens are left alone).
        token = re.compile(r"(?<![\w./:-])((?:[a-z0-9.-]+/)*[a-z0-9._-]+:[A-Za-z0-9._-]+(?:@sha256:[a-f0-9]{64})?)(?![\w.@/-])")
        for i, line in enumerate(lines, 1):
            for m in token.finditer(line):
                if normalize(split_ref(m.group(1))[0]) in repos:
                    refs.append(Ref(path, i, m.start(1), m.end(1), m.group(1), "image"))
    elif path.endswith(".md"):
        for i, line in enumerate(lines, 1):
            for m in re.finditer(r"helm (?:upgrade --install|install) \S+ (\S+/\S+)[^`\n]*", line):
                refs.append(Ref(path, i, m.start(), m.end(), m.group(0), "helm"))
    return [r for r in refs if not ("{{" in r.text or "$" in r.text)]


def expected(ref: Ref, repos: dict[str, Entry], charts: dict) -> tuple[str | None, str | None]:
    """(replacement text or None if fine, problem message or None)."""
    if ref.kind == "helm":
        m = re.match(r"helm (?:upgrade --install|install) \S+ (\S+/\S+)", ref.text)
        chart_ref = m.group(1)
        chart = next((c for c in charts.values() if c["chart"] == chart_ref), None)
        if chart is None:
            return None, f"chart {chart_ref} is not in {CATALOG} (charts:)"
        want = str(chart["version"])
        v = re.search(r"--version (\S+)", ref.text)
        if v and v.group(1) == want:
            return None, None
        if v:
            new = ref.text[:v.start(1)] + want + ref.text[v.end(1):]
        else:
            new = ref.text.replace(chart_ref, f"{chart_ref} --version {want}", 1)
        return new, f"helm install {chart_ref} should use --version {want}"
    if ref.kind == "pair-tag":
        repo, tag = ref.text.split("|", 1)
        if repo.startswith(OWN) or not repo:
            return None, None
        e = repos.get(normalize(repo))
        if e is None:
            return None, f"{repo} is not in {CATALOG}"
        if tag == e.pin:
            return None, None
        return e.pin, f"{repo} tag {tag!r} -> catalog says {e.pin!r}"
    repo, tag, digest = split_ref(ref.text)
    if repo.startswith(OWN):
        return None, None
    e = repos.get(normalize(repo))
    if e is None:
        return None, f"{ref.text} is not in {CATALOG}"
    want = e.ref(repo)
    if ref.text == want:
        return None, None
    return want, f"{ref.text} -> catalog says {want}"


def scan_list(entries: dict[str, Entry]) -> list[tuple[str, str]]:
    return [(normalize(e.image), e.ref()) for e in entries.values() if e.digest]


def render_scan_values(entries: dict[str, Entry]) -> str:
    body = "".join(f"    {asset} {ref}\n" for asset, ref in scan_list(entries))
    return (
        "# Generated by `just images-sync` from docker/images.yaml - do not edit.\n"
        "# The prod vuln-scanner scans these every night (IMAGES_FILE) and files the\n"
        "# results in Cerberoast under the image's repository as the asset.\n"
        "files:\n"
        "  images.txt: |\n" + body
    )


def run(fix: bool) -> int:
    entries, data = load_catalog()
    repos = by_repo(entries)
    charts = data.get("charts") or {}
    problems: list[str] = []
    changed_files = set()
    for path in tracked_files():
        p = ROOT / path
        text = p.read_text()
        lines = text.splitlines(keepends=True)
        edits: dict[int, list[tuple[int, int, str]]] = {}
        for ref in find_refs(path, text, repos, charts):
            new, problem = expected(ref, repos, charts)
            if problem:
                problems.append(f"{path}:{ref.line}: {problem}")
            if new is not None:
                edits.setdefault(ref.line, []).append((ref.start, ref.end, new))
        if fix and edits:
            for ln, spans in edits.items():
                line = lines[ln - 1]
                for start, end, new in sorted(spans, reverse=True):
                    line = line[:start] + new + line[end:]
                lines[ln - 1] = line
            p.write_text("".join(lines))
            changed_files.add(path)
    want = render_scan_values(entries)
    scan_file = ROOT / SCAN_LIST
    if not scan_file.exists() or scan_file.read_text() != want:
        problems.append(f"{SCAN_LIST}: out of date")
        if fix:
            scan_file.write_text(want)
            changed_files.add(SCAN_LIST)
    if fix:
        unfixable = [p for p in problems if "is not in" in p]
        for f in sorted(changed_files):
            print(f"updated {f}")
        if unfixable:
            print("\nAdd these to docker/images.yaml first:\n" + "\n".join(unfixable), file=sys.stderr)
            return 1
        return 0
    if problems:
        print("\n".join(f"FAIL {p}" for p in problems))
        print(f"\n{len(problems)} image reference(s) out of line with {CATALOG}. Fix with: just images-sync")
        return 1
    print(f"ok - every image reference matches {CATALOG} ({len(entries)} images)")
    return 0


def changed(base: str) -> list[tuple[str, str]]:
    entries, _ = load_catalog()
    try:
        old_text = subprocess.run(["git", "show", f"{base}:{CATALOG}"], cwd=ROOT, check=True,
                                  capture_output=True, text=True).stdout
        old, _ = load_catalog(old_text)
    except subprocess.CalledProcessError:
        # No catalog at the base revision: this change introduces it, which
        # records what already runs rather than bumping anything. The nightly
        # scan covers those images.
        return []
    out = []
    for key, e in entries.items():
        o = old.get(key)
        if e.digest and (o is None or (o.image, o.tag, o.digest) != (e.image, e.tag, e.digest)):
            out.append((normalize(e.image), e.ref()))
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check")
    sub.add_parser("sync")
    sub.add_parser("list")
    c = sub.add_parser("changed")
    c.add_argument("--base", required=True)
    a = ap.parse_args()
    if a.cmd in ("check", "sync"):
        return run(fix=a.cmd == "sync")
    rows = scan_list(load_catalog()[0]) if a.cmd == "list" else changed(a.base)
    for asset, ref in rows:
        print(f"{asset} {ref}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
