#!/usr/bin/env bash
# github.sh - thin GitHub I/O for the lifecycle hooks.
#
# All calls go through `gh api`, which uses GH_TOKEN. curl is deliberately not
# used: the base sandbox policy forbids raw HTTP with the injected token, and the
# runner follows the same rule. Every mutating call is idempotent so a reapplied
# transition converges without duplicate side effects (SDLC-HOOK-09), and read
# and write calls use bounded retries for transient API failures.
#
# These functions perform network I/O; the pure decision logic they feed lives
# in lib/lifecycle-logic.sh and lib/lifecycle-contract.sh.

GH_MAX_ATTEMPTS="${GH_MAX_ATTEMPTS:-4}"

# Run `gh "$@"`, retrying on failure with capped backoff. Returns the last
# exit code. stdout from the final attempt is forwarded to the caller.
gh_retry() {
  local attempt=1 rc=0 out errf
  errf="$(mktemp)"
  while true; do
    if out="$(gh "$@" 2>"${errf}")"; then
      printf '%s' "${out}"
      rm -f "${errf}"
      return 0
    fi
    rc=$?
    if (( attempt >= GH_MAX_ATTEMPTS )); then
      cat "${errf}" >&2 || true
      rm -f "${errf}"
      return "${rc}"
    fi
    sleep $(( attempt * 2 ))
    attempt=$(( attempt + 1 ))
  done
}

# Print the labels currently on an issue, one per line.
gh_get_labels() {
  local repo="$1" number="$2"
  gh_retry api "repos/${repo}/issues/${number}/labels" --paginate \
    --jq '.[].name'
}

# Create a label if it does not already exist. A 422 (already exists) is fine.
gh_ensure_label() {
  local repo="$1" name="$2"
  if gh_retry api "repos/${repo}/labels/$(_gh_urlencode "${name}")" >/dev/null 2>&1; then
    return 0
  fi
  gh api -X POST "repos/${repo}/labels" -f "name=${name}" -f "color=ededed" \
    >/dev/null 2>&1 || true
}

