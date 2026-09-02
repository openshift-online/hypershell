#!/usr/bin/env python3
"""Check the repository source-release policy."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
TITLE_FORM = "<type>(<optional-scope>)<optional-!>: <description>"
ALLOWED_TYPES = (
    "build",
    "chore",
    "ci",
    "deps",
    "docs",
    "feat",
    "fix",
    "perf",
    "refactor",
    "revert",
    "spec",
    "style",
    "test",
)
_TYPE_PATTERN = "|".join(ALLOWED_TYPES)
_TITLE_PATTERN = re.compile(
    rf"^(?P<type>{_TYPE_PATTERN})"
    r"(?:\((?P<scope>[a-z0-9][a-z0-9._/-]*)\))?"
    r"(?P<breaking>!)?: (?P<description>\S(?:.*\S)?)$"
)
_SEMVER_PATTERN = re.compile(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)")
_RELEASE_TITLE_PATTERN = re.compile(r"^chore\(main\): release (?P<version>" + _SEMVER_PATTERN.pattern + r")$")
_BREAKING_FOOTER_PATTERN = re.compile(
    r"(?m)^BREAKING(?: |-)+CHANGE:\s+\S"
)


class ReleasePolicyError(Exception):
    """Report a release policy input error."""


def title_error(title: str) -> str | None:
    """Return an error for an invalid pull request title."""
    if _TITLE_PATTERN.fullmatch(title):
        return None
    allowed = ", ".join(ALLOWED_TYPES)
    return f"title must use {TITLE_FORM}; allowed types: {allowed}"


def is_releasing_commit(message: str) -> bool:
    """Return true when one commit must run Release Please."""
    header = message.splitlines()[0] if message else ""
    if _RELEASE_TITLE_PATTERN.fullmatch(header):
        return True
    match = _TITLE_PATTERN.fullmatch(header)
    if match and (
        match.group("type") in {"feat", "fix"} or match.group("breaking")
    ):
        return True
    return _BREAKING_FOOTER_PATTERN.search(message) is not None


def should_run_release(open_release_pr: bool, messages: list[str]) -> bool:
    """Return true when the release workflow must run."""
    return open_release_pr or any(is_releasing_commit(message) for message in messages)


def _git_output(*arguments: str) -> str:
    result = subprocess.run(
        ("git", *arguments),
        cwd=REPOSITORY_ROOT,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if result.returncode != 0:
        detail = result.stderr.strip() or "git command failed"
        raise ReleasePolicyError(detail)
    return result.stdout


def _load_json(path: Path) -> object:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ReleasePolicyError(f"cannot read {path}: {error}") from error


def _release_base(head_ref: str) -> str:
    tags = _git_output(
        "tag", "--merged", head_ref, "--sort=-version:refname", "--list", "v[0-9]*"
    ).splitlines()
    for tag in tags:
        if re.fullmatch(r"v" + _SEMVER_PATTERN.pattern, tag):
            return tag

    config = _load_json(REPOSITORY_ROOT / "release-please-config.json")
    if not isinstance(config, dict):
        raise ReleasePolicyError("release-please-config.json must contain an object")
    baseline = config.get("bootstrap-sha")
    if not isinstance(baseline, str) or not re.fullmatch(r"[0-9a-f]{40}", baseline):
        raise ReleasePolicyError("release-please-config.json has no valid bootstrap-sha")
    return baseline


def _messages_since_release(head_ref: str) -> list[str]:
    base_ref = _release_base(head_ref)
    output = _git_output("log", "--format=%B%x00", f"{base_ref}..{head_ref}")
    return [message.strip() for message in output.split("\0") if message.strip()]


def release_file_errors(root: Path) -> list[str]:
    """Return all release-file consistency errors."""
    errors: list[str] = []
    try:
        version = (root / "VERSION").read_text(encoding="utf-8").strip()
    except OSError as error:
        return [f"cannot read VERSION: {error}"]
    if not _SEMVER_PATTERN.fullmatch(version):
        errors.append("VERSION must contain one stable semantic version")

    try:
        build_lines = (root / "build-version.env").read_text(
            encoding="utf-8"
        ).splitlines()
    except OSError as error:
        errors.append(f"cannot read build-version.env: {error}")
        build_lines = []
    prefixes = [line.removeprefix("BUILD_PREFIX=") for line in build_lines if line.startswith("BUILD_PREFIX=")]
    if prefixes != [f"v{version}"]:
        errors.append("build-version.env BUILD_PREFIX must equal v plus VERSION")

    try:
        manifest = _load_json(root / ".release-please-manifest.json")
    except ReleasePolicyError as error:
        errors.append(str(error))
        manifest = None
    if not isinstance(manifest, dict) or manifest.get(".") != version:
        errors.append("the release manifest root version must equal VERSION")

    try:
        config = _load_json(root / "release-please-config.json")
    except ReleasePolicyError as error:
        errors.append(str(error))
        config = None
    if isinstance(config, dict):
        packages = config.get("packages")
        root_package = packages.get(".") if isinstance(packages, dict) else None
        if not isinstance(root_package, dict):
            errors.append("the Release Please root package is missing")
        else:
            if root_package.get("release-type") != "simple":
                errors.append("the Release Please root package must use the simple type")
            if root_package.get("version-file") != "VERSION":
                errors.append("Release Please must manage VERSION")
            if "build-version.env" not in root_package.get("extra-files", []):
                errors.append("Release Please must manage build-version.env")
        if config.get("bump-minor-pre-major") is not True:
            errors.append("breaking changes before 1.0.0 must increment the minor version")
        if config.get("bump-patch-for-minor-pre-major") is not False:
            errors.append("features before 1.0.0 must increment the minor version")
        if config.get("always-update") is not True:
            errors.append("Release Please must update an open Release PR")
        if config.get("include-v-in-tag") is not True:
            errors.append("release tags must have a v prefix")
        sections = config.get("changelog-sections")
        visible_types = {
            item.get("type")
            for item in sections
            if isinstance(item, dict) and item.get("hidden") is False
        } if isinstance(sections, list) else set()
        if visible_types != set(ALLOWED_TYPES):
            errors.append("each allowed commit type must have a visible changelog section")
    elif config is not None:
        errors.append("release-please-config.json must contain an object")
    return errors


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)

    title = commands.add_parser("check-title", help="check one pull request title")
    title.add_argument("title")

    gate = commands.add_parser("should-run", help="check whether Release Please must run")
    gate.add_argument("--head-ref", default="HEAD")
    gate.add_argument("--open-release-pr", choices=("true", "false"), default="false")

    files = commands.add_parser("check-files", help="check managed release files")
    files.add_argument("--root", type=Path, default=REPOSITORY_ROOT)
    return parser


def main() -> int:
    arguments = _build_parser().parse_args()
    try:
        if arguments.command == "check-title":
            error = title_error(arguments.title)
            if error:
                print(error, file=sys.stderr)
                return 1
            return 0
        if arguments.command == "should-run":
            messages = _messages_since_release(arguments.head_ref)
            decision = should_run_release(arguments.open_release_pr == "true", messages)
            print(str(decision).lower())
            return 0
        errors = release_file_errors(arguments.root)
        for error in errors:
            print(f"release policy error: {error}", file=sys.stderr)
        return 1 if errors else 0
    except ReleasePolicyError as error:
        print(f"release policy error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
