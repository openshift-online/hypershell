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
#   - deferred (checks)       -> approval not applied, no label change, but a
#                                visible deferral record is posted so the pending
#                                approval is recoverable (reapply when checks green)
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
  # Fallback result discovery: Fullsend did not point us at a validated iteration
  # dir, so there is no proof the chosen result passed the native validation_loop.
  # This is a HUMAN RECOVERY path only (see .fullsend/docs/lifecycle-hooks.md,
  # Recovery): the normal flow always sets FULLSEND_VALIDATED_ITERATION_DIR. We
  # still re-check every trusted field below (schema_version, phase, outcome,
  # pr_number against the recorded PR, head freshness), but we do NOT re-run the
  # full JSON Schema here, so announce the path loudly instead of advancing
  # silently on possibly-unvalidated output.
  _best=-1
  for _d in iteration-*/output; do
    [[ -f "${_d}/agent-result.json" ]] || continue
    _n="${_d%/output}"; _n="${_n#iteration-}"
    [[ "${_n}" =~ ^[0-9]+$ ]] || continue
    if (( _n > _best )); then _best="${_n}"; RESULT_FILE="${_d}/agent-result.json"; fi
  done
  if [[ -n "${RESULT_FILE}" ]]; then
    echo "post-lifecycle: WARNING: FULLSEND_VALIDATED_ITERATION_DIR is unset; selecting ${RESULT_FILE} by highest iteration number without a validated-iteration marker (human recovery path). The trusted-field and evidence checks below still apply, but the full JSON Schema is not re-run here." >&2
  fi
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
# Flatten CR/LF so an untrusted scalar echoed into the runner log cannot forge a
# log line (SEC-STD-02), matching the pre-script's skip-reason handling. The
# comment body is intentionally NOT flattened: it is posted verbatim as markdown.
_oneline() { printf '%s' "${1//[$'\r\n']/ }"; }

schema_version="$(_oneline "$(_field_str schema_version)")"
r_phase="$(_oneline "$(_field_str phase)")"
outcome="$(_oneline "$(_field_str outcome)")"
comment="$(_field_str comment)"
r_pr_number="$(_field_num pr_number)"
r_head_sha="$(_oneline "$(_field_str head_sha)")"

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
_authorized() {
  local labels
  labels="$(gh_get_labels "${REPO}" "${NUMBER}")" || return 1
  grep -qxF agent/allowed <<< "${labels}" || return 1
  ! grep -qxF agent/blocked <<< "${labels}" || return 1
  [[ -z "${LABEL_SCOPE:-}" ]] || grep -qxF "${LABEL_SCOPE}" <<< "${labels}"
}
_authorized || { echo "post-lifecycle: authorization/scope removed or blocked; no mutation" >&2; exit 0; }


# A review verdict normally applies while the record is still awaiting that
# review (the reviewable-* label is present). When the reviewable-* label is
# gone we must still distinguish three cases, so an interrupted transition
# converges on reapply without ever overriding a settled verdict (SDLC-HOOK-09.02):
#   1. this verdict label is already present  -> the transition already completed
#      (reviewable removed, verdict added); reapplying is a verified no-op.
#   2. the competing verdict label is present -> this review phase already settled
#      the other way; do not override it.
#   3. neither verdict is present             -> the add/remove calls are separate
#      GitHub requests, so a transition can stop between them. With add-first
#      ordering the only self-produced partial state keeps the reviewable-* label,
#      but a record stranded by an older remove-first run (reviewable removed,
#      verdict never added) still reaches here; recover it by applying the verdict
#      rather than exiting with no change. Freshness is still gated below.
_rev_kind="${PHASE%-review}"                   # spec | code (review phases only)
_reviewable="agent/reviewable-${_rev_kind}"
_my_verdict="agent/review-${_rev_kind}-${outcome}"
case "${outcome}" in
  approved) _other_verdict="agent/review-${_rev_kind}-rejected" ;;
  rejected) _other_verdict="agent/review-${_rev_kind}-approved" ;;
  *)        _other_verdict="" ;;
