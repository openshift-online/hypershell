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

# GITHUB_RUN_ID / GITHUB_REPOSITORY / GITHUB_SERVER_URL are already present in
# the environment when this suite runs as a CI step itself; unset them so
# pr_env_run_link's no-run-context behavior is deterministic here, and opt
# individual assertions back in with a scoped assignment.
unset GITHUB_RUN_ID GITHUB_REPOSITORY GITHUB_SERVER_URL KEYCLOAK_URL

# --- Namespace + identity derivation ---
assert_eq 'hypershell.redhat.io/ci-keycloak' "${PR_ENV_CI_KEYCLOAK_LABEL}" 'CI Keycloak ESO selector label'
assert_eq 'true' "${PR_ENV_CI_KEYCLOAK_VALUE}" 'CI Keycloak ESO selector value'
assert_eq 'hypershell-ci-pr-232-keycloak' "$(pr_env_keycloak_namespace "$(pr_env_namespace 232)")" 'keycloak namespace derivation'
assert_eq 'pr-232' "$(pr_env_environment_id 232)" 'environment id from PR number'
assert_eq 'hypershell-ci-main-abcdef1' "$(pr_env_main_namespace 'abcdef1234567890')" 'main namespace from short SHA'
assert_eq 'hypershell-ci-main-abcdef1' "$(pr_env_main_namespace 'ABCDEF1234567890')" 'main namespace lowercases SHA'
assert_eq 'hypershell-ci-mq-abcdef1' "$(pr_env_merge_queue_namespace 'abcdef1234567890')" 'merge-queue namespace from short SHA'
assert_eq 'hypershell-ci-mq-abcdef1' "$(pr_env_merge_queue_namespace 'ABCDEF1234567890')" 'merge-queue namespace lowercases SHA'
assert_eq 'true' "$([[ "$(pr_env_main_namespace 'abcdef1234567890')" != "$(pr_env_merge_queue_namespace 'abcdef1234567890')" ]] && echo true || echo false)" 'main and merge-queue namespaces never collide for the same SHA'

# The platform namespace must remain an RFC 1123 label within 54 chars so the
# derived -keycloak name stays under 63. Even a large PR number fits easily.
big_ns="$(pr_env_namespace 999999)"
assert_eq 'true' "$([[ ${#big_ns} -le 54 ]] && echo true || echo false)" 'platform namespace within 54 chars'
main_ns="$(pr_env_main_namespace '0123456789abcdef')"
assert_eq 'true' "$([[ ${#main_ns} -le 54 ]] && echo true || echo false)" 'main namespace within 54 chars'
main_kc="$(pr_env_keycloak_namespace "${main_ns}")"
assert_eq 'true' "$([[ ${#main_kc} -le 63 ]] && echo true || echo false)" 'main keycloak namespace within 63 chars'
mq_ns="$(pr_env_merge_queue_namespace '0123456789abcdef')"
assert_eq 'true' "$([[ ${#mq_ns} -le 54 ]] && echo true || echo false)" 'merge-queue namespace within 54 chars'
mq_kc="$(pr_env_keycloak_namespace "${mq_ns}")"
assert_eq 'true' "$([[ ${#mq_kc} -le 63 ]] && echo true || echo false)" 'merge-queue keycloak namespace within 63 chars'

# Full SHA in the namespace would push -keycloak over 63 (19+40+9=68).
full_sha_ns="hypershell-ci-main-0123456789abcdef0123456789abcdef01234567"
full_sha_kc="$(pr_env_keycloak_namespace "${full_sha_ns}")"
assert_eq 'true' "$([[ ${#full_sha_kc} -gt 63 ]] && echo true || echo false)" 'full SHA main keycloak would exceed 63 chars'

# --- Timebox round-trip (injected clock for determinism) ---
base=1000000000  # 2001-09-09T01:46:40Z
assert_eq '2001-09-09T01:46:40Z' "$(pr_env_epoch_to_rfc3339 "${base}")" 'epoch -> rfc3339'
assert_eq "${base}" "$(pr_env_rfc3339_to_epoch "$(pr_env_epoch_to_rfc3339 "${base}")")" 'rfc3339 -> epoch round-trip'
# 72-hour retained max is exactly 72*3600 seconds ahead.
assert_eq "$(pr_env_epoch_to_rfc3339 $((base + 72 * 3600)))" "$(pr_env_expires_at_hours 72 "${base}")" 'expires_at_hours retained default'
# 24-hour unretained max is exactly 24*3600 seconds ahead.
assert_eq '24' "${PR_ENV_UNRETAINED_MAX_HOURS}" 'unretained default hours'
assert_eq "$(pr_env_epoch_to_rfc3339 $((base + 24 * 3600)))" "$(pr_env_expires_at_hours 24 "${base}")" 'expires_at_hours unretained default'
assert_eq "$(pr_env_expires_at_hours 72 "${base}")" "$(pr_env_inactivity_expires_at true "${base}")" 'retained inactivity expiry uses 72h'
assert_eq "$(pr_env_expires_at_hours 24 "${base}")" "$(pr_env_inactivity_expires_at false "${base}")" 'unretained inactivity expiry uses 24h'

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
  *'Deploying commit `abcdef1` to an ephemeral OpenShift environment. This comment will update in place once the environment is ready.'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: first-deploy placeholder missing deploying wording' ;;
