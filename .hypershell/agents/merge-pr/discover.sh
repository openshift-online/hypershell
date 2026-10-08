#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

if [[ ! ${REPOSITORY:-} =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] ||
   [[ ! ${MAX_ITEMS:-} =~ ^[1-9][0-9]*$ ]] ||
   [[ ! ${DRY_RUN:-} =~ ^(true|false)$ ]]; then
  printf 'Merge-pr discovery: invalid REPOSITORY, MAX_ITEMS, or DRY_RUN.\n' >&2
  exit 0
fi

work_file=$(mktemp)
trap 'rm -f "$work_file"' EXIT
printf 'Merge-pr discovery: scanning %s.\n' "$REPOSITORY" >&2
if ! gh api --paginate --slurp \
  "repos/$REPOSITORY/issues?state=all&labels=agent/review-code-approved&sort=created&direction=asc&per_page=100" \
  > "$work_file"; then
  printf 'Merge-pr discovery: GitHub request failed.\n' >&2
  exit 0
fi

# The issues endpoint also returns PRs. Only issues are work items.
jq -c --argjson limit "$MAX_ITEMS" '
  [ .[][] | select(.pull_request == null) ] | sort_by(.number) | .[:$limit][] |
  {number, url: .html_url, title}
' "$work_file" || exit 0
