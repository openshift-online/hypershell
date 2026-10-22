#!/usr/bin/env python3
"""Build the combined unit + e2e coverage report for `make coverage`.

Spec: specs/platform/local-development.spec.md DEV-LOC-18.

Inputs, all under COVERAGE_DIR (written by scripts/code-coverage/run.sh):
  unit/go/<component>.out              Go unit-test profiles (go test -coverprofile)
  e2e/go.out                           Go e2e profile (go tool covdata textfmt)
  unit/frontend/<package>/coverage-summary.json   vitest json-summary output
  e2e/bff-summary.json                 BFF unit/e2e/both line totals (bff_report.mjs)

Outputs:
  report/summary.md                    Markdown summary (also printed)
  report/go/<component>.out            Merged unit + e2e Go profile per component

A Go block counts as covered when either run executed it. Generated code is
reported separately so it does not hide the hand-written code's numbers.
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path

MODULE_PREFIX = "github.com/openshift-online/hypershell/components/"
GO_COMPONENTS = ("api-server", "control-plane", "cli")

# Package path prefixes (relative to components/) holding generated code.
GENERATED_PREFIXES = (
    "api-server/pkg/api/openapi",
    "api-server/pkg/api/grpc",
    "cli/data/generated",
)

# Package path prefixes holding test helpers rather than product code; skipped.
TEST_SUPPORT_PREFIXES = ("api-server/test",)


@dataclass
class Block:
    """One coverage block: statement count and the hit flag per source."""

    stmts: int
    unit: bool = False
    e2e: bool = False


@dataclass
class Totals:
    stmts: int = 0
    unit: int = 0
    e2e: int = 0
    both: int = 0

    def add(self, block: Block) -> None:
        self.stmts += block.stmts
        self.unit += block.stmts if block.unit else 0
        self.e2e += block.stmts if block.e2e else 0
        self.both += block.stmts if (block.unit or block.e2e) else 0


@dataclass
class GoCoverage:
    blocks: dict[str, Block] = field(default_factory=dict)
    # Components that appear in the e2e data. The others show "not run".
    e2e_components: set[str] = field(default_factory=set)

    def load(self, profile: Path, source: str) -> None:
        """Merge a Go cover profile; source is 'unit' or 'e2e'."""
        for raw in profile.read_text().splitlines():
            line = raw.strip()
            if not line or line.startswith("mode:"):
                continue
            key, stmts, count = line.rsplit(" ", 2)
            block = self.blocks.setdefault(key, Block(stmts=int(stmts)))
            if int(count) > 0:
                setattr(block, source, True)
            package = package_of(key)
            if source == "e2e" and package is not None:
                self.e2e_components.add(package.split("/", 1)[0])


def package_of(key: str) -> str | None:
    """Return 'component/pkg/path' for a profile key, or None if outside the repo."""
    file_path = key.split(":", 1)[0]
    if not file_path.startswith(MODULE_PREFIX):
        return None
    package = file_path[len(MODULE_PREFIX):].rsplit("/", 1)[0]
    if any(under(package, prefix) for prefix in TEST_SUPPORT_PREFIXES):
        return None
    return package


def under(package: str, prefix: str) -> bool:
    """True when package is prefix itself or nested below it."""
    return package == prefix or package.startswith(prefix + "/")


def is_generated(package: str) -> bool:
    return any(under(package, prefix) for prefix in GENERATED_PREFIXES)


def summarize(cov: GoCoverage) -> tuple[dict[str, Totals], dict[str, Totals], dict[str, Totals]]:
    """Return (per package, per component hand-written, per component generated)."""
    packages: dict[str, Totals] = {}
    written: dict[str, Totals] = {}
    generated: dict[str, Totals] = {}
    for key, block in cov.blocks.items():
        package = package_of(key)
        if package is None:
            continue
        component = package.split("/", 1)[0]
        packages.setdefault(package, Totals()).add(block)
        bucket = generated if is_generated(package) else written
        bucket.setdefault(component, Totals()).add(block)
    return packages, written, generated


def write_merged_profiles(cov: GoCoverage, out_dir: Path) -> list[Path]:
    """Write one merged profile per component for `go tool cover -html`."""
    out_dir.mkdir(parents=True, exist_ok=True)
    lines: dict[str, list[str]] = {}
    for key, block in sorted(cov.blocks.items()):
        package = package_of(key)
        if package is None:
            continue
        component = package.split("/", 1)[0]
        hit = 1 if (block.unit or block.e2e) else 0
        lines.setdefault(component, []).append(f"{key} {block.stmts} {hit}")
    written = []
    for component, body in lines.items():
        path = out_dir / f"{component}.out"
        path.write_text("mode: set\n" + "\n".join(body) + "\n")
        written.append(path)
    return written


def pct(part: int, whole: int) -> str:
    return f"{100 * part / whole:.1f}%" if whole else "-"


def go_section(cov: GoCoverage) -> list[str]:
    packages, written, generated = summarize(cov)

    def e2e_cell(name: str, t: Totals) -> str:
        """E2E coverage, or "not run" when no e2e data reached that component."""
        component = name.split("/", 1)[0]
        return pct(t.e2e, t.stmts) if component in cov.e2e_components else "not run"

    out = [
        "## Go services",
        "",
        "Hand-written code. A block counts as covered when either run executed it.",
        "",
        "| Component | Statements | Unit | E2E | Unit + e2e |",
        "|---|---|---|---|---|",
    ]
    for component in GO_COMPONENTS:
        t = written.get(component)
        if t is None:
            continue
        e2e = e2e_cell(component, t)
        out.append(f"| {component} | {t.stmts} | {pct(t.unit, t.stmts)} | {e2e} | **{pct(t.both, t.stmts)}** |")
    if generated:
        out += ["", "Generated code (reported apart):", "", "| Component | Statements | Unit + e2e |", "|---|---|---|"]
        for component, t in sorted(generated.items()):
            out.append(f"| {component} | {t.stmts} | {pct(t.both, t.stmts)} |")
    out += [
        "",
        "### Per package (hand-written, lowest combined coverage first)",
        "",
        "| Package | Statements | Unit | E2E | Unit + e2e |",
        "|---|---|---|---|---|",
    ]
    rows = [(p, t) for p, t in packages.items() if not is_generated(p) and t.stmts]
    rows.sort(key=lambda r: (r[1].both / r[1].stmts, r[0]))
    for package, t in rows:
        e2e = e2e_cell(package, t)
        out.append(f"| {package} | {t.stmts} | {pct(t.unit, t.stmts)} | {e2e} | {pct(t.both, t.stmts)} |")
    return out


def frontend_section(frontend_dir: Path, bff_summary: Path) -> list[str]:
    out = ["## Web frontend", ""]
    summaries = sorted(frontend_dir.glob("*/coverage-summary.json")) if frontend_dir.is_dir() else []
    if summaries:
        out += [
            "Unit tests, measured over all source files (not only the files CI gates on).",
            "",
            "| Package | Lines | Line coverage | Branch coverage |",
            "|---|---|---|---|",
        ]
        for path in summaries:
            total = json.loads(path.read_text())["total"]
            out.append(
                f"| {path.parent.name} | {total['lines']['total']} | {total['lines']['pct']}% | {total['branches']['pct']}% |"
            )
    else:
        out.append("Frontend unit tests were not run.")
    out.append("")
    if bff_summary.is_file():
        s = json.loads(bff_summary.read_text())
        out += [
            "### Web console BFF (unit + e2e)",
            "",
            "| Lines | Unit | E2E | Unit + e2e |",
            "|---|---|---|---|",
            f"| {s['lines']} | {pct(s['unit'], s['lines'])} | {pct(s['e2e'], s['lines'])} | **{pct(s['both'], s['lines'])}** |",
            "",
            "Browser-side React code is not measured during e2e.",
        ]
    else:
        out.append("BFF e2e coverage was not collected.")
    return out


def build_report(coverage_dir: Path) -> str:
    cov = GoCoverage()
    for component in GO_COMPONENTS:
        profile = coverage_dir / "unit" / "go" / f"{component}.out"
        if profile.is_file():
            cov.load(profile, "unit")
    # Cluster services and the hsctl smoke test, merged by covdata textfmt.
    e2e_profile = coverage_dir / "e2e" / "go.out"
    if e2e_profile.is_file():
        cov.load(e2e_profile, "e2e")
    write_merged_profiles(cov, coverage_dir / "report" / "go")
    lines = ["# HyperShell coverage report", ""]
    lines += go_section(cov)
    lines += [""]
    lines += frontend_section(coverage_dir / "unit" / "frontend", coverage_dir / "e2e" / "bff-summary.json")
    return "\n".join(lines) + "\n"


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("coverage_dir", type=Path)
    args = parser.parse_args(argv)
    report = build_report(args.coverage_dir)
    out = args.coverage_dir / "report" / "summary.md"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(report)
    print(report)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
