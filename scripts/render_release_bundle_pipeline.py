#!/usr/bin/env python3
"""Embed the publisher in the Tekton pipeline so each run has one code version."""

from pathlib import Path
import textwrap

ROOT = Path(__file__).resolve().parents[1]
PIPELINE = ROOT / "pipelines/release-bundle/pipeline.yaml"
PUBLISHER = ROOT / "scripts/release_bundle.py"
SCRIPT_MARKER = "            script: |\n"
BOOTSTRAP = r'''#!/usr/bin/env bash
set -euo pipefail
source_dir=$(mktemp -d)
trap 'rm -rf "$source_dir"' EXIT
git -C "$source_dir" init --quiet
git -C "$source_dir" remote add origin https://github.com/openshift-online/hypershell.git
# Fetch main only to check the ancestry of the component source revisions.
git -C "$source_dir" fetch --quiet --filter=blob:none origin main:refs/remotes/origin/main
python3 - --release "$RELEASE" --snapshot "$SNAPSHOT" \
  --source-directory "$source_dir" --result-path "$BUNDLE_RESULT" <<'PYTHON_PUBLISHER'
'''


def render(pipeline, publisher):
    """Replace the final script block with the bootstrap and publisher source."""
    if pipeline.count(SCRIPT_MARKER) != 1:
        raise ValueError("The pipeline must have exactly one final script block")
    prefix = pipeline.split(SCRIPT_MARKER, 1)[0]
    script = BOOTSTRAP + publisher.rstrip() + "\nPYTHON_PUBLISHER\n"
    return prefix + SCRIPT_MARKER + textwrap.indent(script, " " * 14)


if __name__ == "__main__":
    PIPELINE.write_text(render(PIPELINE.read_text(), PUBLISHER.read_text()))
