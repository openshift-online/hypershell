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
CHART_VALUES_FILE = REPO_ROOT / "charts" / "openshell" / "values.yaml"

# Manifests that embed OPENSHELL_TAG as an image tag in GATEWAY_IMAGE or
# GATEWAY_SUPERVISOR_IMAGE env var values.  Each entry maps a file to the
# env-var names whose image tags must match OPENSHELL_TAG.
MANAGED_FILES: list[tuple[Path, list[str]]] = [
    (
        REPO_ROOT / "deploy" / "base" / "platform-resources" / "controller.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE", "GATEWAY_SANDBOX_RUNTIME_IMAGE"],
    ),
    (
        REPO_ROOT / "deploy" / "base" / "control-plane" / "deployment.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE", "GATEWAY_SANDBOX_RUNTIME_IMAGE"],
    ),
    (
        REPO_ROOT / "deploy" / "ibm" / "kustomization.yaml",
        ["GATEWAY_IMAGE", "GATEWAY_SUPERVISOR_IMAGE", "GATEWAY_SANDBOX_RUNTIME_IMAGE"],
    ),
]

# Manifests that advertise the control plane's provisioning version through the
# single-valued gateway-version capability in HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES.
# The label value is rendered from OPENSHELL_TAG so the advertised provisioning
# version always matches the images the overlay deploys, with no runtime image-tag
# parsing (managed-cluster-registration.spec.md, gateway-version-visibility.spec.md).
CAPABILITY_FILES: list[Path] = [
    REPO_ROOT / "deploy" / "base" / "platform-resources" / "controller.yaml",
    REPO_ROOT / "deploy" / "gitops" / "platform" / "kustomization.yaml",
    REPO_ROOT / "deploy" / "ibm" / "kustomization.yaml",
    REPO_ROOT / "deploy" / "kind" / "kustomization.yaml",
    REPO_ROOT / "deploy" / "openshift" / "kustomization.yaml",
]

CAPABILITY_ENV = "HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES"
GATEWAY_VERSION_NAMESPACE = "gateway-version"

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


def chart_sandbox_image(path: Path) -> str:
    """Read the chart's plain sandbox.image mapping without a YAML dependency.

    Fail on schema or scalar-format changes so an upgrade cannot silently keep
    using the previous workload image. This is deliberately not a YAML parser.
    """
    sandbox = re.search(
        r"(?m)^sandbox:[ \t]*\n((?:[ \t].*\n|\n)*)", path.read_text() + "\n"
    )
    image = (
        re.search(r"(?m)^  image:[ \t]*\n((?: {4}.*\n|\n)*)", sandbox[1])
        if sandbox else None
    )
    if not image:
        raise ValueError(f"{path}: expected sandbox.image mapping; review the chart schema")
    keys = set(re.findall(r"(?m)^    ([\w-]+):", image[1]))
    unexpected = keys - {"repository", "tag", "digest", "pullPolicy"}
    if unexpected:
        raise ValueError(f"{path}: unsupported sandbox.image keys {sorted(unexpected)}; review the chart schema")
    fields: dict[str, str] = {}
    for key in ("repository", "tag", "digest"):
        line = re.search(rf"(?m)^    {key}:([^\n]*)$", image[1])
        if not line:
            continue
        scalar = re.fullmatch(
            r'''[ \t]*(?:"([^"\\]*)"|'([^']*)'|([^\s#'"\[\]{}&*!]+))?[ \t]*(?:#.*)?''',
            line[1],
        )
        if not scalar:
            raise ValueError(f"{path}: unsupported sandbox.image.{key}; review the chart schema")
        value = next((value for value in scalar.groups() if value is not None), "")
        fields[key] = "" if scalar[3] in ("null", "Null", "NULL", "~") else value
    repository = fields.get("repository", "")
    if not repository:
        raise ValueError(f"{path}: sandbox.image.repository must be nonempty")
    digest = fields.get("digest", "")
    if digest:
        return f"{repository}@{digest}"
    return f"{repository}:{fields.get('tag') or 'latest'}"


