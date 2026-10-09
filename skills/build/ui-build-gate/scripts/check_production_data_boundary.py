#!/usr/bin/env python3
"""Check that production loads real data while Storybook owns fixtures."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


FORBIDDEN_RUNTIME_IMPORT = re.compile(
    r"(?:from\s+|import\s*)[\"'][^\"']*(?:fixtures?|mocks?|stories|storybook)[^\"']*[\"']",
    re.IGNORECASE,
)
LIVE_STORY_IMPORT = re.compile(
    r"(?:composition/|use-session|useQuery|useLoaderData)", re.IGNORECASE
)
SCENARIO_ARGS = re.compile(r"args\s*:\s*{[^}]*\bstate\s*:", re.DOTALL)
SCENARIO_VIEW = re.compile(
    r"(?:export\s+)?type\s+\w*State\s*=\s*[\"']|"
    r"\{[^}]*\bstate\s*(?::|=)[^}]*\}",
    re.DOTALL,
)
MODULE_RECORDS = re.compile(r"^const\s+\w+\s*=\s*\[\s*{", re.MULTILINE)
DEMO_LITERAL = re.compile(
    r"[\"'][^\"']*(?:demo|sample|research-gateway|team-gateway)[^\"']*[\"']",
    re.IGNORECASE,
)


def read(path: Path, label: str, findings: list[str]) -> str:
    if not path.is_file():
        findings.append(f"{label} file does not exist: {path}")
        return ""
    return path.read_text()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--route", required=True, type=Path)
    parser.add_argument("--view", required=True, type=Path)
    parser.add_argument("--story", required=True, type=Path)
    parser.add_argument("--required-source", action="append", required=True)
    args = parser.parse_args()

    findings: list[str] = []
    route = read(args.route, "route", findings)
    view = read(args.view, "view", findings)
    story = read(args.story, "story", findings)

    for path, text in ((args.route, route), (args.view, view)):
        if FORBIDDEN_RUNTIME_IMPORT.search(text):
            findings.append(f"{path}: production imports a mock/fixture/story module")
        if DEMO_LITERAL.search(text):
            findings.append(f"{path}: production contains demo/sample resource data")

    if SCENARIO_VIEW.search(view):
        findings.append(
            f"{args.view}: production view accepts a named scenario/state selector; "
            "replace it with typed data and status props"
        )
    if MODULE_RECORDS.search(view):
        findings.append(
            f"{args.view}: production view declares module-level record fixtures; "
            "receive records through props"
        )
    if LIVE_STORY_IMPORT.search(story):
        findings.append(
            f"{args.story}: story imports a live hook/composition boundary; render "
            "the production view with deterministic props or in-memory adapters"
        )
    if SCENARIO_ARGS.search(story):
        findings.append(
            f"{args.story}: story passes a scenario through the production state prop; "
            "map the scenario to complete production view props in the story"
        )
    for source in args.required_source:
        if source not in route:
            findings.append(
                f"{args.route}: required real runtime source is not used: {source}"
            )

    if findings:
        print("\n".join(findings), file=sys.stderr)
        return 1
    print("Production/story data boundary check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
