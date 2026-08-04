#!/usr/bin/env python3
"""Fail when repository files contain disallowed terminology."""

from __future__ import annotations

import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from typing import Iterable


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
WHITELIST_PATH = REPOSITORY_ROOT / ".forbidden-terms-whitelist.json"
WHITELIST_GUIDANCE = "scripts/README.md#whitelist-guidance"

_TERM_PARTS = (
    ("a", "cp"),
    ("agent", "-control", "-plane"),
    ("amb", "ient", "-code"),
    ("amb", "ient"),
)
_DISPLAY_TERMS = tuple("".join(parts) for parts in _TERM_PARTS)
_PATTERNS = tuple(
    (term, re.compile(re.escape(term), re.IGNORECASE)) for term in _DISPLAY_TERMS
) + (
    (
        " ".join(("agent", "control", "plane")),
        re.compile(r"agent\s+control\s+plane", re.IGNORECASE),
    ),
)


class ConfigurationError(Exception):
    """Raised when the whitelist is invalid."""


def _tracked_files() -> list[str]:
    result = subprocess.run(
        ("git", "ls-files", "-z", "--cached"),
        cwd=REPOSITORY_ROOT,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(f"could not list tracked files: {detail}")

    return [
        os.fsdecode(raw_path)
        for raw_path in result.stdout.split(b"\0")
        if raw_path
    ]


def _read_text(relative_path: str) -> str | None:
    path = REPOSITORY_ROOT / relative_path
    if not path.exists() and not path.is_symlink():
        return None

    if path.is_symlink():
        data = os.fsencode(os.readlink(path))
    else:
        try:
            data = path.read_bytes()
        except OSError as error:
            raise RuntimeError(f"could not read {relative_path}: {error}") from error

    if b"\0" in data:
        return None
    return data.decode("utf-8", errors="replace")


def _find_matches(relative_path: str, content: str) -> dict[tuple[str, int], set[str]]:
    matches: dict[tuple[str, int], set[str]] = {}
    for display_term, pattern in _PATTERNS:
        for match in pattern.finditer(content):
            line = content.count("\n", 0, match.start()) + 1
            matches.setdefault((relative_path, line), set()).add(display_term)
    return matches


def _validate_filename(filename: object, entry_number: int) -> str:
    if not isinstance(filename, str) or not filename:
        raise ConfigurationError(
            f"whitelist entry {entry_number}: filename must be a non-empty string"
        )

    path = PurePosixPath(filename)
    if path.is_absolute() or ".." in path.parts or str(path) != filename:
        raise ConfigurationError(
            f"whitelist entry {entry_number}: filename must be a normalized, "
            "repository-relative path"
        )
    return filename


def _load_whitelist() -> dict[tuple[str, int], str]:
    try:
        raw_entries = json.loads(WHITELIST_PATH.read_text(encoding="utf-8"))
    except FileNotFoundError as error:
        raise ConfigurationError(
            f"required whitelist file is missing: {WHITELIST_PATH.name}"
        ) from error
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise ConfigurationError(
            f"could not parse {WHITELIST_PATH.name}: {error}"
        ) from error

    if not isinstance(raw_entries, list):
        raise ConfigurationError("whitelist must be a JSON array")

    whitelist: dict[tuple[str, int], str] = {}
    required_fields = {"filename", "line", "rationale"}
    for index, entry in enumerate(raw_entries, start=1):
        if not isinstance(entry, dict):
            raise ConfigurationError(f"whitelist entry {index}: must be an object")
        if set(entry) != required_fields:
            raise ConfigurationError(
                f"whitelist entry {index}: fields must be exactly filename, line, "
                "and rationale"
            )

        filename = _validate_filename(entry["filename"], index)
        line = entry["line"]
        rationale = entry["rationale"]
        if isinstance(line, bool) or not isinstance(line, int) or line < 1:
            raise ConfigurationError(
                f"whitelist entry {index}: line must be a positive integer"
            )
        if not isinstance(rationale, str) or not rationale.strip():
            raise ConfigurationError(
                f"whitelist entry {index}: rationale must be a non-empty string"
            )

        location = (filename, line)
        if location in whitelist:
            raise ConfigurationError(
                f"whitelist entry {index}: duplicate location {filename}:{line}"
            )
        whitelist[location] = rationale.strip()

    return whitelist


def _format_locations(
    locations: Iterable[tuple[tuple[str, int], set[str]]],
) -> list[str]:
    return [
        f"  {filename}:{line}: {', '.join(sorted(terms))}"
        for (filename, line), terms in locations
    ]


def main() -> int:
    try:
        whitelist = _load_whitelist()
        all_matches: dict[tuple[str, int], set[str]] = {}
        for relative_path in _tracked_files():
            if relative_path == WHITELIST_PATH.name:
                continue
            content = _read_text(relative_path)
            if content is not None:
                all_matches.update(_find_matches(relative_path, content))
    except (ConfigurationError, RuntimeError) as error:
        print(f"forbidden-term check configuration error: {error}", file=sys.stderr)
        return 2

    stale_entries = sorted(set(whitelist) - set(all_matches))
    violations = sorted(set(all_matches) - set(whitelist))
    if stale_entries:
        print("Stale whitelist entries must be removed or updated:", file=sys.stderr)
        for filename, line in stale_entries:
            print(f"  {filename}:{line}", file=sys.stderr)
    if violations:
        if stale_entries:
            print(file=sys.stderr)
        print(
            "Forbidden and deprecated terminology was found. Use the HyperShell "
            "equivalent instead:",
            file=sys.stderr,
        )
        for output_line in _format_locations(
            (location, all_matches[location]) for location in violations
        ):
            print(output_line, file=sys.stderr)
        print(
            "If the usage is unavoidable, follow the whitelist guidance at "
            f"{WHITELIST_GUIDANCE}.",
            file=sys.stderr,
        )

    return 1 if stale_entries or violations else 0


if __name__ == "__main__":
    raise SystemExit(main())
