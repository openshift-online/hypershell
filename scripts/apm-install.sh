#!/usr/bin/env bash
# Install APM dependencies and run a SkillSpector security scan.
# Supports macOS and Linux. Windows users: run under WSL or Git Bash.
#
# Usage: apm-install.sh [--force]
#   --force  Require skillspector (for CI). Fails if skillspector is not installed.
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

_prereq_status() {
  local label="$1"
  local required="$2"
  shift 2
  for cmd in "$@"; do
    if command -v "$cmd" &>/dev/null; then
      echo "✅ $label"
      return 0
    fi
  done
  if [ "$required" = "required" ]; then
    echo "❌ $label"
    return 1
  else
    echo "⚠️ $label"
    return 0
  fi
}

missing_required=0
echo "pre-requisites ---"
_prereq_status apm required apm || missing_required=1
_prereq_status git required git || missing_required=1
if [[ "$FORCE" == true ]]; then
  _prereq_status skillspector required skillspector || missing_required=1
  _prereq_status python3 required python3 || missing_required=1
else
  if command -v skillspector &>/dev/null; then
    _prereq_status skillspector optional skillspector
    _prereq_status python3 required python3 || missing_required=1
  else
    _prereq_status skillspector optional skillspector
    _prereq_status python3 optional python3
  fi
fi

if [ "$missing_required" -ne 0 ]; then
  echo ""
  echo "STOP: install required dependencies marked ❌, then re-run make apm-install."
  if ! command -v apm &>/dev/null; then
    echo "  apm: https://microsoft.github.io/apm/" >&2
  fi
  if ! command -v git &>/dev/null; then
    echo "  git: your platform package manager" >&2
  fi
  if command -v skillspector &>/dev/null && ! command -v python3 &>/dev/null; then
    echo "  python3: your platform package manager (needed to parse skillspector results)" >&2
  fi
  if [[ "$FORCE" == true ]] && ! command -v skillspector &>/dev/null; then
    echo "  skillspector: https://github.com/nvidia/skillspector" >&2
  fi
  exit 1
fi

echo ""

# --- APM ---

echo "Running apm install..."
apm install

# --- SkillSpector ---

if ! command -v skillspector &>/dev/null; then
  echo "WARNING: skillspector is not installed." >&2
  echo "Install skillspector from https://github.com/nvidia/skillspector for full audit warnings." >&2
  echo "WARNING: apm install succeeded but items were NOT scanned." >&2
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

python3 - "$SCAN_OUT" <<'PY'
import json
import sys

scan_out = sys.argv[1]
with open(scan_out) as f:
    data = json.load(f)

issues = data.get('issues', [])
severe = [i for i in issues if i.get('severity') in ('HIGH', 'CRITICAL')]

if not severe:
    suppressed = data.get('suppressed_count', 0)
    print(f'Security scan passed - no HIGH or CRITICAL findings. ({suppressed} suppressed by baseline)')
    print(f'Report saved to: {scan_out}')
    sys.exit(0)

print()
print('=========================================')
print(' SECURITY: HIGH/CRITICAL findings detected')
print('=========================================')
for i in severe:
    sev = i['severity']
    rid = i['id']
    loc = i.get('location', {})
    f = loc.get('file', '?')
    line = loc.get('start_line', '?')
    pattern = i.get('pattern', '')
    print(f'  {sev}: {rid} in {f}:{line} - {pattern}')
print()
print('Review these findings and either fix them or add to .skillspector-baseline.yaml')
print(f'Full report: {scan_out}')
sys.exit(1)
PY
