#!/usr/bin/env bash
# upsert-pr-comment.sh - post or update the single pull-request access comment
# (ephemeral-pr-environments.spec.md: Pull-Request Comment and Access Handoff).
#
# Keeps exactly one comment current for the pull request by locating the comment
# carrying the hidden marker and editing it in place, rather than posting a new
# comment per run. Called twice per deploy run: once at the very start with
# PR_ENV_PHASE=deploying, and again once the environment is ready with the real
# access facts and an `oc login --web` template. The deploying phase posts a
# no-facts placeholder on first deploy (so the comment is normally first on the
# pull request and stays near the top of the timeline). On a later reconcile it
# updates the heading to the new commit and keeps the existing access-fact
# table, because those URLs and namespaces do not change from run to run.
# OpenShift handles token retrieval and refresh interactively, so no credential
# ever appears in the comment.
#
# Requires `gh` (authenticated via GH_TOKEN) and `jq`.
#
# Environment:
#   GH_REPO / GITHUB_REPOSITORY   owner/repo (gh reads GH_REPO)
#   PR_NUMBER                     pull-request number (required)
#   PR_HEAD_SHA                   head commit SHA (required)
#   PR_ENV_PHASE                  "deploying" (in-progress heading; posted
#                                 first, keeps an existing access-fact table)
#                                 or "ready" (default; full access facts)
#   PR_ENV_UPDATED                "true" for the per-commit update wording
#                                 ("ready" phase only)
#   PLATFORM_NS / KEYCLOAK_NS     namespace group ("ready" phase only)
#   CONSOLE_URL / API_URL / WEB_URL / CLUSTER_API_URL   access URLs
#                                 ("ready" phase only)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${PR_NUMBER:?PR_NUMBER is required}"
: "${PR_HEAD_SHA:?PR_HEAD_SHA is required}"
repo="${GH_REPO:-${GITHUB_REPOSITORY:?GH_REPO or GITHUB_REPOSITORY is required}}"

# Find an existing marked comment first (paginate; the marker is unique to
# this bot) so the deploying phase can keep its access-fact table.
existing_id="$(gh api --paginate \
  "repos/${repo}/issues/${PR_NUMBER}/comments" \
  --jq ".[] | select(.body | contains(\"${PR_ENV_COMMENT_MARKER}\")) | .id" \
  2>/dev/null | head -n1 || true)"

phase="${PR_ENV_PHASE:-ready}"
case "${phase}" in
  deploying)
    existing_body=""
    if [[ -n "${existing_id}" ]]; then
      existing_body="$(gh api "repos/${repo}/issues/comments/${existing_id}" --jq .body)"
    fi
    body="$(pr_env_comment_deploying_body "${PR_HEAD_SHA}" "${existing_body}")"
    ;;
  ready)
    body="$(pr_env_comment_body \
      "${PR_NUMBER}" \
      "${PR_HEAD_SHA}" \
      "${PLATFORM_NS:-}" \
      "${KEYCLOAK_NS:-}" \
      "${CONSOLE_URL:-}" \
      "${API_URL:-}" \
      "${WEB_URL:-}" \
      "${CLUSTER_API_URL:-}" \
      "${PR_ENV_UPDATED:-false}")"
    ;;
  *)
    echo "::error::Unknown PR_ENV_PHASE '${phase}' (want deploying or ready)" >&2
    exit 1
    ;;
esac

if [[ -n "${existing_id}" ]]; then
  echo "Updating existing access comment ${existing_id}"
  gh api --method PATCH "repos/${repo}/issues/comments/${existing_id}" \
    -f body="${body}" >/dev/null
else
  echo "Posting initial access comment"
  gh api --method POST "repos/${repo}/issues/${PR_NUMBER}/comments" \
    -f body="${body}" >/dev/null
fi
