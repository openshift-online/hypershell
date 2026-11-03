#!/usr/bin/env python3
"""Static API Kind conformance gate (SDLC-CONF-05).

Enumerates every registered Extension (`ext`) API Kind from the registration
sources -- the plugin `RegisterPrefixedRoutes(_, "ext", ...)` calls and the RBAC
`extKinds` allow-list -- and fails, naming each non-conformant Kind and each
missing manifest item (SDLC-CONF-01):

- plugin files (model.go, dao.go, service.go, handler.go, presenter.go,
  grpc_handler.go, grpc_presenter.go, plugin.go, migration.go),
- a migration that does NOT call GORM AutoMigrate (SDLC-CONF-02); this is also
  enforced for EVERY plugin migration, `/v1` and `ext` alike,
- an OpenAPI path for the Kind,
- a Go SDK type and a TypeScript SDK type,
- an RBAC `extKinds` allow-list entry (a missing entry is a fail-closed 403 for
  every caller, SEC-RBAC-19),
- a database-backed integration test: coverage by the generic CRUD conformance
  test's fixture (SDLC-CONF-03, SDLC-CONF-06).

CLI verb conformance (SDLC-CONF-01's CLI row) is enforced by the companion
`check-cli-drift` gate in `make check`, which owns the deliberately curated
hsctl extension-command surface (for example Repository is read-only by design,
`scripts/generate-cli.sh`, hsctl-extension-commands.spec.md). Duplicating that
curation here would produce false positives, so this gate defers to it.

The check runs unconditionally as a whole-repository policy check.
"""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
API = ROOT / "components" / "api-server"
PLUGINS = API / "plugins"
AUTHZ_GO = API / "pkg" / "rbac" / "authorization.go"
OPENAPI = API / "pkg" / "api" / "openapi" / "api" / "openapi.yaml"
CONFORMANCE_TEST = API / "test" / "authz" / "ext_crud_conformance_test.go"
SDK_GO = ROOT / "components" / "sdk-go" / "types"
SDK_TS = ROOT / "components" / "sdk-typescript" / "src"

# The uniform plugin file set every ext Kind carries. mock_dao.go is listed in
# the manifest prose but is not used by any plugin in this repository, so it is
# not enforced here.
REQUIRED_PLUGIN_FILES = (
    "model.go",
    "dao.go",
    "service.go",
    "handler.go",
    "presenter.go",
    "grpc_handler.go",
    "grpc_presenter.go",
    "plugin.go",
    "migration.go",
)

AUTOMIGRATE_CALL = re.compile(r"\.AutoMigrate\(")
# The ext registration key argument is a plugin-chosen label, not the Kind path;
# the authoritative plural path is the presenters.RegisterPath second argument.
REGISTER_PATH = re.compile(r'RegisterPath\([A-Za-z0-9]+\{\},\s*"([a-z0-9_]+)"')
EXT_REGISTRATION = re.compile(r'RegisterPrefixedRoutes\([^)]*,\s*"ext"\s*,')
MODEL_TYPE = re.compile(r"^type\s+([A-Za-z0-9]+)\s+struct", re.MULTILINE)


