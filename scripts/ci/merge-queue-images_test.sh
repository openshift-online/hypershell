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

assert_ok "merge_group skips docs-only batches via detect-konflux, not wait flags" \
  grep -q 'should_run stays path-gated via detect-konflux' "${E2E_YML}"

# plan-images' merge_group case is the second `merge_group)` in the file
# (the first is Compute changed files). Wait flags there are literals, not
# detect-konflux outputs, so unchanged components still get on-merge-queue
# images.
mq_plan_wait_api() {
  awk '
    /^            merge_group)/ { n++ }
    n == 2 && /echo "wait_api_server=true"/ { found = 1 }
    n == 2 && /^            push)/ { exit }
    END { exit found ? 0 : 1 }
  ' "${E2E_YML}"
}
mq_plan_wait_cp() {
  awk '
    /^            merge_group)/ { n++ }
    n == 2 && /echo "wait_control_plane=true"/ { found = 1 }
    n == 2 && /^            push)/ { exit }
    END { exit found ? 0 : 1 }
  ' "${E2E_YML}"
}
mq_plan_wait_wc() {
  awk '
    /^            merge_group)/ { n++ }
    n == 2 && /echo "wait_web_console=true"/ { found = 1 }
    n == 2 && /^            push)/ { exit }
    END { exit found ? 0 : 1 }
  ' "${E2E_YML}"
}

assert_ok "merge_group wait_api_server is unconditional" mq_plan_wait_api
assert_ok "merge_group wait_control_plane is unconditional" mq_plan_wait_cp
assert_ok "merge_group wait_web_console is unconditional" mq_plan_wait_wc

assert_ok "Kind waits on merge_group.head_sha" \
  grep -q 'github.event.merge_group.head_sha || github.event.pull_request.head.sha || github.sha' "${E2E_YML}"

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
