#!/usr/bin/env bash
# Unit tests for scripts/cluster/reconcile-test-users.sh. No cluster required.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=reconcile-test-users.sh
source "${SCRIPT_DIR}/reconcile-test-users.sh"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

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

TEST_USER_PASSWORD_SOURCE=static
assert_ok "static passwords load" load_test_user_passwords
assert_eq "admin" "${TEST_USER_PASSWORD_admin}" "static admin password equals username"
assert_eq "developer" "${TEST_USER_PASSWORD_developer}" "static developer password equals username"
assert_eq "platform-admin" "${TEST_USER_PASSWORD_platform_admin}" "static platform-admin password equals username"

TEST_USER_PASSWORD_SOURCE=secret
assert_fail "secret source without namespace fails" load_test_user_passwords

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

TEST_USER_OC="${WORKDIR}/oc"
cat >"${TEST_USER_OC}" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *jsonpath=\{.data.admin\}*) printf '%s' "$(printf '%s' 'rot-admin' | openssl base64 -A)" ;;
  *jsonpath=\{.data.developer\}*) printf '%s' "$(printf '%s' 'rot-dev' | openssl base64 -A)" ;;
  *jsonpath=\{.data.platform-admin\}*) printf '%s' "$(printf '%s' 'rot-pa' | openssl base64 -A)" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "${TEST_USER_OC}"

TEST_USER_PASSWORD_SOURCE=secret
TEST_USER_SECRET_NAMESPACE=hypershell-ci-pr-1-keycloak
assert_ok "secret passwords load" load_test_user_passwords
assert_eq "rot-admin" "${TEST_USER_PASSWORD_admin}" "secret admin password"
assert_eq "rot-dev" "${TEST_USER_PASSWORD_developer}" "secret developer password"
assert_eq "rot-pa" "${TEST_USER_PASSWORD_platform_admin}" "secret platform-admin password"
mask_out="${WORKDIR}/mask.out"
if GITHUB_ACTIONS=true load_test_user_passwords >"${mask_out}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: secret passwords load under GITHUB_ACTIONS'
fi
if grep -q '::add-mask::rot-admin' "${mask_out}" \
  && grep -q '::add-mask::rot-dev' "${mask_out}" \
  && grep -q '::add-mask::rot-pa' "${mask_out}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: secret password load did not mask values for GitHub Actions'
fi

cat >"${TEST_USER_OC}" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *jsonpath=\{.data.admin\}*) printf '%s' "$(printf '%s' 'admin' | openssl base64 -A)" ;;
  *jsonpath=\{.data.developer\}*) printf '%s' "$(printf '%s' 'developer' | openssl base64 -A)" ;;
  *jsonpath=\{.data.platform-admin\}*) printf '%s' "$(printf '%s' 'platform-admin' | openssl base64 -A)" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "${TEST_USER_OC}"
assert_fail "username-equals-password secret is refused" load_test_user_passwords

CURL_LOG="${WORKDIR}/curl.log"
TEST_USER_OC="${WORKDIR}/oc"
cat >"${WORKDIR}/curl" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"${CURL_LOG}"
if [[ " \$* " == *" /token "* ]] || [[ " \$* " == *"/token"* ]]; then
  printf '%s' '{"access_token":"tok"}'
  exit 0
fi
if [[ " \$* " == *"users?username="* ]]; then
  if [[ " \$* " == *"username=developer"* ]]; then
    printf '%s' '[]'
  else
    printf '%s' '[{"id":"uid-1"}]'
  fi
  exit 0
fi
if [[ " \$* " == *" -X DELETE "* ]] || [[ " \$* " == *"-X DELETE"* ]]; then
  printf '%s' '204'
  exit 0
fi
printf '%s' '204'
EOF
chmod +x "${WORKDIR}/curl"
# The helper calls curl as a command; put the stub first on PATH.
PATH="${WORKDIR}:${PATH}"
: >"${CURL_LOG}"
KEYCLOAK_BASE_URL="https://sso.example.com"
assert_ok "de-seed deletes present users and skips missing" keycloak_de_seed_test_users
if grep -q 'username=admin' "${CURL_LOG}" && grep -q 'username=platform-admin' "${CURL_LOG}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: de-seed did not look up admin and platform-admin'
fi
if grep -q 'username=developer' "${CURL_LOG}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: de-seed did not look up developer'
fi

printf 'reconcile-test-users tests: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
