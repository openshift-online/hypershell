#!/usr/bin/env bash
# Check version selection and installation options without a cluster.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

for raw in '0.0.109' 'v0.0.109' 'v0.0.109-rh9a8f8' '  0.0.109-rh9a8f8  '; do
  actual=$(openshell_installer_version "$raw")
  [[ "$actual" == v0.0.109 ]] || { echo "Wrong installer version: $actual"; exit 1; }
done
for raw in '' '  ' '-' '-rh123'; do
  if openshell_installer_version "$raw"; then
    echo "Accepted an empty base version: $raw"
    exit 1
  fi
done
openshell_cli_matches_version 'openshell 0.0.109' v0.0.109
openshell_cli_matches_version 'openshell v0.0.109' v0.0.109
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
echo 'OpenShell installation tests passed'
