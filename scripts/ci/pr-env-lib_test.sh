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
# 72-hour retained max is exactly 72*3600 seconds ahead.
assert_eq "$(pr_env_epoch_to_rfc3339 $((base + 72 * 3600)))" "$(pr_env_expires_at_hours 72 "${base}")" 'expires_at_hours retained default'
# 6-hour unretained max is exactly 6*3600 seconds ahead.
assert_eq "$(pr_env_epoch_to_rfc3339 $((base + 6 * 3600)))" "$(pr_env_expires_at_hours 6 "${base}")" 'expires_at_hours unretained default'

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

assert_eq 'true' "$(pr_env_is_pr_platform_namespace 'hypershell-ci-pr-267' && echo true || echo false)" \
  'pr platform namespace matches'
assert_eq 'false' "$(pr_env_is_pr_platform_namespace 'hypershell-ci-pr-267-keycloak' && echo true || echo false)" \
  'keycloak companion is not a pr platform namespace'
assert_eq 'false' "$(pr_env_is_pr_platform_namespace 'hyp5' && echo true || echo false)" \
  'hub namespace is not a pr platform namespace'

if pr_env_should_reap_instance_workload 'openshell-aaa' 'hypershell-ci-pr-267' 'false'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: leftover pr-267 gateway should be reaped when platform is gone'
fi
if pr_env_should_reap_instance_workload 'openshell-aaa' 'hypershell-ci-pr-267' 'true'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: live pr-267 gateway should be retained'
else
  PASS=$((PASS + 1))
fi
if pr_env_should_reap_instance_workload 'openshell-aaa' 'hyp5' 'false'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: hyp5 gateway should be retained even if platform lookup fails'
else
  PASS=$((PASS + 1))
fi
if pr_env_should_reap_instance_workload 'openshell-aaa' 'alice' 'false'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: local openshift-up gateway should be retained'
else
  PASS=$((PASS + 1))
fi
if pr_env_should_reap_instance_workload 'hypershell-ci-pr-267' 'hypershell-ci-pr-267' 'false'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: instance leftover path must not delete the platform project itself'
else
  PASS=$((PASS + 1))
fi

# --- Deploying placeholder comment (posted before the ready comment) ---
deploying_body="$(pr_env_comment_deploying_body abcdef1234567)"
case "${deploying_body}" in
  *"<!-- hypershell-pr-environment -->"*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing hidden marker' ;;
esac
case "${deploying_body}" in
  *'abcdef1'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing short SHA' ;;
esac
case "${deploying_body}" in
  *'| Fact | Value |'*) FAIL=$((FAIL + 1)); echo 'FAIL: first-deploy placeholder must not include the access-fact table' ;;
  *) PASS=$((PASS + 1)) ;;
esac
case "${deploying_body}" in
  *'Deploying commit `abcdef1` to an ephemeral OpenShift environment.'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: first-deploy placeholder missing deploying wording' ;;
esac
case "${deploying_body}" in
  *'may not be fully responsive'*) FAIL=$((FAIL + 1)); echo 'FAIL: first-deploy placeholder must not warn about an existing environment' ;;
  *) PASS=$((PASS + 1)) ;;
esac

# A later reconcile must keep the existing table: those facts do not change
# from run to run, and wiping them hides login details for the whole swap.
ready_body="$(pr_env_comment_body 232 abcdef1234567 hypershell-ci-pr-232 hypershell-ci-pr-232-keycloak \
  https://console.example.com https://api.pr-232.example.com https://web.pr-232.example.com \
  https://api.cluster.example.com:6443 false)"
updating_body="$(pr_env_comment_deploying_body fffffff111111 "${ready_body}")"
case "${updating_body}" in
  *"<!-- hypershell-pr-environment -->"*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment missing hidden marker' ;;
esac
case "${updating_body}" in
  *'## HyperShell environment updating to commit `fffffff`'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment missing updating heading' ;;
esac
case "${updating_body}" in
  *'may not be fully responsive during the update'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment missing unresponsive-during-update note' ;;