# Add labels to an issue (idempotent: adding a present label is a no-op on the
# GitHub side). Ensures each label exists first. Accepts one or more labels.
gh_add_labels() {
  local repo="$1" number="$2"; shift 2
  local label args=()
  for label in "$@"; do
    [[ -z "${label}" ]] && continue
    gh_ensure_label "${repo}" "${label}"
    args+=(-f "labels[]=${label}")
  done
  (( ${#args[@]} == 0 )) && return 0
  gh_retry api -X POST "repos/${repo}/issues/${number}/labels" "${args[@]}" \
    >/dev/null
}

# Remove a single label from an issue. A 404 (label not present) is success, so
# a reapplied transition does not fail (SDLC-HOOK-09). A transient failure is
# retried with backoff before the final presence check.
gh_remove_label() {
  local repo="$1" number="$2" label="$3"
  [[ -z "${label}" ]] && return 0
  local path="repos/${repo}/issues/${number}/labels/$(_gh_urlencode "${label}")"
  local attempt=1
  while true; do
    if gh api -X DELETE "${path}" >/dev/null 2>&1; then
      return 0
    fi
    # Not present any more is success (idempotent / already removed).
    if ! gh_get_labels "${repo}" "${number}" | grep -qxF "${label}"; then
      return 0
    fi
    if (( attempt >= GH_MAX_ATTEMPTS )); then
      echo "github: failed to remove label ${label} after ${attempt} attempts" >&2
      return 1
    fi
    sleep $(( attempt * 2 ))
    attempt=$(( attempt + 1 ))
  done
}

# Close an issue, only if it is currently open (idempotent).
gh_close_issue() {
  local repo="$1" number="$2" state
  state="$(gh_retry api "repos/${repo}/issues/${number}" --jq '.state')"
  [[ "${state}" == "closed" ]] && return 0
  gh_retry api -X PATCH "repos/${repo}/issues/${number}" -f "state=closed" \
    >/dev/null
}

# Print the head commit SHA of a pull request.
gh_pr_head_sha() {
  local repo="$1" number="$2"
  gh_retry api "repos/${repo}/pulls/${number}" --jq '.head.sha'
}

# True when pull request <pr> is an open pull request in THIS repo whose body
# carries a GitHub closing reference to issue <issue>
# (Closes/Fixes/Resolves #<issue>), matching the spec-implementation contract.
# This is the trust check for a pull request identifier an agent returns
# (SDLC-HOOK-04): it confirms the pull request really belongs to this work item
# and is not an unrelated same-numbered pull request or a bare mention.
gh_pr_is_linked() {
  local repo="$1" pr="$2" issue="$3" state body
  [[ -z "${pr}" ]] && return 1
  state="$(gh_retry api "repos/${repo}/pulls/${pr}" --jq '.state' 2>/dev/null || true)"
  [[ "${state}" == "open" ]] || return 1
  body="$(gh_retry api "repos/${repo}/pulls/${pr}" --jq '.body // ""' 2>/dev/null || true)"
  # Closing keyword followed by #<issue>, case-insensitive.
  printf '%s' "${body}" \
    | grep -qiE "(close[sd]?|fix(e[sd])?|resolve[sd]?)[[:space:]]+#${issue}([^0-9]|$)"
}

# Print the number of the open pull request linked to an issue by a closing
# reference, or nothing when none is found. A candidate from the issue timeline
# cross-references is accepted only when gh_pr_is_linked confirms it, so an
# unrelated same-numbered pull request or a bare mention is never matched.
gh_linked_pr_number() {
  local repo="$1" number="$2" candidate
  while IFS= read -r candidate; do
    [[ -z "${candidate}" ]] && continue
    if gh_pr_is_linked "${repo}" "${candidate}" "${number}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done < <(gh_retry api "repos/${repo}/issues/${number}/timeline" --paginate \
             --jq '.[] | select(.event == "cross-referenced")
                        | .source.issue | select(.pull_request != null) | .number' \
           | awk '!seen[$0]++')
  return 0
}

# Print the branch-protection required status check context names for branch
# <branch> of <repo>, one per line. Combines the legacy `contexts` list and the
# newer `checks[].context` objects. A 404 (no protection) or 403 (no permission)
# is a definitive "no required contexts", not a transient error, so this does a
# single attempt and never retries; it prints nothing in that case and the caller
# falls back to the all-signals gate.
gh_required_contexts() {
  local repo="$1" branch="$2" out
  [[ -z "${branch}" ]] && return 0
  out="$(gh api \
    "repos/${repo}/branches/$(_gh_urlencode "${branch}")/protection/required_status_checks" \
    --jq '[.contexts[]?] + [.checks[]?.context] | .[]' 2>/dev/null || true)"
  printf '%s' "${out}" | awk 'NF' | awk '!seen[$0]++'
}

# Normalize a check-runs array (stdin) to "<name>\t<state>" lines, where state is
# success|pending|failing. A run that is not completed is pending; a completed
# run is success for success/neutral/skipped and failing otherwise.
_gh_runs_states() {
  jq -r '.[]? | [ .name,
      (if .status != "completed" then "pending"
       elif ((.conclusion // "") as $c | ($c == "success" or $c == "neutral" or $c == "skipped")) then "success"
       else "failing" end) ] | @tsv' 2>/dev/null || true
}

# Normalize a statuses array (stdin, objects with .context and .state) to
# "<context>\t<state>" lines, where state is success|pending|failing.
_gh_status_states() {
  jq -r '.[]? | [ .context,
      (if .state == "success" then "success"
       elif (.state == "failure" or .state == "error") then "failing"
       else "pending" end) ] | @tsv' 2>/dev/null || true
}

# Reduce a set of "<name>\t<state>" lines (stdin) to one of none|pending|failing|
# green. failing dominates pending, which dominates green; no lines is "none".
_gh_reduce_states() {
  awk -F'\t' '
    { seen=1; if ($2=="failing") fail=1; else if ($2=="pending") pend=1 }
    END {
      if (!seen) { print "none" }
      else if (fail) { print "failing" }
      else if (pend) { print "pending" }
      else { print "green" }
    }'
}

# Report whether the checks on a pull request head are green (SDLC-HOOK-07).
# Prints "green", "pending", or "failing".
#
# When the pull request's base branch defines required status checks in branch
# protection, the gate is scoped to those contexts only, so an advisory or
# always-pending non-required check never blocks an approval. A required context
# that has not reported yet is "pending" (not green), so an approval is never
# applied before a required check runs.
#
# When branch protection is unreadable (no permission) or defines no required
# checks, the gate falls back to requiring EVERY reported signal to be green,
# across BOTH the Checks API (check-runs, e.g. GitHub Actions) and the classic
# Statuses API (status contexts, e.g. Prow/OpenShift). This all-signals fallback
# is intentional: with no required-check list to scope to, the safe choice is to
# require everything green, and a commit with no signals at all is "pending" so an
# approval is never applied on a PR whose CI has not reported.
gh_pr_checks_state() {
  local repo="$1" number="$2" meta sha base required
  local runs statuses runs_state status_state

  meta="$(gh_retry api "repos/${repo}/pulls/${number}" \
            --jq '[.head.sha, (.base.ref // "")] | @tsv' 2>/dev/null || true)"
  sha="${meta%%$'\t'*}"
  base="${meta##*$'\t'}"
  [[ -z "${sha}" ]] && { echo "pending"; return 0; }

  # Latest check run per name (handles reruns: higher id is newer).
  runs="$(gh_retry api "repos/${repo}/commits/${sha}/check-runs" --paginate --slurp \
            --jq '[.[].check_runs[]] | group_by(.name) | map(max_by(.id))' \
          2>/dev/null || echo '[]')"
  # Latest status per context (the combined status endpoint already dedupes).
  statuses="$(gh_retry api "repos/${repo}/commits/${sha}/status" \
                --jq '[.statuses[]? | {context, state}]' 2>/dev/null || echo '[]')"

  required="$(gh_required_contexts "${repo}" "${base}")"
  if [[ -n "${required}" ]]; then
    # Scope to the required contexts only.
    local observed ctx state any_fail=0 any_pending=0
    observed="$(
      { printf '%s' "${runs}" | _gh_runs_states
        printf '%s' "${statuses}" | _gh_status_states; } )"
    while IFS= read -r ctx; do
      [[ -z "${ctx}" ]] && continue
      # Aggregate every signal that matches this required context; failing
      # dominates pending, which dominates success. An empty result means the
      # required context has not reported yet.
      state="$(printf '%s\n' "${observed}" | awk -F'\t' -v c="${ctx}" '
        $1==c { if ($2=="failing") f=1; else if ($2=="pending") p=1; else if ($2=="success") g=1 }
        END { if (f) print "failing"; else if (p) print "pending"; else if (g) print "success"; else print "" }')"
      if [[ -z "${state}" ]]; then any_pending=1      # required but not reported
      elif [[ "${state}" == "failing" ]]; then any_fail=1
      elif [[ "${state}" == "pending" ]]; then any_pending=1
      fi
    done <<< "${required}"
    if (( any_fail )); then echo "failing"; elif (( any_pending )); then echo "pending"; else echo "green"; fi
    return 0
  fi

  # Fallback: all reported signals must be green (documented behavior).
  runs_state="$(printf '%s' "${runs}" | _gh_runs_states | _gh_reduce_states)"
  status_state="$(printf '%s' "${statuses}" | _gh_status_states | _gh_reduce_states)"
  if [[ "${runs_state}" == "failing" || "${status_state}" == "failing" ]]; then
    echo "failing"; return 0
  fi
  if [[ "${runs_state}" == "pending" || "${status_state}" == "pending" ]]; then
    echo "pending"; return 0
  fi
  # Both systems are either green or absent. Require at least one real signal.
  if [[ "${runs_state}" == "none" && "${status_state}" == "none" ]]; then
    echo "pending"; return 0
  fi
  echo "green"
}

_gh_urlencode() {
  local s="$1" out="" c i
  for (( i = 0; i < ${#s}; i++ )); do
    c="${s:i:1}"
    case "${c}" in
      [a-zA-Z0-9.~_-]) out+="${c}" ;;
      *) out+="$(printf '%%%02X' "'${c}")" ;;
    esac
  done
  printf '%s' "${out}"
}
