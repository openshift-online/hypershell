#!/usr/bin/env bash
# upsert-pr-comment.sh - post or update the single pull-request access comment
# (ephemeral-pr-environments.spec.md: Pull-Request Comment and Access Handoff).
#
# Keeps exactly one comment current for the pull request by locating the comment
# carrying the hidden marker and editing it in place, rather than posting a new
# comment per run. The comment carries only non-secret access facts and an
# `oc login --web` template; OpenShift handles token retrieval and refresh
# interactively, so no credential ever appears in the comment.
#
# Requires `gh` (authenticated via GH_TOKEN) and `jq`.
#
# Environment:
#   GH_REPO / GITHUB_REPOSITORY   owner/repo (gh reads GH_REPO)
#   PR_NUMBER                     pull-request number (required)
#   PR_HEAD_SHA                   head commit SHA (required)
#   PR_ENV_UPDATED                "true" for the per-commit update wording
#   PLATFORM_NS / KEYCLOAK_NS     namespace group
#   CONSOLE_URL / API_URL / WEB_URL   access URLs
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${PR_NUMBER:?PR_NUMBER is required}"
: "${PR_HEAD_SHA:?PR_HEAD_SHA is required}"
repo="${GH_REPO:-${GITHUB_REPOSITORY:?GH_REPO or GITHUB_REPOSITORY is required}}"

body="$(pr_env_comment_body \
  "${PR_NUMBER}" \
  "${PR_HEAD_SHA}" \
  "${PLATFORM_NS:-}" \
  "${KEYCLOAK_NS:-}" \
  "${CONSOLE_URL:-}" \
  "${API_URL:-}" \
  "${WEB_URL:-}" \
  "${PR_ENV_UPDATED:-false}")"

# Find an existing marked comment (paginate; the marker is unique to this bot).
existing_id="$(gh api --paginate \
  "repos/${repo}/issues/${PR_NUMBER}/comments" \
  --jq ".[] | select(.body | contains(\"${PR_ENV_COMMENT_MARKER}\")) | .id" \
  2>/dev/null | head -n1 || true)"

if [[ -n "${existing_id}" ]]; then
  echo "Updating existing access comment ${existing_id}"
  gh api --method PATCH "repos/${repo}/issues/comments/${existing_id}" \
    -f body="${body}" >/dev/null
else
  echo "Posting initial access comment"
  gh api --method POST "repos/${repo}/issues/${PR_NUMBER}/comments" \
    -f body="${body}" >/dev/null
fi
