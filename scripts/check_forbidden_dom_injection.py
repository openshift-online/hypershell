#!/usr/bin/env python3
"""Reject raw HTML injection sinks in browser source files."""

from __future__ import annotations

import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent

SOURCE_SUFFIXES = frozenset(
    {
        ".js",
        ".jsx",
        ".mjs",
        ".cjs",
        ".ts",
        ".tsx",
        ".mts",
        ".cts",
        ".html",
        ".htm",
        ".vue",
        ".svelte",
        ".astro",
    }
)

FORBIDDEN_PATTERNS = (
    (
        "React dangerouslySetInnerHTML prop",
        re.compile(r"\bdangerouslySetInnerHTML\b"),
    ),
    (
        "DOM innerHTML or outerHTML property",
        re.compile(r"\b(?:innerHTML|outerHTML)\b"),
    ),
    (
        "DOM insertAdjacentHTML method",
        re.compile(r"\binsertAdjacentHTML\b"),
    ),
    (
        "document.write method",
        re.compile(r"\bdocument\s*\.\s*write\s*\("),
    ),
    (
        "DOM createContextualFragment method",
        re.compile(r"\bcreateContextualFragment\s*\("),
    ),
    (
        "jQuery-style html method",
        re.compile(r"\.\s*html\s*\("),
    ),
)


def _candidate_files() -> list[str]:
    result = subprocess.run(
        (
            "git",
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
        ),
        cwd=REPOSITORY_ROOT,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(f"could not list repository source files: {detail}")

    return [
        os.fsdecode(raw_path)
        for raw_path in result.stdout.split(b"\0")
        if raw_path
    ]


def _scannable(relative_path: str) -> bool:
    path = PurePosixPath(relative_path)
    return path.suffix.lower() in SOURCE_SUFFIXES


def _read_text(relative_path: str) -> str | None:
    path = REPOSITORY_ROOT / relative_path
    if not path.is_file():
        return None
    try:
        data = path.read_bytes()
    except OSError as error:
        raise RuntimeError(f"could not read {relative_path}: {error}") from error
    if b"\0" in data:
        return None
    return data.decode("utf-8", errors="replace")


def _find_matches(relative_path: str, content: str) -> dict[tuple[str, int], set[str]]:
    matches: dict[tuple[str, int], set[str]] = {}
    for display_name, pattern in FORBIDDEN_PATTERNS:
        for match in pattern.finditer(content):
            line = content.count("\n", 0, match.start()) + 1
            matches.setdefault((relative_path, line), set()).add(display_name)
    return matches


def _print_matches(matches: dict[tuple[str, int], set[str]]) -> None:
    for (relative_path, line), display_names in sorted(matches.items()):
        for display_name in sorted(display_names):
            print(f"{relative_path}:{line}: forbidden {display_name}")


def main() -> int:
    try:
        matches: dict[tuple[str, int], set[str]] = {}
        for relative_path in _candidate_files():
            if not _scannable(relative_path):
                continue
            content = _read_text(relative_path)
            if content is None:
                continue
            for location, display_names in _find_matches(
                relative_path, content
            ).items():
                matches.setdefault(location, set()).update(display_names)
    except RuntimeError as error:
        print(f"forbidden DOM-injection check error: {error}", file=sys.stderr)
        return 2

    if matches:
        _print_matches(matches)
        print(
            "Raw HTML injection sinks are forbidden. Render React elements or "
            "escape text through an approved safe renderer.",
            file=sys.stderr,
        )
        return 1

    print("No forbidden DOM-injection sinks found.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
