#!/usr/bin/env bash
# post-lifecycle.sh - shared lifecycle post-script (SDLC-HOOK-07, 08, 09, 12).
#
# Runs on the trusted runner after the sandbox exits. It consumes only a
# validated agent result, re-reads live GitHub state, checks evidence, applies
# only the label changes the matched transition names, and verifies the final
# state before reporting success.
#
# The agent result is untrusted model output. Only an allowlist of fields is
# read, each type-checked. The target repository and issue come from the trusted
# work-item URL, never from the result; any pull request number the result names
# is verified against the pre-script's recorded value (SDLC-HOOK-04).
#
# Failure classes are kept distinct in the logs (SDLC-HOOK-10.02):
#   - no validated result   -> workflow does not advance, no mutation
#   - invalid/mismatched     -> no mutation, non-zero exit
#   - stale review           -> no verdict transition, no mutation
#   - not eligible (checks)   -> approval not applied, no mutation
#   - label-application error -> actionable error, non-zero exit, not success
#
# Environment:
#   PHASE                              required, the phase slug
#   ISSUE_URL | GITHUB_ISSUE_URL | FULLSEND_WORK_ITEM_URL   the work item URL
#   GH_TOKEN                           required
#   LIFECYCLE_PR_NUMBER                recorded by the pre-script (PR phases)
#   LIFECYCLE_PR_HEAD_SHA              recorded by the pre-script (PR phases)
#   FULLSEND_VALIDATED_ITERATION_DIR   set by Fullsend when validation passed
#   DRY_RUN                            when truthy, decide and log but do not write

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

: "${PHASE:?PHASE must be set}"
: "${ISSUE_URL:?ISSUE_URL must be set}"
: "${GH_TOKEN:?GH_TOKEN must be set}"

_is_truthy() { case "${1:-}" in 1 | true | TRUE | yes | YES) return 0 ;; *) return 1 ;; esac; }
DRY_RUN_ON=0
_is_truthy "${DRY_RUN:-}" && DRY_RUN_ON=1

if ! lifecycle_outcomes "${PHASE}" >/dev/null; then
  echo "post-lifecycle: unknown PHASE ${PHASE}" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Locate the validated result.
# ---------------------------------------------------------------------------
RESULT_FILE=""
if [[ -n "${FULLSEND_VALIDATED_ITERATION_DIR:-}" ]]; then
  if [[ -f "${FULLSEND_VALIDATED_ITERATION_DIR}/agent-result.json" ]]; then
    RESULT_FILE="${FULLSEND_VALIDATED_ITERATION_DIR}/agent-result.json"
  else
    echo "post-lifecycle: FULLSEND_VALIDATED_ITERATION_DIR set but has no agent-result.json" >&2
    exit 1
  fi
else
  _best=-1
  for _d in iteration-*/output; do
    [[ -f "${_d}/agent-result.json" ]] || continue
    _n="${_d%/output}"; _n="${_n#iteration-}"
    [[ "${_n}" =~ ^[0-9]+$ ]] || continue
    if (( _n > _best )); then _best="${_n}"; RESULT_FILE="${_d}/agent-result.json"; fi
  done
fi

if [[ -z "${RESULT_FILE}" || ! -f "${RESULT_FILE}" ]]; then
  # No validated output: the workflow does not advance and no label changes
  # (SDLC-HOOK-06.02). This is distinct from agent/invalid/stale failures.
  echo "post-lifecycle: no validated agent result; not advancing the workflow, no label change" >&2
  exit 0
fi
if ! jq -e 'type == "object"' "${RESULT_FILE}" >/dev/null 2>&1; then
  echo "post-lifecycle: ${RESULT_FILE} is not a JSON object" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Read the allowlisted fields (type-checked; everything else is ignored).
# ---------------------------------------------------------------------------
_field_str() { jq -r --arg k "$1" 'if (.[$k] | type) == "string" then .[$k] else empty end' "${RESULT_FILE}"; }
_field_num() { jq -r --arg k "$1" 'if (.[$k] | type) == "number" then (.[$k] | floor | tostring) else empty end' "${RESULT_FILE}"; }

schema_version="$(_field_str schema_version)"
r_phase="$(_field_str phase)"
outcome="$(_field_str outcome)"
comment="$(_field_str comment)"
r_pr_number="$(_field_num pr_number)"
r_head_sha="$(_field_str head_sha)"

if [[ "${schema_version}" != "1" ]]; then
  echo "post-lifecycle: unsupported or missing schema_version (got '${schema_version:0:16}')" >&2
  exit 1
fi
if [[ "${r_phase}" != "${PHASE}" ]]; then
  echo "post-lifecycle: result phase '${r_phase:0:32}' does not match harness PHASE '${PHASE}'" >&2
  exit 1