esac
case "${deploying_body}" in
  *$'. This\ncomment'*) FAIL=$((FAIL + 1)); echo 'FAIL: deploying sentence must not wrap at 80 chars' ;;
  *) PASS=$((PASS + 1)) ;;
esac
case "${deploying_body}" in
  *'may not be fully responsive'*) FAIL=$((FAIL + 1)); echo 'FAIL: first-deploy placeholder must not warn about an existing environment' ;;
  *) PASS=$((PASS + 1)) ;;
esac
case "${deploying_body}" in
  *'/pr-extend'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing /pr-extend' ;;
esac
case "${deploying_body}" in
  *'destroyed once e2e testing concludes'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing destroy-after-e2e wording' ;;
esac
case "${deploying_body}" in
  *'already been destroyed'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing redeploy-if-destroyed wording' ;;
esac
case "${deploying_body}" in
  *'Track this deploy'*) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment has a run link outside a run' ;;
  *) PASS=$((PASS + 1)) ;;
esac

# --- Run link (Track this deploy), so /pr-extend and synchronize deploys are
# traceable to the issue_comment / pull_request run posting the comment ---
assert_eq '' "$(pr_env_run_link)" 'no run link outside a GitHub Actions run'
assert_eq '' "$(GITHUB_RUN_ID=42 pr_env_run_link)" 'no run link without GITHUB_REPOSITORY'
assert_eq '' "$(GITHUB_REPOSITORY=openshift-online/hypershell pr_env_run_link)" 'no run link without GITHUB_RUN_ID'
assert_eq '[Track this deploy](https://github.com/openshift-online/hypershell/actions/runs/42)' \
  "$(GITHUB_RUN_ID=42 GITHUB_REPOSITORY=openshift-online/hypershell pr_env_run_link)" \
  'run link defaults to github.com when GITHUB_SERVER_URL is unset'
assert_eq '[Track this deploy](https://ghe.example.com/openshift-online/hypershell/actions/runs/42)' \
  "$(GITHUB_RUN_ID=42 GITHUB_REPOSITORY=openshift-online/hypershell GITHUB_SERVER_URL=https://ghe.example.com pr_env_run_link)" \
  'run link honors a non-default GITHUB_SERVER_URL'

deploying_with_link="$(GITHUB_RUN_ID=42 GITHUB_REPOSITORY=openshift-online/hypershell \
  pr_env_comment_deploying_body abcdef1234567)"
case "${deploying_with_link}" in
  *'[Track this deploy](https://github.com/openshift-online/hypershell/actions/runs/42)'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: deploying comment missing run link when GITHUB_RUN_ID is set' ;;
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
assert_eq 'extend' "$(pr_env_command_from_body $'/pr-extend\r\n\r\n')" 'GitHub web UI CRLF /pr-extend'
assert_eq 'extend' "$(pr_env_command_from_body $'\r\n/pr-extend\r\n\r\n')" 'leading blank line then CRLF /pr-extend'
assert_eq 'destroy' "$(pr_env_command_from_body '/pr-destroy')" 'bare /pr-destroy'
assert_eq 'destroy' "$(pr_env_command_from_body '/pr-destroy now')" '/pr-destroy with trailing text'
assert_eq 'destroy' "$(pr_env_command_from_body $'/pr-destroy\r\n')" 'GitHub web UI CRLF /pr-destroy'
assert_eq '' "$(pr_env_command_from_body '/pr-extended')" '/pr-extended is not /pr-extend'
assert_eq '' "$(pr_env_command_from_body $'/pr-extended\r\n')" 'CRLF /pr-extended is not /pr-extend'
assert_eq '' "$(pr_env_command_from_body '/pr-destroyed')" '/pr-destroyed is not /pr-destroy'
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
  $'2026-09-16T11:00:00Z\talice\twrite\t/pr-destroy' \
  $'2026-09-16T12:00:00Z\talice\twrite\t/pr-extend' \
  | pr_env_select_latest_command)" 'latest of extend/destroy/extend is extend'

