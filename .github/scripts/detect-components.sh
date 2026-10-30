#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config="${script_dir}/../component-paths.json"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"

# Validate before using jq in process substitutions: malformed metadata must
# fail the required detection job instead of emitting a partial/empty result.
jq -e 'type == "object" and length > 0 and all(.[];
  type == "object" and (.paths | type == "array" and length > 0 and
  all(.[]; type == "string")))' "${config}" >/dev/null

all_components_changed=false

get_changed_files() {
  if [[ -n "${CHANGED_FILES:-}" ]]; then
    printf '%s\n' "${CHANGED_FILES}"
    return
  fi

  local base_sha head_sha
  case "${GITHUB_EVENT_NAME:-}" in
    pull_request)
      base_sha="$(jq -r '.pull_request.base.sha' "${GITHUB_EVENT_PATH}")"
      head_sha="$(jq -r '.pull_request.head.sha' "${GITHUB_EVENT_PATH}")"
      # For Mergify draft PRs the head includes every preceding queued PR.
      # Diff against the target branch, never the preceding speculative head,
      # so the detector selects the union of all cumulative changes.
      # Disable rename detection so both old and new component paths are tested.
      # Three-dot (merge-base) diff so component changes that landed on the base
      # branch after this PR forked are not misattributed to the PR. A two-dot
      # "${base_sha} HEAD" diff (when HEAD is a merge commit) would flag base-branch drift as PR changes.
      git diff --no-renames --name-only "${base_sha}...${head_sha}"
      ;;
    merge_group)
      base_sha="$(jq -r '.merge_group.base_sha' "${GITHUB_EVENT_PATH}")"
      git diff --no-renames --name-only "${base_sha}...HEAD"
      ;;
    push)
      base_sha="$(jq -r '.before' "${GITHUB_EVENT_PATH}")"
      if [[ "${base_sha}" =~ ^0+$ ]]; then
        git diff-tree --no-renames --no-commit-id --name-only -r HEAD
      else
        git diff --no-renames --name-only "${base_sha}" HEAD
      fi
      ;;
    *)
      if git rev-parse --verify HEAD^ >/dev/null 2>&1; then
        git diff --no-renames --name-only HEAD^ HEAD
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
  # A process substitution hides get_changed_files failures from set -e.
  # Capture first so missing SHAs / failed git or jq abort detection and fail
  # the required gate, rather than silently reporting no affected components.
  changed_files_text="$(get_changed_files)"
  changed_files=()
  if [[ -n "${changed_files_text}" ]]; then
    mapfile -t changed_files <<< "${changed_files_text}"
  fi
fi

while IFS= read -r component; do
  matched="${all_components_changed}"

  if [[ "${matched}" != true ]]; then
    while IFS= read -r pattern; do
      for file in "${changed_files[@]}"; do
        # Patterns from component-paths.json intentionally use shell globs.
        # shellcheck disable=SC2053
        if [[ "${file}" == ${pattern} ]]; then
          matched=true
          break 2
        fi
      done
    done < <(jq -r --arg component "${component}" '.[$component].paths[]' "${config}")
  fi

  printf '%s=%s\n' "${component}" "${matched}" >> "${output_file}"
done < <(jq -r 'keys[]' "${config}")

# Shell unit tests are auto-discovered: any *_test.sh, or any file in the same
# directory as one. Changing the runner or the unit-test workflow also selects
# them so CI still exercises the discovery path.
bash_tests="${all_components_changed}"
if [[ "${bash_tests}" != true ]]; then
  for file in "${changed_files[@]}"; do
    if [[ "${file}" == ".github/workflows/unit-tests.yml" ||
          "${file}" == "scripts/run-shell-unit-tests.sh" ||
          "${file}" == ".github/scripts/detect-components.sh" ||
          "${file}" == "Makefile" ]]; then
      bash_tests=true
      break
    fi
  done
fi
if [[ "${bash_tests}" != true ]]; then
  while IFS= read -r testfile; do
    [[ -z "${testfile}" ]] && continue
    testdir="${testfile%/*}"
    for file in "${changed_files[@]}"; do
      if [[ "${file}" == "${testfile}" || "${file}" == "${testdir}/"* ]]; then
        bash_tests=true
        break 2
      fi
    done
  done < <(
    find . -type d \( -name .git -o -name node_modules -o -name vendor \) -prune -o \
      -type f -name '*_test.sh' -print |
      sed 's|^\./||' |
      sort
  )
fi
printf '%s=%s\n' "bash_tests" "${bash_tests}" >> "${output_file}"
