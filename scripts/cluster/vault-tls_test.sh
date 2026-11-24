#!/usr/bin/env bash
# Verifies: OSG-CRED-14.03
# Tests vault_tls_pair_valid (scripts/cluster/vault-tls.sh): vault.sh must reuse a valid
# openshell-gateway-vault-tls Secret and regenerate a corrupted one.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=vault-tls.sh
source "${SCRIPT_DIR}/vault-tls.sh"

PASS=0
FAIL=0
check() {
  local want="$1" label="$2" dir="$3" got=0
  vault_tls_pair_valid "${dir}" || got=1
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want rc=%s got rc=%s)\n' "${label}" "${want}" "${got}"
  fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
host="openshell-gateway-vault.openshell-vault.svc"
bash "${REPO_ROOT}/scripts/gen-postgres-tls.sh" "${tmp}/a" "${host}" >/dev/null 2>&1
bash "${REPO_ROOT}/scripts/gen-postgres-tls.sh" "${tmp}/b" "${host}" >/dev/null 2>&1

check 0 "freshly generated pair is valid" "${tmp}/a"

mkdir -p "${tmp}/broken"
cp "${tmp}/a/tls.crt" "${tmp}/broken/tls.crt"
printf 'fail-closed-e2e: not a certificate' > "${tmp}/broken/ca.crt"
check 1 "non-PEM ca.crt (P2.7 placeholder) is invalid" "${tmp}/broken"

mkdir -p "${tmp}/other"
cp "${tmp}/a/tls.crt" "${tmp}/other/tls.crt"
cp "${tmp}/b/ca.crt" "${tmp}/other/ca.crt"
check 1 "ca.crt from a different authority is invalid" "${tmp}/other"

mkdir -p "${tmp}/empty"
check 1 "missing files are invalid" "${tmp}/empty"

printf 'vault-tls_test: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
