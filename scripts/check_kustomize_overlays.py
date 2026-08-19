#!/usr/bin/env python3
"""Validate that kustomize overlays render valid YAML with expected resources."""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parent.parent

OVERLAYS: dict[str, dict] = {
    "deploy/kind": {},
    "deploy/kind-keycloak-optimized": {
        "keycloak_image": "localhost/hypershell-keycloak:dev-optimized",
        "keycloak_args": ["start", "--optimized", "--import-realm"],
        "keycloak_env": {"KC_HTTP_ENABLED": "true", "KC_CACHE": "local"},
    },
}


def _kustomize_build(overlay: str) -> str:
    result = subprocess.run(
        ("kustomize", "build", str(REPOSITORY_ROOT / overlay)),
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return ""
    return result.stdout


def _grep_keycloak_container(output: str) -> dict[str, str | list[str]]:
    """Extract image, args, and env from the rendered keycloak container."""
    info: dict[str, str | list[str]] = {}
    image_match = re.search(
        r"image:\s*(\S+)",
        output[output.find("name: keycloak\n  namespace: keycloak"):],
    )
    if image_match:
        info["image"] = image_match.group(1)

    args: list[str] = []
    in_args = False
    for line in output.splitlines():
        stripped = line.strip()
        if stripped == "- args:":
            in_args = True
            continue
        if in_args:
            if stripped.startswith("- "):
                args.append(stripped[2:])
            else:
                break
    info["args"] = args

    env: dict[str, str] = {}
    lines = output.splitlines()
    for i, line in enumerate(lines):
        if line.strip() == "env:" and any(
            "name: keycloak" in lines[j] for j in range(max(0, i - 20), i)
        ):
            j = i + 1
            while j < len(lines) and lines[j].strip().startswith("- name:"):
                name = lines[j].strip().removeprefix("- name:").strip()
                if j + 1 < len(lines) and "value:" in lines[j + 1]:
                    val = lines[j + 1].strip().removeprefix("value:").strip().strip('"')
                    env[name] = val
                j += 2
            break
    info["env"] = env
    return info


def main() -> int:
    errors: list[str] = []

    for overlay, expectations in OVERLAYS.items():
        output = _kustomize_build(overlay)
        if not output:
            errors.append(f"{overlay}: kustomize build failed or produced no output")
            continue

        if "kind: Deployment" not in output or "name: keycloak" not in output:
            errors.append(f"{overlay}: missing Keycloak Deployment")
            continue

        if not expectations:
            continue

        info = _grep_keycloak_container(output)

        expected_image = expectations.get("keycloak_image")
        if expected_image and info.get("image") != expected_image:
            errors.append(
                f"{overlay}: expected image '{expected_image}', "
                f"got '{info.get('image', '<missing>')}'"
            )

        expected_args = expectations.get("keycloak_args")
        if expected_args and info.get("args") != expected_args:
            errors.append(
                f"{overlay}: expected args {expected_args}, "
                f"got {info.get('args')}"
            )

        expected_env = expectations.get("keycloak_env")
        if expected_env:
            actual_env = info.get("env", {})
            for key, value in expected_env.items():
                if actual_env.get(key) != value:
                    errors.append(
                        f"{overlay}: expected env {key}={value}, "
                        f"got {actual_env.get(key, '<missing>')}"
                    )

    if not errors:
        return 0

    print("Kustomize overlay validation failures:", file=sys.stderr)
    for error in errors:
        print(f"  {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
