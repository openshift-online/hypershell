#!/usr/bin/env python3
"""Validate the Mergify dependency-aware scope graph in ``.mergify.yml``.

Desired state: specs/platform/mergify-parallel-merge-queue.spec.md (CICD-MRG).

This policy check does three things:

1. Scenario table (CICD-MRG-03 / CICD-MRG-11): assert that representative file
   changes select exactly the scopes the specification requires, so the scope
   graph cannot silently drift away from the documented dependency model.

2. Coverage audit (CICD-MRG-08): every tracked file must be classified by at
   least one scope include pattern or a barrier file, or be listed in the
   acknowledged-unclassified allowlist. A new, unmapped production path fails
   the check until an owner either maps it to a scope/barrier or records an
   explicit decision in the allowlist. There is deliberately no universal ``**``
   fallback scope.

3. Detector alignment (CICD-MRG-09): spot-check that ``.github/component-paths.json``
   encodes the same cross-component dependencies the scopes do (control-plane
   consuming API helper packages and the CLI, OpenAPI propagating to its
   consumers), so queue independence and what-is-tested stay in agreement.

Glob matching follows Mergify's documented doublestar semantics -- ``**`` spans
zero or more path segments, ``*`` and ``?`` stay within a single segment -- NOT
shell ``fnmatch`` (where ``*`` crosses ``/``). The glob-to-regex translation is
exercised directly by scripts/test_check_mergify_scopes.py. Validate the real
globs against the Mergify service during rollout; matching the documented
semantics here is necessary but not a substitute for service simulation.
"""

import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MERGIFY_PATH = ROOT / ".mergify.yml"
COMPONENT_PATHS_PATH = ROOT / ".github" / "component-paths.json"
ALLOWLIST_PATH = ROOT / ".mergify-scope-allowlist.txt"


# --------------------------------------------------------------------------- #
# Glob matching (Mergify doublestar semantics)
# --------------------------------------------------------------------------- #

_REGEX_CACHE: dict[str, re.Pattern[str]] = {}


def glob_to_regex(pattern: str) -> str:
    """Translate a Mergify file-pattern glob into an anchored regex string.

    ``**`` matches any number of path segments (including zero); ``*`` matches a
    run of non-separator characters within one segment; ``?`` matches a single
    non-separator character. Everything else is matched literally.
    """
    out = ["^"]
    i = 0
    n = len(pattern)
    while i < n:
        char = pattern[i]
        if char == "*":
            if i + 1 < n and pattern[i + 1] == "*":
                # '**' -- spans segments.
                i += 2
                if i < n and pattern[i] == "/":
                    # '**/' -- zero or more leading segments.
                    out.append("(?:[^/]+/)*")
                    i += 1
                else:
                    # Trailing '**' -- the rest of the path, if any.
                    out.append(".*")
            else:
                out.append("[^/]*")
                i += 1
        elif char == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(char))
            i += 1
    out.append("$")
    return "".join(out)


def _compiled(pattern: str) -> re.Pattern[str]:
    regex = _REGEX_CACHE.get(pattern)
    if regex is None:
        regex = re.compile(glob_to_regex(pattern))
        _REGEX_CACHE[pattern] = regex
    return regex


def matches_any(path: str, patterns: list[str]) -> bool:
    return any(_compiled(pattern).match(path) for pattern in patterns)


# --------------------------------------------------------------------------- #
# .mergify.yml parsing (dependency-free; no PyYAML in the policy environment)
# --------------------------------------------------------------------------- #


def parse_mergify(text: str) -> tuple[dict[str, dict[str, list[str]]], list[str]]:
    """Parse ``scopes.source.files`` and ``scopes.barrier_files.include``.

    Returns ``(scopes, barriers)`` where ``scopes`` maps a scope name to its
    ``include``/``exclude`` lists. Tailored to the known indentation of
    ``.mergify.yml`` (scope names at 6 spaces, their include/exclude at 8,
    list items at 10; barrier include at 4, its items at 6).
    """
    scopes: dict[str, dict[str, list[str]]] = {}
    barriers: list[str] = []
    in_files = False
    in_barriers = False
    cur_scope: str | None = None
    section: str | None = None  # 'include' | 'exclude' | 'barrier'

    for raw in text.splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        stripped = raw.strip()

        if stripped.startswith("- "):
            item = stripped[2:].strip().strip('"').strip("'")
            if section == "barrier":
                barriers.append(item)
            elif section in ("include", "exclude") and cur_scope is not None:
                scopes[cur_scope][section].append(item)
            continue

        key = stripped[:-1] if stripped.endswith(":") else stripped

        if indent == 0:
            in_files = False
            in_barriers = False
            cur_scope = None
            section = None
            continue
        if indent == 4 and key == "files":
            in_files = True
            in_barriers = False
            cur_scope = None
            section = None
            continue
        if indent == 2 and key == "barrier_files":
            in_files = False
            in_barriers = True
            cur_scope = None
            section = None
            continue
        if in_files and indent == 6 and stripped.endswith(":"):
            cur_scope = key
            scopes.setdefault(cur_scope, {"include": [], "exclude": []})
            section = None
            continue
        if in_files and indent == 8 and key in ("include", "exclude"):
            section = key
            continue
        if in_barriers and indent == 4 and key == "include":
            section = "barrier"
            continue
        # Any other key resets the active list but keeps the block context.
        if indent <= 4:
            section = None

    return scopes, barriers


