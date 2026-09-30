#!/usr/bin/env python3
"""Fail when a YAML file under deploy/base is not reachable from any kustomization.

Orphaned manifests drift silently from the live copies (they still get edited
but nothing deploys them), so every deploy/base YAML must be referenced, directly
or through a referenced directory, by a kustomization.yaml under deploy/.
"""

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEPLOY = ROOT / "deploy"
BASE = DEPLOY / "base"
KUSTOMIZATION_NAMES = ("kustomization.yaml", "kustomization.yml", "Kustomization")
# Any scalar that looks like a relative path: resources, components, patches
# (path:), configMapGenerator files (key=path or bare path), etc.
PATH_TOKEN = re.compile(r"""^\s*-?\s*(?:[\w.-]+:\s*)?(?:[\w.-]+=)?['"]?([./\w-][^\s'"#]*)['"]?\s*(?:#.*)?$""")


def kustomization_files() -> list[Path]:
    return [p for p in DEPLOY.rglob("*") if p.name in KUSTOMIZATION_NAMES]


def referenced_paths(kfile: Path) -> set[Path]:
    found: set[Path] = set()
    for line in kfile.read_text(encoding="utf-8").splitlines():
        match = PATH_TOKEN.match(line)
        if not match:
            continue
        token = match.group(1)
        if "://" in token or token.startswith("github.com"):
            continue
        candidate = (kfile.parent / token).resolve()
        if candidate.exists():
            found.add(candidate)
    return found


def main() -> int:
    reachable: set[Path] = set()
    queue = kustomization_files()
    reachable.update(k.resolve() for k in queue)
    while queue:
        kfile = queue.pop()
        for path in referenced_paths(kfile):
            if path.is_dir():
                # A referenced directory contributes only through its own kustomization.
                for name in KUSTOMIZATION_NAMES:
                    nested = path / name
                    if nested.exists() and nested.resolve() not in reachable:
                        reachable.add(nested.resolve())
                        queue.append(nested)
            else:
                reachable.add(path)

    orphans = sorted(
        p.relative_to(ROOT)
        for p in BASE.rglob("*.y*ml")
        if p.resolve() not in reachable
    )
    if orphans:
        print("Orphaned manifests under deploy/base (not referenced by any kustomization):")
        for orphan in orphans:
            print(f"  {orphan}")
        print("Reference them from a kustomization.yaml or delete them.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
