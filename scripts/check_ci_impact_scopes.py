#!/usr/bin/env python3
"""Validate the CI dependency-aware scope graph in ``.github/ci-impact-scopes.yml``.

This metadata is local only; Mergify uses a serial queue without scopes.
CI selection comes from component-paths.json.

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
   consumers), so critical CI dependencies remain covered.

Glob matching follows Mergify's documented doublestar semantics -- ``**`` spans
zero or more path segments, ``*`` and ``?`` stay within a single segment -- NOT
shell ``fnmatch`` (where ``*`` crosses ``/``). The glob-to-regex translation is
exercised directly by scripts/test_check_ci_impact_scopes.py. This audit is entirely local and does not upload scopes to Mergify.
"""

import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
IMPACT_PATH = ROOT / ".github" / "ci-impact-scopes.yml"
COMPONENT_PATHS_PATH = ROOT / ".github" / "component-paths.json"
ALLOWLIST_PATH = ROOT / ".github" / "ci-impact-allowlist.txt"


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
# .github/ci-impact-scopes.yml parsing (dependency-free; no PyYAML in the policy environment)
# --------------------------------------------------------------------------- #


def parse_impact_scopes(text: str) -> tuple[dict[str, dict[str, list[str]]], list[str]]:
    """Parse ``scopes.source.files`` and ``scopes.barrier_files.include``.

    Returns ``(scopes, barriers)`` where ``scopes`` maps a scope name to its
    ``include``/``exclude`` lists.

    Indentation is read *relative* to the ``files:`` and ``barrier_files:``
    anchors rather than from fixed columns, so reindenting or shifting the
    scopes block as a whole does not silently drop scopes. A two-space step is
    still assumed, but any structural break produces a scope set that
    ``main()`` rejects against ``EXPECTED_SCOPES`` instead of passing a partial
    graph (CICD-MRG-03/08).
    """
    scopes: dict[str, dict[str, list[str]]] = {}
    barriers: list[str] = []
    files_indent: int | None = None  # indent of the `files:` key
    barriers_indent: int | None = None  # indent of the `barrier_files:` key
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

        is_key = stripped.endswith(":")
        key = stripped[:-1] if is_key else stripped

        # A top-level key ends any scopes/barriers context.
        if indent == 0:
            files_indent = barriers_indent = None
            cur_scope = None
            section = None
            continue

        # The `files:` / `barrier_files:` anchors establish the relative frame.
        if is_key and key == "files":
            files_indent = indent
            barriers_indent = None
            cur_scope = None
            section = None
            continue
        if is_key and key == "barrier_files":
            barriers_indent = indent
            files_indent = None
            cur_scope = None
            section = None
            continue

        if files_indent is not None:
            if indent <= files_indent:
                # Left the files block without hitting an anchor above.
                files_indent = None
                cur_scope = None
                section = None
            elif indent == files_indent + 2 and is_key and key not in (
                "include",
                "exclude",
            ):
                cur_scope = key
                scopes.setdefault(cur_scope, {"include": [], "exclude": []})
                section = None
                continue
            elif (
                cur_scope is not None
                and indent == files_indent + 4
                and key in ("include", "exclude")
            ):
                section = key
                continue
            else:
                continue

        if barriers_indent is not None:
            if indent <= barriers_indent:
                barriers_indent = None
                section = None
            elif indent == barriers_indent + 2 and is_key and key == "include":
                section = "barrier"
                continue
            else:
                continue

    return scopes, barriers


# The exact dependency-aware consumer scopes the specification defines
# (CICD-MRG-03). main() asserts the parsed set matches this, so a reindented or
# restructured .github/ci-impact-scopes.yml that the heuristic parser reads partially fails
# loudly on every invocation path (hook, CI, or `make check`) instead of
# passing a wrong or truncated scope graph. Adding or removing a scope is a
# deliberate owner decision that must update this set too.
EXPECTED_SCOPES: frozenset[str] = frozenset(
    {
        "api-server",
        "cli",
        "control-plane",
        "sdk-typescript",
        "sdk-go",
        "web-console",
        "fleet-dashboard",
        "agents",
        "deployment",
        "test-infrastructure",
        "architecture-site",
        "documentation",
        "repository-tooling",
    }
)


