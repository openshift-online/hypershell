#!/usr/bin/env bash
# teardown-unretained-pr-env.sh - destroy the per-PR OpenShift environment
# unless it is retained (ephemeral-pr-environments.spec.md).
#
# Skip ONLY when the pull request currently has pr-environment/pr-extended.
# Success, failure, and cancel of the caller all run this; a retained label
# is the sole opt-out. Requires gh (GH_TOKEN) and a logged-in oc
# (OPENSHIFT_NAMESPACE, make openshift-down).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${PR_NUMBER:?PR_NUMBER is required}"
repo="${GH_REPO:-${GITHUB_REPOSITORY:?GH_REPO or GITHUB_REPOSITORY is required}}"

has="$(gh api "repos/${repo}/issues/${PR_NUMBER}/labels" \
  | pr_env_label_list_has "${PR_ENV_RETAINED_LABEL}")"
echo "Retained label present: ${has}"
if [[ "${has}" == "true" ]]; then
  echo "Skipping teardown; environment is retained"
  exit 0
fi

make openshift-down

# Keep the one marked comment honest: it must not keep claiming a live
# environment. Best-effort; a GitHub API failure must not fail teardown.
if ! PR_ENV_PHASE=destroyed bash "${SCRIPT_DIR}/upsert-pr-comment.sh"; then
  echo "::warning::Could not update the access comment after destroy; the environment is still gone"
fi