esac
case "${updating_body}" in
  *'| Fact | Value |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment dropped the access-fact table' ;;
esac
case "${updating_body}" in
  *'| Namespaces | Platform: `hypershell-ci-pr-232` Keycloak: `hypershell-ci-pr-232-keycloak` |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment dropped namespace facts' ;;
esac
case "${updating_body}" in
  *'| OpenShift console | https://console.example.com |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment dropped console URL' ;;
esac
case "${updating_body}" in
  *'oc login --server=https://api.cluster.example.com:6443 --web'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment dropped CLI login' ;;
esac
case "${updating_body}" in
  *'abcdef1'*) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment kept the previous commit SHA' ;;
  *) PASS=$((PASS + 1)) ;;
esac
# A still-in-progress first deploy has no table yet; a cancelled run that
# never reached ready must keep posting the no-facts placeholder.
still_deploying="$(pr_env_comment_deploying_body fffffff111111 "${deploying_body}")"
case "${still_deploying}" in
  *'| Fact | Value |'*) FAIL=$((FAIL + 1)); echo 'FAIL: in-progress first deploy must not invent a table' ;;
  *) PASS=$((PASS + 1)) ;;
esac
case "${still_deploying}" in
  *'Deploying commit `fffffff` to an ephemeral OpenShift environment.'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: in-progress first deploy missing updated SHA' ;;
esac
# A mid-reconcile comment already has the table; the next deploying edit
# must keep it and only advance the SHA.
second_update="$(pr_env_comment_deploying_body 1234567890abc "${updating_body}")"
case "${second_update}" in
  *'## HyperShell environment updating to commit `1234567`'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: second updating comment missing new SHA heading' ;;
esac
case "${second_update}" in
  *'| Namespaces | Platform: `hypershell-ci-pr-232` Keycloak: `hypershell-ci-pr-232-keycloak` |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: second updating comment dropped namespace facts' ;;
esac
case "${second_update}" in
  *'fffffff'*) FAIL=$((FAIL + 1)); echo 'FAIL: second updating comment kept the previous commit SHA' ;;
  *) PASS=$((PASS + 1)) ;;
esac

# --- Slash commands: body parse, permission, latest-wins ---
assert_eq 'extend' "$(pr_env_command_from_body '/pr-extend')" 'bare /pr-extend'
assert_eq 'extend' "$(pr_env_command_from_body $'/pr-extend\nplease keep it')" '/pr-extend with trailing text'
assert_eq 'extend' "$(pr_env_command_from_body '  /pr-extend  ')" '/pr-extend with surrounding whitespace'
assert_eq 'release' "$(pr_env_command_from_body '/pr-release')" 'bare /pr-release'
assert_eq 'release' "$(pr_env_command_from_body '/pr-release now')" '/pr-release with trailing text'
assert_eq '' "$(pr_env_command_from_body '/pr-extended')" '/pr-extended is not /pr-extend'
assert_eq '' "$(pr_env_command_from_body 'please /pr-extend')" 'command must begin the body'
assert_eq '' "$(pr_env_command_from_body '')" 'empty body is not a command'

if pr_env_permission_is_authorized write; then PASS=$((PASS + 1)); else FAIL=$((FAIL + 1)); echo 'FAIL: write is authorized'; fi
if pr_env_permission_is_authorized maintain; then PASS=$((PASS + 1)); else FAIL=$((FAIL + 1)); echo 'FAIL: maintain is authorized'; fi
if pr_env_permission_is_authorized admin; then PASS=$((PASS + 1)); else FAIL=$((FAIL + 1)); echo 'FAIL: admin is authorized'; fi
if pr_env_permission_is_authorized read; then FAIL=$((FAIL + 1)); echo 'FAIL: read must not be authorized'; else PASS=$((PASS + 1)); fi
if pr_env_permission_is_authorized triage; then FAIL=$((FAIL + 1)); echo 'FAIL: triage must not be authorized'; else PASS=$((PASS + 1)); fi
if pr_env_permission_is_authorized ''; then FAIL=$((FAIL + 1)); echo 'FAIL: empty permission must not be authorized'; else PASS=$((PASS + 1)); fi

