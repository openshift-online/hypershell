#!/usr/bin/env bash
# teardown-unretained-pr-env.sh - destroy the OpenShift environment unless
# it is retained (ephemeral-pr-environments.spec.md).
#
# Skip ONLY when the pull request currently has pr-environment/pr-extended.
# Push to main has no pull request and therefore no retainment label, so it
# always tears down. Success, failure, and cancel of the caller all run
# this; a retained label is the sole opt-out. Requires a logged-in oc
# (OPENSHIFT_NAMESPACE, make openshift-down). gh (GH_TOKEN) is required only
# when PR_NUMBER is set.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

if [[ -n "${PR_NUMBER:-}" ]]; then
  repo="${GH_REPO:-${GITHUB_REPOSITORY:?GH_REPO or GITHUB_REPOSITORY is required}}"
  has="$(gh api "repos/${repo}/issues/${PR_NUMBER}/labels" \
    | pr_env_label_list_has "${PR_ENV_RETAINED_LABEL}")"
  echo "Retained label present: ${has}"
  if [[ "${has}" == "true" ]]; then
    echo "Skipping teardown; environment is retained"
    exit 0
  fi
else
  echo "No PR_NUMBER; tearing down unretained environment ${OPENSHIFT_NAMESPACE:-}"
fi

make openshift-down

# Keep the one marked comment honest: it must not keep claiming a live
# environment. Best-effort; a GitHub API failure must not fail teardown.
# Push to main has no access comment to update.
if [[ -n "${PR_NUMBER:-}" ]]; then
  if ! PR_ENV_PHASE=destroyed bash "${SCRIPT_DIR}/upsert-pr-comment.sh"; then
    echo "::warning::Could not update the access comment after destroy; the environment is still gone"
  fi
fi
