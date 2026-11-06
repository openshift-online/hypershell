#!/usr/bin/env python3
"""Validate the controller image's app.hypershell.openshell.version label.

This inspects a built controller image's *configuration* (the OCI image config
Labels), not the Dockerfile text, and confirms the single supported OpenShell
release label is:

- present and non-empty on every published architecture variant,
- a single release identifier (never a list, range, or wildcard),
- identical across all architecture variants,
- equal to the expected OPENSHELL_TAG, and
- distinct from org.opencontainers.image.version (the controller's own version),
  which must still be present and is never overloaded with the release value.

See specs/platform/openshell-image-auto-update.spec.md (CICD-IMG-01, CICD-IMG-03,
CICD-IMG-04, CICD-IMG-05).

Usage:
    python3 scripts/check_controller_image_label.py <image-ref> [--expected TAG]

The image reference is inspected per architecture with `skopeo inspect --config`.
When --expected is omitted it defaults to OPENSHELL_TAG in OPENSHELL_VERSION.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
VERSION_FILE = REPO_ROOT / "OPENSHELL_VERSION"
CONTROLLER_DOCKERFILE = REPO_ROOT / "components" / "control-plane" / "Dockerfile"

RELEASE_LABEL = "app.hypershell.openshell.version"
CONTROLLER_VERSION_LABEL = "org.opencontainers.image.version"
BUILD_ARG = "OPENSHELL_TAG"

# The LABEL must derive its value from the OPENSHELL_TAG build arg, so it can
# never be a hardcoded second copy of the release string.
_LABEL_WIRING_RE = re.compile(
    re.escape(RELEASE_LABEL) + r'="\$\{?' + re.escape(BUILD_ARG) + r"\}?"
)

# A single OpenShell release identifier, e.g. v0.1.6-rhaiv.4. No whitespace,
# commas, ranges, or wildcards - those would express more than one release.
_SINGLE_IDENTIFIER_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]*$")


class LabelError(Exception):
    """A controller image label violates the single-release contract."""


def expected_tag() -> str:
    for line in VERSION_FILE.read_text().splitlines():
        line = line.strip()
        if line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        if key.strip() == "OPENSHELL_TAG":
            return value.strip()
    raise LabelError(f"OPENSHELL_TAG not found in {VERSION_FILE}")


def validate_release_value(value: str | None) -> None:
    """Fail unless value is a single, non-empty release identifier."""
    if value is None:
        raise LabelError(
            f"{RELEASE_LABEL} label is missing from the image configuration"
        )
    if not value.strip():
        raise LabelError(f"{RELEASE_LABEL} label is empty")
    if not _SINGLE_IDENTIFIER_RE.fullmatch(value):
        raise LabelError(
            f"{RELEASE_LABEL} value {value!r} is not a single release "
            "identifier (no lists, ranges, or wildcards are allowed)"
        )


def validate_dockerfile_wiring(dockerfile_text: str) -> None:
    """Fail unless the controller Dockerfile derives the label from the arg.

    This is a static guard against regressions; the authoritative check inspects
    the built image configuration (validate_image_configs).
    """
    if not re.search(r"(?m)^\s*ARG\s+" + re.escape(BUILD_ARG) + r"\b", dockerfile_text):
        raise LabelError(
            f"controller Dockerfile is missing 'ARG {BUILD_ARG}'"
        )
    if not _LABEL_WIRING_RE.search(dockerfile_text):
        raise LabelError(
            f"controller Dockerfile must set {RELEASE_LABEL} from "
            f"${{{BUILD_ARG}}}, not a hardcoded release string"
        )


def validate_arch_labels(
    arch: str, labels: dict[str, str], expected: str
) -> None:
    """Validate one architecture variant's image-config labels."""
    value = labels.get(RELEASE_LABEL)
    validate_release_value(value)
    if value != expected:
        raise LabelError(
            f"{arch}: {RELEASE_LABEL} is {value!r}, expected {expected!r} "
            "(it must match OPENSHELL_TAG and the gateway/supervisor release)"
        )
    controller_version = labels.get(CONTROLLER_VERSION_LABEL)
    if not controller_version:
        raise LabelError(
            f"{arch}: {CONTROLLER_VERSION_LABEL} is missing; the controller's "
            "own version must stay present and distinct"
        )
    if controller_version == value:
        raise LabelError(
            f"{arch}: {CONTROLLER_VERSION_LABEL} must not be overloaded with "
            f"the OpenShell release {value!r}"
        )


