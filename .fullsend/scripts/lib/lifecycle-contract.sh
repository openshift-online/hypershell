#!/usr/bin/env bash
# lifecycle-contract.sh - the shared lifecycle transition contract (SDLC-HOOK-01,
# SDLC-HOOK-02, SDLC-HOOK-12).
#
# This file is the single source of truth for which GitHub phase labels each
# phase outcome adds and removes. The pre-script, the post-script, and the unit
# tests all source it, so local and cloud Fullsend runs apply identical rules
# (SDLC-HOOK-11). It is pure: it performs no network calls and no label writes,
# so it is safe to source from a read-only validation context.
#
# Vocabulary:
#   phase   - one of: triage, spec-implementation, spec-review,
#             code-implementation, code-review
#   outcome - a fixed, per-phase enum value (see lifecycle_outcomes)
#
# A transition names ONLY phase labels. Scope (agent/scope/*), authorization
# (agent/allowed), the execution-record marker (agent/execution-record), a
# human block (agent/blocked), and every unrelated label are never named by a
# transition and so are preserved by construction (SDLC-HOOK-02). agent/blocked
# only ever appears in an ADD set, never in a REMOVE set, so a transition can
# raise a block but never silently clears one.

# The complete set of phase labels (SDLC-HOOK-02 "Phase labels").
lifecycle_phase_labels() {
  printf '%s\n' \
    agent/workable \
    agent/needs-input \
    agent/duplicate \
    agent/reviewable-spec \
    agent/review-spec-approved \
    agent/review-spec-rejected \
    agent/reviewable-code \
    agent/review-code-approved \
    agent/review-code-rejected
}

# True when the argument is a phase label.
lifecycle_is_phase_label() {
  local candidate="$1" label
  while IFS= read -r label; do
    [[ "${label}" == "${candidate}" ]] && return 0
  done < <(lifecycle_phase_labels)
  return 1
}

# True when the argument is a protected label that a transition must never
# add or remove on its own: scope, authorization, the execution-record marker.
# agent/blocked is handled separately: a transition may ADD it (retry limit or
# a hard implementation block) but must never REMOVE it.
lifecycle_is_protected_label() {
  local candidate="$1"
  case "${candidate}" in
    agent/allowed | agent/execution-record) return 0 ;;
    agent/scope/*) return 0 ;;
    *) return 1 ;;
  esac
}

# The permitted outcome enum for a phase. Prints one outcome per line.
# Returns non-zero for an unknown phase.
lifecycle_outcomes() {
  case "$1" in
    triage) printf '%s\n' workable needs-input duplicate ;;
    spec-implementation) printf '%s\n' completed blocked ;;
    spec-review) printf '%s\n' approved rejected ;;
    code-implementation) printf '%s\n' completed blocked ;;
    code-review) printf '%s\n' approved rejected ;;
    *) return 1 ;;
  esac
}

# True when outcome ($2) is permitted for phase ($1).
lifecycle_outcome_valid() {
  local phase="$1" outcome="$2" candidate
  while IFS= read -r candidate; do
    [[ "${candidate}" == "${outcome}" ]] && return 0
  done < <(lifecycle_outcomes "${phase}")
  return 1
}

# The rejection phase label and block threshold for a review phase, used by the
# post-script to count prior rejections from the timeline (SDLC-HOOK-12).
# Prints "<rejection-label> <threshold>"; returns non-zero for a non-review phase.
lifecycle_block_rule() {
  case "$1" in
    spec-review) printf '%s %s\n' agent/review-spec-rejected 3 ;;
    code-review) printf '%s %s\n' agent/review-code-rejected 10 ;;
    *) return 1 ;;
  esac
}

# Resolve a transition. On success prints exactly three lines:
#   add=<space-separated labels or empty>
#   remove=<space-separated labels or empty>
#   close=<0|1>
# The remove list contains only superseded phase labels. agent/blocked for a
# review retry limit is added by the post-script, not here, because it depends
# on timeline evidence rather than the outcome alone (SDLC-HOOK-12.02).
# Returns non-zero for an unknown phase/outcome pair.
lifecycle_transition() {
  local phase="$1" outcome="$2"
  local add="" remove="" close=0
  case "${phase}:${outcome}" in
    triage:workable)
      add="agent/workable" ;;
    triage:needs-input)
      add="agent/needs-input" ;;
    triage:duplicate)
      add="agent/duplicate"; close=1 ;;
    spec-implementation:completed)
      add="agent/reviewable-spec"
      remove="agent/workable agent/review-spec-rejected agent/review-spec-approved" ;;
    spec-implementation:blocked)
      add="agent/blocked" ;;
    spec-review:approved)
      add="agent/review-spec-approved"
      remove="agent/reviewable-spec agent/review-spec-rejected" ;;
    spec-review:rejected)
      add="agent/review-spec-rejected"
      remove="agent/reviewable-spec agent/review-spec-approved" ;;
    code-implementation:completed)
      add="agent/reviewable-code"
      remove="agent/review-code-rejected agent/review-code-approved" ;;
    code-implementation:blocked)
      add="agent/blocked" ;;
    code-review:approved)
      add="agent/review-code-approved"
      remove="agent/reviewable-code agent/review-code-rejected" ;;
    code-review:rejected)
      add="agent/review-code-rejected"
      remove="agent/reviewable-code agent/review-code-approved" ;;
    *)
      return 1 ;;
  esac
  printf 'add=%s\n' "${add}"
  printf 'remove=%s\n' "${remove}"
  printf 'close=%s\n' "${close}"
}

# True when a phase result requires a linked pull request that ALREADY exists as
# evidence (SDLC-HOOK-07). These phases run against a pull request the
# spec-implementation phase created earlier, so the pre-script skips when none is
# linked and the post-script requires a trusted pull request number. Triage
# operates on an issue only; spec-implementation CREATES the pull request (see
# lifecycle_phase_creates_pr) rather than requiring an existing one.
lifecycle_phase_needs_pr() {
  case "$1" in
    spec-review | code-implementation | code-review) return 0 ;;
    *) return 1 ;;
  esac
}

# True when a phase CREATES the linked pull request rather than requiring one to
# already exist (SDLC-HOOK-07). The spec-implementation agent opens the work
# item's pull request, so on a fresh issue there is no pull request when the
# pre-script runs: the pre-script must NOT skip for a missing pull request, and
# the post-script accepts the agent-reported pull request number as the trusted
# target only after verifying it is an open pull request in this repository that
# closes this issue. A phase is never both "needs" and "creates".
lifecycle_phase_creates_pr() {
  case "$1" in
    spec-implementation) return 0 ;;
    *) return 1 ;;
  esac
}

# True when a phase result is a review verdict whose freshness must be checked
# against the current pull request head (SDLC-HOOK-07).
lifecycle_phase_is_review() {
  case "$1" in
    spec-review | code-review) return 0 ;;
    *) return 1 ;;
  esac
}
