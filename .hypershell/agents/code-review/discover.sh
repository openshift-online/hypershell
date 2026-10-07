#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

required_variables=(REPOSITORY MAX_ITEMS DRY_RUN)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Required variable %s is empty.\n' "$variable" >&2
    exit 1
  fi
done

if [[ ! "$REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  printf 'REPOSITORY is not valid.\n' >&2
  exit 1
fi

if [[ ! "$MAX_ITEMS" =~ ^[1-9][0-9]*$ ]]; then
  printf 'MAX_ITEMS is not valid.\n' >&2
  exit 1
fi

printf 'Code-review discovery: scanning %s.\n' "$REPOSITORY" >&2

readonly work_file=/tmp/code-review-candidates.json

# Issues with agent/reviewable-code, without a verdict.
gh api --paginate \
  "repos/$REPOSITORY/issues?state=open&labels=agent/reviewable-code&sort=created&direction=asc&per_page=100" \
  --jq '.[] | select(.labels | map(.name) | (contains(["agent/review-code-approved"]) or contains(["agent/review-code-rejected"])) | not) | {number: .number, url: .html_url, title: .title}' \
  > "$work_file" 2>/dev/null || true

total=$(wc -l < "$work_file" | tr -d ' ')
printf 'Code-review discovery: %s candidate(s).\n' "$total" >&2

emitted=0
while IFS= read -r item && (( emitted < MAX_ITEMS )); do
  [[ -n "$item" ]] || continue
  printf '%s\n' "$item"
  ((emitted += 1))
done < "$work_file"

printf 'Code-review discovery: emitting %s of %s item(s).\n' "$emitted" "$total" >&2
