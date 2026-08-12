#!/usr/bin/env python3
"""Require immutable references for external build and automation inputs."""

from __future__ import annotations

import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
from typing import Iterable


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
OPENSHIFT_INTERNAL_REGISTRY = "image-registry.openshift-image-registry.svc:5000/"
PROJECT_IMAGE_PREFIX = "quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/"

_DIGEST_PIN = re.compile(r"@sha256:[0-9a-f]{64}$", re.IGNORECASE)
_COMMIT_PIN = re.compile(r"[0-9a-f]{40}$", re.IGNORECASE)
_USES_LINE = re.compile(r"^\s*(?:-\s*)?uses:\s*([^\s#]+)")
_FROM_LINE = re.compile(
    r"^\s*FROM\s+(?:--[^\s]+\s+)*([^\s]+)", re.IGNORECASE
)
_IMAGE_LINE = re.compile(r"^(\s*)image:\s*([^\s#]+)")
_MAKE_IMAGE = re.compile(
    r"^\s*([A-Z][A-Z0-9_]*_IMAGE)\s*(?::|\?)?=\s*([^\s#]+)"
)
_CONTAINER_RUN = re.compile(
    r"(?:^|\s)[@+-]?(?:docker|podman|\$\((?:CONTAINER_ENGINE|DOCKER|PODMAN)\))\s+"
    r"(?:run|create)\s+(.+)"
)
_REMOTE_PLUGIN = re.compile(r"^(\s*)-\s+remote:\s*([^\s#]+)")
_OS_PACKAGE_INSTALL = re.compile(
    r"\b(?:apt(?:-get)?\s+install|apk\s+add|dnf\s+install|"
    r"microdnf\s+install|yum\s+install)\b",
    re.IGNORECASE,
)

_OPTIONS_WITH_VALUES = {
    "--add-host",
    "--annotation",
    "--device",
    "--entrypoint",
    "--env",
    "--env-file",
    "--hostname",
    "--label",
    "--mount",
    "--name",
    "--network",
    "--platform",
    "--publish",
    "--pull",
    "--user",
    "--volume",
    "--workdir",
    "-e",
    "-h",
    "-l",
    "-p",
    "-u",
    "-v",
    "-w",
}


class CheckError(Exception):
    """Raised when repository inputs cannot be inspected."""


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
        raise CheckError(f"could not list tracked files: {detail}")
    return [
        os.fsdecode(raw_path)
        for raw_path in result.stdout.split(b"\0")
        if raw_path
    ]


def _read_text(relative_path: str) -> str | None:
    path = REPOSITORY_ROOT / relative_path
    if not path.exists() and not path.is_symlink():
        return None
    if path.is_dir():
        return None
    if path.is_symlink():
        data = os.fsencode(os.readlink(path))
    else:
        try:
            data = path.read_bytes()
        except OSError as error:
            raise CheckError(f"could not read {relative_path}: {error}") from error
    if b"\0" in data:
        return None
    return data.decode("utf-8", errors="replace")


def _unquote(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1]
    return value


def _is_digest_pinned(reference: str) -> bool:
    return _DIGEST_PIN.search(reference) is not None


def _is_local_image(reference: str) -> bool:
    return reference.startswith("localhost/") or reference.startswith("localhost:")


def _is_openshift_internal_dev_image(reference: str) -> bool:
    return reference.startswith(OPENSHIFT_INTERNAL_REGISTRY) and reference.endswith(
        ":dev"
    )


def _is_project_image(reference: str) -> bool:
    return reference.startswith(PROJECT_IMAGE_PREFIX)


