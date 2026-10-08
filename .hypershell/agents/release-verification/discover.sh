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

printf 'Release-verification discovery: scanning %s.\n' "$REPOSITORY" >&2

readonly issues_file=/tmp/release-verify-issues.json

# Issues with agent/release-pending, without a verdict.
gh api --paginate \
  "repos/$REPOSITORY/issues?state=open&labels=agent/release-pending&sort=created&direction=asc&per_page=100" \
  --jq '.[] | select(.labels | map(.name) | (contains(["agent/release-verified"]) or contains(["agent/release-verification-failed"])) | not) | {number: .number, url: .html_url, title: .title, body: .body}' \
  > "$issues_file" 2>/dev/null || true

total=$(wc -l < "$issues_file" | tr -d ' ')
printf 'Release-verification discovery: %s candidate issue(s).\n' "$total" >&2

emitted=0
while IFS= read -r issue_json && (( emitted < MAX_ITEMS )); do
  [[ -n "$issue_json" ]] || continue

  issue_number=$(printf '%s' "$issue_json" | jq -r '.number')
  issue_body=$(printf '%s' "$issue_json" | jq -r '.body // ""')

  release_bundle=$(printf '%s' "$issue_body" | grep -oP '(?<=RELEASE_BUNDLE:\s)\S+' | head -1 || true)
  gitops_repo=$(printf '%s' "$issue_body" | grep -oP '(?<=GITOPS_REPO:\s)\S+' | head -1 || true)
  strategy_overlay=$(printf '%s' "$issue_body" | grep -oP '(?<=RELEASE_STRATEGY_OVERLAY:\s)\S+' | head -1 || true)

  if [[ -z "$release_bundle" || -z "$gitops_repo" ]]; then
    printf 'Release-verification discovery: issue #%s missing RELEASE_BUNDLE or GITOPS_REPO; skipping.\n' \
      "$issue_number" >&2
    continue
  fi

  output=$(printf '%s' "$issue_json" | jq --arg rb "$release_bundle" --arg gr "$gitops_repo" --arg so "$strategy_overlay" \
    'del(.body) | . + {release_bundle: $rb, gitops_repo: $gr, strategy_overlay: (if $so == "" then null else $so end)}')

  printf '%s\n' "$output"
  ((emitted += 1))
done < "$issues_file"

printf 'Release-verification discovery: emitting %s item(s).\n' "$emitted" >&2
