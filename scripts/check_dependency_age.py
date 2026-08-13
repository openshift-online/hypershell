#!/usr/bin/env python3
"""Reject resolved Go and npm dependencies newer than a minimum age.

Adapted for HyperShell from:
https://github.com/jsell-rh/rh-trex-ai/blob/main/scripts/check-dependency-age.py
"""

import argparse
import concurrent.futures
import datetime as dt
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


UTC = dt.timezone.utc
PSEUDO_VERSION_RE = re.compile(r"-(\d{14})-[0-9a-f]{12,}(?:\.\d+)?$")
EXACT_NPM_VERSION_RE = re.compile(
    r"^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$"
)
ALLOWLIST_PATH = "dependency-age-allowlist.json"
TOOLS_PATH = "dependency-age-tools.json"
EXCLUDED_PARTS = {".claude", ".git", "generated", "node_modules", "tmp", "vendor"}


def parse_time(value):
    if not value or value == "0001-01-01T00:00:00Z":
        return None
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return dt.datetime.fromisoformat(value).astimezone(UTC)


def fetch_json(url):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https":
        raise RuntimeError("dependency metadata URL must use HTTPS")
    if shutil.which("curl"):
        result = subprocess.run(
            [
                "curl",
                "-fsSL",
                "--proto",
                "=https",
                "--tlsv1.2",
                "--retry",
                "3",
                "--retry-all-errors",
                "--retry-delay",
                "1",
                "--connect-timeout",
                "10",
                "--max-time",
                "30",
                url,
            ],
            check=True,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        return json.loads(result.stdout)
    request = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))


def repository_files(root, pattern):
    return [
        path
        for path in sorted(root.glob(pattern))
        if not EXCLUDED_PARTS.intersection(path.relative_to(root).parts)
    ]


def npm_name_from_lock_path(package_path):
    marker = "node_modules/"
    if marker not in package_path:
        return None
    name = package_path.rsplit(marker, 1)[1]
    return name or None


def npm_package_versions(lockfile):
    data = json.loads(lockfile.read_text(encoding="utf-8"))
    packages = data.get("packages")
    if not isinstance(packages, dict):
        raise RuntimeError(f"{lockfile} does not contain a package-lock packages map")
    versions = set()
    for package_path, package in packages.items():
        if not package_path or not isinstance(package, dict) or package.get("link"):
            continue
        name = npm_name_from_lock_path(package_path)
        version = package.get("version")
        if name and version:
            versions.add((name, str(version), str(lockfile)))
    return versions


def exact_npm_declaration_failures(lockfile):
    data = json.loads(lockfile.read_text(encoding="utf-8"))
    root_package = data.get("packages", {}).get("", {})
    failures = []
    for group in (
        "dependencies",
        "devDependencies",
        "optionalDependencies",
        "peerDependencies",
    ):
        dependencies = root_package.get(group, {})
        if not isinstance(dependencies, dict):
            failures.append(f"{lockfile} root {group} must be an object")
            continue
        for name, version in sorted(dependencies.items()):
            if not EXACT_NPM_VERSION_RE.fullmatch(str(version)):
                failures.append(
                    f"npm {name}@{version} from {lockfile} is not an exact semantic version"
                )
    return failures


def yaml_scalar(value):
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    if len(value) >= 2 and value[0] == value[-1] == '"':
        return json.loads(value)
    return value.split(" #", 1)[0].strip()


def pnpm_package_reference(key):
    key = yaml_scalar(key)
    name, separator, version = key.rpartition("@")
    if not separator or not name or not EXACT_NPM_VERSION_RE.fullmatch(version):
        raise RuntimeError(
            f"pnpm package key {key!r} is not an exact registry package reference"
        )
    return name, version


def pnpm_lock_data(lockfile):
    lines = lockfile.read_text(encoding="utf-8").splitlines()
    packages = set()
    importers = {}
    section = None
    importer = None
    dependency_group = None
    dependency = None
    dependency_groups = {
        "dependencies",
        "devDependencies",
        "optionalDependencies",
        "peerDependencies",
    }

    for line in lines:
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip())
        stripped = line.strip()

        if indent == 0:
            section = stripped[:-1] if stripped.endswith(":") else None
            importer = None
            dependency_group = None
            dependency = None
            continue

        if section == "packages" and indent == 2 and stripped.endswith(":"):
            name, version = pnpm_package_reference(stripped[:-1])
            packages.add((name, version, str(lockfile)))
            continue

        if section != "importers":
            continue
        if indent == 2 and ":" in stripped:
            importer = yaml_scalar(stripped.split(":", 1)[0])
            importers.setdefault(importer, {})
            dependency_group = None
            dependency = None
            continue
        if importer is None:
            continue
        if indent == 4 and stripped.endswith(":"):
            candidate = yaml_scalar(stripped[:-1])
            dependency_group = candidate if candidate in dependency_groups else None
            dependency = None
            continue
        if dependency_group and indent == 6 and stripped.endswith(":"):
            dependency = yaml_scalar(stripped[:-1])
            continue
        if dependency and indent == 8 and stripped.startswith("specifier:"):
            specifier = yaml_scalar(stripped.split(":", 1)[1])
            importers[importer][dependency] = specifier

    if not importers:
        raise RuntimeError(f"{lockfile} does not contain a pnpm importer map")
    if not packages:
        raise RuntimeError(f"{lockfile} does not contain a pnpm package map")
    return packages, importers