def read(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except OSError:
        return ""


def path_to_plugin() -> dict[str, Path]:
    """Map each Kind plural path to its plugin directory via the plugin's
    presenters.RegisterPath call (a registration source, SDLC-CONF-01)."""
    kinds: dict[str, Path] = {}
    for plugin_go in sorted(PLUGINS.glob("*/plugin.go")):
        for match in REGISTER_PATH.finditer(read(plugin_go)):
            kinds.setdefault(match.group(1), plugin_go.parent)
    return kinds


def ext_serving_plugins() -> set[Path]:
    """Plugin directories that register a route on the ext prefix."""
    return {
        plugin_go.parent
        for plugin_go in PLUGINS.glob("*/plugin.go")
        if EXT_REGISTRATION.search(read(plugin_go))
    }


def ext_kinds_allowlist() -> set[str]:
    """Parse the extKinds allow-list map keys from authorization.go."""
    text = read(AUTHZ_GO)
    block = re.search(r"var extKinds = map\[string\]bool\{(.*?)\}", text, re.DOTALL)
    if not block:
        return set()
    return set(re.findall(r'"([a-z0-9_]+)":\s*true', block.group(1)))


def conformance_fixtures() -> set[str]:
    """Parse the ext Kind keys the generic CRUD conformance test covers."""
    text = read(CONFORMANCE_TEST)
    block = re.search(r"var crudFixtures = map\[string\].*?\{(.*)\n\}", text, re.DOTALL)
    if not block:
        return set()
    return set(re.findall(r'"([a-z0-9_]+)":\s*func', block.group(1)))


def model_type_name(plugin_dir: Path) -> str:
    match = MODEL_TYPE.search(read(plugin_dir / "model.go"))
    return match.group(1) if match else ""


def sdk_has_type(type_name: str) -> tuple[bool, bool]:
    go_pat = re.compile(r"\btype\s+" + re.escape(type_name) + r"\b")
    ts_pat = re.compile(r"\b(class|interface|type)\s+" + re.escape(type_name) + r"\b")
    go_ok = any(go_pat.search(read(f)) for f in SDK_GO.glob("*.go"))
    ts_ok = any(ts_pat.search(read(f)) for f in SDK_TS.glob("*.ts"))
    return go_ok, ts_ok


def check_no_automigrate() -> list[str]:
    """SDLC-CONF-02: no plugin migration may call AutoMigrate (all Kinds)."""
    errors: list[str] = []
    for migration in sorted(PLUGINS.glob("*/*migration*.go")):
        text = read(migration)
        for i, line in enumerate(text.splitlines(), start=1):
            # Ignore comment lines; only a real call is a violation.
            stripped = line.lstrip()
            if stripped.startswith("//"):
                continue
            if AUTOMIGRATE_CALL.search(line):
                kind = migration.parent.name
                try:
                    rel = migration.relative_to(ROOT)
                except ValueError:
                    rel = migration
                errors.append(
                    f"{kind}: migration {rel}:{i} calls AutoMigrate; use "
                    f"dbmigrate.CreateTable / raw tx.Exec DDL (SDLC-CONF-02)"
                )
    return errors


def main() -> int:
    if not PLUGINS.is_dir():
        print(f"plugin directory not found: {PLUGINS}")
        return 1

    all_paths = path_to_plugin()
    ext_plugin_dirs = ext_serving_plugins()
    allowlist = ext_kinds_allowlist()
    fixtures = conformance_fixtures()
    openapi = read(OPENAPI)

    # Kinds a plugin actually serves on the ext prefix (plural -> plugin dir).
    ext_served = {
        kind: plugin_dir
        for kind, plugin_dir in all_paths.items()
        if plugin_dir in ext_plugin_dirs
    }

    # Registered ext Kinds = union of the plugins serving an ext route and the
    # RBAC allow-list, so a Kind present in only one source is still evaluated
    # (a served Kind missing from the allow-list is a latent 403; an allow-list
    # entry with no serving plugin is an orphan).
    registered = sorted(set(ext_served) | allowlist)

    errors: list[str] = []
    errors.extend(check_no_automigrate())

    for kind in registered:
        problems: list[str] = []
        plugin_dir = ext_served.get(kind) or all_paths.get(kind)

        if plugin_dir is None:
            problems.append(
                "in the RBAC extKinds allow-list but no plugin serves it "
                "(orphaned entry)"
            )
        else:
            for filename in REQUIRED_PLUGIN_FILES:
                if not (plugin_dir / filename).is_file():
                    problems.append(f"missing plugin file {filename}")

        if kind in ext_served and kind not in allowlist:
            problems.append(
                "missing RBAC extKinds allow-list entry (fail-closed 403 for "
                "every caller, SEC-RBAC-19)"
            )

        if f"/api/hypershell/ext/{kind}:" not in openapi:
            problems.append(f"missing OpenAPI path /api/hypershell/ext/{kind}")

        if plugin_dir is not None:
            type_name = model_type_name(plugin_dir)
            if not type_name:
                problems.append("could not resolve the model type name from model.go")
            else:
                go_ok, ts_ok = sdk_has_type(type_name)
                if not go_ok:
                    problems.append(f"missing Go SDK type {type_name} in components/sdk-go")
                if not ts_ok:
                    problems.append(f"missing TypeScript SDK type {type_name} in components/sdk-typescript")

        if kind not in fixtures:
            problems.append(
                "missing database-backed integration test: no fixture in the "
                "generic CRUD conformance test "
                "(test/authz/ext_crud_conformance_test.go, SDLC-CONF-06)"
            )

        for problem in problems:
            errors.append(f"{kind}: {problem}")

    if errors:
        print("API Kind conformance check FAILED (SDLC-CONF). Non-conformant Kinds:")
        for error in errors:
            print(f"  - {error}")
        print(
            "\nEvery registered API Kind must satisfy the Kind Completeness "
            "Manifest (specs/standards/sdlc/kind-conformance.spec.md)."
        )
        return 1

    print(f"API Kind conformance check passed: {len(registered)} ext Kinds conform.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
