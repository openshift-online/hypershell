#!/usr/bin/env bash
# Verify the make e2e wrapper keeps the test failure verdict and highlights it
# after the Go suite has finished its diagnostics and cleanup.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

cat >"${workdir}/fake-go" <<'EOF'
#!/usr/bin/env bash
echo 'fake Error Trace: assertion details'
exit "${FAKE_GO_EXIT:-0}"
EOF
chmod +x "${workdir}/fake-go"

PASS=0
FAIL=0

run_make() {
  local rc=0
  OUTPUT="$(
    GITHUB_ACTIONS=true FAKE_GO_EXIT="$1" \
      make --no-print-directory -s -C "${REPO_ROOT}" e2e E2E_GO="${workdir}/fake-go" 2>&1
  )" || rc=$?
  RC="${rc}"
}

run_make 0
if [[ "${RC}" -eq 0 && "${OUTPUT}" != *"E2E SUITE FAILED"* ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: a successful suite must not print the failure banner'
fi

run_make 7
if [[ "${RC}" -ne 0 ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: make e2e must fail when the Go suite fails'
fi
if [[ "${OUTPUT}" == *"fake Error Trace: assertion details"* &&
      "${OUTPUT}" == *"================ E2E SUITE FAILED ================"* &&
      "${OUTPUT}" == *"::error title=E2E suite failed::"* ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: a failed suite must end with a visible banner and GitHub annotation'
  printf '%s\n' "${OUTPUT}"
fi

printf 'e2e make logging tests: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
