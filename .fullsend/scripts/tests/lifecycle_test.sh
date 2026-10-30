#!/usr/bin/env bash
# lifecycle_test.sh - unit tests for the pure lifecycle decision logic
# (SDLC-HOOK-13). No network, no label writes: the contract and logic libraries
# are exercised directly. Auto-discovered by scripts/run-shell-unit-tests.sh.

set -uo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="${_here}/../lib"
# shellcheck source=../lib/lifecycle-contract.sh
. "${LIB}/lifecycle-contract.sh"
# shellcheck source=../lib/lifecycle-logic.sh
. "${LIB}/lifecycle-logic.sh"

PASS=0
FAIL=0
_ok() { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
_no() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; [[ -n "${2:-}" ]] && printf '       %s\n' "$2"; }

# Assert that `lifecycle_transition PHASE OUTCOME` yields the expected add/remove.
assert_transition() {
  local desc="$1" phase="$2" outcome="$3" want_add="$4" want_remove="$5"
  local add="" remove="" close=0 k v
  while IFS='=' read -r k v; do
    case "${k}" in add) add="${v}" ;; remove) remove="${v}" ;; close) close="${v}" ;; esac
  done < <(lifecycle_transition "${phase}" "${outcome}")
  if [[ "${add}" == "${want_add}" && "${remove}" == "${want_remove}" ]]; then
    _ok "${desc}"
  else
    _no "${desc}" "got add=[${add}] remove=[${remove}] want add=[${want_add}] remove=[${want_remove}]"
  fi
}

# ---- Transition contract (SDLC-HOOK-01) ----------------------------------
assert_transition "triage workable" triage workable "agent/workable" ""
assert_transition "triage needs-input" triage needs-input "agent/needs-input" ""
assert_transition "triage duplicate" triage duplicate "agent/duplicate" ""

assert_transition "spec-impl completed re-opens spec review" \
  spec-implementation completed \
  "agent/reviewable-spec" \
  "agent/workable agent/review-spec-rejected agent/review-spec-approved"

# SDLC-HOOK-01.03 approved spec review leaves only the approval
assert_transition "spec-review approved" spec-review approved \
  "agent/review-spec-approved" "agent/reviewable-spec agent/review-spec-rejected"
assert_transition "spec-review rejected" spec-review rejected \
  "agent/review-spec-rejected" "agent/reviewable-spec agent/review-spec-approved"

# SDLC-HOOK-01.02 completed code implementation re-opens code review for a new revision
assert_transition "code-impl completed re-opens code review" \
  code-implementation completed \
  "agent/reviewable-code" \
  "agent/review-code-rejected agent/review-code-approved"

# SDLC-HOOK-01.01 rejected code review ends with one code phase label
assert_transition "code-review rejected" code-review rejected \
  "agent/review-code-rejected" "agent/reviewable-code agent/review-code-approved"
assert_transition "code-review approved" code-review approved \
  "agent/review-code-approved" "agent/reviewable-code agent/review-code-rejected"

# The code-implementation remove set must never include the spec approval, so a
# completed prerequisite is preserved (SDLC-HOOK-01.02).
{
  _add=""; _remove=""
  while IFS='=' read -r k v; do [[ "${k}" == remove ]] && _remove="${v}"; done \
    < <(lifecycle_transition code-implementation completed)
  if [[ " ${_remove} " != *" agent/review-spec-approved "* ]]; then
    _ok "code-impl preserves agent/review-spec-approved"
  else
    _no "code-impl preserves agent/review-spec-approved" "remove=[${_remove}]"
  fi
}

# ---- Protected label categories (SDLC-HOOK-02) ---------------------------
# No transition's remove set names a protected label or agent/blocked.
{
  violation=""
  for combo in "triage:workable" "triage:needs-input" "triage:duplicate" \
               "spec-implementation:completed" "spec-implementation:blocked" \
               "spec-review:approved" "spec-review:rejected" \
               "code-implementation:completed" "code-implementation:blocked" \
               "code-review:approved" "code-review:rejected"; do
    phase="${combo%:*}"; outcome="${combo#*:}"
    rem=""
    while IFS='=' read -r k v; do [[ "${k}" == remove ]] && rem="${v}"; done \
      < <(lifecycle_transition "${phase}" "${outcome}")
    for label in ${rem}; do
      if lifecycle_is_protected_label "${label}" || [[ "${label}" == "agent/blocked" ]]; then
        violation="${combo} removes ${label}"
      fi
    done
  done
  [[ -z "${violation}" ]] && _ok "no transition removes a protected or block label" \
    || _no "no transition removes a protected or block label" "${violation}"
}

lifecycle_is_protected_label "agent/allowed" && _ok "agent/allowed is protected" || _no "agent/allowed is protected"
lifecycle_is_protected_label "agent/scope/jsell/fullsend" && _ok "scope label is protected" || _no "scope label is protected"
lifecycle_is_protected_label "agent/execution-record" && _ok "execution-record is protected" || _no "execution-record is protected"
lifecycle_is_protected_label "agent/workable" || _ok "phase label is not protected"

