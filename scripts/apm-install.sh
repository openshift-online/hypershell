#!/usr/bin/env bash
# Install APM dependencies and run a SkillSpector security scan.
# Supports macOS and Linux. Windows users: run under WSL or Git Bash.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BASELINE="$REPO_ROOT/.skillspector-baseline.yaml"

OS="$(uname -s)"

# --- APM ---

if ! command -v apm &>/dev/null; then
  echo "Installing APM..."
  case "$OS" in
    Darwin)
      if command -v brew &>/dev/null; then
        brew install apm
      else
        curl -fsSL https://aka.ms/apm-unix | bash
      fi
      ;;
    Linux|MINGW*|MSYS*|CYGWIN*)
      curl -fsSL https://aka.ms/apm-unix | bash
      ;;
    *)
      echo "ERROR: unsupported OS '$OS' — install APM manually: https://microsoft.github.io/apm/" >&2
      exit 1
      ;;
  esac
fi

echo "Running apm install..."
apm install

# --- SkillSpector ---

if ! command -v skillspector &>/dev/null; then
  echo "Installing skillspector..."
  if command -v uv &>/dev/null; then
    uv tool install skillspector
  elif command -v pipx &>/dev/null; then
    pipx install skillspector
  else
    echo "ERROR: uv or pipx required to install skillspector" >&2
    exit 1
  fi
fi

echo "Running skillspector scan..."

BASELINE_FLAG=()
if [[ -f "$BASELINE" ]]; then
  BASELINE_FLAG=(--baseline "$BASELINE")
else
  echo "WARNING: baseline file not found at $BASELINE — running without suppression" >&2
fi

SCAN_OUT=$(mktemp "${TMPDIR:-/tmp}/skillspector-XXXXXX.json")
trap 'rm -f "$SCAN_OUT"' EXIT

skillspector scan "$REPO_ROOT" \
  --no-llm \
  "${BASELINE_FLAG[@]}" \
  --format json \
  --output "$SCAN_OUT"

python3 -c "
import json, sys

with open('$SCAN_OUT') as f:
    data = json.load(f)

issues = data.get('issues', [])
severe = [i for i in issues if i.get('severity') in ('HIGH', 'CRITICAL')]

if not severe:
    suppressed = data.get('suppressed_count', 0)
    print(f'Security scan passed — no HIGH or CRITICAL findings. ({suppressed} suppressed by baseline)')
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
    print(f'  {sev}: {rid} in {f}:{line} — {pattern}')
print()
print('Review these findings and either fix them or add to .skillspector-baseline.yaml')
sys.exit(1)
"