fi
if ! lifecycle_outcome_valid "${PHASE}" "${outcome}"; then
  echo "post-lifecycle: outcome '${outcome:0:32}' is not permitted for phase ${PHASE}" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Trusted target. Repo and number come from the work-item URL, never the result.
# ---------------------------------------------------------------------------
read -r REPO NUMBER _kind < <(lifecycle_parse_issue_url "${ISSUE_URL}") || {
  echo "post-lifecycle: ISSUE_URL is not a GitHub issue or pull request URL" >&2
  exit 1
}

# Verify any identifier the agent returned against the trusted recorded value.
# The pre-script records LIFECYCLE_PR_NUMBER; the review harnesses also pass the
# forge-resolved PR_NUMBER, used as a trusted fallback if the relay is absent.
trusted_pr=""
if lifecycle_phase_needs_pr "${PHASE}"; then
  trusted_pr="${LIFECYCLE_PR_NUMBER:-${PR_NUMBER:-}}"
  if [[ -z "${trusted_pr}" ]]; then
    echo "post-lifecycle: no recorded pull request number for ${PHASE}; cannot verify evidence" >&2
    exit 1
  fi
  if [[ -n "${r_pr_number}" && "${r_pr_number}" != "${trusted_pr}" ]]; then
    echo "post-lifecycle: result pr_number ${r_pr_number} does not match recorded ${trusted_pr}" >&2
    exit 1
  fi
elif lifecycle_phase_creates_pr "${PHASE}"; then
  # This phase created the pull request. Prefer a value the pre-script recorded
  # (the pull request already existed - an update or a rerun). When none was
  # recorded (a fresh issue the agent just opened a pull request for), accept the
  # agent-reported pr_number only after verifying it is a genuine open pull
  # request in this repository that closes this issue (SDLC-HOOK-04.03).
  trusted_pr="${LIFECYCLE_PR_NUMBER:-${PR_NUMBER:-}}"
  if [[ -n "${trusted_pr}" ]]; then
    if [[ -n "${r_pr_number}" && "${r_pr_number}" != "${trusted_pr}" ]]; then
      echo "post-lifecycle: result pr_number ${r_pr_number} does not match recorded ${trusted_pr}" >&2
      exit 1
    fi
  elif [[ -n "${r_pr_number}" ]]; then
    if gh_pr_is_linked "${REPO}" "${r_pr_number}" "${NUMBER}"; then
      trusted_pr="${r_pr_number}"
    else
      echo "post-lifecycle: result pr_number ${r_pr_number} is not a verified open pull request that closes ${REPO}#${NUMBER}" >&2
      exit 1
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Re-read live state (SDLC-HOOK-08) and re-verify eligibility independently.
# ---------------------------------------------------------------------------
current_labels="$(gh_get_labels "${REPO}" "${NUMBER}")"
_cur_has() { grep -qxF "$1" <<< "${current_labels}"; }

# A review verdict only applies while the record is still awaiting that review.
if lifecycle_phase_is_review "${PHASE}"; then
  case "${PHASE}" in
    spec-review) _reviewable="agent/reviewable-spec" ;;
    code-review) _reviewable="agent/reviewable-code" ;;
  esac
  if ! _cur_has "${_reviewable}"; then
    echo "post-lifecycle: ${_reviewable} is no longer present; the review result no longer applies, no mutation" >&2
    exit 0
  fi

  # Stale check: the pull request head must not have moved since the pre-script
  # recorded it (SDLC-HOOK-07). The reference head is ONLY the trusted value the
  # pre-script recorded; the agent-supplied head_sha is never used as the anchor,
  # so a result cannot assert its own freshness. When no recorded head is
  # available (relay lost / manual reapply without it) freshness cannot be
  # verified, so the verdict is not applied.
  recorded_head="${LIFECYCLE_PR_HEAD_SHA:-}"
  # Capture a failed head read instead of letting set -e abort before the
  # graceful "cannot verify freshness" path below runs.
  current_head="$(gh_pr_head_sha "${REPO}" "${trusted_pr}" 2>/dev/null || true)"
  if [[ -z "${recorded_head}" || -z "${current_head}" ]]; then
    echo "post-lifecycle: no trusted recorded pull request head; cannot verify freshness, not applying verdict" >&2
    exit 0
  fi
  if [[ "${current_head}" != "${recorded_head}" ]]; then
    echo "post-lifecycle: pull request head changed (${recorded_head} -> ${current_head}); review result is stale, no verdict transition" >&2
    exit 0
  fi
  # If the agent reported which head it reviewed, it must be consistent with the
  # recorded head (supports a short-SHA prefix).
  if [[ -n "${r_head_sha}" && "${recorded_head}" != "${r_head_sha}"* && "${r_head_sha}" != "${recorded_head}"* ]]; then
    echo "post-lifecycle: reviewed head ${r_head_sha} is inconsistent with recorded head ${recorded_head}; review result is stale" >&2
    exit 0
  fi
