#!/usr/bin/env bash
# Discover and run every *_test.sh file. Adding a new shell unit test is
# enough; this runner does not keep an allowlist.
# Portable to macOS bash 3.2 (no mapfile).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

tmp="$(mktemp)"
failed_tmp="$(mktemp)"
trap 'rm -f "${tmp}" "${failed_tmp}"' EXIT

find . -type d \( -name .git -o -name node_modules -o -name vendor \) -prune -o \
  -type f -name '*_test.sh' -print |
  sed 's|^\./||' |
  sort > "${tmp}"

if [[ ! -s "${tmp}" ]]; then
  echo "No *_test.sh tests found."
  exit 0
fi

count="$(wc -l < "${tmp}" | tr -d ' ')"
echo "Discovered ${count} shell unit test file(s):"
sed 's/^/  /' "${tmp}"
echo ""

while IFS= read -r testfile; do
  echo "==> ${testfile}"
  if ! bash "${testfile}"; then
    printf '%s\n' "${testfile}" >> "${failed_tmp}"
  fi
  echo ""
done < "${tmp}"

if [[ -s "${failed_tmp}" ]]; then
  echo "Shell unit tests failed:"
  sed 's/^/  /' "${failed_tmp}"
  exit 1
fi

echo "All ${count} shell unit test file(s) passed."
