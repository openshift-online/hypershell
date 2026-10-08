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

printf 'Review-response discovery: scanning %s for PRs with agent/needs-review-response.\n' "$REPOSITORY" >&2

readonly prs_file=/tmp/review-response-prs.json

gh api --paginate \
  "repos/$REPOSITORY/pulls?state=open&sort=created&direction=asc&per_page=100" \
  --jq '.[] | select(.user.login == "hypershell-builder[bot]") | select(.labels | map(.name) | contains(["agent/needs-review-response"])) | {number: .number, url: .html_url, pr_number: .number}' \
  > "$prs_file" 2>/dev/null || true

total=$(wc -l < "$prs_file" | tr -d ' ')
printf 'Review-response discovery: %s candidate PR(s).\n' "$total" >&2

emitted=0
while IFS= read -r pr_json && (( emitted < MAX_ITEMS )); do
  [[ -n "$pr_json" ]] || continue
  printf '%s\n' "$pr_json"
  ((emitted += 1))
done < "$prs_file"

printf 'Review-response discovery: emitted %s item(s).\n' "$emitted" >&2
