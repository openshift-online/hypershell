#!/usr/bin/env bash
# Unit tests for pr-env-is-up.sh.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PASS=0
FAIL=0

assert_ok() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected success)\n' "${label}"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected failure)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

cat >"${WORKDIR}/oc-present" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == get && "$2" == namespace ]]
EOF
chmod +x "${WORKDIR}/oc-present"
assert_ok "reports up when the namespace exists" \
  env OPENSHIFT_NAMESPACE=hypershell-ci-pr-1 PR_ENV_KUBECTL="${WORKDIR}/oc-present" \
  bash "${SCRIPT_DIR}/pr-env-is-up.sh"

cat >"${WORKDIR}/oc-absent" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${WORKDIR}/oc-absent"
assert_fail "reports down when the namespace is gone" \
  env OPENSHIFT_NAMESPACE=hypershell-ci-pr-1 PR_ENV_KUBECTL="${WORKDIR}/oc-absent" \
  bash "${SCRIPT_DIR}/pr-env-is-up.sh"

assert_fail "requires OPENSHIFT_NAMESPACE" \
  env -u OPENSHIFT_NAMESPACE PR_ENV_KUBECTL="${WORKDIR}/oc-present" \
  bash "${SCRIPT_DIR}/pr-env-is-up.sh"

echo "pr-env-is-up tests: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