def scopes_for(
    path: str, scopes: dict[str, dict[str, list[str]]]
) -> set[str]:
    """Return the set of scope names that select ``path``."""
    selected = set()
    for name, spec in scopes.items():
        if matches_any(path, spec["include"]) and not matches_any(
            path, spec["exclude"]
        ):
            selected.add(name)
    return selected


def is_classified(
    path: str, scopes: dict[str, dict[str, list[str]]], barriers: list[str]
) -> bool:
    if matches_any(path, barriers):
        return True
    return bool(scopes_for(path, scopes))


# --------------------------------------------------------------------------- #
# Scenario table (CICD-MRG-03 / CICD-MRG-11)
# --------------------------------------------------------------------------- #

# Each case: (changed file, exact expected scope set). Sourced directly from the
# specification scenarios so a scope-graph edit that breaks the documented
# dependency model fails here.
SCENARIOS: list[tuple[str, set[str]]] = [
    # CICD-MRG-03.01 -- API-only change is independent of frontend.
    ("components/api-server/internal/handlers/gateway.go", {"api-server", "control-plane"}),
    # CICD-MRG-03.02 -- OpenAPI change propagates to its consumers, not fleet.
    (
        "components/api-server/openapi/openapi.yaml",
        {"api-server", "cli", "control-plane", "sdk-typescript", "sdk-go", "web-console", "agents"},
    ),
    # CICD-MRG-03.03 -- frontend lockfile does not select Go-only scopes.
    ("pnpm-lock.yaml", {"sdk-typescript", "web-console", "fleet-dashboard", "architecture-site"}),
    # CICD-MRG-03.04 -- fleet dashboard excludes the TypeScript SDK.
    ("components/sdk-typescript/src/client.ts", {"sdk-typescript", "web-console"}),
    # CICD-MRG-03.05 -- control plane includes the API gRPC packages.
    ("components/api-server/pkg/api/grpc/hypershell/v1/service.go", {"api-server", "control-plane"}),
    # CICD-MRG-03.06 -- agent images embed the CLI.
    ("components/cli/cmd/root.go", {"cli", "control-plane", "agents"}),
    # CICD-MRG-11.01 -- chart change selects deployment and all runtime consumers.
    (
        "charts/hypershell/values.yaml",
        {"api-server", "cli", "control-plane", "web-console", "fleet-dashboard", "agents", "deployment"},
    ),
    # CICD-MRG-11.02 -- API implementation stays independent of charts.
    ("components/api-server/internal/dao/gateway.go", {"api-server", "control-plane"}),
    # Documentation lane and its deliberate exclusions.
    ("README.md", {"documentation"}),
    ("specs/platform/e2e-testing.spec.md", {"documentation"}),
    ("components/api-server/README.md", {"documentation"}),
    # Compiled mockups are web-console, not prose.
    ("specs/web-console/mockups/GatewayList.tsx", {"web-console"}),
    # Executable doc helper is repository tooling, not prose-only.
    ("docs/assign-platform-admin-role.sh", {"repository-tooling"}),
    # Architecture source/site is its own lane.
    ("docs/architecture/overview.md", {"architecture-site"}),
    ("tools/architecture-site/astro.config.mjs", {"architecture-site"}),
]


# --------------------------------------------------------------------------- #
# Detector alignment (CICD-MRG-09)
# --------------------------------------------------------------------------- #