def exact_pnpm_declaration_failures(lockfile, root, importers):
    failures = []
    package_manifests = repository_files(root, "**/package.json")
    expected_importers = {
        "." if manifest.parent == root else manifest.parent.relative_to(root).as_posix()
        for manifest in package_manifests
    }
    actual_importers = set(importers)

    for missing in sorted(expected_importers - actual_importers):
        failures.append(f"pnpm workspace package {missing} is missing from {lockfile}")
    for extra in sorted(actual_importers - expected_importers):
        failures.append(f"pnpm importer {extra} in {lockfile} has no package.json")

    for importer in sorted(expected_importers & actual_importers):
        manifest = root / "package.json" if importer == "." else root / importer / "package.json"
        try:
            package = json.loads(manifest.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            failures.append(f"{manifest} is not valid JSON: {exc}")
            continue

        declared = {}
        for group in (
            "dependencies",
            "devDependencies",
            "optionalDependencies",
            "peerDependencies",
        ):
            dependencies = package.get(group, {})
            if not isinstance(dependencies, dict):
                failures.append(f"{manifest} {group} must be an object")
                continue
            declared.update((name, str(version)) for name, version in dependencies.items())

        locked = importers[importer]
        for name, specifier in sorted(declared.items()):
            if specifier.startswith("workspace:"):
                workspace_version = specifier.removeprefix("workspace:")
                if not EXACT_NPM_VERSION_RE.fullmatch(workspace_version):
                    failures.append(
                        f"pnpm {name}@{specifier} from {manifest} must use an exact workspace version"
                    )
            elif not EXACT_NPM_VERSION_RE.fullmatch(specifier):
                failures.append(
                    f"npm {name}@{specifier} from {manifest} is not an exact semantic version"
                )
            if locked.get(name) != specifier:
                failures.append(
                    f"pnpm importer {importer} does not lock {name}@{specifier} exactly"
                )
        for name in sorted(set(locked) - set(declared)):
            failures.append(
                f"pnpm importer {importer} contains undeclared dependency {name}"
            )

    root_manifest = root / "package.json"
    if root_manifest.exists():
        try:
            package_manager = json.loads(root_manifest.read_text(encoding="utf-8")).get(
                "packageManager", ""
            )
        except json.JSONDecodeError:
            package_manager = ""
        manager, separator, version = str(package_manager).partition("@")
        if manager != "pnpm" or not separator or not EXACT_NPM_VERSION_RE.fullmatch(version):
            failures.append(
                f"{root_manifest} packageManager must pin an exact pnpm semantic version"
            )
    return failures


def npm_published_at(name, version):
    escaped_name = urllib.parse.quote(name, safe="")
    data = fetch_json(f"https://registry.npmjs.org/{escaped_name}")
    try:
        return parse_time(data["time"][version])
    except KeyError as exc:
        raise RuntimeError(
            f"npm metadata for {name}@{version} did not include a publish time"
        ) from exc


def module_reference(module):
    reference = module["Path"]
    if module.get("Version"):
        reference += f"@{module['Version']}"
    return reference


def is_within(path, root):
    return path == root or root in path.parents


def go_modules(go_mod, repository_root=None):
    repository_root = (repository_root or go_mod.parent).resolve()
    env = os.environ.copy()
    env["GOWORK"] = "off"

    edit = subprocess.run(
        ["go", "mod", "edit", "-json"],
        cwd=go_mod.parent,
        env=env,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    replacements = json.loads(edit.stdout).get("Replace") or []
    local_replacements = [
        replacement
        for replacement in replacements
        if replacement.get("New", {}).get("Path")
        and not replacement.get("New", {}).get("Version")
    ]

    temporary = None
    working_directory = go_mod.parent
    module_mode = "-mod=readonly"
    if not go_mod.with_name("go.sum").exists() or local_replacements:
        temporary = tempfile.TemporaryDirectory()
        working_directory = Path(temporary.name)
        module_mode = "-mod=mod"
        shutil.copy2(go_mod, working_directory / "go.mod")
        if go_mod.with_name("go.sum").exists():
            shutil.copy2(go_mod.with_name("go.sum"), working_directory / "go.sum")

        for replacement in local_replacements:
            old_reference = module_reference(replacement["Old"])
            replacement_path = Path(replacement["New"]["Path"])
            if not replacement_path.is_absolute():
                replacement_path = go_mod.parent / replacement_path
            replacement_path = replacement_path.resolve()

            if is_within(replacement_path, repository_root):
                edit_argument = f"-replace={old_reference}={replacement_path}"
            else:
                edit_argument = f"-dropreplace={old_reference}"
            subprocess.run(
                ["go", "mod", "edit", edit_argument],
                cwd=working_directory,
                env=env,
                check=True,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )

    try:
        result = subprocess.run(
            ["go", "list", module_mode, "-m", "-json", "all"],
            cwd=working_directory,
            env=env,
            check=True,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    finally:
        if temporary is not None:
            temporary.cleanup()

    modules = []
    decoder = json.JSONDecoder()
    output = result.stdout.lstrip()
    while output:
        module, index = decoder.raw_decode(output)
        output = output[index:].lstrip()
        if module.get("Main"):
            continue
        replacement = module.get("Replace") or {}
        if replacement.get("Dir"):
            continue
        resolved = replacement if replacement.get("Version") else module
        if not resolved.get("Version"):
            continue
        modules.append(
            (
                resolved["Path"],
                resolved["Version"],
                resolved.get("Time"),
                str(go_mod),
            )
        )
    return modules


def go_tool_module(name, version, root):
    env = os.environ.copy()
    env["GOWORK"] = "off"
    result = subprocess.run(
        ["go", "list", "-m", "-json", f"{name}@{version}"],
        cwd=root,
        env=env,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    module = json.loads(result.stdout)
    return module["Path"], module["Version"], module.get("Time"), str(root / TOOLS_PATH)


def pseudo_version_time(version):
    match = PSEUDO_VERSION_RE.search(version)
    if not match:
        return None
    return dt.datetime.strptime(match.group(1), "%Y%m%d%H%M%S").replace(tzinfo=UTC)


def go_published_at(path, version, module_time):
    parsed = parse_time(module_time)
    if parsed is not None:
        return parsed
    parsed = pseudo_version_time(version)
    if parsed is not None:
        return parsed
    raise RuntimeError(f"Go metadata for {path}@{version} did not include a publish time")


def load_allowlist(root):
    path = root / ALLOWLIST_PATH
    if not path.exists():
        return set(), []
    try:
        entries = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return set(), [f"{ALLOWLIST_PATH} is not valid JSON: {exc}"]
    if not isinstance(entries, list):
        return set(), [f"{ALLOWLIST_PATH} must contain a JSON array"]

    allowed = set()
    failures = []
    required_fields = (
        "kind",
        "name",
        "version",
        "reason",
        "compensatingVerification",
    )
    for index, entry in enumerate(entries):
        if not isinstance(entry, dict):
            failures.append(f"{ALLOWLIST_PATH} entry {index} must be an object")
            continue
        missing = [
            field
            for field in required_fields
            if not isinstance(entry.get(field), str) or not entry[field].strip()
        ]
        if missing:
            failures.append(
                f"{ALLOWLIST_PATH} entry {index} is missing required field(s): "
                f"{', '.join(missing)}"
            )
            continue
        kind = entry["kind"].strip()
        if kind not in {"go", "npm"}:
            failures.append(
                f"{ALLOWLIST_PATH} entry {index} has unsupported kind {kind!r}"
            )
            continue
        allowed.add((kind, entry["name"].strip(), entry["version"].strip()))
    return allowed, failures


def load_tools(root):
    path = root / TOOLS_PATH
    if not path.exists():
        return [], []
    try:
        entries = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return [], [f"{TOOLS_PATH} is not valid JSON: {exc}"]
    if not isinstance(entries, list):
        return [], [f"{TOOLS_PATH} must contain a JSON array"]

    tools = []
    failures = []
    for index, entry in enumerate(entries):
        if not isinstance(entry, dict):
            failures.append(f"{TOOLS_PATH} entry {index} must be an object")
            continue
        missing = [
            field
            for field in ("kind", "name", "version")
            if not isinstance(entry.get(field), str) or not entry[field].strip()
        ]
        if missing:
            failures.append(
                f"{TOOLS_PATH} entry {index} is missing required field(s): {', '.join(missing)}"
            )
            continue
        kind, name, version = (
            entry[field].strip() for field in ("kind", "name", "version")
        )
        if kind not in {"go", "npm"}:
            failures.append(f"{TOOLS_PATH} entry {index} has unsupported kind {kind!r}")
            continue
        if kind == "npm" and not EXACT_NPM_VERSION_RE.fullmatch(version):
            failures.append(
                f"{TOOLS_PATH} entry {index} npm version is not exact: {version!r}"
            )
            continue
        if kind == "go" and not version.startswith("v"):
            failures.append(
                f"{TOOLS_PATH} entry {index} Go version is not exact: {version!r}"
            )
            continue
        tools.append((kind, name, version, str(path)))
    return tools, failures


def check_age(kind, name, version, published_at, source, cutoff, allowlist):
    if published_at is None:
        return f"{kind} {name}@{version} from {source} has no known publish time"
    if published_at > cutoff and (kind, name, version) not in allowlist:
        return (
            f"{kind} {name}@{version} from {source} was published "
            f"{published_at.isoformat()} after cutoff {cutoff.isoformat()}"
        )
    return None


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Reject npm and Go dependencies newer than the allowed age."
    )
    parser.add_argument("--min-age-days", type=int, default=14)
    parser.add_argument(
        "--now", help="UTC timestamp override, for example 2026-06-20T00:00:00Z"
    )
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--skip-npm", action="store_true")
    parser.add_argument("--skip-go", action="store_true")
    args = parser.parse_args(argv)

    if args.min_age_days < 0:
        parser.error("--min-age-days must not be negative")
    root = args.root.resolve()
    try:
        now = parse_time(args.now) if args.now else dt.datetime.now(UTC)
    except ValueError as exc:
        parser.error(f"--now must be an ISO-8601 timestamp: {exc}")
    if now is None:
        parser.error("--now must contain a non-zero ISO-8601 timestamp")
    cutoff = now - dt.timedelta(days=args.min_age_days)
    allowlist, failures = load_allowlist(root)
    tools, tool_failures = load_tools(root)
    failures.extend(tool_failures)

    if not args.skip_npm:
        npm_versions = set()
        package_locks = repository_files(root, "**/package-lock.json")
        pnpm_locks = repository_files(root, "**/pnpm-lock.yaml")
        if len(pnpm_locks) > 1 or (pnpm_locks and pnpm_locks[0] != root / "pnpm-lock.yaml"):
            failures.append("pnpm must use one shared lockfile at the repository root")
        if pnpm_locks and package_locks:
            failures.append("package-lock.json is prohibited after the pnpm workspace migration")

        for lockfile in package_locks:
            try:
                npm_versions.update(npm_package_versions(lockfile))
                failures.extend(exact_npm_declaration_failures(lockfile))
            except (json.JSONDecodeError, RuntimeError) as exc:
                failures.append(str(exc))
        for lockfile in pnpm_locks:
            try:
                versions, importers = pnpm_lock_data(lockfile)
                npm_versions.update(versions)
                failures.extend(
                    exact_pnpm_declaration_failures(lockfile, root, importers)
                )
            except (json.JSONDecodeError, RuntimeError) as exc:
                failures.append(str(exc))
        for kind, name, version, source in tools:
            if kind == "npm":
                npm_versions.add((name, version, source))

        def check_npm(item):
            name, version, source = item
            try:
                return check_age(
                    "npm",
                    name,
                    version,
                    npm_published_at(name, version),
                    source,
                    cutoff,
                    allowlist,
                )
            except (
                RuntimeError,
                ValueError,
                json.JSONDecodeError,
                urllib.error.URLError,
                TimeoutError,
                subprocess.CalledProcessError,
            ) as exc:
                return f"npm {name}@{version} from {source} metadata check failed: {exc}"

        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as executor:
            for failure in executor.map(check_npm, sorted(npm_versions)):
                if failure:
                    failures.append(failure)

    if not args.skip_go:
        for go_mod in repository_files(root, "**/go.mod"):
            try:
                modules = go_modules(go_mod, root)
            except (json.JSONDecodeError, subprocess.CalledProcessError) as exc:
                details = getattr(exc, "stderr", "") or str(exc)
                failures.append(f"Go module graph failed for {go_mod}: {details.strip()}")
                continue
            for path, version, module_time, source in modules:
                try:
                    failure = check_age(
                        "go",
                        path,
                        version,
                        go_published_at(path, version, module_time),
                        source,
                        cutoff,
                        allowlist,
                    )
                except (RuntimeError, ValueError) as exc:
                    failure = str(exc)
                if failure:
                    failures.append(failure)

        for _, name, version, _ in (tool for tool in tools if tool[0] == "go"):
            try:
                path, resolved_version, module_time, source = go_tool_module(
                    name, version, root
                )
                failure = check_age(
                    "go",
                    path,
                    resolved_version,
                    go_published_at(path, resolved_version, module_time),
                    source,
                    cutoff,
                    allowlist,
                )
            except (
                RuntimeError,
                ValueError,
                json.JSONDecodeError,
                subprocess.CalledProcessError,
            ) as exc:
                failure = f"go {name}@{version} metadata check failed: {exc}"
            if failure:
                failures.append(failure)

    if failures:
        print("dependency age check failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1

    print(
        "dependency age check passed; all checked versions were published on or before "
        f"{cutoff.isoformat()}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
