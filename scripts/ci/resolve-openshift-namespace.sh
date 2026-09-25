#!/usr/bin/env bash
# resolve-openshift-namespace.sh - write OPENSHIFT_NAMESPACE for this CI run.
#
# Origin PRs use hypershell-ci-pr-<n>. Push to main uses
# hypershell-ci-main-<short-sha>, and a merge queue entry uses
# hypershell-ci-mq-<short-sha>, so a cancelled older run cannot
# openshift-down a newer deploy's namespace (e2e-testing.spec.md).
#
# Environment:
#   PR_NUMBER          pull-request number; empty on push to main or merge_group
#   GITHUB_EVENT_NAME  GitHub Actions default; selects main vs merge-queue
#                      naming when PR_NUMBER is empty
#   GITHUB_SHA         full commit SHA (required when PR_NUMBER is empty)
#   GITHUB_ENV         if set, also append OPENSHIFT_NAMESPACE for later steps
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

if [[ -n "${PR_NUMBER:-}" ]]; then
  ns="$(pr_env_namespace "${PR_NUMBER}")"
elif [[ "${GITHUB_EVENT_NAME:-}" == "merge_group" ]]; then
  : "${GITHUB_SHA:?GITHUB_SHA is required when PR_NUMBER is empty}"
  ns="$(pr_env_merge_queue_namespace "${GITHUB_SHA}")"
else
  : "${GITHUB_SHA:?GITHUB_SHA is required when PR_NUMBER is empty}"
  ns="$(pr_env_main_namespace "${GITHUB_SHA}")"
fi

if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "OPENSHIFT_NAMESPACE=${ns}" >> "${GITHUB_ENV}"
fi

echo "Using OpenShift namespace ${ns}"
