#!/usr/bin/env bash
# Check version selection and installation options without a cluster.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

for raw in '0.0.109' 'v0.0.109'; do
  actual=$(openshell_cli_image_tag "$raw")
  [[ "$actual" == v0.0.109 ]] || { echo "Wrong CLI image tag: $actual"; exit 1; }
done
for raw in 'v0.0.116-rhaiv.6' '  0.0.116-rhaiv.6  '; do
  actual=$(openshell_cli_image_tag "$raw")
  [[ "$actual" == v0.0.116-rhaiv.6 ]] || { echo "Wrong CLI image tag (suffix should be kept): $actual"; exit 1; }
done
for raw in '' '  '; do
  if openshell_cli_image_tag "$raw"; then
    echo "Accepted an empty version: $raw"
    exit 1
  fi
done
openshell_cli_matches_version 'openshell 0.0.109' v0.0.109
openshell_cli_matches_version 'openshell v0.0.109' v0.0.109
openshell_cli_matches_version 'openshell 0.0.116-rhaiv.6' v0.0.116-rhaiv.6
for reported in 'openshell 0.0.110' 'openshell 0.0.109-rh123' 'error: openshell 0.0.109'; do
  if openshell_cli_matches_version "$reported" v0.0.109; then
    echo "Accepted the wrong CLI version: $reported"
    exit 1
  fi
done
if openshell_cli_matches_version 'openshell 0.0.110' v0.0.11; then
  echo 'Accepted a version substring'
  exit 1
fi
for mode in auto always never; do
  E2E_OPENSHELL_INSTALL="$mode" E2E_GATEWAY_VERSION_TIMEOUT=120 e2e_validate_openshell_install
done
if E2E_OPENSHELL_INSTALL=invalid e2e_validate_openshell_install >/dev/null; then
  echo 'Accepted an invalid installation mode'
  exit 1
fi
for timeout in 0 -1 invalid; do
  if E2E_GATEWAY_VERSION_TIMEOUT="$timeout" e2e_validate_openshell_install >/dev/null; then
    echo "Accepted an invalid timeout: $timeout"
    exit 1
  fi
done
# Exercise extraction without a container daemon, including failure atomicity.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/install" "$work/tmp"
export TMPDIR="$work/tmp"
export EXTRACT_TEST_LOG="$work/commands"
export EXTRACT_TEST_MODE=success
cat > "$work/bin/oc" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$EXTRACT_TEST_LOG"
[[ "$1" == image && "$2" == extract && "$3" == example.invalid/cli:v1 && "$4" == --only-files ]]
[[ "$5" == --path=/usr/local/bin/openshell:* ]]
destination="${5#--path=/usr/local/bin/openshell:}"
case "$EXTRACT_TEST_MODE" in
  success) printf '#!/bin/sh\necho openshell-test\n' > "$destination/openshell" ;;
  failure) printf partial > "$destination/openshell"; exit 1 ;;
  missing) ;;
  symlink) ln -s /bin/sh "$destination/openshell" ;;
esac
SH
cat > "$work/bin/engine" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$EXTRACT_TEST_LOG"
case "$1" in
  create|rm) ;;
  cp) [[ "$EXTRACT_TEST_MODE" != failure ]] || exit 1
      printf '#!/bin/sh\necho openshell-test\n' > "$3" ;;
  *) exit 1 ;;
esac
SH
chmod +x "$work/bin/oc" "$work/bin/engine"
export PATH="$work/bin:$PATH"
e2e_extract_cli_image example.invalid/cli:v1 "$work/install" ''
[[ "$("$work/install/openshell")" == openshell-test ]]
for EXTRACT_TEST_MODE in failure missing symlink; do
  if e2e_extract_cli_image example.invalid/cli:v1 "$work/install" ''; then
    echo "Accepted invalid oc extraction: $EXTRACT_TEST_MODE"; exit 1
  fi
  [[ "$("$work/install/openshell")" == openshell-test ]]
done
EXTRACT_TEST_MODE=success
: > "$EXTRACT_TEST_LOG"
e2e_extract_cli_image example.invalid/cli:v1 "$work/install" "$work/bin/engine"
grep -q '^create ' "$EXTRACT_TEST_LOG"
grep -q '^cp ' "$EXTRACT_TEST_LOG"
grep -q '^rm ' "$EXTRACT_TEST_LOG"
EXTRACT_TEST_MODE=failure
: > "$EXTRACT_TEST_LOG"
if e2e_extract_cli_image example.invalid/cli:v1 "$work/install" "$work/bin/engine"; then
  echo 'Accepted failed runtime extraction'; exit 1
fi
grep -q '^rm ' "$EXTRACT_TEST_LOG"
[[ "$("$work/install/openshell")" == openshell-test ]]
[[ -z "$(ls -A "$TMPDIR")" ]]
echo 'OpenShell installation tests passed'
