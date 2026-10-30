#!/usr/bin/env python3
"""Immutable-migration-body gate (SDLC-CONF-10).

A gormigrate migration ID is recorded as applied exactly once per environment.
Changing a migration's BODY while keeping its ID therefore only affects fresh
databases; every environment that already recorded the ID skips the corrected
body. That is the exact defect in issue #862: #853 rewrote the ci_providers /
ci_trusts bodies (AutoMigrate -> raw CreateTable) under their already-shipped
IDs, so upgraded hubs never ran the fix and kept serving 500s.

Corrections must therefore ship as a NEW migration ID (a convergence migration),
never as an edit to an already-shipped body. This gate enforces that: it hashes
the normalized source of every `&gormigrate.Migration{...}` literal under
components/api-server/plugins, keyed by migration ID, and compares each hash to
the committed lock file. A changed hash under an existing ID fails the build and
tells the author to ship a new ID instead.

The lock is regenerated with `--update` (make update-migration-lock) when a new
migration is added or an unshipped one is removed. Regenerating it to absorb a
changed body under an existing ID is a reviewable lock-file diff that this gate
deliberately makes visible: never do it; ship a new ID.
"""

import hashlib
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLUGINS = ROOT / "components" / "api-server" / "plugins"
LOCK = ROOT / "components" / "api-server" / "migration-bodies.lock"

MIGRATION_LITERAL = "&gormigrate.Migration{"
ID_RE = re.compile(r'ID:\s*"([0-9A-Za-z_]+)"')
LINE_COMMENT_RE = re.compile(r"//.*")
WS_RE = re.compile(r"\s+")


def migration_files() -> list[Path]:
    return sorted(p for p in PLUGINS.rglob("*migration*.go") if not p.name.endswith("_test.go"))


def brace_match(text: str, open_idx: int) -> int:
    """Return the index just past the brace that matches the one at open_idx."""
    depth = 0
    i = open_idx
    n = len(text)
    while i < n:
        c = text[i]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    raise ValueError("unbalanced braces in migration literal")


def normalize(body: str) -> str:
    """Strip line comments and collapse whitespace so reformatting and comment
    edits do not trip the gate, but any functional change to the body does."""
    stripped = LINE_COMMENT_RE.sub("", body)
    return WS_RE.sub(" ", stripped).strip()


def extract(text: str) -> list[tuple[str, str]]:
    """Return (id, normalized-body) for every migration literal in the source."""
    results: list[tuple[str, str]] = []
    start = 0
    while True:
        idx = text.find(MIGRATION_LITERAL, start)
        if idx == -1:
            break
        open_brace = text.index("{", idx)
        end = brace_match(text, open_brace)
        body = text[idx:end]
        m = ID_RE.search(body)
        if not m:
            raise ValueError(f"migration literal without an ID literal near offset {idx}")
        results.append((m.group(1), normalize(body)))
        start = end
    return results


def current_hashes() -> dict[str, str]:
    hashes: dict[str, str] = {}
    duplicates: list[str] = []
    for path in migration_files():
        for mig_id, body in extract(path.read_text(encoding="utf-8")):
            digest = hashlib.sha256(body.encode("utf-8")).hexdigest()
            if mig_id in hashes and hashes[mig_id] != digest:
                duplicates.append(mig_id)
            hashes[mig_id] = digest
    if duplicates:
        raise ValueError(
            "duplicate migration IDs with differing bodies: " + ", ".join(sorted(set(duplicates)))
        )
    return hashes


def load_lock() -> dict[str, str]:
    if not LOCK.exists():
        return {}
    lock: dict[str, str] = {}
    for line in LOCK.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        mig_id, digest = line.split()
        lock[mig_id] = digest
    return lock


def write_lock(hashes: dict[str, str]) -> None:
    lines = [
        "# Migration body hashes (SDLC-CONF-10). Regenerate with"
        " `make update-migration-lock`.",
        "# A changed hash under an existing ID means a shipped body was edited:"
        " ship a NEW ID instead.",
    ]
    lines.extend(f"{mig_id} {hashes[mig_id]}" for mig_id in sorted(hashes))
    LOCK.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main(argv: list[str]) -> int:
    current = current_hashes()

    if "--update" in argv:
        write_lock(current)
        print(f"wrote {LOCK.relative_to(ROOT)} ({len(current)} migrations)")
        return 0

    lock = load_lock()
    errors: list[str] = []

    for mig_id, digest in sorted(current.items()):
        if mig_id not in lock:
            errors.append(
                f"migration {mig_id}: not recorded in the lock; run"
                " `make update-migration-lock` and commit the result"
            )
        elif lock[mig_id] != digest:
            errors.append(
                f"migration {mig_id}: body changed under an already-shipped ID"
                " (SDLC-CONF-10). A changed body is skipped on every environment"
                " that already recorded this ID. Ship the correction as a NEW"
                " migration ID (a convergence migration), do not edit this body."
            )
    for mig_id in sorted(lock):
        if mig_id not in current:
            errors.append(
                f"migration {mig_id}: recorded in the lock but no longer present"
                " in the source; if this migration was intentionally removed, run"
                " `make update-migration-lock` and commit the result"
            )

    if errors:
        print("Immutable-migration-body gate failed (SDLC-CONF-10):", file=sys.stderr)
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        return 1

    print(f"migration bodies OK ({len(current)} migrations)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
