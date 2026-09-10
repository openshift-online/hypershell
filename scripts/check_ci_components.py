#!/usr/bin/env python3

import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOTS = (ROOT / "components", ROOT / "packages")
CONFIG_PATH = ROOT / ".github" / "component-paths.json"
CHECKS_WORKFLOW_PATH = ROOT / ".github" / "workflows" / "checks.yml"
TESTS_WORKFLOW_PATH = ROOT / ".github" / "workflows" / "tests.yml"
UNIT_TESTS_WORKFLOW_PATH = ROOT / ".github" / "workflows" / "unit-tests.yml"


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
        checks_workflow = CHECKS_WORKFLOW_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {CHECKS_WORKFLOW_PATH.relative_to(ROOT)}: {exc}")
        return 1

    try:
        tests_workflow = TESTS_WORKFLOW_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {TESTS_WORKFLOW_PATH.relative_to(ROOT)}: {exc}")
        return 1

    try:
        unit_tests_workflow = UNIT_TESTS_WORKFLOW_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {UNIT_TESTS_WORKFLOW_PATH.relative_to(ROOT)}: {exc}")
        return 1

    source_directories = {
        str(path.relative_to(ROOT))
        for source_root in SOURCE_ROOTS
        if source_root.is_dir()
        for path in source_root.iterdir()
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

        unit_tested = registration.get("unit_tested", False)
        if not isinstance(unit_tested, bool):
            errors.append(f"detector entry {component!r} has a non-boolean unit_tested")
            unit_tested = False

        if lint_job is not None and (
            not isinstance(lint_job, str)
            or not re.fullmatch(r"[a-z][a-z0-9-]*", lint_job or "")
        ):
            errors.append(f"detector entry {component!r} has an invalid lint_job")
            lint_job = None

        if lint_job is not None:
            # checks.yml is a standalone, independently-triggered workflow
            # (not called from tests.yml) with its own detect-changes job.
            # Verify the whole wiring lives there: detector output, lint job,
            # and the job's gating condition on that same job's output.
            component_checks = (
                (
                    "detector output",
                    checks_workflow,
                    CHECKS_WORKFLOW_PATH,
                    rf"steps\.detect\.outputs\.{re.escape(component)}\b",
                ),
                (
                    "lint job",
                    checks_workflow,
                    CHECKS_WORKFLOW_PATH,
                    rf"(?m)^  {re.escape(lint_job)}:$",
                ),
                (
                    "job condition",
                    checks_workflow,
                    CHECKS_WORKFLOW_PATH,
                    rf"needs\.detect-changes\.outputs\.{re.escape(component)}\b",
                ),
            )
            for description, text, path, pattern in component_checks:
                if re.search(pattern, text) is None:
                    errors.append(
                        f"{component!r} is missing its {description} in "
                        f"{path.relative_to(ROOT)}"
                    )

        if unit_tested:
            # unit-tests.yml is a reusable workflow called from tests.yml's
            # `unit` job, so a component's unit-test wiring spans both files:
            # tests.yml passes the detection output into `unit` as a `with:`
            # input, unit-tests.yml declares the matching `workflow_call`
            # input, and some job in unit-tests.yml gates on it (jobs are not
            # 1:1 with components here -- e.g. test-frontend and test-cli
            # each cover several -- so this checks that the input is
            # referenced somewhere, not a specific job name).
            unit_test_checks = (
                (
                    "detection input passed to the unit stage",
                    tests_workflow,
                    TESTS_WORKFLOW_PATH,
                    rf"needs\.detect-changes\.outputs\.{re.escape(component)}\b",
                ),
                (
                    "unit-tests.yml workflow_call input",
                    unit_tests_workflow,
                    UNIT_TESTS_WORKFLOW_PATH,
                    rf"(?m)^      {re.escape(component)}:\s*$",
                ),
                (
                    "unit-tests.yml job condition",
                    unit_tests_workflow,
                    UNIT_TESTS_WORKFLOW_PATH,
                    rf"inputs\.{re.escape(component)}\b",
                ),
            )
            for description, text, path, pattern in unit_test_checks:
                if re.search(pattern, text) is None:
                    errors.append(
                        f"{component!r} is missing its {description} in "
                        f"{path.relative_to(ROOT)}"
                    )

    for directory in sorted(source_directories - registrations.keys()):
        errors.append(f"{directory} is not registered for component-aware CI")
    for directory in sorted(registrations.keys() - source_directories):
        if not (ROOT / directory).is_dir():
            errors.append(f"registered component directory {directory} does not exist")

    if errors:
        print("CI component registration is incomplete:")
        for error in errors:
            print(f"- {error}")
        print(
            "Use the maintain-ci skill and update .github/component-paths.json, "
            ".github/workflows/checks.yml, and (for unit_tested components) "
            ".github/workflows/tests.yml and .github/workflows/unit-tests.yml together."
        )
        return 1

    print(f"All {len(source_directories)} components and packages are registered in CI.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
