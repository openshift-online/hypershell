#!/usr/bin/env python3
"""Stamp or verify OPENSHELL_VERSION tags in kustomize deployment manifests.

Usage:
    python3 scripts/sync_openshell_version.py          # check mode (CI)
    python3 scripts/sync_openshell_version.py --stamp   # update manifests in-place
"""
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
VERSION_FILE = REPO_ROOT / "OPENSHELL_VERSION"

# Manifests that embed OPENSHELL_TAG as an image tag in GATEWAY_IMAGE or
# GATEWAY_SUPERVISOR_IMAGE env var values.  Each entry maps a file to the
# env-var names whose image tags must match OPENSHELL_TAG.
MANAGED_FILES: list[tuple[Path, list[str]]] = [
    (
        REPO_ROOT / "deploy" / "base" / "platform-resources" / "controller.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
    ),
    (
        REPO_ROOT / "deploy" / "base" / "control-plane" / "deployment.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
    ),
    (
        REPO_ROOT / "deploy" / "ibm" / "kustomization.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE"],
    ),
]

# Go source file containing the defaultConsoleImage constant, pinned by
# digest (image@sha256:...).  OPENSHELL_CONSOLE_IMAGE and
# OPENSHELL_CONSOLE_DIGEST from OPENSHELL_VERSION are the source of truth.
CONSOLE_IMAGE_FILE = (
    REPO_ROOT
    / "components"
    / "control-plane"
    / "internal"
    / "gateway"
    / "config.go"
)

_CONSOLE_CONST_RE = re.compile(
    r'(const defaultConsoleImage\s*=\s*")'
    r'([^"]+)'
    r'(")'
)


def parse_openshell_version() -> dict[str, str]:
    variables: dict[str, str] = {}
    for line in VERSION_FILE.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            key, _, value = line.partition("=")
            variables[key.strip()] = value.strip()
    return variables


# Matches a YAML env-var value line whose image reference ends with a
# :tag (possibly followed by @sha256:...).  Captures the tag group so it
# can be replaced.
def _tag_pattern(tag: str) -> re.Pattern[str]:
    return re.compile(r"(:\s*\S+:)" + re.escape(tag) + r"(\b)")


def stamp_file(
    path: Path, env_names: list[str], tag: str, *, check_only: bool
) -> list[str]:
    lines = path.read_text().splitlines(keepends=True)
    mismatches: list[str] = []
    result: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        matched_env = False
        for env_name in env_names:
            if re.search(rf"name:\s*{env_name}\s*$", line):
                matched_env = True
                result.append(line)
                i += 1
                if i < len(lines):
                    value_line = lines[i]
                    # Extract the image tag from the value line.
                    # Image refs: registry/image:TAG or
                    # registry:port/image:TAG@sha256:...
                    # The tag is the text after the last colon that
                    # follows the last slash, up to @ or whitespace.
                    m = re.search(r"value:\s*(\S+)", value_line)
                    if m:
                        ref = m.group(1)
                        last_slash = ref.rfind("/")
                        tag_colon = ref.find(":", last_slash + 1)
                        if tag_colon > last_slash:
                            at_pos = ref.find("@", tag_colon)
                            if at_pos == -1:
                                current_tag = ref[tag_colon + 1 :]
                            else:
                                current_tag = ref[tag_colon + 1 : at_pos]
                            if current_tag != tag:
                                try:
                                    rel = path.relative_to(REPO_ROOT)
                                except ValueError:
                                    rel = path
                                mismatches.append(
                                    f"  {rel}: {env_name} has "
                                    f":{current_tag}, expected :{tag}"
                                )
                                if not check_only:
                                    new_ref = ref[: tag_colon + 1] + tag
                                    value_line = value_line.replace(
                                        ref, new_ref
                                    )
                    result.append(value_line)
                    i += 1
                break
        if not matched_env:
            result.append(line)
            i += 1

    if not check_only and mismatches:
        path.write_text("".join(result))

    return mismatches


def stamp_console_image(
    path: Path,
    image: str,
    digest: str,
    *,
    check_only: bool,
) -> list[str]:
    """Stamp or verify the defaultConsoleImage Go constant."""
    if not path.exists():
        return []

    text = path.read_text()
    m = _CONSOLE_CONST_RE.search(text)
    if not m:
        raise RuntimeError(
            f"{path}: defaultConsoleImage constant not found - "
            "was it renamed or reformatted?"
        )

    expected = f"{image}@{digest}"
    current = m.group(2)
    if current == expected:
        return []

    try:
        rel = path.relative_to(REPO_ROOT)
    except ValueError:
        rel = path
    mismatch = f"  {rel}: defaultConsoleImage has {current}, expected {expected}"

    if not check_only:
        text = _CONSOLE_CONST_RE.sub(rf"\g<1>{expected}\3", text)
        path.write_text(text)

    return [mismatch]


def main() -> int:
    check_only = "--stamp" not in sys.argv

    env = parse_openshell_version()
    tag = env.get("OPENSHELL_TAG", "")
    if not tag:
        print("ERROR: OPENSHELL_TAG not found in", VERSION_FILE)
        return 1

    all_mismatches: list[str] = []
    for path, env_names in MANAGED_FILES:
        if not path.exists():
            continue
        all_mismatches.extend(
            stamp_file(path, env_names, tag, check_only=check_only)
        )

    console_image = env.get("OPENSHELL_CONSOLE_IMAGE", "")
    console_digest = env.get("OPENSHELL_CONSOLE_DIGEST", "")
    if console_image and console_digest:
        all_mismatches.extend(
            stamp_console_image(
                CONSOLE_IMAGE_FILE,
                console_image,
                console_digest,
                check_only=check_only,
            )
        )

    if all_mismatches:
        if check_only:
            print(
                "Kustomize manifests out of sync with OPENSHELL_VERSION "
                f"(expected tag {tag}):"
            )
            for m in all_mismatches:
                print(m)
            print(
                "\nRun `make sync-openshell-version` to fix, "
                "or update OPENSHELL_VERSION."
            )
            return 1
        else:
            print(f"Stamped {len(all_mismatches)} image tag(s) to {tag}:")
            for m in all_mismatches:
                print(m.replace("has", "was").replace(", expected", " ->"))
    else:
        if not check_only:
            print(f"All manifests already at {tag}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