fi

# Required-checks gate for an approval (SDLC-HOOK-07.02).
if [[ "${PHASE}" == "code-review" && "${outcome}" == "approved" ]]; then
  checks_state="$(gh_pr_checks_state "${REPO}" "${trusted_pr}")"
  if [[ "${checks_state}" != "green" ]]; then
    echo "post-lifecycle: required checks are ${checks_state}, not green; approval is not eligible, no mutation" >&2
    exit 0
  fi
fi

# ---------------------------------------------------------------------------
# Resolve the transition and augment with a timeline-driven block if needed.
# ---------------------------------------------------------------------------
add=""; remove=""; close=0
while IFS='=' read -r k v; do
  case "${k}" in
    add) add="${v}" ;;
    remove) remove="${v}" ;;
    close) close="${v}" ;;
  esac
done < <(lifecycle_transition "${PHASE}" "${outcome}")

if lifecycle_phase_is_review "${PHASE}" && [[ "${outcome}" == "rejected" ]]; then
  read -r rej_label threshold < <(lifecycle_block_rule "${PHASE}")
  # The block decision depends on authoritative timeline evidence (SDLC-HOOK-12).
  # A failed read must not silently under-count and skip the block, so a genuine
  # fetch failure aborts before any mutation rather than being swallowed.
  if ! timeline="$(gh_retry api "repos/${REPO}/issues/${NUMBER}/timeline" --paginate --slurp \
                     --jq 'add // []')"; then
    echo "post-lifecycle: could not read the issue timeline to count rejections; not applying the transition" >&2
    exit 1
  fi
  prior="$(printf '%s' "${timeline}" | lifecycle_count_rejections "${rej_label}")"
  prior="${prior:-0}"
  total=$(( prior + 1 ))
  if (( total >= threshold )); then
    add="${add} agent/blocked"
    echo "post-lifecycle: ${PHASE} rejection ${total} reaches the limit of ${threshold}; adding agent/blocked" >&2
  fi
fi

echo "post-lifecycle: ${PHASE}/${outcome} on ${REPO}#${NUMBER}: add=[${add# }] remove=[${remove}] close=${close}" >&2

# ---------------------------------------------------------------------------
# Apply (or preview under DRY_RUN).
# ---------------------------------------------------------------------------
MARKER="<!-- fullsend:${PHASE}-agent -->"

if (( DRY_RUN_ON )); then
  echo "post-lifecycle: DRY_RUN set, not applying any change" >&2
  [[ -n "${comment}" ]] && echo "post-lifecycle: (dry run) would post a ${PHASE} comment" >&2
  exit 0
fi

# Post the agent's comment BEFORE any label change (SDLC-HOOK-10). For a review
# phase the comment is the required review record, so it must be posted first: if
# it cannot be posted we fail here, before the verdict labels move, so the verdict
# never lands without its record and a rerun retries the sticky post and then the
# transition. For a non-review phase the comment is optional and best effort. The
# sticky marker makes a reapplied result converge without a duplicate (HOOK-09).
if [[ -n "${comment}" ]]; then
  _comment_err="$(mktemp)"
  if gh_post_sticky_comment "${REPO}" "${NUMBER}" "${MARKER}" "${comment}" \
        >/dev/null 2>"${_comment_err}"; then
    rm -f "${_comment_err}"
  else
    cat "${_comment_err}" >&2 || true
    rm -f "${_comment_err}"
    if lifecycle_phase_requires_comment "${PHASE}"; then
      echo "post-lifecycle: FAILED to post the required ${PHASE} review comment; not applying the transition so a rerun can retry the comment and the labels" >&2
      exit 1
    fi
    echo "post-lifecycle: WARNING: failed to post the optional ${PHASE} comment; continuing with the transition" >&2
  fi
fi

# Remove superseded phase labels first, then add. Order does not affect the
# final set, but removing first keeps competing outcomes from briefly coexisting.
for label in ${remove}; do
  gh_remove_label "${REPO}" "${NUMBER}" "${label}"
done
# shellcheck disable=SC2086
gh_add_labels "${REPO}" "${NUMBER}" ${add}

if [[ "${close}" == "1" ]]; then
  gh_close_issue "${REPO}" "${NUMBER}"
fi

# ---------------------------------------------------------------------------
# Verify the final state before reporting success (SDLC-HOOK-08).
# ---------------------------------------------------------------------------
final_labels="$(gh_get_labels "${REPO}" "${NUMBER}")"
if ! printf '%s\n' "${final_labels}" | lifecycle_verify_final "${add}" "${remove}"; then
  echo "post-lifecycle: final-state verification FAILED for ${REPO}#${NUMBER}; the transition did not complete" >&2
  exit 1
fi

echo "post-lifecycle: ${PHASE}/${outcome} applied and verified for ${REPO}#${NUMBER}" >&2
exit 0
