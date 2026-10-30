#!/usr/bin/env bash
# lifecycle-logic.sh - pure decision logic for the lifecycle hooks.
#
# Every function here is deterministic and free of network and label writes, so
# the unit tests exercise the real decision code (SDLC-HOOK-13). GitHub I/O
# lives in lib/github.sh; this file only parses, counts, and verifies.
#
# Requires jq for the timeline counting helpers.

# Parse a GitHub issue or pull request URL into "<owner/repo> <number> <kind>".
# Rejects anything that is not an https://github.com issues or pull URL so an
# agent result can never redirect a transition to another repository
# (SDLC-HOOK-04). Returns non-zero on a malformed URL.
lifecycle_parse_issue_url() {
  local url="$1"
  local re='^https://github[.]com/([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)/(issues|pull)/([0-9]+)$'
  if [[ ! "${url}" =~ ${re} ]]; then
    return 1
  fi
  printf '%s %s %s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[2]}"
}

# Count prior applications of a rejection label since the most recent removal of
# agent/blocked (SDLC-HOOK-12). Reads a GitHub issue timeline JSON array on
# stdin. The count is authoritative evidence from the timeline and is never
# taken from the agent result (SDLC-HOOK-12.02). The current rejection being
# decided is NOT in the timeline yet, so callers add 1 to include it.
#
# Usage: lifecycle_count_rejections <rejection-label> < timeline.json
lifecycle_count_rejections() {
  local rejection_label="$1"
  jq -r --arg rej "${rejection_label}" '
    # Keep only label add/remove events, in timeline order.
    [ .[]
      | select(.event == "labeled" or .event == "unlabeled")
      | {event: .event, label: (.label.name // "")}
    ] as $events
    # Index after the most recent removal of agent/blocked, or 0 if never.
    | ( [ $events | to_entries[] | select(.value.event == "unlabeled" and .value.label == "agent/blocked") | .key ]
        | if length == 0 then 0 else (max + 1) end ) as $since
    | [ $events[$since:][] | select(.event == "labeled" and .label == $rej) ] | length
  '
}

# Verify the final label set (SDLC-HOOK-08). Reads the current labels (one per
# line) on stdin. Confirms every label in <add> is present, every label in
# <remove> is absent, and that the within-phase review outcomes are mutually
# exclusive (SDLC-HOOK-01). Prints an actionable message and returns non-zero on
# any violation.
#
# Usage: lifecycle_verify_final "<add labels>" "<remove labels>" < labels.txt
lifecycle_verify_final() {
  local add="$1" remove="$2"
  local labels label
  labels="$(cat)"

  _has_label() {
    local needle="$1" l
    while IFS= read -r l; do
      [[ -z "${l}" ]] && continue
      [[ "${l}" == "${needle}" ]] && return 0
    done <<< "${labels}"
    return 1
  }

  for label in ${add}; do
    if ! _has_label "${label}"; then
      printf 'verify: expected label %q is not present after the transition\n' "${label}" >&2
      return 1
    fi
  done
  for label in ${remove}; do
    if _has_label "${label}"; then
      printf 'verify: superseded label %q is still present after the transition\n' "${label}" >&2
      return 1
    fi
  done

  # Within-phase mutual exclusivity.
  local group count
  for group in \
    "agent/reviewable-spec agent/review-spec-approved agent/review-spec-rejected" \
    "agent/reviewable-code agent/review-code-approved agent/review-code-rejected"; do
    count=0
    for label in ${group}; do
      if _has_label "${label}"; then
        count=$((count + 1))
      fi
    done
    if (( count > 1 )); then
      printf 'verify: more than one competing phase label is active in group: %s\n' "${group}" >&2
      return 1
    fi
  done
  return 0
}