assert_eq 'extend' "$(printf '%s\n' \
  $'2026-09-16T12:00:00Z\talice\twrite\t/pr-extend\r' \
  | pr_env_select_latest_command)" 'CRLF extend in command history still counts'

assert_eq 'destroy' "$(printf '%s\n' \
  $'2026-09-16T12:00:00Z\talice\twrite\t/pr-destroy' \
  $'2026-09-16T10:00:00Z\talice\twrite\t/pr-extend' \
  | pr_env_select_latest_command)" 'out-of-order rows still pick latest created_at'

assert_eq 'extend' "$(printf '%s\n' \
  $'2026-09-16T10:00:00Z\talice\twrite\t/pr-extend' \
  $'2026-09-16T11:00:00Z\tbob\tread\t/pr-destroy' \
  | pr_env_select_latest_command)" 'unauthorized /pr-destroy does not count'

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
case "${body}" in
  *'Log in through the web console with your GitHub account'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing GitHub web-console login' ;;
esac
case "${body}" in
  *'impersonate that user'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing Keycloak impersonation guidance' ;;
esac
case "${body}" in
  *'| Keycloak admin console |'*) FAIL=$((FAIL + 1)); echo 'FAIL: comment body included Keycloak admin console without KEYCLOAK_URL' ;;
  *) PASS=$((PASS + 1)) ;;
esac
kc_body="$(KEYCLOAK_URL=https://keycloak.pr-232.example.com/ pr_env_comment_body 232 abcdef1234567 hypershell-ci-pr-232 hypershell-ci-pr-232-keycloak \
  https://console.example.com https://api.pr-232.example.com https://web.pr-232.example.com \
  https://api.cluster.example.com:6443 false)"
case "${kc_body}" in
  *'| Keycloak admin console | https://keycloak.pr-232.example.com/admin/hypershell/console/ |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: comment body missing hypershell-realm Keycloak admin console URL' ;;
esac
case "${kc_body}" in
  *'/admin/'*'/admin/'*) FAIL=$((FAIL + 1)); echo 'FAIL: comment body linked master-realm /admin/ instead of hypershell console' ;;
  *) PASS=$((PASS + 1)) ;;
esac
kc_updating="$(pr_env_comment_deploying_body fffffff111111 "${kc_body}")"
case "${kc_updating}" in
  *'| Keycloak admin console | https://keycloak.pr-232.example.com/admin/hypershell/console/ |'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: updating comment dropped Keycloak admin console URL' ;;
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
  *'/pr-extend'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing /pr-extend' ;;
esac
case "${body}" in
  *'keep this environment active'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing keep-active wording' ;;
esac
case "${body}" in
  *'destroyed once e2e testing concludes'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing destroy-after-e2e wording' ;;
esac
case "${body}" in
  *'already been destroyed'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment missing redeploy-if-destroyed wording' ;;
esac
case "${body}" in
  *'refreshed on every new commit'*) FAIL=$((FAIL + 1)); echo 'FAIL: unretained comment implied persistence' ;;
  *) PASS=$((PASS + 1)) ;;
esac
retained_body="$(PR_ENV_EXPIRES_AT=2026-09-20T17:15:00Z pr_env_comment_body 232 abcdef1234567 ns ns-keycloak c a w https://api.cluster.example.com false true)"
case "${retained_body}" in
  *'retained and renewed on every commit'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment missing retained wording' ;;
esac
case "${retained_body}" in
  *'inactivity timebox'*'2026-09-20T17:15:00Z'*' UTC'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment missing UTC inactivity expiry' ;;
esac
case "${retained_body}" in
  *'destroyed once e2e testing concludes'*) FAIL=$((FAIL + 1)); echo 'FAIL: retained comment said env is about to be destroyed' ;;
  *) PASS=$((PASS + 1)) ;;
esac
retained_deploying="$(PR_ENV_EXPIRES_AT=2026-09-20T17:15:00Z pr_env_comment_deploying_body abcdef1234567 '' true)"
case "${retained_deploying}" in
  *'retained and renewed on every commit'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained deploying comment missing retained wording' ;;
esac
case "${retained_deploying}" in
  *'2026-09-20T17:15:00Z'*' UTC'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: retained deploying comment missing UTC inactivity expiry' ;;
esac
case "${retained_deploying}" in
  *'destroyed once e2e testing concludes'*) FAIL=$((FAIL + 1)); echo 'FAIL: retained deploying comment said env is about to be destroyed' ;;
  *) PASS=$((PASS + 1)) ;;
