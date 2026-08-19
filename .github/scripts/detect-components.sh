#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config="${script_dir}/../component-paths.json"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"

all_components_changed=false

get_changed_files() {
  if [[ -n "${CHANGED_FILES:-}" ]]; then
    printf '%s\n' "${CHANGED_FILES}"
    return
  fi

  local base_sha
  case "${GITHUB_EVENT_NAME:-}" in
    pull_request)
      base_sha="$(jq -r '.pull_request.base.sha' "${GITHUB_EVENT_PATH}")"
      head_sha="$(jq -r '.pull_request.head.sha' "${GITHUB_EVENT_PATH}")"
      # Three-dot (merge-base) diff so component changes that landed on the base
      # branch after this PR forked are not misattributed to the PR. A two-dot
      # "${base_sha} HEAD" diff (when HEAD is a merge commit) would flag base-branch drift as PR changes.
      git diff --name-only "${base_sha}...${head_sha}"
      ;;
    merge_group)
      base_sha="$(jq -r '.merge_group.base_sha' "${GITHUB_EVENT_PATH}")"
      git diff --name-only "${base_sha}...HEAD"
      ;;
    push)
      base_sha="$(jq -r '.before' "${GITHUB_EVENT_PATH}")"
      if [[ "${base_sha}" =~ ^0+$ ]]; then
        git diff-tree --no-commit-id --name-only -r HEAD
      else
        git diff --name-only "${base_sha}" HEAD
      fi
      ;;
    *)
      if git rev-parse --verify HEAD^ >/dev/null 2>&1; then
        git diff --name-only HEAD^ HEAD
      else
        git ls-files
      fi
      ;;
  esac
}

if [[ "${GITHUB_EVENT_NAME:-}" == workflow_dispatch && -z "${CHANGED_FILES:-}" ]]; then
  all_components_changed=true
  changed_files=()
else
  mapfile -t changed_files < <(get_changed_files)
fi

while IFS= read -r component; do
  matched="${all_components_changed}"

  if [[ "${matched}" != true ]]; then
    while IFS= read -r pattern; do
      for file in "${changed_files[@]}"; do
        if [[ "${file}" == ${pattern} ]]; then
          matched=true
          break 2
        fi
      done
    done < <(jq -r --arg component "${component}" '.[$component].paths[]' "${config}")
  fi

  printf '%s=%s\n' "${component}" "${matched}" >> "${output_file}"
done < <(jq -r 'keys[]' "${config}")