esac
_recovering=0
if lifecycle_phase_is_review "${PHASE}"; then
  if ! _cur_has "${_reviewable}"; then
    if _cur_has "${_my_verdict}" && [[ "${PHASE}:${outcome}" != "code-review:approved" ]]; then
      echo "post-lifecycle: ${_my_verdict} already present and ${_reviewable} gone; transition already complete, no mutation" >&2
      exit 0
    fi
    if [[ -n "${_other_verdict}" ]] && _cur_has "${_other_verdict}"; then
      echo "post-lifecycle: ${_other_verdict} is present; this review phase already settled the other way, not overriding, no mutation" >&2
      exit 0
    fi
    # The record has no reviewable-* label and no verdict: an older remove-first
    # run stranded it with no phase label. We try to recover by applying the
    # verdict, but the freshness gate below still applies. If that gate aborts the
    # recovery (the head moved, or no trusted recorded head is available), the
    # record stays with no phase label, so emit a loud, distinct signal for that
    # case instead of a quiet "stale" exit (SDLC-HOOK-09.02, review follow-up).
    if ! _cur_has "${_my_verdict}"; then _recovering=1; fi
    echo "post-lifecycle: rechecking completed/interrupted ${_my_verdict} transition (freshness still checked)" >&2
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
    if [[ "${_recovering}" == 1 ]]; then
      echo "post-lifecycle: ATTENTION: ${REPO}#${NUMBER} has no ${PHASE} phase label and freshness could not be verified; an interrupted transition could not be recovered automatically and needs a human to restore a phase label" >&2
    fi
    exit 0
  fi
  if [[ "${current_head}" != "${recorded_head}" ]]; then
    echo "post-lifecycle: pull request head changed (${recorded_head} -> ${current_head}); review result is stale, no verdict transition" >&2
    if [[ "${_recovering}" == 1 ]]; then
      echo "post-lifecycle: ATTENTION: ${REPO}#${NUMBER} has no ${PHASE} phase label and the reviewed head moved; the interrupted transition could not be recovered automatically and needs a human to restore a phase label or re-review" >&2
    fi
    exit 0
  fi
  # If the agent reported which head it reviewed, it must be consistent with the
  # recorded head (supports a short-SHA prefix). The agent value is untrusted and,
  # on the fallback result-discovery path, has not passed the JSON Schema, so
  # re-validate it as a 7-40 char lowercase-hex commit sha BEFORE the glob compare.
  # Without this, a value like "*" would be used as a glob pattern and match any
  # recorded head, silently passing the consistency check.
  if [[ -n "${r_head_sha}" ]]; then
    if [[ ! "${r_head_sha}" =~ ^[0-9a-f]{7,40}$ ]]; then
      echo "post-lifecycle: result head_sha '${r_head_sha:0:48}' is not a 7-40 char hex commit sha; treating the review result as stale, no verdict transition" >&2
      exit 0
    fi
    if [[ "${recorded_head}" != "${r_head_sha}"* && "${r_head_sha}" != "${recorded_head}"* ]]; then
      echo "post-lifecycle: reviewed head ${r_head_sha} is inconsistent with recorded head ${recorded_head}; review result is stale" >&2
      exit 0
    fi
  fi
fi