assert_eq 'extend' "$(printf '%s\n' \
  $'2026-09-16T10:00:00Z\talice\twrite\t/pr-extend' \
  $'2026-09-16T11:00:00Z\talice\twrite\t/pr-release' \
  $'2026-09-16T12:00:00Z\talice\twrite\t/pr-extend' \
  | pr_env_select_latest_command)" 'latest of extend/release/extend is extend'

assert_eq 'release' "$(printf '%s\n' \
  $'2026-09-16T12:00:00Z\talice\twrite\t/pr-release' \
  $'2026-09-16T10:00:00Z\talice\twrite\t/pr-extend' \
  | pr_env_select_latest_command)" 'out-of-order rows still pick latest created_at'

assert_eq 'extend' "$(printf '%s\n' \
  $'2026-09-16T10:00:00Z\talice\twrite\t/pr-extend' \
  $'2026-09-16T11:00:00Z\tbob\tread\t/pr-release' \
  | pr_env_select_latest_command)" 'unauthorized /pr-release does not count'

assert_eq 'none' "$(printf '%s\n' \
  $'2026-09-16T11:00:00Z\tbob\tread\t/pr-extend' \
  | pr_env_select_latest_command)" 'only unauthorized commands -> none'

assert_eq 'none' "$(printf '%s\n' | pr_env_select_latest_command)" 'no comments -> none'

refusal="$(pr_env_refusal_comment bob /pr-extend)"
case "${refusal}" in
  *'@bob'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: refusal comment missing user'; ;;
esac
case "${refusal}" in
  *'/pr-extend'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: refusal comment missing command'; ;;
esac
case "${refusal}" in
  *'unchanged'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: refusal comment missing unchanged wording'; ;;
esac

# --- Comment body ---
body="$(pr_env_comment_body 232 abcdef1234567 hypershell-ci-pr-232 hypershell-ci-pr-232-keycloak \
  https://console.example.com https://api.pr-232.example.com https://web.pr-232.example.com \
  https://api.cluster.example.com:6443 false)"
case "${body}" in
  *"<!-- hypershell-pr-environment -->"*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing hidden marker' ;;
esac
case "${body}" in
  *'abcdef1'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing short SHA' ;;
esac
# The CLI template must use --web against the cluster API, never the app Route.
case "${body}" in
  *'oc login --server=https://api.cluster.example.com:6443 --web'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body oc login missing cluster API --web' ;;
esac
case "${body}" in
  *'oc login --server=https://api.pr-232.example.com'*) FAIL=$((FAIL + 1)); echo 'FAIL: oc login used app API Route' ;;
  *) PASS=$((PASS + 1)) ;;
esac
updated_body="$(pr_env_comment_body 232 abcdef1234567 ns ns-keycloak c a w https://api.cluster.example.com true)"
case "${updated_body}" in
  *'updated to commit'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updated comment missing update wording' ;;
esac
# Unretained comment advertises /pr-extend and never implies the env persists.
case "${body}" in
  *'destroyed after the e2e run'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing ephemeral wording' ;;
esac
case "${body}" in
  *'/pr-extend'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing /pr-extend' ;;
esac
case "${body}" in
  *'/pr-release'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing /pr-release' ;;
esac
case "${body}" in
  *'refreshed on every new commit'*) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment implied persistence' ;;
  *) PASS=$((PASS + 1)) ;;
esac
retained_body="$(pr_env_comment_body 232 abcdef1234567 ns ns-keycloak c a w https://api.cluster.example.com false true)"
case "${retained_body}" in
  *'retained and renewed on every commit'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment missing retained wording' ;;
esac
case "${retained_body}" in
  *'inactivity timebox'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment missing inactivity timebox' ;;
esac
case "${retained_body}" in
  *'destroyed after the e2e run'*) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment said env is about to be destroyed' ;;
  *) PASS=$((PASS + 1)) ;;
esac

printf 'pr-env-lib tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
