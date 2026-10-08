#!/usr/bin/env bash
# Contract tests: a merge queue builds and waits only for components whose
# image inputs changed. All other e2e images are immutable accepted-base refs.
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
    /echo "should_run=false"/ { skip = 1 }
    /No e2e-relevant components changed in merge batch/ { message = 1 }
    END { exit (detect && skip && message) ? 0 : 1 }
  '
}

mq_tag_uses_konflux_ref() {
  mq_plan_awk | grep -q 'tag="on-merge-queue-${konflux_ref}"'
}

mq_tag_not_push_sha() {
  ! { mq_plan_awk | grep -q 'tag="on-merge-queue-${PUSH_SHA}"'; }
}

mq_plan_wait_api() {
  mq_plan_awk | grep -q 'if \[\[ "${KONFLUX_API_SERVER}" == "true" \]\]'
}
mq_plan_wait_cp() {
  mq_plan_awk | grep -q 'if \[\[ "${KONFLUX_CONTROL_PLANE}" == "true" \]\]'
}
mq_plan_wait_wc() {
  mq_plan_awk | grep -q 'if \[\[ "${KONFLUX_WEB_CONSOLE}" == "true" \]\]'
}

pipeline_image_inputs() {
  rg -o '"[^"]+"\.pathChanged\(\)' "$1" |
    sed -E 's/^"([^"]+)"\.pathChanged\(\)$/\1/' |
    sed 's/-main-merge-queue\.yaml/-main-pull-request.yaml/' |
    sort
}

mq_inputs_match_pr_inputs() {
  local merge_queue_pipeline="$1"
  local pull_request_pipeline="${merge_queue_pipeline/-merge-queue/-pull-request}"
  diff -u \
    <(pipeline_image_inputs "${REPO_ROOT}/.tekton/${pull_request_pipeline}") \
    <(pipeline_image_inputs "${REPO_ROOT}/.tekton/${merge_queue_pipeline}")
}

assert_ok "merge_group skips docs-only batches via detect_e2e_relevant, not wait flags" \
  mq_skip_uses_detect

assert_ok "merge_group waits for api-server only when its image inputs changed" mq_plan_wait_api
assert_ok "merge_group waits for control-plane only when its image inputs changed" mq_plan_wait_cp
assert_ok "merge_group waits for web-console only when its image inputs changed" mq_plan_wait_wc
assert_ok "merge_group resolves unchanged images from the accepted base" \
  grep -q 'base_image_ref' "${E2E_YML}"
assert_ok "merge_group planner receives the accepted base SHA" \
  grep -q 'MERGE_GROUP_BASE_SHA: \${{ github.event.merge_group.base_sha }}' "${E2E_YML}"
assert_ok "accepted base images are digest pinned" \
  grep -q 'could not resolve immutable digest' "${E2E_YML}"

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
  hypershell-fleet-dashboard-main-merge-queue.yaml \
  hypershell-web-console-main-merge-queue.yaml; do
  f="${REPO_ROOT}/.tekton/${pipeline}"
  assert_ok "${pipeline} fires on gh-readonly-queue/main/" \
    grep -q 'target_branch.startsWith("gh-readonly-queue/main/")' "${f}"
  assert_ok "${pipeline} is filtered to image-input changes" \
    grep -q 'pathChanged' "${f}"
  assert_ok "${pipeline} image-input filters mirror pull-request Konflux" \
    mq_inputs_match_pr_inputs "${pipeline}"
done

assert_ok "api-server merge queue filter matches its source path" \
  grep -q '"components/api-server/\*\*\*".pathChanged()' "${REPO_ROOT}/.tekton/hypershell-api-server-main-merge-queue.yaml"
assert_ok "control-plane merge queue filter matches its source path" \
  grep -q '"components/control-plane/\*\*\*".pathChanged()' "${REPO_ROOT}/.tekton/hypershell-control-plane-main-merge-queue.yaml"
assert_ok "fleet-dashboard merge queue filter matches its source path" \
  grep -q '"components/fleet-dashboard/\*\*\*".pathChanged()' "${REPO_ROOT}/.tekton/hypershell-fleet-dashboard-main-merge-queue.yaml"
assert_ok "web-console merge queue filter matches its source path" \
  grep -q '"components/web-console/\*\*\*".pathChanged()' "${REPO_ROOT}/.tekton/hypershell-web-console-main-merge-queue.yaml"

echo "PASS=${PASS} FAIL=${FAIL}"
if [[ "${FAIL}" -ne 0 ]]; then
  exit 1
fi