# Required-checks gate for an approval (SDLC-HOOK-07.02). When the approval is not
# yet eligible because the required checks are not green, this is NOT a log-only
# exit: post a visible deferral record (the review record, SDLC-HOOK-10) and leave
# agent/reviewable-code in place so the item is still awaiting review. A later
# green check is not a label event, so the approval is completed by reapplying this
# same validated result once the checks are green (the recovery procedure in
# .fullsend/docs/lifecycle-hooks.md). The deferral uses the phase sticky marker, so
# the reapplied verdict edits this record in place with no duplicate (SDLC-HOOK-09).
if [[ "${PHASE}" == "code-review" && "${outcome}" == "approved" ]]; then
  gh_pr_has_execution_link "${REPO}" "${trusted_pr}" "${NUMBER}" || {
    echo "post-lifecycle: PR is not linked to this execution record; no approval" >&2; exit 1;
  }
  code_state="$(gh_pr_code_state "${REPO}" "${trusted_pr}")"
  if [[ "$(jq -r '.head_sha' <<< "${code_state}")" != "${recorded_head}" ]]; then
    echo "post-lifecycle: head changed during approval checks; not applying approval" >&2
    exit 0
  fi
  checks_state="$(jq -r '.checks' <<< "${code_state}")"
  if ! jq -e '.state == "open" and .draft == false and .mergeable == true and .checks == "green"' \
       <<< "${code_state}" >/dev/null; then
    readiness="$(jq -r '"checks=\(.checks), state=\(.state), draft=\(.draft), mergeable=\(.mergeable), mergeable_state=\(.mergeable_state)"' <<< "${code_state}")"
    echo "post-lifecycle: approval deferred on PR readiness: ${readiness}" >&2
    if (( ! DRY_RUN_ON )); then
      _authorized || { echo "post-lifecycle: authorization changed during deferral" >&2; exit 0; }
      gh_remove_label "${REPO}" "${trusted_pr}" agent/review-code-approved
    fi
    # Defense in depth, consistent with the apply path below: a code-review verdict
    # requires its review record, so refuse an empty comment here too rather than
    # posting a banner-only deferral and exiting 0 (SDLC-HOOK-10).
    if lifecycle_phase_requires_comment "${PHASE}" && [[ -z "${comment}" ]]; then
      echo "post-lifecycle: ${PHASE} approval is deferred on ${checks_state} checks but the result has no review comment; not posting a record, failing so a rerun retries with a record" >&2
      exit 1
    fi
    _defer_marker="<!-- fullsend:${PHASE}-agent -->"
    if [[ "${_recovering}" == 1 ]]; then
      # No phase label remains (an older interrupted transition stranded this item),
      # so be honest: do NOT claim the item keeps agent/reviewable-code and do NOT
      # promise an automatic reapply, which cannot fire without a phase-label event.
      _defer_banner="**Code review approved - deferred, and this item currently has no code-review phase label.** The approval cannot be applied yet because the pull request's readiness is \`${readiness}\`, and an earlier interrupted transition left this item with no code-review phase label. A maintainer needs to restore \`agent/reviewable-code\` or re-run the review once the required checks are green (see .fullsend/docs/lifecycle-hooks.md, Recovery)."
    else
      _defer_banner="**Code review approved - deferred pending required checks.** The approval has been recorded but is not yet applied because the pull request's PR readiness is \`${readiness}\`. The item keeps \`agent/reviewable-code\` and the approval is applied when this validated result is reapplied once the required checks are green (see .fullsend/docs/lifecycle-hooks.md, Recovery)."
    fi
    if [[ -n "${comment}" ]]; then
      _defer_body="$(printf '%s\n\n---\n\n%s' "${comment}" "${_defer_banner}")"
    else
      _defer_body="${_defer_banner}"
    fi
    if (( DRY_RUN_ON )); then
      echo "post-lifecycle: DRY_RUN set, not posting the approval-deferral record" >&2
      exit 0
    fi
    # The deferral record is the whole point of this path: a failed post must NOT
    # be downgraded to a log-only exit (the behavior this change removes) or
    # silently swallowed. Fail non-zero so a rerun retries the record; the labels
    # are unchanged, so the rerun is safe and idempotent (SDLC-HOOK-09).
    if gh_post_sticky_comment "${REPO}" "${NUMBER}" "${_defer_marker}" "${_defer_body}" >/dev/null 2>&1; then
      echo "post-lifecycle: posted a visible approval-deferral record on ${REPO}#${NUMBER}; no label change" >&2
      if [[ "${_recovering}" == 1 ]]; then
        echo "post-lifecycle: ATTENTION: ${REPO}#${NUMBER} has no ${PHASE} phase label and required checks are ${checks_state}; the interrupted approval needs a human to restore a phase label or re-review once checks are green" >&2
      fi
      exit 0
    fi
    echo "post-lifecycle: FAILED to post the approval-deferral record on ${REPO}#${NUMBER}; failing so a rerun retries the record rather than leaving a log-only exit" >&2
    exit 1
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
  # Fail closed: the block decision depends on this count, so an empty or
  # non-numeric result (a jq failure counting an unparseable timeline) must abort
  # before any mutation rather than defaulting to 0 and silently skipping a required
  # agent/blocked (SDLC-HOOK-12). Capture a counting failure into an empty value so
  # the actionable guard below runs instead of a raw jq abort under set -e.
  prior=""
  prior="$(printf '%s' "${timeline}" | lifecycle_count_rejections "${rej_label}")" || prior=""
  if [[ ! "${prior}" =~ ^[0-9]+$ ]]; then
    echo "post-lifecycle: could not compute a numeric prior-rejection count (got '${prior:0:32}'); not applying the transition" >&2
    exit 1
  fi
  # Idempotent count: if this rejection verdict label is already present, a prior
  # (possibly interrupted add-first) attempt already applied it, so the timeline
  # already counts this rejection; adding +1 again on reapply would double-count
  # and could cross the block threshold spuriously (SDLC-HOOK-09). Only add the
  # current rejection when it is not yet recorded.
  if _cur_has "${rej_label}"; then
    total="${prior}"
  else
    total=$(( prior + 1 ))
  fi
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

