#!/usr/bin/env bash
# Contract tests: merge-queue Kind consumes on-merge-queue images for every
# component, and the Konflux merge-queue pipelines fire on every queue push.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PASS=0
FAIL=0

assert_ok() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s\n' "${label}"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected no match)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

E2E_YML="${REPO_ROOT}/.github/workflows/e2e.yml"

# plan-images' merge_group case is the second standalone `merge_group)` in the
# file (the first is Compute changed files). The one-line
# `merge_group) konflux_ref=...` assignment is not standalone.
mq_plan_awk() {
  awk '
    /^            merge_group)$/ { n++ }
    n == 2 { print }
    n == 2 && /^            push)/ { exit }
  ' "${E2E_YML}"
}

mq_skip_uses_detect() {
  mq_plan_awk | awk '
    /detect_e2e_relevant/ { detect = 1 }
    /KONFLUX_WEB_CONSOLE/ { wc = 1 }
    /echo "should_run=false"/ { skip = 1 }
    /echo "wait_api_server=\$\{KONFLUX_API_SERVER\}"/ { gated = 1 }
    END { exit (detect && wc && skip && !gated) ? 0 : 1 }
  '
}

mq_tag_uses_konflux_ref() {
  mq_plan_awk | grep -q 'tag="on-merge-queue-${konflux_ref}"'
}

mq_tag_not_push_sha() {
  ! { mq_plan_awk | grep -q 'tag="on-merge-queue-${PUSH_SHA}"'; }
}

mq_plan_wait_api() {
  mq_plan_awk | grep -q 'echo "wait_api_server=true"'
}
mq_plan_wait_cp() {
  mq_plan_awk | grep -q 'echo "wait_control_plane=true"'
}
mq_plan_wait_wc() {
  mq_plan_awk | grep -q 'echo "wait_web_console=true"'
}

assert_ok "merge_group skips docs-only batches via detect_e2e_relevant, not wait flags" \
  mq_skip_uses_detect

assert_ok "merge_group wait_api_server is unconditional" mq_plan_wait_api
assert_ok "merge_group wait_control_plane is unconditional" mq_plan_wait_cp
assert_ok "merge_group wait_web_console is unconditional" mq_plan_wait_wc

assert_ok "plan-images exports konflux_ref" \
  grep -q 'konflux_ref: ${{ steps.plan.outputs.konflux_ref }}' "${E2E_YML}"
assert_ok "merge_group image tag uses konflux_ref" mq_tag_uses_konflux_ref
assert_ok "merge_group image tag does not use PUSH_SHA" mq_tag_not_push_sha
assert_ok "Kind wait-on-check uses plan-images konflux_ref" \
  grep -q 'ref: ${{ needs.plan-images.outputs.konflux_ref }}' "${E2E_YML}"
assert_ok "OpenShift wait uses plan-images konflux_ref" \
  grep -q 'head_sha: ${{ needs.plan-images.outputs.konflux_ref }}' "${E2E_YML}"

for pipeline in \
  hypershell-api-server-main-merge-queue.yaml \
  hypershell-control-plane-main-merge-queue.yaml \
  hypershell-web-console-main-merge-queue.yaml; do
  f="${REPO_ROOT}/.tekton/${pipeline}"
  assert_ok "${pipeline} fires on gh-readonly-queue/main/" \
    grep -q 'target_branch.startsWith("gh-readonly-queue/main/")' "${f}"
  assert_fail "${pipeline} has no pathChanged filter" \
    grep -q 'pathChanged' "${f}"
done

echo "PASS=${PASS} FAIL=${FAIL}"
if [[ "${FAIL}" -ne 0 ]]; then
  exit 1
fi
