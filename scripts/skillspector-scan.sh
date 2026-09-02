#!/usr/bin/env bash
# Run SkillSpector scan with JSON output and HIGH/CRITICAL severity filtering.
# Supports macOS and Linux. Windows users: run under WSL or Git Bash.
#
# Usage: skillspector-scan.sh [--force]
#   --force  Require skillspector (for CI). Fails if skillspector is not installed
#            or if the scan does not produce valid output.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BASELINE="$REPO_ROOT/.skillspector-baseline.yaml"

FORCE=false
for arg in "$@"; do
  case "$arg" in
    --force|-f)
      FORCE=true
      ;;
    *)
      echo "ERROR: unknown argument: $arg" >&2
      echo "Usage: $0 [--force]" >&2
      exit 1
      ;;
  esac
done

if ! command -v skillspector &>/dev/null; then
  if [[ "$FORCE" == true ]]; then
    echo "ERROR: skillspector is required but not installed." >&2
    echo "  skillspector: https://github.com/nvidia/skillspector" >&2
    exit 1
  fi
  echo "WARNING: skillspector is not installed." >&2
  echo "Install skillspector from https://github.com/nvidia/skillspector for full audit warnings." >&2
  echo "WARNING: items were NOT scanned." >&2
  exit 0
fi

if ! command -v python3 &>/dev/null; then
  if [[ "$FORCE" == true ]]; then
    echo "ERROR: python3 is required to parse skillspector results but is not installed." >&2
    exit 1
  fi
  echo "WARNING: python3 is not installed; cannot parse skillspector results." >&2
  echo "WARNING: items were NOT scanned." >&2
  exit 0
fi

echo "Running skillspector scan..."

BASELINE_FLAG=()
if [[ -f "$BASELINE" ]]; then
  BASELINE_FLAG=(--baseline "$BASELINE")
else
  echo "WARNING: baseline file not found at $BASELINE - running without suppression" >&2
fi

SCAN_OUT_DIR="$REPO_ROOT/.skillspector-reports"
mkdir -p "$SCAN_OUT_DIR"
SCAN_OUT="$SCAN_OUT_DIR/scan-results-$(date +%s).json"

skillspector scan "$REPO_ROOT" \
  --no-llm \
  "${BASELINE_FLAG[@]}" \
  --format json \
  --output "$SCAN_OUT" || true

python3 - "$SCAN_OUT" "$FORCE" <<'PY'
import json
import os
import sys

scan_out = sys.argv[1]
force = sys.argv[2] == "true"


def scan_incomplete(message: str) -> None:
    print(message, file=sys.stderr)
    if force:
        print("ERROR: SkillSpector scan did not complete successfully.", file=sys.stderr)
        sys.exit(1)
    print("WARNING: Skipping severity filtering because the scan did not complete.", file=sys.stderr)
    sys.exit(0)


if not os.path.isfile(scan_out):
    scan_incomplete(f"WARNING: SkillSpector scan output not found at {scan_out}")

try:
    with open(scan_out) as f:
        data = json.load(f)
except json.JSONDecodeError as exc:
    scan_incomplete(
        f"WARNING: SkillSpector scan output at {scan_out} is not valid JSON: {exc}"
    )

issues = data.get("issues", [])
severe = [i for i in issues if i.get("severity") in ("HIGH", "CRITICAL")]

if not severe:
    suppressed = data.get("suppressed_count", 0)
    print(
        f"Security scan passed - no HIGH or CRITICAL findings. ({suppressed} suppressed by baseline)"
    )
    print(f"Report saved to: {scan_out}")
    sys.exit(0)

print()
print("=========================================")
print(" SECURITY: HIGH/CRITICAL findings detected")
print("=========================================")
for issue in severe:
    sev = issue.get("severity", "?")
    rid = issue.get("id", "?")
    loc = issue.get("location", {})
    path = loc.get("file", "?")
    line = loc.get("start_line", "?")
    pattern = issue.get("pattern", "")
    print(f"  {sev}: {rid} in {path}:{line} - {pattern}")
print()
print("Review these findings and either fix them or add to .skillspector-baseline.yaml")
print(f"Full report: {scan_out}")
sys.exit(1)
PY
