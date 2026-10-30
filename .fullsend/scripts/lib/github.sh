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

# Print the number of the open pull request linked to an issue by a closing
# reference, or nothing when none is found. A candidate from the issue timeline
# cross-references is accepted only when it is an open pull request in THIS repo
# whose body carries a GitHub closing reference to this issue
# (Closes/Fixes/Resolves #<n>), matching the spec-implementation contract. This
# avoids matching an unrelated same-numbered PR or a bare mention.
gh_linked_pr_number() {
  local repo="$1" number="$2" candidate meta state is_pr body
  while IFS= read -r candidate; do
    [[ -z "${candidate}" ]] && continue
    meta="$(gh_retry api "repos/${repo}/pulls/${candidate}" \
              --jq '[.state, (.html_url // "")] | @tsv' 2>/dev/null || true)"
    state="${meta%%$'\t'*}"
    [[ "${state}" == "open" ]] || continue
    body="$(gh_retry api "repos/${repo}/pulls/${candidate}" --jq '.body // ""' 2>/dev/null || true)"
    # Closing keyword followed by #<this issue number>, case-insensitive.
    if printf '%s' "${body}" \
         | grep -qiE "(close[sd]?|fix(e[sd])?|resolve[sd]?)[[:space:]]+#${number}([^0-9]|$)"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done < <(gh_retry api "repos/${repo}/issues/${number}/timeline" --paginate \
             --jq '.[] | select(.event == "cross-referenced")
                        | .source.issue | select(.pull_request != null) | .number' \
           | awk '!seen[$0]++')
  return 0
}

# Report whether the checks on a pull request head are green (SDLC-HOOK-07).
# Prints "green", "pending", or "failing". Considers BOTH the Checks API
# (check-runs, e.g. GitHub Actions) and the classic Statuses API (status
# contexts, e.g. Prow/OpenShift), because this repo's CI uses both. The result
# is green only when neither system reports a pending or failing signal;
# "failing" dominates "pending", which dominates "green". A commit with no
# signals at all is "pending" (not green), so an approval is never applied on a
# PR whose CI has not reported.
gh_pr_checks_state() {
  local repo="$1" number="$2" sha runs_state status_state runs status total
  sha="$(gh_pr_head_sha "${repo}" "${number}")"
  [[ -z "${sha}" ]] && { echo "pending"; return 0; }

  # Checks API: flatten all pages of check runs into one array.
  runs="$(gh_retry api "repos/${repo}/commits/${sha}/check-runs" --paginate --slurp \
            --jq '[.[].check_runs[]]' 2>/dev/null || echo '[]')"
  runs_state="$(printf '%s' "${runs}" | jq -r '
      if length == 0 then "none"
      elif any(.[]; .status != "completed") then "pending"
      elif any(.[]; (.conclusion // "") as $c
               | ($c != "success" and $c != "neutral" and $c != "skipped")) then "failing"
      else "green" end' 2>/dev/null || echo "pending")"

  # Classic combined status (status contexts). .state is success|pending|failure;
  # total_count is 0 when no contexts exist.
  status="$(gh_retry api "repos/${repo}/commits/${sha}/status" \
              --jq '[.state, (.total_count | tostring)] | @tsv' 2>/dev/null || printf 'pending\t0')"
  status_state="${status%%$'\t'*}"
  total="${status##*$'\t'}"
  if [[ "${total}" == "0" ]]; then
    status_state="none"
  elif [[ "${status_state}" == "failure" ]]; then
    status_state="failing"
  elif [[ "${status_state}" == "success" ]]; then
    status_state="green"
  else
    status_state="pending"
  fi

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
