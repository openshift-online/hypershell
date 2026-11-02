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

# Fake CLI: records its arguments, the token it received, and a copy of the
# coverage file it was asked to upload.
cat > "${tmp}/codecov" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "${FAKE_ARGS_FILE}"
printf '%s' "${CODECOV_TOKEN}" > "${FAKE_TOKEN_FILE}"
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--file" ]]; then
    cp "$2" "${FAKE_UPLOAD_FILE}"
  fi
  shift
done
EOF
chmod +x "${tmp}/codecov"
echo "mode: atomic" > "${tmp}/coverage.out"

export FAKE_ARGS_FILE="${tmp}/args" FAKE_TOKEN_FILE="${tmp}/token" FAKE_UPLOAD_FILE="${tmp}/uploaded"

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

# 7. Generated code is removed from the uploaded profile; the original stays.
# Verifies: E2E-CORE-24.07
m="github.com/openshift-online/hypershell/components"
cat > "${tmp}/mixed.out" <<EOF
mode: atomic
${m}/api-server/plugins/gateways/handler.go:10.2,12.3 2 1
${m}/api-server/pkg/api/openapi/client.go:5.1,9.2 4 0
${m}/api-server/pkg/api/grpc/hypershell/v1/gateways.pb.go:7.1,8.2 1 0
${m}/api-server/pkg/api/grpc/hypershell/v1/gateways_grpc.pb.go:7.1,8.2 1 0
${m}/api-server/plugins/gateways/mock_dao.go:3.1,4.2 1 0
${m}/control-plane/internal/reconciler/zz_generated.deepcopy.go:3.1,4.2 1 0
${m}/control-plane/internal/reconciler/reconciler.go:20.1,22.2 3 0
${m}/api-server/pkg/api/openapi_embed.go:1.1,2.2 1 1
EOF
cp "${tmp}/mixed.out" "${tmp}/mixed.orig"
rm -f "${tmp}/uploaded"
check "filtered upload succeeds" run CODECOV_TOKEN=secret CODECOV_ENTERPRISE_URL=https://cc.example \
  CODECOV_COMMIT_SHA=abc123 CODECOV_BRANCH=main \
  bash "${SCRIPT}" api-server "${tmp}/mixed.out" >/dev/null
printf '%s\n' "mode: atomic" \
  "${m}/api-server/plugins/gateways/handler.go:10.2,12.3 2 1" \
  "${m}/control-plane/internal/reconciler/reconciler.go:20.1,22.2 3 0" \
  "${m}/api-server/pkg/api/openapi_embed.go:1.1,2.2 1 1" > "${tmp}/expected"
check "filter: only hand-written blocks are uploaded" cmp -s "${tmp}/expected" "${tmp}/uploaded"
check "filter: the original profile is unchanged" cmp -s "${tmp}/mixed.orig" "${tmp}/mixed.out"

echo "codecov-upload_test.sh: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
