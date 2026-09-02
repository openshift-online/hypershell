#!/usr/bin/env bash
set -euo pipefail

mode="${1:-local}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "${script_dir}/.." && pwd)"
git_work_tree="${HYPERSHELL_GIT_WORK_TREE:-${repository_root}}"
revision="${HYPERSHELL_VCS_REF:-$(git -C "${git_work_tree}" rev-parse HEAD 2>/dev/null || true)}"
revision="${revision,,}"

if [[ ! "${revision}" =~ ^[0-9a-f]{40}$ ]]; then
  echo "build version error: the revision must be a full 40-character Git SHA" >&2
  exit 1
fi

case "${mode}" in
  local)
    prefix=dev
    dirty_state="${HYPERSHELL_GIT_DIRTY:-}"
    if [[ -z "${dirty_state}" ]]; then
      if ! work_tree_status="$(git -C "${git_work_tree}" status --porcelain --untracked-files=normal 2>/dev/null)"; then
        echo "build version error: the Git work tree could not be inspected" >&2
        exit 1
      fi
      if [[ -n "${work_tree_status}" ]]; then
        dirty_state=1
      else
        dirty_state=0
      fi
    fi
    case "${dirty_state}" in
      0)
        suffix=""
        ;;
      1)
        suffix=-modified
        ;;
      *)
        echo "build version error: HYPERSHELL_GIT_DIRTY must be 0 or 1" >&2
        exit 1
        ;;
    esac
    ;;
  ci)
    version_file="${HYPERSHELL_VERSION_FILE:-${repository_root}/VERSION}"
    version="$(tr -d '\r\n' < "${version_file}")"
    if [[ ! "${version}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
      echo "build version error: VERSION must contain one stable semantic version" >&2
      exit 1
    fi
    prefix="v${version}"
    suffix=""
    ;;
  *)
    echo "build version error: mode must be local or ci" >&2
    exit 1
    ;;
esac

printf '%s-%s%s\n' "${prefix}" "${revision:0:7}" "${suffix}"
