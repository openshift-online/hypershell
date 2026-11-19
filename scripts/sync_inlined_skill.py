#!/usr/bin/env python3
"""Keep the update-openshell agent's inlined tooling skill equal to SKILL.md.

The scheduled `.fullsend/agents/update-openshell.md` agent carries an inlined copy
of `skills/tooling/update-openshell/SKILL.md` after the `# Tooling skill (inlined)`
marker, so the non-interactive agent and the interactive skill never diverge. This
script regenerates that inlined section from SKILL.md.

Usage:
    python3 scripts/sync_inlined_skill.py          # check mode (CI): exit 1 on drift
    python3 scripts/sync_inlined_skill.py --stamp   # rewrite the agent in-place
"""

import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SKILL_FILE = REPO_ROOT / "skills" / "tooling" / "update-openshell" / "SKILL.md"
AGENT_FILE = REPO_ROOT / ".fullsend" / "agents" / "update-openshell.md"

# The agent preamble ends with this marker line; everything after it is the
# regenerated inlined skill body.
MARKER = "# Tooling skill (inlined)"


def skill_body(skill_text: str) -> str:
    """Return SKILL.md with its leading YAML front matter removed."""
    lines = skill_text.splitlines()
    if lines and lines[0].strip() == "---":
        # Drop the front matter block (first two '---' fences).
        for i in range(1, len(lines)):
            if lines[i].strip() == "---":
                lines = lines[i + 1 :]
                break
    # Trim leading blank lines so the body starts at its first heading.
    while lines and lines[0].strip() == "":
        lines.pop(0)
    body = "\n".join(lines).rstrip("\n")
    return body + "\n"


def expected_agent(agent_text: str, skill_text: str) -> str:
    """Compose the agent file: preamble through MARKER, then the inlined body."""
    lines = agent_text.splitlines()
    marker_idx = None
    for i, line in enumerate(lines):
        if line.strip() == MARKER:
            marker_idx = i
            break
    if marker_idx is None:
        raise SystemExit(
            f"ERROR: {AGENT_FILE.relative_to(REPO_ROOT)} is missing the "
            f"'{MARKER}' marker; cannot regenerate the inlined skill"
        )
    preamble = "\n".join(lines[: marker_idx + 1])
    return preamble + "\n\n\n" + skill_body(skill_text)


def read_required(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except FileNotFoundError:
        raise SystemExit(
            f"ERROR: {path.relative_to(REPO_ROOT)} not found; cannot sync the "
            "inlined skill"
        )


def main() -> int:
    stamp = "--stamp" in sys.argv[1:]

    skill_text = read_required(SKILL_FILE)
    agent_text = read_required(AGENT_FILE)
    expected = expected_agent(agent_text, skill_text)

    rel = AGENT_FILE.relative_to(REPO_ROOT)
    if agent_text == expected:
        print(f"OK: {rel} inlined skill matches SKILL.md")
        return 0

    if stamp:
        AGENT_FILE.write_text(expected, encoding="utf-8")
        print(f"Stamped: regenerated the inlined skill in {rel} from SKILL.md")
        return 0

    print(
        f"ERROR: {rel} inlined skill is out of date with "
        f"{SKILL_FILE.relative_to(REPO_ROOT)}.\n"
        "Run `make sync-inlined-skill` (or "
        "`python3 scripts/sync_inlined_skill.py --stamp`) and commit the result.",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