# Shared input groups (CICD-MRG-03 drift guard). CI impact scope include/exclude
# lists are flat, and a YAML anchor/alias replaces a whole value -- it cannot
# splice a shared block into part of a larger list. So the producer inputs that
# must appear in several consumer scopes (a deployment change rebuilding every
# runtime lane, the OpenAPI contract propagating to its consumers, and so on) are
# necessarily duplicated across include lists. The risk the review called out is
# silent drift: an owner adds an input to one lane's list but forgets the others,
# and a later change to that file then fails to classify all affected consumers. These groups make the coupling explicit and
# machine-checked: every member scope must contain every pattern in the group, so
# a half-applied edit fails this check loudly instead of drifting. Adding a shared
# input means adding it to the group's tuple AND to each member scope's list.
#
# Each entry maps a human label to (member scopes, required patterns).
SHARED_INCLUDE_GROUPS: dict[str, tuple[frozenset[str], tuple[str, ...]]] = {
    "deployment/release inputs": (
        frozenset(
            {
                "api-server",
                "cli",
                "control-plane",
                "web-console",
                "fleet-dashboard",
                "agents",
                "deployment",
            }
        ),
        (
            "deploy/**",
            "charts/**",
            "dashboards/**",
            "OPENSHELL_VERSION",
            "scripts/cluster/**",
            "scripts/kind/**",
            "scripts/gen-postgres-tls*",
            "scripts/install-openshell*",
            "scripts/sync_openshell_version.py",
            "scripts/test_sync_openshell_version.py",
            "scripts/check_deploy_orphans.py",
            "scripts/test_check_deploy_orphans.py",
            "scripts/test_gitops_manifests.py",
            "pipelines/**",
            "scripts/release_bundle.py",
            "scripts/test_release_bundle.py",
            "scripts/render_release_bundle_pipeline.py",
        ),
    ),
    "API contract/generation inputs": (
        frozenset(
            {"cli", "control-plane", "sdk-typescript", "sdk-go", "web-console", "agents"}
        ),
        (
            "components/api-server/openapi/**",
            "components/api-server/Dockerfile.openapi",
            "components/api-server/Makefile",
        ),
    ),
    "CLI toolchain inputs": (
        frozenset({"cli", "control-plane", "agents"}),
        (
            "scripts/generate-cli.sh",
            "scripts/generate-tui.sh",
            "scripts/trex-checkout.sh",
            "scripts/rh-trex-ai.ref",
            "scripts/lib/**",
        ),
    ),
    "SDK generator inputs": (
        frozenset({"sdk-typescript", "sdk-go", "web-console"}),
        (
            "scripts/sdk-generator/**",
            "scripts/openapi-ir/**",
        ),
    ),
    "frontend toolchain inputs": (
        frozenset({"sdk-typescript", "web-console", "fleet-dashboard", "architecture-site"}),
        (
            "package.json",
            "pnpm-lock.yaml",
            "pnpm-workspace.yaml",
            ".node-version",
            "scripts/bootstrap_pnpm.sh",
        ),
    ),
    "brand asset inputs": (
        frozenset({"web-console", "fleet-dashboard"}),
        ("images/brand/**",),
    ),
    # Per-component Tekton pipelines are shared by the component itself and every
    # downstream consumer that embeds or builds on it, so a pipeline change
    # revalidates all of those lanes.
    "api-server Tekton pipeline inputs": (
        frozenset({"api-server", "control-plane"}),
        (".tekton/hypershell-api-server-*",),
    ),
    "cli Tekton pipeline inputs": (
        frozenset({"cli", "control-plane", "agents"}),
        (".tekton/hypershell-cli-*",),
    ),
}

# Shared exclude groups, same drift guard applied to exclude lists.
SHARED_EXCLUDE_GROUPS: dict[str, tuple[frozenset[str], tuple[str, ...]]] = {
    "informational-file excludes": (
        frozenset(
            {
                "api-server",
                "cli",
                "control-plane",
                "sdk-typescript",
                "sdk-go",
                "web-console",
                "fleet-dashboard",
                "agents",
                "deployment",
                "test-infrastructure",
                "architecture-site",
            }
        ),
        (
            "**/README.md",
            "**/AGENTS.md",
            "**/CLAUDE.md",
        ),
    ),
}


