#!/usr/bin/env bash
set -euo pipefail

mode="${1:-local}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "${script_dir}/.." && pwd)"
revision="${HYPERSHELL_VCS_REF:-$(git -C "${repository_root}" rev-parse HEAD 2>/dev/null || true)}"
revision="${revision,,}"

if [[ ! "${revision}" =~ ^[0-9a-f]{40}$ ]]; then
  echo "build version error: the revision must be a full 40-character Git SHA" >&2
  exit 1
fi

case "${mode}" in
  local)
    prefix=dev
    ;;
  ci)
    version_file="${HYPERSHELL_VERSION_FILE:-${repository_root}/VERSION}"
    version="$(tr -d '\r\n' < "${version_file}")"
    if [[ ! "${version}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
      echo "build version error: VERSION must contain one stable semantic version" >&2
      exit 1
    fi
    prefix="v${version}"
    ;;
  *)
    echo "build version error: mode must be local or ci" >&2
    exit 1
    ;;
esac

printf '%s-%s\n' "${prefix}" "${revision:0:7}"