# Defense in depth (SDLC-HOOK-10): a review verdict must never land without its
# review record. The schema already requires a comment for a review verdict, but
# this post-script re-verifies independently of the validator, so refuse to apply
# a review transition when the comment is empty rather than silently skipping the
# required-record post below (where the hard-fail is nested inside "comment is
# non-empty") and applying the verdict labels with no record.
if lifecycle_phase_requires_comment "${PHASE}" && [[ -z "${comment}" ]]; then
  echo "post-lifecycle: ${PHASE} requires a review comment but the result has none; not applying the transition" >&2
  exit 1
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

# Queue eligibility is granted only for the same open, conflict-free, green
# head that was reviewed. Do this before the issue transition so failures can
# retry while the execution record is still reviewable.
_authorized || { echo "post-lifecycle: authorization changed before label application" >&2; exit 0; }
if [[ "${PHASE}:${outcome}" == "code-review:approved" ]]; then
  latest="$(gh_pr_code_state "${REPO}" "${trusted_pr}")"
  if ! jq -e --arg sha "${recorded_head}" --arg base "$(jq -r '.base_sha' <<< "${code_state}")" \
      '.head_sha == $sha and .base_sha == $base and .state == "open" and .draft == false and .mergeable == true and .checks == "green"' \
      <<< "${latest}" >/dev/null; then
    echo "post-lifecycle: PR eligibility changed before approval; retry when stable" >&2
    exit 1
  fi
  gh_add_labels "${REPO}" "${trusted_pr}" agent/review-code-approved
  gh_get_labels "${REPO}" "${trusted_pr}" | grep -qxF agent/review-code-approved || {
    echo "post-lifecycle: PR approval label verification failed" >&2; exit 1;
  }
fi

if [[ "${PHASE}:${outcome}" == "code-review:rejected" ]]; then
  gh_remove_label "${REPO}" "${trusted_pr}" agent/review-code-approved
fi

# Add the new phase label(s) BEFORE removing the superseded ones (SDLC-HOOK-09.02).
# The add and the removes are separate GitHub API calls, not one transaction, so
# the process can stop between them. Adding first guarantees that window always
# holds both the new and the superseded phase label, never neither: for a review
# phase the reviewable-* label is still present, so a reapplication passes the
# review guard above, removes it, and converges on the verdict. Removing first
# would instead strip the reviewable-* label before the verdict landed, and a
# reapplication would then see the record no longer awaiting review and exit with
# no change, stranding the work item. The brief overlap of the new and the
# superseded label is safe because lifecycle_verify_final runs only after both
# steps complete.
# shellcheck disable=SC2086
gh_add_labels "${REPO}" "${NUMBER}" ${add}
for label in ${remove}; do
  gh_remove_label "${REPO}" "${NUMBER}" "${label}"
done

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