def shared_group_errors(scopes: dict[str, dict[str, list[str]]]) -> list[str]:
    """Verify every shared input group is present in all its member scopes.

    Turns the unavoidable duplication of producer inputs across flat Mergify
    scope lists into a checked invariant, so a shared input added to one lane but
    not the others fails loudly instead of drifting silently (CICD-MRG-03).
    """
    errors: list[str] = []
    for section, groups in (
        ("include", SHARED_INCLUDE_GROUPS),
        ("exclude", SHARED_EXCLUDE_GROUPS),
    ):
        for label, (members, patterns) in groups.items():
            for scope in sorted(members):
                spec = scopes.get(scope)
                if spec is None:
                    errors.append(
                        f"shared {section} group {label!r} references unknown scope "
                        f"{scope!r}"
                    )
                    continue
                present = set(spec[section])
                missing = [pattern for pattern in patterns if pattern not in present]
                if missing:
                    errors.append(
                        f"scope {scope!r} {section} is missing shared {label!r} "
                        f"patterns {missing} (CICD-MRG-03 drift guard): shared "
                        f"inputs must stay identical across every member scope so a "
                        f"producer change revalidates all affected lanes."
                    )
    return errors


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
    # Prose Markdown under a tooling directory is the documentation lane, not an
    # unclassified coverage gap (CICD-MRG-08): `**/*.md` is the documentation
    # catch-all and repository-tooling excludes Markdown.
    ("scripts/dev-notes.md", {"documentation"}),
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
    # Skip non-dict top-level values (for example a future "$schema" string) so a
    # metadata key cannot raise an opaque AttributeError on ``.get``. Only mapping
    # entries describe a component's paths.
    return {
        name: entry.get("paths", [])
        for name, entry in config.items()
        if isinstance(entry, dict)
    }


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
        impact_text = IMPACT_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"Unable to read {IMPACT_PATH.relative_to(ROOT)}: {exc}")
        return 1

    scopes, barriers = parse_impact_scopes(impact_text)
    parsed_names = set(scopes)
    if parsed_names != EXPECTED_SCOPES:
        missing = sorted(EXPECTED_SCOPES - parsed_names)
        unexpected = sorted(parsed_names - EXPECTED_SCOPES)
        print(
            "Parsed .github/ci-impact-scopes.yml scopes do not match the expected scope graph "
            "(CICD-MRG-03). This usually means the file was reindented or "
            "restructured and the parser read it partially."
        )
        if missing:
            print(f"- missing scopes: {missing}")
        if unexpected:
            print(f"- unexpected scopes: {unexpected}")
        print(
            "If this is an intentional scope change, update EXPECTED_SCOPES in "
            f"{Path(__file__).name}."
        )
        return 1
    empty = sorted(name for name, spec in scopes.items() if not spec["include"])
    if empty:
        print(f"Scopes with no include patterns parsed from .github/ci-impact-scopes.yml: {empty}")
        return 1
    if not barriers:
        print("No barrier_files parsed from .github/ci-impact-scopes.yml.")
        return 1

    # 0. Shared-input drift guard (CICD-MRG-03). Run before the scenario table so
    # a half-applied shared edit is reported against the group, not just as a
    # changed scenario outcome.
    errors.extend(shared_group_errors(scopes))

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
            f"tracked path {path!r} is not classified by any CI impact scope or "
            f"barrier (CICD-MRG-08). Map it to a scope in .github/ci-impact-scopes.yml, add it to "
            f"barrier_files if it affects all lanes, or record an explicit owner "
            f"decision in {ALLOWLIST_PATH.name}."
        )

    if errors:
        print("CI impact scope validation failed:")
        for error in errors:
            print(f"- {error}")
        return 1

    print(
        f"CI impact scope graph OK: {len(scopes)} scopes, {len(barriers)} barrier "
        f"patterns, {len(SCENARIOS)} scenarios, "
        f"{len(DETECTOR_EXPECTATIONS)} detector expectations; all tracked files "
        f"classified."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
