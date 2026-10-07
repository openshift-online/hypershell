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

printf 'Spec-review discovery: scanning %s.\n' "$REPOSITORY" >&2

readonly issues_file=/tmp/spec-review-issues.json

# Issues with agent/reviewable-spec that have not yet been given a verdict.
gh api --paginate \
  "repos/$REPOSITORY/issues?state=open&labels=agent/reviewable-spec&sort=created&direction=asc&per_page=100" \
  --jq '.[] | select(.labels | map(.name) | (contains(["agent/review-spec-approved"]) or contains(["agent/review-spec-rejected"])) | not) | {number: .number, url: .html_url, title: .title}' \
  > "$issues_file" 2>/dev/null || true

total=$(wc -l < "$issues_file" | tr -d ' ')
printf 'Spec-review discovery: %s candidate issue(s).\n' "$total" >&2

emitted=0
while IFS= read -r issue_json && (( emitted < MAX_ITEMS )); do
  [[ -n "$issue_json" ]] || continue

  issue_number=$(printf '%s' "$issue_json" | jq -r '.number')
  issue_url=$(printf '%s' "$issue_json" | jq -r '.url')

  # Find the linked PR: look for open PRs on branch agent/work/issue/<number> or matching issue ref.
  pr_data=$(gh api --paginate \
    "repos/$REPOSITORY/pulls?state=open&head=${REPOSITORY%%/*}:agent/work/issue/$issue_number&per_page=10" \
    --jq 'first | {pr_number: .number, pr_url: .html_url} // empty' 2>/dev/null || true)

  if [[ -z "$pr_data" ]]; then
    # Fall back: search PRs that mention this issue number in their body.
    pr_data=$(gh api --paginate \
      "repos/$REPOSITORY/pulls?state=open&per_page=100" \
      --jq "[.[] | select(.body != null and (.body | contains(\"#$issue_number\") or contains(\"/issues/$issue_number\"))) | {pr_number: .number, pr_url: .html_url}] | first // empty" \
      2>/dev/null || true)
  fi

  if [[ -z "$pr_data" ]]; then
    printf 'Spec-review discovery: no linked PR for issue #%s; skipping.\n' "$issue_number" >&2
    continue
  fi

  printf '%s\n' "$(printf '%s' "$issue_json $pr_data" | jq -s 'add')"
  ((emitted += 1))
done < "$issues_file"

printf 'Spec-review discovery: emitting %s item(s).\n' "$emitted" >&2
