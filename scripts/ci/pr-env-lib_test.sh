#!/usr/bin/env bash
# Unit tests for scripts/ci/pr-env-lib.sh. No cluster required.
# Run: bash scripts/ci/pr-env-lib_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

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

assert_reapable() {
  local label="$1"; shift
  if pr_env_is_reapable "$@"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected reapable)\n' "${label}"
  fi
}

assert_not_reapable() {
  local label="$1"; shift
  if pr_env_is_reapable "$@"; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected retained)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

# --- Namespace + identity derivation ---
assert_eq 'hypershell-ci-pr-232' "$(pr_env_namespace 232)" 'platform namespace from PR number'
assert_eq 'hypershell-ci-pr-232-keycloak' "$(pr_env_keycloak_namespace "$(pr_env_namespace 232)")" 'keycloak namespace derivation'
assert_eq 'pr-232' "$(pr_env_environment_id 232)" 'environment id from PR number'

# The platform namespace must remain an RFC 1123 label within 54 chars so the
# derived -keycloak name stays under 63. Even a large PR number fits easily.
big_ns="$(pr_env_namespace 999999)"
assert_eq 'true' "$([[ ${#big_ns} -le 54 ]] && echo true || echo false)" 'platform namespace within 54 chars'

# --- Timebox round-trip (injected clock for determinism) ---
base=1000000000  # 2001-09-09T01:46:40Z
assert_eq '2001-09-09T01:46:40Z' "$(pr_env_epoch_to_rfc3339 "${base}")" 'epoch -> rfc3339'
assert_eq "${base}" "$(pr_env_rfc3339_to_epoch "$(pr_env_epoch_to_rfc3339 "${base}")")" 'rfc3339 -> epoch round-trip'
# 3-day expiry is exactly 3*86400 seconds ahead.
assert_eq "$(pr_env_epoch_to_rfc3339 $((base + 3 * 86400)))" "$(pr_env_expires_at 3 "${base}")" 'expires_at is now + days'

# --- Reaper predicate ---
now=2000000000
past="$(pr_env_epoch_to_rfc3339 $((now - 60)))"
future="$(pr_env_epoch_to_rfc3339 $((now + 3600)))"

assert_reapable 'expired owned pr env' \
  'hypershell-ci-pr-232' 'true' 'pr-232' "${past}" "${now}"
assert_not_reapable 'not yet expired' \
  'hypershell-ci-pr-232' 'true' 'pr-232' "${future}" "${now}"
assert_not_reapable 'missing expiry annotation' \
  'hypershell-ci-pr-232' 'true' 'pr-232' '' "${now}"
assert_not_reapable 'not owned' \
  'hypershell-ci-pr-232' 'false' 'pr-232' "${past}" "${now}"
# Local `make openshift-up` env: right owner labels but opaque (non pr-*) id.
assert_not_reapable 'local openshift-up env (uuid id)' \
  'hypershell-ci-pr-232' 'true' '3f9a1c2e-uuid' "${past}" "${now}"
# A HyperShell env that is not a pr namespace at all.
assert_not_reapable 'non pr-prefixed namespace' \
  'my-dev-namespace' 'true' 'pr-232' "${past}" "${now}"
# Env id must be pr-<digits>, not pr-anything.
assert_not_reapable 'env id pr- without a number' \
  'hypershell-ci-pr-232' 'true' 'pr-branchname' "${past}" "${now}"
# Reserved names are refused even if they somehow carry the labels/prefix.
assert_not_reapable 'reserved openshift- namespace refused' \
  'openshift-config' 'true' 'pr-1' "${past}" "${now}"

# --- Comment body ---
body="$(pr_env_comment_body 232 abcdef1234567 hypershell-ci-pr-232 hypershell-ci-pr-232-keycloak \
  https://console.example.com https://api.pr-232.example.com https://web.pr-232.example.com false)"
case "${body}" in
  *"<!-- hypershell-pr-environment -->"*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing hidden marker' ;;
esac
case "${body}" in
  *'abcdef1'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing short SHA' ;;
esac
# The CLI template must use --web, never carry a real token.
case "${body}" in
  *'--web'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body oc login missing --web' ;;
esac
updated_body="$(pr_env_comment_body 232 abcdef1234567 ns ns-keycloak c a w true)"
case "${updated_body}" in
  *'updated to commit'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updated comment missing update wording' ;;
esac

printf 'pr-env-lib tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
