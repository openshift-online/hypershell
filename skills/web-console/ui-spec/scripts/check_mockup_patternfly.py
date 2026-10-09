#!/usr/bin/env python3
"""Reject common custom reimplementations in implementation-intent mockups."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]
RAW_COMPONENT_TAGS = (
    "a",
    "button",
    "details",
    "dialog",
    "input",
    "li",
    "nav",
    "ol",
    "progress",
    "select",
    "table",
    "textarea",
    "ul",
)
RAW_TAG = re.compile(r"<\s*(" + "|".join(RAW_COMPONENT_TAGS) + r")(?:\s|>)")
LITERAL_COLOR = re.compile(
    r"(?:#[0-9a-fA-F]{3,8}\b|\b(?:rgb|rgba|hsl|hsla)\s*\()"
)
LEGACY_PATTERNFLY = re.compile(r"\bpf-v[1-5]-")
UTILITY_CLASS = re.compile(r"\b(pf-v6-u-[A-Za-z0-9_-]+)\b")
CSS_GAP = re.compile(
    r"/\*\s*patternfly-gap:\s*specs/[^*\n]+\.md\s+-\s+[^*\n]+\*/"
)
PARITY_SHELL_COMPONENTS = (
    "MockupShell",
    "MockupTemplate",
    "Masthead",
    "MastheadBrand",
    "MastheadContent",
    "MastheadMain",
)


def check_tsx(path: Path, text: str) -> list[str]:
    findings: list[str] = []
    is_parity_story = ".parity.stories." in path.name or re.search(
        r"\btitle\s*:\s*[\"']Parity/", text
    )
    for line_number, line in enumerate(text.splitlines(), 1):
        match = RAW_TAG.search(line)
        if match:
            findings.append(
                f"{path}:{line_number}: raw <{match.group(1)}> recreates a "
                "PatternFly component or pattern"
            )
        if LEGACY_PATTERNFLY.search(line):
            findings.append(f"{path}:{line_number}: legacy PatternFly class")
    utility_classes = sorted(set(UTILITY_CLASS.findall(text)))
    if utility_classes:
        utility_indexes = sorted(
            (ROOT / "node_modules/.pnpm").glob(
                "@patternfly+react-styles@*/node_modules/"
                "@patternfly/react-styles/css/utilities/_index.css"
            )
        )
        installed_css = utility_indexes[-1].read_text() if utility_indexes else ""
        for class_name in utility_classes:
            if f".{class_name}" not in installed_css:
                findings.append(
                    f"{path}: {class_name} is not present in the installed PatternFly utilities"
                )
        preview = ROOT / "components/web-console/.storybook-mockups/preview.tsx"
        preview_text = preview.read_text() if preview.is_file() else ""
        if "@patternfly/react-styles/css/utilities/" not in preview_text:
            findings.append(
                f"{path}: PatternFly utility classes are used, but the mockup "
                "Storybook preview imports no utility stylesheet"
            )
    if (
        ".stories." not in path.name
        and "@patternfly/react-core" not in text
        and "/common/" not in text
        and "../common/" not in text
    ):
        findings.append(
            f"{path}: mockup imports neither PatternFly React nor a shared mockup component"
        )
    if is_parity_story:
        component = re.search(r"\bcomponent\s*:\s*([A-Za-z0-9_]+)", text)
        if component is None or not component.group(1).endswith("PageSurface"):
            findings.append(
                f"{path}: page parity story must render a <Name>PageSurface "
                "containing the title, page sections, background, and content"
            )
        for shell_component in PARITY_SHELL_COMPONENTS:
            if re.search(rf"\b{re.escape(shell_component)}\b", text):
                findings.append(
                    f"{path}: parity story includes application-shell component "
                    f"{shell_component}; use only the shell-free parity frame"
                )
    return findings


def check_css(path: Path, text: str) -> list[str]:
    stripped = re.sub(r"/\*.*?\*/", "", text, flags=re.DOTALL).strip()
    if not stripped:
        return []
    findings: list[str] = []
    if not CSS_GAP.search(text):
        findings.append(
            f"{path}: page-specific CSS requires a spec-backed patternfly-gap comment"
        )
    for line_number, line in enumerate(text.splitlines(), 1):
        if LITERAL_COLOR.search(line):
            findings.append(f"{path}:{line_number}: literal color is forbidden")
        if LEGACY_PATTERNFLY.search(line):
            findings.append(f"{path}:{line_number}: legacy PatternFly token/class")
    return findings


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("files", nargs="+", type=Path)
    args = parser.parse_args()
    findings: list[str] = []
    for path in args.files:
        if not path.is_file():
            findings.append(f"{path}: file does not exist")
            continue
        text = path.read_text()
        if path.suffix == ".tsx":
            findings.extend(check_tsx(path, text))
        elif path.suffix == ".css":
            findings.extend(check_css(path, text))
    if findings:
        print("\n".join(findings), file=sys.stderr)
        return 1
    print("PatternFly mockup check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
