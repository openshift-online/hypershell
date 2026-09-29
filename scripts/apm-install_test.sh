#!/usr/bin/env bash
# Unit tests for scripts/apm-install.sh. Mocks apm and skillspector-scan.
# Run: bash scripts/apm-install_test.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

PASS=0
FAIL=0

assert_exit() {
  local want="$1" label="$2"
  shift 2
  local got=0
  "$@" >"${TEST_DIR}/out.log" 2>"${TEST_DIR}/err.log" || got=$?
  if [[ "${got}" -eq "${want}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want exit %s got %s)\n' "${label}" "${want}" "${got}"
    printf '--- stdout ---\n%s\n--- stderr ---\n%s\n' \
      "$(cat "${TEST_DIR}/out.log")" "$(cat "${TEST_DIR}/err.log")"
  fi
}

assert_grep() {
  local file="$1" pattern="$2" label="$3"
  if grep -qE -- "${pattern}" "${file}"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (pattern %s not in %s)\n' "${label}" "${pattern}" "${file}"
    cat "${file}"
  fi
}

FAKE_REPO="${TEST_DIR}/repo"
mkdir -p "${FAKE_REPO}/scripts"
cp "${ROOT}/scripts/apm-install.sh" "${FAKE_REPO}/scripts/apm-install.sh"
chmod +x "${FAKE_REPO}/scripts/apm-install.sh"

MOCK_BIN="${TEST_DIR}/bin"
mkdir -p "${MOCK_BIN}"
export PATH="${MOCK_BIN}:/usr/bin:/bin"

# Real git is required by the prereq check.
ln -s "$(command -v git)" "${MOCK_BIN}/git"

cat >"${MOCK_BIN}/apm" <<'EOF'
#!/bin/sh
echo "mock apm install $*"
EOF
chmod +x "${MOCK_BIN}/apm"

# skillspector-scan.sh is invoked by relative path from SCRIPT_DIR.
cat >"${FAKE_REPO}/scripts/skillspector-scan.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "mock skillspector-scan $*"
# Record args for assertions
printf '%s\n' "$*" >"$(dirname "$0")/../scan-args.txt"
exit 0
EOF
chmod +x "${FAKE_REPO}/scripts/skillspector-scan.sh"

# --- unknown argument ---
assert_exit 1 'unknown argument rejected' \
  bash "${FAKE_REPO}/scripts/apm-install.sh" --nope

# --- soft install without skillspector on PATH ---
assert_exit 0 'soft install without skillspector' \
  bash "${FAKE_REPO}/scripts/apm-install.sh"
assert_grep "${FAKE_REPO}/scan-args.txt" '^$' 'soft mode calls scan without --force'

# --- force requires skillspector ---
assert_exit 1 'force requires skillspector binary' \
  bash "${FAKE_REPO}/scripts/apm-install.sh" --force
assert_grep "${TEST_DIR}/out.log" 'skillspector' 'force lists skillspector as required'

# --- force succeeds when skillspector is present ---
cat >"${MOCK_BIN}/skillspector" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "${MOCK_BIN}/skillspector"
# python3 is required when skillspector is present (force path).
ln -sf "$(command -v python3)" "${MOCK_BIN}/python3"

rm -f "${FAKE_REPO}/scan-args.txt"
assert_exit 0 'force install with skillspector' \
  bash "${FAKE_REPO}/scripts/apm-install.sh" --force
assert_grep "${FAKE_REPO}/scan-args.txt" '--force' 'force mode passes --force to scan'

printf '\napm-install tests: %s passed, %s failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
