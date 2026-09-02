#!/usr/bin/env python3
"""Check image identity and event-specific component build rules."""

from pathlib import Path
import re
import sys


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
COMPONENTS = {
    "api-server": {
        "dockerfile": "components/api-server/Dockerfile",
        "image": "hypershell-api-server-main",
    },
    "control-plane": {
        "dockerfile": "components/control-plane/Dockerfile",
        "image": "hypershell-control-plane-main",
    },
    "web-console": {
        "dockerfile": "components/web-console/Dockerfile",
        "image": "hypershell-web-console-main",
    },
}
NON_PUSH_RELEASE_FILES = ("VERSION", "CHANGELOG.md", "build-version.env")
SHORT_REVISION_TRIM = 33
_ASSIGNMENT_PATTERN = (
    r"(?:^|\s){name}\s*=\s*(?:\"(?P<double>[^\"]*)\"|"
    r"'(?P<single>[^']*)'|(?P<bare>[^\s]+))"
)
_BUILD_VERSION_PATTERN = re.compile(
    r"(?:\$BUILD_PREFIX|\$\{BUILD_PREFIX\})-"
    r"\$\{VCS_REF%(?P<trim>\?+)\}"
)


def _logical_instructions(dockerfile: str) -> list[str]:
    """Return Dockerfile instructions with joined continuation lines."""
    joined = re.sub(r"\\[ \t]*\n[ \t]*", " ", dockerfile)
    return [
        line.strip()
        for line in joined.splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]


def _assignment_values(instructions: list[str], name: str) -> list[str]:
    """Return all values for one Dockerfile assignment."""
    pattern = re.compile(_ASSIGNMENT_PATTERN.format(name=re.escape(name)))
    values: list[str] = []
    for instruction in instructions:
        for match in pattern.finditer(instruction):
            values.append(
                match.group("double")
                or match.group("single")
                or match.group("bare")
            )
    return values


def _is_variable_reference(value: str, name: str) -> bool:
    return value in {f"${name}", f"${{{name}}}"}


def image_metadata_errors(relative_path: str, dockerfile: str) -> list[str]:
    """Return errors for image identity metadata."""
    errors: list[str] = []
    instructions = _logical_instructions(dockerfile)

    for name, label in (
        ("org.opencontainers.image.version", "OCI build version"),
        ("HYPERSHELL_BUILD_VERSION", "runtime build version"),
    ):
        values = _assignment_values(instructions, name)
        if len(values) != 1:
            errors.append(f"{relative_path} must set one {label}")
            continue
        match = _BUILD_VERSION_PATTERN.fullmatch(values[0])
        if match is None:
            errors.append(
                f"{relative_path} {label} must combine BUILD_PREFIX and a shortened VCS_REF"
            )
            continue
        if len(match.group("trim")) != SHORT_REVISION_TRIM:
            errors.append(
                f"{relative_path} {label} must shorten the revision to seven characters"
            )

    for name, label in (
        ("org.opencontainers.image.revision", "OCI revision"),
        ("HYPERSHELL_BUILD_REVISION", "runtime revision"),
    ):
        values = _assignment_values(instructions, name)
        if len(values) != 1 or not _is_variable_reference(values[0], "VCS_REF"):
            errors.append(f"{relative_path} must set {label} from VCS_REF")

    return errors


def check_repository(root: Path) -> list[str]:
    """Return image build policy errors."""
    errors: list[str] = []
    for component, values in COMPONENTS.items():
        for event in ("push", "pull-request", "merge-queue"):
            relative_path = f".tekton/hypershell-{component}-main-{event}.yaml"
            text = (root / relative_path).read_text(encoding="utf-8")
            if "VCS_REF={{revision}}" not in text:
                errors.append(f"{relative_path} must pass the full revision")
            if "value: build-version.env" not in text:
                errors.append(f"{relative_path} must use build-version.env")
            if event == "push":
                if '"VERSION".pathChanged()' not in text:
                    errors.append(f"{relative_path} must build for a VERSION change")
                expected_tag = f"{values['image']}:{{{{revision}}}}"
            else:
                for release_file in NON_PUSH_RELEASE_FILES:
                    if f'"{release_file}".pathChanged()' in text:
                        errors.append(
                            f"{relative_path} must not build for {release_file}"
                        )
                tag_prefix = (
                    "on-pr-" if event == "pull-request" else "on-merge-queue-"
                )
                expected_tag = f"{values['image']}:{tag_prefix}{{{{revision}}}}"
            if expected_tag not in text:
                errors.append(f"{relative_path} must keep its current revision tag")

        dockerfile_path = root / values["dockerfile"]
        dockerfile = dockerfile_path.read_text(encoding="utf-8")
        errors.extend(image_metadata_errors(values["dockerfile"], dockerfile))

    e2e = (root / ".github/workflows/e2e.yml").read_text(encoding="utf-8")
    if "release_version_changed=false" not in e2e:
        errors.append("the E2E plan must identify a release-version push")
    if '[[ "${EVENT_NAME}" == "push" ]]' not in e2e:
        errors.append("the release image rule must check for a push event")
    if 'grep -qx "VERSION"' not in e2e:
        errors.append("the release image rule must check the VERSION file")
    return errors


def main() -> int:
    errors = check_repository(REPOSITORY_ROOT)
    for error in errors:
        print(f"image build policy error: {error}", file=sys.stderr)
    if errors:
        return 1
    print("Image build policy is valid.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