def _workflow_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        image_match = _IMAGE_LINE.match(line)
        if image_match:
            image_reference = _unquote(image_match.group(2))
            if (
                image_reference.lower() != "dockerfile"
                and not image_reference.startswith("./")
                and not _is_digest_pinned(image_reference)
            ):
                violations.append(
                    (
                        relative_path,
                        line_number,
                        "workflow container image lacks a sha256 digest",
                    )
                )

        match = _USES_LINE.match(line)
        if not match:
            continue
        reference = _unquote(match.group(1))
        if reference.startswith("./"):
            continue
        if reference.startswith("docker://"):
            if not _is_digest_pinned(reference):
                violations.append(
                    (relative_path, line_number, "container action lacks a sha256 digest")
                )
            continue
        separator, pin = reference.rsplit("@", 1) if "@" in reference else ("", "")
        if not separator or not _COMMIT_PIN.fullmatch(pin):
            violations.append(
                (relative_path, line_number, "GitHub Action lacks a full commit SHA")
            )
    return violations


def _dockerfile_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        if line.lstrip().startswith("#"):
            continue
        if _OS_PACKAGE_INSTALL.search(line):
            violations.append(
                (
                    relative_path,
                    line_number,
                    "OS package installation is mutable; use a digest-pinned build stage",
                )
            )
        match = _FROM_LINE.match(line)
        if not match:
            continue
        reference = _unquote(match.group(1))
        if reference.lower() != "scratch" and not _is_digest_pinned(reference):
            violations.append(
                (relative_path, line_number, "base image lacks a sha256 digest")
            )
    return violations


def _has_never_pull_policy(lines: list[str], image_index: int) -> bool:
    for following_line in lines[image_index + 1 : image_index + 6]:
        if re.match(r"^\s*imagePullPolicy:\s*Never\s*(?:#.*)?$", following_line):
            return True
    return False


def _manifest_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    for line_index, line in enumerate(lines):
        match = _IMAGE_LINE.match(line)
        if not match:
            continue
        reference = _unquote(match.group(2))
        if _is_openshift_internal_dev_image(reference):
            continue
        if _is_project_image(reference):
            continue
        if _is_local_image(reference):
            if not _has_never_pull_policy(lines, line_index):
                violations.append(
                    (
                        relative_path,
                        line_index + 1,
                        "locally built image must use imagePullPolicy: Never",
                    )
                )
        elif not _is_digest_pinned(reference):
            violations.append(
                (relative_path, line_index + 1, "container image lacks a sha256 digest")
            )
    return violations


def _logical_lines(lines: list[str]) -> Iterable[tuple[int, str]]:
    start = 0
    parts: list[str] = []
    for line_number, line in enumerate(lines, start=1):
        if not parts:
            start = line_number
        stripped = line.rstrip()
        continued = stripped.endswith("\\")
        parts.append(stripped[:-1] if continued else stripped)
        if not continued:
            yield start, " ".join(parts)
            parts = []
    if parts:
        yield start, " ".join(parts)


def _container_command_image(command: str) -> str | None:
    match = _CONTAINER_RUN.search(command)
    if not match:
        return None
    try:
        arguments = shlex.split(match.group(1), comments=True)
    except ValueError:
        return ""

    skip_value = False
    for argument in arguments:
        if skip_value:
            skip_value = False
            continue
        if argument in _OPTIONS_WITH_VALUES:
            skip_value = True
            continue
        if argument.startswith("-"):
            continue
        return argument
    return ""


def _makefile_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    image_variables: dict[str, str] = {}
    for line_number, line in enumerate(lines, start=1):
        match = _MAKE_IMAGE.match(line)
        if not match:
            continue
        name, reference = match.groups()
        reference = _unquote(reference)
        image_variables[name] = reference
        if not _is_local_image(reference) and not _is_digest_pinned(reference):
            violations.append(
                (relative_path, line_number, f"{name} lacks a sha256 digest")
            )

    for line_number, command in _logical_lines(lines):
        reference = _container_command_image(command)
        if reference is None:
            continue
        variable_match = re.fullmatch(r"\$\(([A-Z][A-Z0-9_]*)\)", reference)
        if variable_match:
            reference = image_variables.get(variable_match.group(1), "")
        if not reference:
            violations.append(
                (relative_path, line_number, "container command image cannot be verified")
            )
        elif not _is_local_image(reference) and not _is_digest_pinned(reference):
            violations.append(
                (relative_path, line_number, "container command image lacks a sha256 digest")
            )
    return violations