def sync_sandbox_image(
    version_file: Path, chart_values: Path, *, check_only: bool
) -> list[str]:
    """Keep the explicit e2e/preload image aligned with the vendored chart."""
    expected = f"OPENSHELL_SANDBOX_IMAGE={chart_sandbox_image(chart_values)}"
    text = version_file.read_text()
    pattern = re.compile(r"(?m)^OPENSHELL_SANDBOX_IMAGE=[^\n]*$")
    current = pattern.search(text)
    if current and current[0] == expected:
        return []
    if not check_only:
        if current:
            text = pattern.sub(lambda _: expected, text)
        else:
            text = text.rstrip("\n") + "\n" + expected + "\n"
        version_file.write_text(text)
    previous = current[0] if current else "missing sandbox image"
    return [f"  {version_file.name}: {previous}, expected {expected}"]


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


def stamp_capabilities(
    path: Path, tag: str, *, check_only: bool
) -> list[str]:
    """Stamp or verify the gateway-version capability label in a manifest's
    HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES value so it carries exactly one
    gateway-version/<tag> label matching OPENSHELL_TAG."""
    lines = path.read_text().splitlines(keepends=True)
    mismatches: list[str] = []
    expected_label = f"{GATEWAY_VERSION_NAMESPACE}/{tag}"
    try:
        rel = path.relative_to(REPO_ROOT)
    except ValueError:
        rel = path

    for i, line in enumerate(lines):
        if not re.search(rf"name:\s*{CAPABILITY_ENV}\s*$", line):
            continue
        if i + 1 >= len(lines):
            continue
        value_line = lines[i + 1]
        m = re.search(r'(value:\s*")([^"]*)(")', value_line)
        if not m:
            raise RuntimeError(
                f"{rel}: {CAPABILITY_ENV} value must be a double-quoted "
                "comma-separated label string"
            )
        labels = [lbl for lbl in m.group(2).split(",") if lbl]
        rebuilt = [
            lbl for lbl in labels
            if not lbl.startswith(f"{GATEWAY_VERSION_NAMESPACE}/")
        ]
        rebuilt.append(expected_label)
        new_value = ",".join(rebuilt)
        if new_value != m.group(2):
            mismatches.append(
                f"  {rel}: {CAPABILITY_ENV} has {m.group(2)!r}, "
                f"expected {new_value!r}"
            )
            if not check_only:
                lines[i + 1] = (
                    value_line[: m.start()]
                    + m.group(1) + new_value + m.group(3)
                    + value_line[m.end():]
                )
        # Stamp every HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES block in the file,
        # not just the first: a manifest that patches more than one controller
        # must have each block's gateway-version label kept in sync.

    if not check_only and mismatches:
        path.write_text("".join(lines))

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
    try:
        all_mismatches.extend(
            sync_sandbox_image(VERSION_FILE, CHART_VALUES_FILE, check_only=check_only)
        )
    except ValueError as exc:
        print(f"ERROR: {exc}")
        return 1
    for path, env_names in MANAGED_FILES:
        if not path.exists():
            continue
        all_mismatches.extend(
            stamp_file(path, env_names, tag, check_only=check_only)
        )

    for path in CAPABILITY_FILES:
        if not path.exists():
            continue
        all_mismatches.extend(
            stamp_capabilities(path, tag, check_only=check_only)
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
                "OpenShell version pins out of sync with OPENSHELL_VERSION or the vendored chart:"
            )
            for m in all_mismatches:
                print(m)
            print(
                "\nRun `make sync-openshell-version` to fix, "
                "or update OPENSHELL_VERSION."
            )
            return 1
        else:
            print(f"Stamped {len(all_mismatches)} OpenShell version value(s):")
            for m in all_mismatches:
                print(m.replace("has", "was").replace(", expected", " ->"))
    else:
        if not check_only:
            print("All OpenShell version pins match OPENSHELL_VERSION and the vendored chart")

    return 0


if __name__ == "__main__":
    sys.exit(main())
