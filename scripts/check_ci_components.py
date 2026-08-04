#!/usr/bin/env python3

import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
COMPONENTS_DIR = ROOT / "components"
CONFIG_PATH = ROOT / ".github" / "component-paths.json"
LINT_WORKFLOW_PATH = ROOT / ".github" / "workflows" / "lint.yml"


def main() -> int:
    errors: list[str] = []

    try:
        config = json.loads(CONFIG_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        print(f"Unable to read {CONFIG_PATH.relative_to(ROOT)}: {exc}")
        return 1

    if not isinstance(config, dict):
        print(f"{CONFIG_PATH.relative_to(ROOT)} must contain a JSON object.")
        return 1

    try:
        lint_workflow = LINT_WORKFLOW_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {LINT_WORKFLOW_PATH.relative_to(ROOT)}: {exc}")
        return 1

    component_directories = {
        str(path.relative_to(ROOT))
        for path in COMPONENTS_DIR.iterdir()
        if path.is_dir()
    }
    registrations: dict[str, str] = {}

    for component, registration in config.items():
        if not re.fullmatch(r"[a-z][a-z0-9_]*", component):
            errors.append(
                f"detector key {component!r} must use lowercase letters, numbers, and underscores"
            )
            continue
        if not isinstance(registration, dict):
            errors.append(f"detector entry {component!r} must be an object")
            continue

        directory = registration.get("directory")
        lint_job = registration.get("lint_job")
        paths = registration.get("paths")

        if not isinstance(directory, str) or not directory:
            errors.append(f"detector entry {component!r} requires a directory")
        elif directory in registrations:
            errors.append(
                f"{directory} is registered by both {registrations[directory]!r} and {component!r}"
            )
        else:
            registrations[directory] = component

        if not isinstance(paths, list) or not all(
            isinstance(path, str) and path for path in paths
        ):
            errors.append(f"detector entry {component!r} requires a nonempty paths list")
        elif isinstance(directory, str) and f"{directory}/**" not in paths:
            errors.append(
                f"detector entry {component!r} must include its full component path "
                f"{directory}/**"
            )

        if not isinstance(lint_job, str) or not re.fullmatch(
            r"[a-z][a-z0-9-]*", lint_job or ""
        ):
            errors.append(f"detector entry {component!r} requires a valid lint_job")
            continue

        required_patterns = {
            "detector output": rf"(?m)^      {re.escape(component)}:.*steps\.detect\.outputs\.{re.escape(component)}.*$",
            "lint job": rf"(?m)^  {re.escape(lint_job)}:$",
            "job condition": rf"(?m)^    if:.*needs\.detect-changes\.outputs\.{re.escape(component)}.*$",
            "summary dependency": rf"(?m)^      - {re.escape(lint_job)}$",
        }
        for description, pattern in required_patterns.items():
            if re.search(pattern, lint_workflow) is None:
                errors.append(
                    f"{component!r} is missing its {description} in "
                    f"{LINT_WORKFLOW_PATH.relative_to(ROOT)}"
                )

    for directory in sorted(component_directories - registrations.keys()):
        errors.append(f"{directory} is not registered for component-aware CI")
    for directory in sorted(registrations.keys() - component_directories):
        errors.append(f"registered component directory {directory} does not exist")

    if errors:
        print("CI component registration is incomplete:")
        for error in errors:
            print(f"- {error}")
        print(
            "Use the maintain-ci skill and update .github/component-paths.json and "
            ".github/workflows/lint.yml together."
        )
        return 1

    print(f"All {len(component_directories)} components are registered in CI.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