def validate_image_configs(
    per_arch_labels: dict[str, dict[str, str]], expected: str
) -> None:
    """Validate the label across every published architecture variant."""
    if not per_arch_labels:
        raise LabelError("No architecture variants were found to inspect")
    for arch, labels in sorted(per_arch_labels.items()):
        validate_arch_labels(arch, labels, expected)
    values = {labels[RELEASE_LABEL] for labels in per_arch_labels.values()}
    if len(values) != 1:
        raise LabelError(
            "Architecture variants carry different "
            f"{RELEASE_LABEL} values: {sorted(values)}"
        )


def _skopeo(*args: str) -> str:
    return subprocess.run(
        ["skopeo", *args], check=True, text=True, capture_output=True
    ).stdout


def architectures(image_ref: str, runner=_skopeo) -> list[str]:
    """Return the linux architectures published for image_ref."""
    raw = json.loads(runner("inspect", "--raw", f"docker://{image_ref}"))
    manifests = raw.get("manifests")
    if not manifests:
        # A single-arch image; inspect its own architecture.
        config = json.loads(runner("inspect", "--config", f"docker://{image_ref}"))
        return [config.get("architecture", "amd64")]
    return [
        m["platform"]["architecture"]
        for m in manifests
        if m.get("platform", {}).get("os") == "linux"
        and m.get("platform", {}).get("architecture")
    ]


def fetch_local_labels(image_ref: str, engine: str) -> dict[str, dict[str, str]]:
    """Read the config Labels of a locally built image via the container engine.

    Local builds produce a single-architecture image; this inspects that built
    image's configuration (not the Dockerfile) so the build pipeline fails before
    a broken label ships (specs/platform/openshell-image-auto-update.spec.md,
    CICD-IMG-05).
    """
    out = subprocess.run(
        [engine, "inspect", "--format", "{{json .Config.Labels}}", image_ref],
        check=True,
        text=True,
        capture_output=True,
    ).stdout
    labels = json.loads(out) or {}
    arch = subprocess.run(
        [engine, "inspect", "--format", "{{.Architecture}}", image_ref],
        check=True,
        text=True,
        capture_output=True,
    ).stdout.strip()
    return {arch or "amd64": labels}


def fetch_per_arch_labels(image_ref: str, runner=_skopeo) -> dict[str, dict[str, str]]:
    """Read the config Labels of each architecture variant via skopeo."""
    result: dict[str, dict[str, str]] = {}
    for arch in architectures(image_ref, runner):
        config = json.loads(
            runner(
                "inspect",
                "--config",
                "--override-os",
                "linux",
                "--override-arch",
                arch,
                f"docker://{image_ref}",
            )
        )
        result[arch] = (config.get("config") or {}).get("Labels") or {}
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image", help="Controller image reference to inspect")
    parser.add_argument(
        "--expected",
        default=None,
        help="Expected OpenShell release (defaults to OPENSHELL_TAG)",
    )
    parser.add_argument(
        "--engine",
        default=None,
        help="Inspect a locally built image with this container engine "
        "(podman/docker) instead of a published image via skopeo",
    )
    args = parser.parse_args()

    try:
        expected = args.expected or expected_tag()
        if args.engine:
            per_arch = fetch_local_labels(args.image, args.engine)
        else:
            per_arch = fetch_per_arch_labels(args.image)
        validate_image_configs(per_arch, expected)
    except LabelError as exc:
        print(f"ERROR: {exc}")
        return 1
    except subprocess.CalledProcessError as exc:
        print(f"ERROR: skopeo failed: {exc.stderr or exc}")
        return 1

    print(
        f"{args.image}: {RELEASE_LABEL}={expected} on "
        f"{', '.join(sorted(per_arch))}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
