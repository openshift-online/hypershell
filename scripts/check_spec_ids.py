#!/usr/bin/env python3
"""Enforce spec identifier rules and report test-to-requirement coverage.

Rules come from Requirement STD-IDS-06 in
specs/standards/platform/spec-identifiers.spec.md. The registry is
specs/ids.md and the old-to-new map is specs/id-rename-map.md.

Default mode checks the rules and exits 1 on any error.
`--report` prints a coverage report (markdown or JSON) built from
`Verifies:` marker comments in tests and always exits 0.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from typing import Dict, Iterable, List, Optional, Sequence, Set, Tuple


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent

SPECS_DIR = "specs/"
REGISTRY_PATH = "specs/ids.md"
RENAME_MAP_PATH = "specs/id-rename-map.md"
IDENTIFIER_SPEC_PATH = "specs/standards/platform/spec-identifiers.spec.md"

# Generated lockfiles carry integrity digests whose base64 bytes can look like
# IDs and nothing in them is human-authored. Keep this list small.
SKIPPED_PATHS = frozenset({"pnpm-lock.yaml", "package-lock.json", "yarn.lock", "go.sum"})
SKIPPED_PREFIXES = ("components/agents/locks/",)
# The checker and its tests hold fake and old IDs as fixtures: rules 5 and 6
# skip them, but their Verifies markers still count.
SELF_PATHS = frozenset({"scripts/check_spec_ids.py", "scripts/test_check_spec_ids.py"})

# Not scanned for citations (rule 5): the registry and the rename map list IDs
# as data.
CITATION_EXEMPT_DATA = frozenset({REGISTRY_PATH, RENAME_MAP_PATH})
# Not scanned for citations (rule 5): these show illustrative example IDs.
CITATION_EXEMPT_EXAMPLES = frozenset(
    {IDENTIFIER_SPEC_PATH, "skills/plan/spec/SKILL.md"}
)
# May mention old IDs (rule 6) because they document the migration.
OLD_ID_ALLOWED = frozenset({RENAME_MAP_PATH, IDENTIFIER_SPEC_PATH})

NON_TEST_METHODS = ("inspection", "analysis", "demonstration")

_CODE = r"[A-Z][A-Z0-9]{2,3}"
_REQ = rf"{_CODE}-{_CODE}-\d{{2,3}}"
_ID = rf"{_REQ}(?:\.\d{{2}})?"
_BEFORE = r"(?<![A-Za-z0-9_-])"
_AFTER = r"(?![A-Za-z0-9_]|-\w)"

REQ_RE = re.compile(rf"^{_REQ}$")
SCENARIO_RE = re.compile(rf"^{_REQ}\.\d{{2}}$")
CITATION_RE = re.compile(rf"{_BEFORE}({_ID}){_AFTER}")
HEADING_START_RE = re.compile(r"^(#{1,6})\s+(Requirement|Legacy compatibility|Scenario)\s*:")
HEADING_RE = re.compile(rf"^(#{{3,4}}) (Requirement|Legacy compatibility|Scenario): (\S+) -- (\S.*)$")
REGISTRY_ROW_RE = re.compile(
    rf"^\|\s*`({_CODE}-{_CODE})`\s*\|\s*`([^`]+)`\s*\|"
)
MARKER_START_RE = re.compile(r"^\s*(?://|#)\s*Verifies:")
MARKER_RE = re.compile(rf"^\s*(?://|#) Verifies: ({_ID}(?:, {_ID})*)\s*$")
VERIFICATION_RE = re.compile(r"^\*\*Verification:\*\*\s*([A-Za-z]+)")
RENAME_ROW_RE = re.compile(r"^\|\s*`([^`]+)`\s*\|\s*`([^`]+)`\s*\|")

Error = Tuple[str, int, str]


# --------------------------------------------------------------------------
# Loading files
# --------------------------------------------------------------------------

def tracked_files(root: Path = REPOSITORY_ROOT) -> List[str]:
    result = subprocess.run(
        ("git", "ls-files", "-z", "--cached"),
        cwd=root,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(f"could not list tracked files: {detail}")
    return [os.fsdecode(p) for p in result.stdout.split(b"\0") if p]


def _skipped(path: str) -> bool:
    return path in SKIPPED_PATHS or path.startswith(SKIPPED_PREFIXES) or (
        path.rsplit("/", 1)[-1] in SKIPPED_PATHS
    )


def load_files(root: Path, paths: Iterable[str]) -> Dict[str, str]:
    """Read text files; binary, missing and skipped files are left out."""
    files: Dict[str, str] = {}
    for rel in paths:
        if _skipped(rel):
            continue
        path = root / rel
        try:
            if path.is_symlink() or not path.is_file():
                continue
            data = path.read_bytes()
        except OSError:
            continue
        if b"\0" in data:
            continue
        files[rel] = data.decode("utf-8", errors="replace")
    return files


# --------------------------------------------------------------------------
# Parsing specs
# --------------------------------------------------------------------------

class Scenario:
    def __init__(self, id_: str, name: str, line: int) -> None:
        self.id, self.name, self.line = id_, name, line


class Requirement:
    def __init__(self, id_: str, name: str, line: int, kind: str) -> None:
        self.id, self.name, self.line, self.kind = id_, name, line, kind
        self.method = "Test"
        self.scenarios: List[Scenario] = []


class SpecFile:
    def __init__(self, path: str) -> None:
        self.path = path
        self.requirements: List[Requirement] = []
        self.retired: Set[str] = set()


def is_spec_file(path: str) -> bool:
    return path.startswith(SPECS_DIR) and path.endswith(".spec.md")


def parse_spec(path: str, text: str) -> Tuple[SpecFile, List[Error]]:
    """Parse headings, verification methods and retired IDs of one spec."""
    spec = SpecFile(path)
    errors: List[Error] = []
    current: Optional[Requirement] = None
    in_fence = False
    in_retired = False
    for number, line in enumerate(text.split("\n"), start=1):
        stripped = line.lstrip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        if line.startswith("#"):
            level = len(line) - len(line.lstrip("#"))
            if level <= 2:
                in_retired = bool(re.match(r"^##\s+Retired IDs\s*$", line))
            if level <= 3 and not HEADING_START_RE.match(line):
                current = None
        # A retired entry is a list item; the IDs before its first ": " are
        # retired, and anything after is a note such as "replaced by X".
        if in_retired and re.match(r"^\s*[-*]\s", line):
            for match in CITATION_RE.finditer(line.split(": ", 1)[0]):
                spec.retired.add(match.group(1))
        if not HEADING_START_RE.match(line):
            if current is not None:
                found = VERIFICATION_RE.match(line)
                if found and found.group(1).lower() in NON_TEST_METHODS:
                    current.method = found.group(1).capitalize()
            continue
        match = HEADING_RE.match(line)
        if not match:
            errors.append((path, number, f"malformed heading (expected '<keyword>: <ID> -- <Name>'): {line.strip()}"))
            continue
        hashes, keyword, id_, name = match.groups()
        if keyword == "Scenario":
            if hashes != "####":
                errors.append((path, number, "scenario heading must use '####'"))
                continue
            if not SCENARIO_RE.match(id_):
                errors.append((path, number, f"malformed scenario ID '{id_}' (expected MODULE-GROUP-NN.NN)"))
                continue
            if current is None:
                errors.append((path, number, f"scenario {id_} is not under a requirement"))
                continue
            if id_.rsplit(".", 1)[0] != current.id:
                errors.append((path, number, f"scenario {id_} does not belong to requirement {current.id}"))
            current.scenarios.append(Scenario(id_, name, number))
        else:
            if hashes != "###":
                errors.append((path, number, f"{keyword.lower()} heading must use '###'"))
                continue
            if not REQ_RE.match(id_):
                errors.append((path, number, f"malformed requirement ID '{id_}' (expected MODULE-GROUP-NN)"))
                current = None
                continue
            current = Requirement(id_, name, number, keyword)
            spec.requirements.append(current)
    return spec, errors


def prefix_of(id_: str) -> str:
    return "-".join(id_.split("-")[:2])


def parse_registry(text: str) -> Tuple[Dict[str, Set[str]], List[Error]]:
    """Return prefix -> set of owning files (relative to specs/)."""
    owners: Dict[str, Set[str]] = {}
    errors: List[Error] = []
    seen: Dict[str, Tuple[str, int]] = {}
    for number, line in enumerate(text.split("\n"), start=1):
        match = REGISTRY_ROW_RE.match(line)
        if not match:
            continue
        prefix, file_ = match.groups()
        # Multi-file prefixes are recorded as one row per file; the same
        # (prefix, file) pair twice is harmless, a different file is not
        # allowed to share the prefix unless the registry lists it as a
        # row too, which STD-IDS-03 forbids.
        if prefix in owners and file_ not in owners[prefix]:
            errors.append((REGISTRY_PATH, number, f"prefix {prefix} registered to more than one file (also line {seen[prefix][1]})"))
        owners.setdefault(prefix, set()).add(file_)
        seen.setdefault(prefix, (file_, number))
    return owners, errors


def parse_rename_map(text: str) -> Dict[str, str]:
    mapping: Dict[str, str] = {}
    for line in text.split("\n"):
        match = RENAME_ROW_RE.match(line)
        if match and match.group(1) != "Old ID":
            mapping[match.group(1)] = match.group(2)
    return mapping


# --------------------------------------------------------------------------
# Markers
# --------------------------------------------------------------------------

class Marker:
    def __init__(self, path: str, line: int, ids: List[str], test: str) -> None:
        self.path, self.line, self.ids, self.test = path, line, ids, test


_QUOTED = r"""["'`]([^"'`]+)["'`]"""
_NAME_PATTERNS = (
    re.compile(r"^\s*func\s+(?:\([^)]*\)\s*)?(\w+)\s*\("),
    re.compile(rf"^\s*(?:\w+\.)*(?:t|s|suite)\.Run\(\s*{_QUOTED}"),
    re.compile(rf"""^\s*(?:await\s+)?(?:test|it|describe|context|specify)(?:\.\w+)*(?:\([^)]*\))?\s*\(\s*{_QUOTED}"""),
    re.compile(r"^\s*(?:async\s+)?def\s+(\w+)\s*\("),
    re.compile(r"^\s*class\s+(\w+)"),
    re.compile(r"^\s*(?:function\s+)?(\w+)\s*\(\)\s*\{?"),
    re.compile(r"^\s*function\s+(\w+)"),
)


def _is_comment(stripped: str) -> bool:
    return stripped.startswith(("//", "#", "/*", "*", "*/"))


def test_name(lines: Sequence[str], index: int) -> str:
    """Name of the test following the marker at lines[index]."""
    for follow in lines[index + 1:]:
        stripped = follow.strip()
        if not stripped or _is_comment(stripped):
            continue
        for pattern in _NAME_PATTERNS:
            match = pattern.match(follow)
            if match:
                return match.group(1)
        return stripped[:100]
    return ""


# Pytest-style helper names must not be collected as tests by unittest.
test_name.__test__ = False  # type: ignore[attr-defined]


def find_markers(files: Dict[str, str]) -> Tuple[List[Marker], List[Error]]:
    """Collect well-formed markers; malformed ones become errors."""
    markers: List[Marker] = []
    errors: List[Error] = []
    for path in sorted(files):
        if path.endswith(".md"):
            continue
        text = files[path]
        if "Verifies:" not in text:
            continue
        lines = text.split("\n")
        for index, line in enumerate(lines):
            if not MARKER_START_RE.match(line):
                continue
            match = MARKER_RE.match(line)
            if not match:
                errors.append((path, index + 1, "malformed marker (expected '// Verifies: ID[, ID]*' or '# Verifies: ID[, ID]*')"))
                continue
            markers.append(Marker(path, index + 1, match.group(1).split(", "), test_name(lines, index)))
    return markers, errors


# --------------------------------------------------------------------------
# Checks
# --------------------------------------------------------------------------

class Analysis:
    def __init__(self) -> None:
        self.specs: List[SpecFile] = []
        self.known: Set[str] = set()
        self.registry: Dict[str, Set[str]] = {}
        self.rename: Dict[str, str] = {}
        self.markers: List[Marker] = []
        self.errors: List[Error] = []


def analyze(files: Dict[str, str]) -> Analysis:
    """Run every rule over an in-memory set of tracked text files."""
    result = Analysis()
    errors = result.errors

    if REGISTRY_PATH in files:
        result.registry, registry_errors = parse_registry(files[REGISTRY_PATH])
        errors.extend(registry_errors)
    if RENAME_MAP_PATH in files:
        result.rename = parse_rename_map(files[RENAME_MAP_PATH])

    # Rules 1-4: headings, duplicates, registry, retired IDs.
    locations: Dict[str, List[Tuple[str, int]]] = {}
    retired: Set[str] = set()
    for path in sorted(files):
        if not is_spec_file(path):
            continue
        spec, spec_errors = parse_spec(path, files[path])
        errors.extend(spec_errors)
        result.specs.append(spec)
        retired |= spec.retired
        for req in spec.requirements:
            locations.setdefault(req.id, []).append((path, req.line))
            for scenario in req.scenarios:
                locations.setdefault(scenario.id, []).append((path, scenario.line))
    if REGISTRY_PATH in files:
        retired |= parse_spec(REGISTRY_PATH, files[REGISTRY_PATH])[0].retired

    for id_, places in sorted(locations.items()):
        if len(places) > 1:
            where = ", ".join(f"{p}:{n}" for p, n in places)
            for path, line in places:
                errors.append((path, line, f"duplicate ID {id_} ({where})"))

    for spec in result.specs:
        relative = spec.path[len(SPECS_DIR):]
        for req in spec.requirements:
            prefix = prefix_of(req.id)
            owners = result.registry.get(prefix)
            if owners is None:
                errors.append((spec.path, req.line, f"prefix {prefix} is not registered in {REGISTRY_PATH}"))
            elif relative not in owners:
                errors.append((spec.path, req.line, f"prefix {prefix} is registered to {', '.join(sorted(owners))}, not this file"))
            for id_ in [req.id] + [s.id for s in req.scenarios]:
                if id_ in retired or req.id in retired:
                    line = req.line if id_ == req.id else next(s.line for s in req.scenarios if s.id == id_)
                    errors.append((spec.path, line, f"retired ID {id_} is reused"))

    result.known = set(locations) | retired

    # Rule 7: markers.
    result.markers, marker_errors = find_markers(files)
    errors.extend(marker_errors)
    marker_lines: Set[Tuple[str, int]] = set()
    for marker in result.markers:
        marker_lines.add((marker.path, marker.line))
        for id_ in marker.ids:
            if id_ not in result.known:
                errors.append((marker.path, marker.line, f"unknown ID {id_} in Verifies marker"))

    # Rules 5 and 6: citations and stale old IDs.
    old_re = None
    if result.rename:
        alternatives = sorted(result.rename, key=len, reverse=True)
        old_re = re.compile(_BEFORE + "(" + "|".join(re.escape(a) for a in alternatives) + ")" + _AFTER)
    registered = set(result.registry)
    for path in sorted(files):
        text = files[path]
        check_citations = path not in CITATION_EXEMPT_DATA and path not in CITATION_EXEMPT_EXAMPLES and path not in SELF_PATHS
        check_old = path not in OLD_ID_ALLOWED and path not in SELF_PATHS
        if not (check_citations or check_old):
            continue
        for number, line in enumerate(text.split("\n"), start=1):
            if check_citations and (path, number) not in marker_lines:
                for match in CITATION_RE.finditer(line):
                    id_ = match.group(1)
                    if prefix_of(id_) in registered and id_ not in result.known:
                        errors.append((path, number, f"unknown ID {id_}"))
            if check_old and old_re is not None:
                for match in old_re.finditer(line):
                    old = match.group(1)
                    errors.append((path, number, f"old ID {old}, now {result.rename[old]}"))

    errors[:] = sorted(set(errors))
    return result


def check_root(root: Path) -> Analysis:
    return analyze(load_files(root, tracked_files(root)))


# --------------------------------------------------------------------------
# Report
# --------------------------------------------------------------------------

def build_report(analysis: Analysis) -> dict:
    citations: Dict[str, List[dict]] = {}
    for marker in analysis.markers:
        for id_ in marker.ids:
            citations.setdefault(id_, []).append(
                {"location": f"{marker.path}:{marker.line}", "test": marker.test}
            )

    totals = {
        "requirements": 0, "requirements_covered": 0, "requirements_non_test": 0,
        "requirements_uncovered": 0, "scenarios": 0, "scenarios_covered": 0,
        "scenarios_non_test": 0, "scenarios_uncovered": 0,
    }
    files_out = []
    uncovered_scenarios = []
    for spec in sorted(analysis.specs, key=lambda s: s.path):
        summary = dict.fromkeys(totals, 0)
        reqs = []
        for req in spec.requirements:
            direct = citations.get(req.id, [])
            scenarios = []
            any_scenario = False
            for scenario in req.scenarios:
                cites = citations.get(scenario.id, [])
                any_scenario = any_scenario or bool(cites)
                scenarios.append({
                    "id": scenario.id, "name": scenario.name, "line": scenario.line,
                    "covered": bool(cites), "citations": cites,
                })
            covered = bool(direct) or any_scenario
            non_test = req.method != "Test"
            summary["requirements"] += 1
            if non_test:
                summary["requirements_non_test"] += 1
            elif covered:
                summary["requirements_covered"] += 1
            else:
                summary["requirements_uncovered"] += 1
            for item in scenarios:
                summary["scenarios"] += 1
                if item["covered"]:
                    summary["scenarios_covered"] += 1
                elif non_test:
                    summary["scenarios_non_test"] += 1
                else:
                    summary["scenarios_uncovered"] += 1
                    uncovered_scenarios.append({"id": item["id"], "name": item["name"], "file": spec.path, "line": item["line"]})
            reqs.append({
                "id": req.id, "name": req.name, "line": req.line, "kind": req.kind,
                "method": req.method, "covered": covered, "citations": direct,
                "scenarios": scenarios,
            })
        for key, value in summary.items():
            totals[key] += value
        files_out.append({
            "path": spec.path,
            "directory": os.path.dirname(spec.path[len(SPECS_DIR):]) or ".",
            "summary": summary,
            "requirements": reqs,
        })
    return {"totals": totals, "files": files_out, "uncovered_scenarios": uncovered_scenarios}


def render_markdown(report: dict) -> str:
    t = report["totals"]
    out = ["# Spec Test Coverage", "", "## Totals", ""]
    out.append(
        f"- Requirements: {t['requirements']} total, {t['requirements_covered']} covered, "
        f"{t['requirements_non_test']} non-Test, {t['requirements_uncovered']} uncovered"
    )
    out.append(
        f"- Scenarios: {t['scenarios']} total, {t['scenarios_covered']} covered, "
        f"{t['scenarios_non_test']} under non-Test requirements, {t['scenarios_uncovered']} uncovered"
    )
    out += ["", "## Per-file summary", "",
            "| File | Reqs | Covered | Non-Test | Uncovered | Scenarios | Covered | Uncovered |",
            "|------|------|---------|----------|-----------|-----------|---------|-----------|"]
    for f in report["files"]:
        s = f["summary"]
        out.append(
            f"| `{f['path'][len(SPECS_DIR):]}` | {s['requirements']} | {s['requirements_covered']} | "
            f"{s['requirements_non_test']} | {s['requirements_uncovered']} | {s['scenarios']} | "
            f"{s['scenarios_covered']} | {s['scenarios_uncovered']} |"
        )
    out += ["", "## Uncovered scenarios", ""]
    if not report["uncovered_scenarios"]:
        out.append("None.")
    for item in report["uncovered_scenarios"]:
        out.append(f"- {item['id']} - {item['name']} ({item['file']}:{item['line']})")

    def cites(items: List[dict]) -> str:
        return "; ".join(f"{c['location']} ({c['test']})" if c["test"] else c["location"] for c in items)

    current_dir = None
    for f in report["files"]:
        if f["directory"] != current_dir:
            current_dir = f["directory"]
            out += ["", f"## specs/{current_dir}/" if current_dir != "." else "## specs/"]
        out += ["", f"### {f['path']}", ""]
        for req in f["requirements"]:
            if req["method"] != "Test":
                state = req["method"]
            else:
                state = "covered" if req["covered"] else "uncovered"
            line = f"- {req['id']} - {req['name']} [{state}]"
            if req["citations"]:
                line += f": {cites(req['citations'])}"
            out.append(line)
            for sc in req["scenarios"]:
                mark = "covered" if sc["covered"] else "uncovered"
                line = f"  - {sc['id']} - {sc['name']} [{mark}]"
                if sc["citations"]:
                    line += f": {cites(sc['citations'])}"
                out.append(line)
    return "\n".join(out) + "\n"


# --------------------------------------------------------------------------
# Entry point
# --------------------------------------------------------------------------

def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--report", action="store_true", help="print a coverage report instead of checking")
    parser.add_argument("--format", choices=("md", "json"), default="md")
    parser.add_argument("--output", help="write the report to this path instead of stdout")
    args = parser.parse_args(argv)

    try:
        analysis = check_root(REPOSITORY_ROOT)
    except RuntimeError as error:
        print(error, file=sys.stderr)
        return 2

    if args.report:
        report = build_report(analysis)
        if args.format == "json":
            body = json.dumps(report, indent=2) + "\n"
        else:
            body = render_markdown(report)
        if args.output:
            Path(args.output).write_text(body, encoding="utf-8")
        else:
            sys.stdout.write(body)
        if analysis.errors:
            print(f"warning: {len(analysis.errors)} spec ID check error(s); run without --report to list them", file=sys.stderr)
        return 0

    if analysis.errors:
        for path, line, message in analysis.errors:
            print(f"{path}:{line}: {message}")
        print(f"\n{len(analysis.errors)} spec ID error(s). See {IDENTIFIER_SPEC_PATH}.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except BrokenPipeError:
        # Output was piped to a reader that closed early (for example head).
        sys.stderr.close()
        sys.exit(0)
