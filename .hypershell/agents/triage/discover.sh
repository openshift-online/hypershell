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

printf 'Triage discovery: scanning %s for untriaged agent/allowed issues.\n' "$REPOSITORY" >&2

triage_labels='agent/workable,agent/needs-input,agent/duplicate'

untriaged_count=$(gh api --paginate \
  "repos/$REPOSITORY/issues?state=open&labels=agent/allowed&per_page=100" \
  --jq '[.[] | select(.labels | map(.name) | (contains(["agent/workable"]) or contains(["agent/needs-input"]) or contains(["agent/duplicate"])) | not)] | length')

printf 'Triage discovery: %s untriaged issue(s) with agent/allowed.\n' "$untriaged_count" >&2

if (( untriaged_count == 0 )); then
  printf 'Triage discovery: nothing to do.\n' >&2
  exit 0
fi

repo_url="https://github.com/$REPOSITORY"
printf '{"repo_url":"%s","issue_count":%s}\n' "$repo_url" "$untriaged_count"
