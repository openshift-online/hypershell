#!/usr/bin/env bash
# pre-lifecycle.sh - shared lifecycle pre-script (SDLC-HOOK-03).
#
# Runs on the trusted runner before the agent starts, with stage-specific
# configuration taken from native Fullsend inputs and environment (never from
# local dispatcher state or a local database). It:
#
#   1. Derives the target repo and issue number from the trusted work-item URL.
#   2. Confirms authorization (agent/allowed), scope (LABEL_SCOPE), the stage
#      prerequisites, and - for PR phases - the issue-to-pull-request link.
#   3. Records the starting phase labels and the pull request head commit so the
#      post-script can detect a changed head and preserve human decisions.
#   4. Skips the run through Fullsend's pre-script output when a prerequisite no
#      longer holds, so the agent does not run.
#
# Relay contract: every KEY=VALUE line written to $FULLSEND_PRESCRIPT_OUTPUT is
# exported into the later stages. Uppercase LIFECYCLE_* keys carry recorded
# state. The lowercase reserved key `skip` requests that Fullsend skip the run;
# its value is the human-readable reason. The post-script re-derives and
# re-verifies all of this independently, so a transition is never applied on
# stale or ineligible state even if the skip is not honored.

set -euo pipefail
export LC_ALL="${LC_ALL:-C.UTF-8}"

_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/lifecycle-contract.sh
. "${_dir}/lib/lifecycle-contract.sh"
# shellcheck source=lib/lifecycle-logic.sh
. "${_dir}/lib/lifecycle-logic.sh"
# shellcheck source=lib/github.sh
. "${_dir}/lib/github.sh"

PHASE="${PHASE:-}"
ISSUE_URL="${ISSUE_URL:-${GITHUB_ISSUE_URL:-${FULLSEND_WORK_ITEM_URL:-}}}"
LABEL_SCOPE="${LABEL_SCOPE:-}"

: "${PHASE:?PHASE must be set}"
: "${ISSUE_URL:?ISSUE_URL must be set}"
: "${GH_TOKEN:?GH_TOKEN must be set}"

if ! lifecycle_outcomes "${PHASE}" >/dev/null; then
  echo "pre-lifecycle: unknown PHASE ${PHASE}" >&2
  exit 1
fi

# Append a KEY=VALUE relay line, or print it when no output path is configured
# (local invocation and tests).
_relay() {
  if [[ -n "${FULLSEND_PRESCRIPT_OUTPUT:-}" ]]; then
    printf '%s=%s\n' "$1" "$2" >> "${FULLSEND_PRESCRIPT_OUTPUT}"
  else
    printf '%s=%s\n' "$1" "$2"
  fi
}

# Request that Fullsend skip the run, then exit 0 so the pre-script itself did
# not fail. The reason is a single flattened line.
_skip() {
  local reason="${1//[$'\r\n']/ }"
  echo "pre-lifecycle: skipping run: ${reason}" >&2
  _relay skip "${reason}"
  exit 0
}

read -r REPO NUMBER _kind < <(lifecycle_parse_issue_url "${ISSUE_URL}") || {
  echo "pre-lifecycle: ISSUE_URL is not a GitHub issue or pull request URL" >&2
  exit 1
}

# Snapshot the starting labels once.
labels="$(gh_get_labels "${REPO}" "${NUMBER}")"
_has() { grep -qxF "$1" <<< "${labels}"; }

# Authorization and scope are human gates: if they are gone, do not run.
_has "agent/allowed" || _skip "agent/allowed is not present"
if [[ -n "${LABEL_SCOPE}" ]]; then
  _has "${LABEL_SCOPE}" || _skip "scope label ${LABEL_SCOPE} is not present"
fi
# A human block halts the funnel for every phase (SDLC-HOOK-12).
_has "agent/blocked" && _skip "agent/blocked is present"

# Stage prerequisites mirror the harness triggers, re-checked against live state.
case "${PHASE}" in
  triage)
    for l in agent/workable agent/needs-input agent/duplicate \
             agent/reviewable-spec agent/review-spec-approved agent/review-spec-rejected \
             agent/reviewable-code agent/review-code-approved agent/review-code-rejected; do
      _has "${l}" && _skip "already triaged or in flight (${l} present)"
    done
    ;;
  spec-implementation)
    if ! { _has "agent/workable" || _has "agent/review-spec-rejected"; }; then
      _skip "no spec-implementation trigger label present"
    fi
    _has "agent/reviewable-spec" && _skip "spec already reviewable"
    _has "agent/review-spec-approved" && _skip "spec already approved"
    ;;
  spec-review)
    _has "agent/reviewable-spec" || _skip "agent/reviewable-spec is not present"
    { _has "agent/review-spec-approved" || _has "agent/review-spec-rejected"; } \
      && _skip "spec already has a review verdict"
    ;;
  code-implementation)
    if ! { _has "agent/review-spec-approved" || _has "agent/review-code-rejected"; }; then
      _skip "no code-implementation trigger label present"
    fi
    _has "agent/reviewable-code" && _skip "code already reviewable"
    _has "agent/review-code-approved" && _skip "code already approved"
    ;;
  code-review)
    _has "agent/reviewable-code" || _skip "agent/reviewable-code is not present"
    { _has "agent/review-code-approved" || _has "agent/review-code-rejected"; } \
      && _skip "code already has a review verdict"
    ;;
esac

# Record the starting state for the post-script.
_relay LIFECYCLE_PHASE "${PHASE}"
_relay LIFECYCLE_REPO "${REPO}"
_relay LIFECYCLE_ISSUE_NUMBER "${NUMBER}"
_relay LIFECYCLE_START_LABELS "$(tr '\n' ',' <<< "${labels}" | sed 's/,$//')"

# For PR phases, record the linked pull request and its head commit.
if lifecycle_phase_needs_pr "${PHASE}"; then
  pr_number="${PR_NUMBER:-}"
  if [[ -z "${pr_number}" ]]; then
    pr_number="$(gh_linked_pr_number "${REPO}" "${NUMBER}")"
  fi
  if [[ -z "${pr_number}" ]]; then
    _skip "no open linked pull request for a ${PHASE} run"
  fi
  head_sha="$(gh_pr_head_sha "${REPO}" "${pr_number}")"
  if [[ -z "${head_sha}" ]]; then
    _skip "could not resolve the pull request head commit"
  fi
  _relay LIFECYCLE_PR_NUMBER "${pr_number}"
  _relay LIFECYCLE_PR_HEAD_SHA "${head_sha}"
fi

echo "pre-lifecycle: ${PHASE} prerequisites satisfied for ${REPO}#${NUMBER}" >&2
exit 0