# (changed file, component key that component-paths.json must flag). Mirrors the
# cross-component dependencies the scopes encode.
DETECTOR_EXPECTATIONS: list[tuple[str, str]] = [
    # CICD-MRG-09.01 -- CLI kustomize change runs control-plane tests.
    ("components/cli/pkg/kustomize/kustomize.go", "control_plane"),
    # Control plane consumes API helper packages.
    ("components/api-server/pkg/gatewayhealth/health.go", "control_plane"),
    ("components/api-server/pkg/secretstore/store.go", "control_plane"),
    # CICD-MRG-09.02 -- OpenAPI change propagates to its consumers.
    ("components/api-server/openapi/openapi.yaml", "cli"),
    ("components/api-server/openapi/openapi.yaml", "sdk_typescript"),
    ("components/api-server/openapi/openapi.yaml", "sdk_go"),
    ("components/api-server/openapi/openapi.yaml", "web_console"),
]


def load_component_paths() -> dict[str, list[str]]:
    config = json.loads(COMPONENT_PATHS_PATH.read_text(encoding="utf-8"))
    return {name: entry.get("paths", []) for name, entry in config.items()}


# --------------------------------------------------------------------------- #
# Tracked-file enumeration
# --------------------------------------------------------------------------- #


def tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return [line for line in result.stdout.splitlines() if line]


def load_allowlist() -> list[str]:
    """Acknowledged-unclassified path globs (CICD-MRG-08 owner decisions).

    Entries are globs (same doublestar semantics as scopes) so a coherent
    tooling tree can be acknowledged once, while a genuinely new, unmapped
    production directory elsewhere is still flagged.
    """
    if not ALLOWLIST_PATH.exists():
        return []
    entries = []
    for line in ALLOWLIST_PATH.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            entries.append(line)
    return entries


# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #


def main() -> int:
    errors: list[str] = []

    try:
        mergify_text = MERGIFY_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {MERGIFY_PATH.relative_to(ROOT)}: {exc}")
        return 1

    scopes, barriers = parse_mergify(mergify_text)
    if not scopes:
        print("No scopes parsed from .mergify.yml -- the scope graph is missing.")
        return 1
    if not barriers:
        print("No barrier_files parsed from .mergify.yml.")
        return 1

    # 1. Scenario table.
    for path, expected in SCENARIOS:
        actual = scopes_for(path, scopes)
        if actual != expected:
            errors.append(
                f"scope scenario for {path!r} selected {sorted(actual)}, "
                f"expected {sorted(expected)}"
            )

    # 2. Detector alignment.
    try:
        component_paths = load_component_paths()
    except (OSError, json.JSONDecodeError) as exc:
        print(f"Unable to read {COMPONENT_PATHS_PATH.relative_to(ROOT)}: {exc}")
        return 1
    for path, component in DETECTOR_EXPECTATIONS:
        patterns = component_paths.get(component)
        if patterns is None:
            errors.append(
                f"detector expectation references unknown component {component!r}"
            )
            continue
        if not matches_any(path, patterns):
            errors.append(
                f"component-paths.json: {component!r} must flag {path!r} as changed "
                f"(CICD-MRG-09 detector/scope alignment)"
            )

    # 3. Coverage audit.
    allowlist = load_allowlist()
    tracked = tracked_files()
    # A tracked file is "otherwise unclassified" when no scope and no barrier
    # select it; the allowlist is the only thing that may rescue it.
    otherwise_unclassified = {
        path for path in tracked if not is_classified(path, scopes, barriers)
    }
    # An allowlist entry earns its place only by rescuing at least one currently
    # otherwise-unclassified tracked file; otherwise it is stale (its paths were
    # scoped, deleted, or never existed) and must be removed.
    for entry in allowlist:
        if not any(
            _compiled(entry).match(path) for path in otherwise_unclassified
        ):
            errors.append(
                f"allowlist entry {entry!r} no longer rescues any unclassified "
                f"tracked file; remove it from {ALLOWLIST_PATH.name}"
            )
    unclassified = sorted(
        path
        for path in otherwise_unclassified
        if not matches_any(path, allowlist)
    )
    for path in unclassified:
        errors.append(
            f"tracked path {path!r} is not classified by any Mergify scope or "
            f"barrier (CICD-MRG-08). Map it to a scope in .mergify.yml, add it to "
            f"barrier_files if it affects all lanes, or record an explicit owner "
            f"decision in {ALLOWLIST_PATH.name}."
        )

    if errors:
        print("Mergify scope validation failed:")
        for error in errors:
            print(f"- {error}")
        return 1

    print(
        f"Mergify scope graph OK: {len(scopes)} scopes, {len(barriers)} barrier "
        f"patterns, {len(SCENARIOS)} scenarios, "
        f"{len(DETECTOR_EXPECTATIONS)} detector expectations; all tracked files "
        f"classified."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
