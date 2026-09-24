#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "${SCRIPT_DIR}/gateway_service_account_test.py"
# shellcheck source=gateway_service_account.sh
source "${SCRIPT_DIR}/gateway_service_account.sh"
red() { printf '%s\n' "$*" >&2; }
E2E_MODE=long
e2e_validate_gateway_auth # Existing long/PR identity path is the default.
E2E_GATEWAY_AUTH=service_account
if e2e_validate_gateway_auth 2>/dev/null; then
  echo 'FAIL: machine gateway auth accepted long mode' >&2
  exit 1
fi
E2E_MODE=short
e2e_validate_gateway_auth
E2E_GATEWAY_AUTH=unknown
if e2e_validate_gateway_auth 2>/dev/null; then
  echo 'FAIL: unknown gateway auth accepted' >&2
  exit 1
fi
