#!/usr/bin/env bash
# Infrastructure adapter for the TypeScript Playwright live-console suite.
# Cluster creation and route discovery stay in shell; browser assertions live in
# components/web-console/e2e-live/console.live.spec.ts.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Paths are resolved from this script at runtime.
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/lib.sh"
e2e_validate_mode
e2e_select_infra_driver

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "${DRIVER_FILE}" ]]; then
  e2e_die_unknown_driver "Unknown driver '${E2E_INFRA_DRIVER}'"
fi
# shellcheck disable=SC1090
source "${DRIVER_FILE}"

if [[ -z "${E2E_CONSOLE_URL:-}" ]]; then
  if ! discover_console_host; then
    red "ERROR: Could not discover the HyperShell web console host"
    exit 1
  fi
  export E2E_CONSOLE_URL="${_DISCOVER_CONSOLE_HOST}"
fi

export E2E_MODE E2E_INFRA_DRIVER
cd "${REPO_ROOT}"
exec pnpm --filter @openshift-online/hypershell-web-console test:e2e:console:live