def _buf_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    for line_index, line in enumerate(lines):
        match = _REMOTE_PLUGIN.match(line)
        if not match:
            continue
        item_indent = len(match.group(1))
        reference = _unquote(match.group(2))
        final_segment = reference.rsplit("/", 1)[-1]
        has_version = ":" in final_segment and bool(final_segment.rsplit(":", 1)[1])
        has_revision = False
        for following_line in lines[line_index + 1 :]:
            if not following_line.strip():
                continue
            following_indent = len(following_line) - len(following_line.lstrip())
            if following_indent <= item_indent:
                break
            if re.match(r"^\s*revision:\s*[1-9][0-9]*\s*(?:#.*)?$", following_line):
                has_revision = True
        if not has_version or not has_revision:
            violations.append(
                (
                    relative_path,
                    line_index + 1,
                    "remote plugin requires an exact version and revision",
                )
            )
    return violations


def _apm_violations(
    relative_path: str, lines: list[str]
) -> list[tuple[str, int, str]]:
    violations = []
    dependencies_indent: int | None = None
    package_indent: int | None = None
    for line_number, line in enumerate(lines, start=1):
        stripped = line.strip()
        indent = len(line) - len(line.lstrip())
        if stripped == "dependencies:":
            dependencies_indent = indent
            package_indent = None
            continue
        if dependencies_indent is None:
            continue
        if stripped and indent <= dependencies_indent:
            dependencies_indent = None
            package_indent = None
            continue
        if stripped == "apm:":
            package_indent = indent
            continue
        if package_indent is None:
            continue
        if not stripped.startswith("- "):
            if stripped and indent <= package_indent:
                package_indent = None
            continue
        if indent < package_indent:
            package_indent = None
            continue
        reference = _unquote(stripped[2:].strip().split()[0])
        pin = reference.rsplit("#", 1)[-1] if "#" in reference else ""
        if not _COMMIT_PIN.fullmatch(pin):
            violations.append(
                (relative_path, line_number, "APM dependency lacks a full commit SHA")
            )
    return violations


def _is_manifest(relative_path: str) -> bool:
    path = Path(relative_path)
    name = path.name.lower()
    return (
        "/deploy/" in f"/{relative_path}"
        and "/deploy/kind/" not in f"/{relative_path}"
        and path.suffix.lower() in {".yaml", ".yml"}
    ) or name in {
        "compose.yaml",
        "compose.yml",
        "docker-compose.yaml",
        "docker-compose.yml",
    }


def _is_action_config(relative_path: str) -> bool:
    path = Path(relative_path)
    if path.suffix.lower() not in {".yaml", ".yml"}:
        return False
    return relative_path.startswith(".github/workflows/") or (
        relative_path.startswith(".github/actions/")
        and path.name.lower() in {"action.yaml", "action.yml"}
    )


def main() -> int:
    violations: list[tuple[str, int, str]] = []
    try:
        for relative_path in _tracked_files():
            content = _read_text(relative_path)
            if content is None:
                continue
            lines = content.splitlines()
            path = Path(relative_path)
            if _is_action_config(relative_path):
                violations.extend(_workflow_violations(relative_path, lines))
            if path.name.lower().startswith("dockerfile"):
                violations.extend(_dockerfile_violations(relative_path, lines))
            if _is_manifest(relative_path):
                violations.extend(_manifest_violations(relative_path, lines))
            if path.name == "Makefile":
                violations.extend(_makefile_violations(relative_path, lines))
            if path.name == "buf.gen.yaml":
                violations.extend(_buf_violations(relative_path, lines))
            if relative_path == "apm.yml":
                violations.extend(_apm_violations(relative_path, lines))
    except CheckError as error:
        print(f"dependency pin check error: {error}", file=sys.stderr)
        return 2

    if not violations:
        return 0

    print(
        "Unpinned external dependencies found. Use full commit SHAs for GitHub "
        "Actions and Git dependencies, sha256 digests for images, and exact "
        "versions plus revisions for registries that do not expose SHAs:",
        file=sys.stderr,
    )
    for relative_path, line_number, reason in sorted(violations):
        print(f"  {relative_path}:{line_number}: {reason}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