# ---- PR dependency classification (SDLC-HOOK-07) --------------------------
# spec-implementation CREATES the PR; it must not be classified as needing one,
# or the funnel can never bootstrap a fresh issue.
lifecycle_phase_creates_pr spec-implementation && _ok "spec-implementation creates the PR" || _no "spec-implementation creates the PR"
lifecycle_phase_needs_pr spec-implementation && _no "spec-implementation does not need an existing PR" || _ok "spec-implementation does not need an existing PR"
# A phase is never both needs and creates.
{
  overlap=""
  for p in triage spec-implementation spec-review code-implementation code-review; do
    if lifecycle_phase_needs_pr "${p}" && lifecycle_phase_creates_pr "${p}"; then overlap="${p}"; fi
  done
  [[ -z "${overlap}" ]] && _ok "no phase both needs and creates a PR" || _no "no phase both needs and creates a PR" "${overlap}"
}
# The phases that run against an existing PR still need one.
for p in spec-review code-implementation code-review; do
  lifecycle_phase_needs_pr "${p}" && _ok "${p} needs an existing PR" || _no "${p} needs an existing PR"
done
lifecycle_phase_needs_pr triage && _no "triage needs no PR" || _ok "triage needs no PR"

# ---- Required-comment classification (SDLC-HOOK-10) ------------------------
# Review phases must post the verdict comment as the review record; a failure to
# post it is fatal in the post-script. Non-review phases treat it as optional.
for p in spec-review code-review; do
  lifecycle_phase_requires_comment "${p}" && _ok "${p} requires its review comment" || _no "${p} requires its review comment"
done
for p in triage spec-implementation code-implementation; do
  lifecycle_phase_requires_comment "${p}" && _no "${p} comment is optional" || _ok "${p} comment is optional"
done

# ---- Outcome enum (SDLC-HOOK-04.01) --------------------------------------
lifecycle_outcome_valid spec-review approved && _ok "approved is valid for spec-review" || _no "approved valid spec-review"
lifecycle_outcome_valid spec-review bogus && _no "bogus rejected for spec-review" || _ok "bogus outcome rejected for spec-review"
lifecycle_transition spec-review bogus >/dev/null 2>&1 && _no "unknown outcome has no transition" || _ok "unknown outcome has no transition"

# ---- URL parsing (SDLC-HOOK-04) ------------------------------------------
{
  out="$(lifecycle_parse_issue_url "https://github.com/openshift-online/hypershell/issues/868")"
  [[ "${out}" == "openshift-online/hypershell 868 issues" ]] && _ok "parses a valid issue URL" || _no "parses a valid issue URL" "${out}"
}
lifecycle_parse_issue_url "https://evil.example/openshift-online/hypershell/issues/1" >/dev/null 2>&1 \
  && _no "rejects non-github host" || _ok "rejects non-github host"
lifecycle_parse_issue_url "not a url" >/dev/null 2>&1 \
  && _no "rejects malformed URL" || _ok "rejects malformed URL"

# ---- Rejection counting from the timeline (SDLC-HOOK-12) ------------------
# Two prior spec rejections, no block removal: count is 2 (this one makes 3).
timeline_two='[
  {"event":"labeled","label":{"name":"agent/reviewable-spec"}},
  {"event":"labeled","label":{"name":"agent/review-spec-rejected"}},
  {"event":"labeled","label":{"name":"agent/reviewable-spec"}},
  {"event":"labeled","label":{"name":"agent/review-spec-rejected"}}
]'
{
  n="$(printf '%s' "${timeline_two}" | lifecycle_count_rejections agent/review-spec-rejected)"
  [[ "${n}" == "2" ]] && _ok "counts two prior spec rejections" || _no "counts two prior spec rejections" "got ${n}"
}
# Counting resets after the most recent removal of agent/blocked.
timeline_reset='[
  {"event":"labeled","label":{"name":"agent/review-spec-rejected"}},
  {"event":"labeled","label":{"name":"agent/blocked"}},
  {"event":"unlabeled","label":{"name":"agent/blocked"}},
  {"event":"labeled","label":{"name":"agent/review-spec-rejected"}}
]'
{
  n="$(printf '%s' "${timeline_reset}" | lifecycle_count_rejections agent/review-spec-rejected)"
  [[ "${n}" == "1" ]] && _ok "rejection count resets after agent/blocked removal" || _no "rejection count resets after agent/blocked removal" "got ${n}"
}
# The count is derived from the timeline, never from a result field.
{
  n="$(printf '%s' '[]' | lifecycle_count_rejections agent/review-code-rejected)"
  [[ "${n}" == "0" ]] && _ok "empty timeline yields zero rejections" || _no "empty timeline yields zero rejections" "got ${n}"
}

# ---- Final-state verification (SDLC-HOOK-08) ------------------------------
# Approved spec review: reviewable removed, approval present, no competing label.
printf '%s\n' "agent/review-spec-approved" "agent/allowed" "agent/scope/x" \
  | lifecycle_verify_final "agent/review-spec-approved" "agent/reviewable-spec agent/review-spec-rejected" \
  && _ok "verify passes on a clean approved spec state" || _no "verify passes on a clean approved spec state"
# A lingering superseded label fails verification.
printf '%s\n' "agent/review-spec-approved" "agent/reviewable-spec" \
  | lifecycle_verify_final "agent/review-spec-approved" "agent/reviewable-spec agent/review-spec-rejected" 2>/dev/null \
  && _no "verify catches a lingering superseded label" || _ok "verify catches a lingering superseded label"
# A missing expected label fails verification.
printf '%s\n' "agent/allowed" \
  | lifecycle_verify_final "agent/review-spec-approved" "" 2>/dev/null \
  && _no "verify catches a missing expected label" || _ok "verify catches a missing expected label"
# Mutual exclusivity: two competing code labels fails.
printf '%s\n' "agent/reviewable-code" "agent/review-code-approved" \
  | lifecycle_verify_final "agent/review-code-approved" "" 2>/dev/null \
  && _no "verify catches competing code labels" || _ok "verify catches competing code labels"

# ---- Summary -------------------------------------------------------------
printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
