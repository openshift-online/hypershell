#!/usr/bin/env bash
# Unit test for scripts/codecov-upload.sh: it must skip cleanly without a
# token (fork PRs), refuse incomplete configuration, and pass the right
# PR or branch arguments to the Codecov CLI.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${SCRIPT_DIR}/codecov-upload.sh"
PASS=0
FAIL=0

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# Fake CLI: records its arguments and the token it received.
cat > "${tmp}/codecov" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "${FAKE_ARGS_FILE}"
printf '%s' "${CODECOV_TOKEN}" > "${FAKE_TOKEN_FILE}"
EOF
chmod +x "${tmp}/codecov"
echo "mode: atomic" > "${tmp}/coverage.out"

export FAKE_ARGS_FILE="${tmp}/args" FAKE_TOKEN_FILE="${tmp}/token"

check() {
  local name="$1"
  shift
  if "$@"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo "FAIL: ${name}"
  fi
}

run() {
  env -u CODECOV_TOKEN -u CODECOV_ENTERPRISE_URL -u CODECOV_PR \
    -u CODECOV_PARENT_SHA -u CODECOV_BRANCH -u CODECOV_COMMIT_SHA \
    CODECOV_BIN="${tmp}/codecov" "$@"
}

has_args() {
  grep -qx -- "$1" "${tmp}/args" && grep -A1 -x -- "$1" "${tmp}/args" | tail -1 | grep -qx -- "$2"
}

# 1. No token: skip with exit 0 and never call the CLI.
rm -f "${tmp}/args"
check "no token exits 0" run bash "${SCRIPT}" api-server "${tmp}/coverage.out" >/dev/null
check "no token does not call CLI" test ! -e "${tmp}/args"

# 2. Token without enterprise URL fails.
check "token without URL fails" \
  bash -c '! "$@" >/dev/null 2>&1' _ run CODECOV_TOKEN=t bash "${SCRIPT}" api-server "${tmp}/coverage.out"

# 3. Missing coverage file fails.
check "missing file fails" \
  bash -c '! "$@" >/dev/null 2>&1' _ run CODECOV_TOKEN=t CODECOV_ENTERPRISE_URL=https://cc.example \
  bash "${SCRIPT}" api-server "${tmp}/missing.out"

# 4. Wrong argument count fails.
check "bad usage fails" bash -c '! "$@" >/dev/null 2>&1' _ run bash "${SCRIPT}" api-server

# 5. PR upload passes --pr and --parent-sha, not --branch.
rm -f "${tmp}/args"
check "PR upload succeeds" run CODECOV_TOKEN=secret CODECOV_ENTERPRISE_URL=https://cc.example \
  CODECOV_COMMIT_SHA=abc123 CODECOV_PR=42 CODECOV_PARENT_SHA=def456 CODECOV_BRANCH=feature \
  bash "${SCRIPT}" control-plane "${tmp}/coverage.out" >/dev/null
check "PR: enterprise URL" has_args --enterprise-url https://cc.example
check "PR: slug" has_args --slug openshift-online/hypershell
check "PR: flag" has_args --flag control-plane
check "PR: commit" has_args --commit-sha abc123
check "PR: number" has_args --pr 42
check "PR: parent" has_args --parent-sha def456
check "PR: no branch" bash -c '! grep -qx -- --branch "$1"' _ "${tmp}/args"
check "PR: token passed" grep -qx secret "${tmp}/token"

# 6. Push upload passes --branch and no PR arguments.
rm -f "${tmp}/args"
check "push upload succeeds" run CODECOV_TOKEN=secret CODECOV_ENTERPRISE_URL=https://cc.example \
  CODECOV_COMMIT_SHA=abc123 CODECOV_BRANCH=main \
  bash "${SCRIPT}" cli "${tmp}/coverage.out" >/dev/null
check "push: branch" has_args --branch main
check "push: no pr" bash -c '! grep -qx -- --pr "$1"' _ "${tmp}/args"

echo "codecov-upload_test.sh: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
