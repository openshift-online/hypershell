#!/usr/bin/env python3
"""Stamp or verify the vendored OpenShell interceptor protos pin.

Usage:
    python3 scripts/sync_openshell_protos.py           # check mode (CI)
    python3 scripts/sync_openshell_protos.py --stamp   # rewrite the pin file

The vendored protos live in
components/sandbox-resource-interceptor/proto/openshell/ and are copied from
OPENSHELL_CHART_REPO at OPENSHELL_TAG by `make vendor-openshell-protos`
(which also rewrites any upstream em dashes, like vendor-openshell-chart
does). This script keeps PROTO_PIN, a checksum manifest of that directory, in
sync with OPENSHELL_VERSION: check mode fails when the vendored files were
hand-edited, when files were added or removed, or when OPENSHELL_TAG moved
without re-vendoring.
"""

import argparse
import hashlib
import sys
from pathlib import Path

DEFAULT_ROOT = Path(__file__).resolve().parents[1]

# The transitive import closure of gateway_interceptor.proto as it exists in
# the upstream proto/ directory. `make vendor-openshell-protos` copies exactly
# these files.
VENDORED_FILES = [
    "datamodel.proto",
    "extension.proto",
    "gateway_interceptor.proto",
    "openshell.proto",
    "options.proto",
    "sandbox.proto",
]


def vendored_dir(root: Path) -> Path:
    return root / "components" / "sandbox-resource-interceptor" / "proto" / "openshell"


def pin_file(root: Path) -> Path:
    return root / "components" / "sandbox-resource-interceptor" / "proto" / "PROTO_PIN"


def parse_openshell_tag(root: Path) -> str:
    version_file = root / "OPENSHELL_VERSION"
    for line in version_file.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line.startswith("OPENSHELL_TAG="):
            return line.partition("=")[2].strip()
    raise SystemExit(f"No OPENSHELL_TAG found in {version_file}")


def parse_pin(root: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in pin_file(root).read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, value = line.partition("=")
        values[key.strip()] = value.strip()
    return values


def sha256(path: Path) -> str:
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    return f"sha256:{digest}"


def check(root: Path) -> int:
    errors: list[str] = []
    tag = parse_openshell_tag(root)
    pin = parse_pin(root)

    if pin.get("OPENSHELL_TAG") != tag:
        errors.append(
            f"PROTO_PIN is stamped for {pin.get('OPENSHELL_TAG')!r} but "
            f"OPENSHELL_VERSION pins {tag!r}; run "
            f"'make vendor-openshell-protos' after a version bump"
        )

    directory = vendored_dir(root)
    present = sorted(path.name for path in directory.iterdir() if path.is_file())
    if present != sorted(VENDORED_FILES):
        errors.append(
            "vendored proto directory does not match the expected file list: "
            f"present {present}, expected {sorted(VENDORED_FILES)}"
        )

    for name in VENDORED_FILES:
        path = directory / name
        if not path.is_file():
            errors.append(f"vendored proto {name} is missing")
            continue
        recorded = pin.get(name)
        if recorded is None:
            errors.append(f"PROTO_PIN has no checksum for {name}")
            continue
        if sha256(path) != recorded:
            errors.append(
                f"vendored proto {name} does not match its PROTO_PIN "
                f"checksum; run 'make vendor-openshell-protos' or revert the "
                f"local edit"
            )

    if errors:
        print("Vendored OpenShell protos are out of sync:")
        for error in errors:
            print(f"- {error}")
        return 1
    print(f"Vendored OpenShell protos match PROTO_PIN at {tag}.")
    return 0


def stamp(root: Path) -> int:
    tag = parse_openshell_tag(root)
    lines = [
        "# Checksum pin for the vendored OpenShell interceptor protos.",
        "# Written by 'make vendor-openshell-protos'; verified by",
        "# 'make check-openshell-protos'. Do not edit by hand.",
        f"OPENSHELL_TAG={tag}",
    ]
    for name in VENDORED_FILES:
        path = vendored_dir(root) / name
        if not path.is_file():
            raise SystemExit(
                f"vendored proto {name} is missing; run the vendor target first"
            )
        lines.append(f"{name}={sha256(path)}")
    pin_file(root).write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Stamped {pin_file(root).relative_to(root)} at {tag}.")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stamp", action="store_true", help="rewrite the pin file")
    parser.add_argument(
        "--root",
        type=Path,
        default=DEFAULT_ROOT,
        help="repository root (test harness only)",
    )
    args = parser.parse_args()

    if args.stamp:
        return stamp(args.root)
    return check(args.root)


if __name__ == "__main__":
    sys.exit(main())