esac

destroyed_body="$(pr_env_comment_destroyed_body)"
case "${destroyed_body}" in
  *"<!-- hypershell-pr-environment -->"*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment missing hidden marker' ;;
esac
case "${destroyed_body}" in
  *'## HyperShell environment destroyed'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment missing destroyed heading' ;;
esac
case "${destroyed_body}" in
  *'has been destroyed'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment missing destroyed wording' ;;
esac
case "${destroyed_body}" in
  *'/pr-extend'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment missing /pr-extend' ;;
esac
case "${destroyed_body}" in
  *'redeploy'*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment missing redeploy wording' ;;
esac
case "${destroyed_body}" in
  *'live ephemeral'*) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment still claims a live environment' ;;
  *) PASS=$((PASS + 1)) ;;
esac
case "${destroyed_body}" in
  *'| Fact | Value |'*) FAIL=$((FAIL + 1)); echo 'FAIL: destroyed comment must not keep the access-fact table' ;;
  *) PASS=$((PASS + 1)) ;;
esac

if grep -q 'PR_ENV_PHASE=destroyed' "${SCRIPT_DIR}/teardown-unretained-pr-env.sh"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: unretained teardown does not update the access comment after destroy'
fi
if grep -q 'No PR_NUMBER' "${SCRIPT_DIR}/teardown-unretained-pr-env.sh"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: unretained teardown does not handle push-to-main (empty PR_NUMBER)'
fi
if grep -q 'PR_ENV_PHASE=destroyed' "${SCRIPT_DIR}/../../.github/workflows/pr-environment-commands.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: /pr-destroy does not update the access comment after destroy'
fi
if grep -q "contains(github.event.comment.body, '/pr-extend')" \
  "${SCRIPT_DIR}/../../.github/workflows/pr-environment-commands.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: command workflow if does not match /pr-extend inside a CRLF body'
fi
if grep -q 'PR_ENV_EXPIRES_AT: ${{ steps.timebox.outputs.expires_at }}' \
  "${SCRIPT_DIR}/../../.github/actions/deploy-pr-environment/action.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: deploying comment / stamp do not receive the precomputed UTC expiry'
fi
if grep -q 'PR_ENV_EXPIRES_AT: ${{ steps.stamp.outputs.expires_at }}' \
  "${SCRIPT_DIR}/../../.github/actions/deploy-pr-environment/action.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ready comment does not receive the stamped UTC expiry'
fi

assert_eq 'true' "$(printf '%s' '[{"name":"pr-environment/pr-extended"}]' \
  | pr_env_label_list_has 'pr-environment/pr-extended')" 'label list has retained'
assert_eq 'false' "$(printf '%s' '[]' \
  | pr_env_label_list_has 'pr-environment/pr-extended')" 'empty label list is not retained'
assert_eq 'false' "$(printf '%s' '[{"name":"other"}]' \
  | pr_env_label_list_has 'pr-environment/pr-extended')" 'unrelated labels are not retained'

resolve_env="$(mktemp)"
PR_NUMBER=232 GITHUB_ENV="${resolve_env}" bash "${SCRIPT_DIR}/resolve-openshift-namespace.sh" >/dev/null
assert_eq 'OPENSHIFT_NAMESPACE=hypershell-ci-pr-232' "$(cat "${resolve_env}")" 'resolve script writes PR namespace'
rm -f "${resolve_env}"
resolve_env="$(mktemp)"
PR_NUMBER= GITHUB_EVENT_NAME=push GITHUB_SHA='abcdef1234567890deadbeef' GITHUB_ENV="${resolve_env}" \
  bash "${SCRIPT_DIR}/resolve-openshift-namespace.sh" >/dev/null
assert_eq 'OPENSHIFT_NAMESPACE=hypershell-ci-main-abcdef1' "$(cat "${resolve_env}")" 'resolve script writes per-commit main namespace'
rm -f "${resolve_env}"
resolve_env="$(mktemp)"
PR_NUMBER= GITHUB_EVENT_NAME=merge_group GITHUB_SHA='abcdef1234567890deadbeef' GITHUB_ENV="${resolve_env}" \
  bash "${SCRIPT_DIR}/resolve-openshift-namespace.sh" >/dev/null
assert_eq 'OPENSHIFT_NAMESPACE=hypershell-ci-mq-abcdef1' "$(cat "${resolve_env}")" 'resolve script writes per-commit merge-queue namespace'
rm -f "${resolve_env}"

printf 'pr-env-lib tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
