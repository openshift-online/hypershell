#!/usr/bin/env bash
# Unit tests for scripts/skillspector-scan.sh. Mocks skillspector; no network.
# Run: bash scripts/skillspector-scan_test.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCAN_SH="${ROOT}/scripts/skillspector-scan.sh"
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

# Isolate PATH so only our mocks are visible (keep python3/coreutils).
MOCK_BIN="${TEST_DIR}/bin"
mkdir -p "${MOCK_BIN}"
# Resolve real python3 before narrowing PATH.
REAL_PYTHON3="$(command -v python3)"
ln -s "${REAL_PYTHON3}" "${MOCK_BIN}/python3"

# Minimal repo root layout: script resolves REPO_ROOT as parent of scripts/.
# Point SCRIPT via copying scan script into a fake repo so reports land in TEST_DIR.
FAKE_REPO="${TEST_DIR}/repo"
mkdir -p "${FAKE_REPO}/scripts"
cp "${SCAN_SH}" "${FAKE_REPO}/scripts/skillspector-scan.sh"
chmod +x "${FAKE_REPO}/scripts/skillspector-scan.sh"
# Empty baseline so --baseline is passed when present.
cat >"${FAKE_REPO}/.skillspector-baseline.yaml" <<'YAML'
version: 2
rules: []
fingerprints: []
YAML

export PATH="${MOCK_BIN}:/usr/bin:/bin"

write_skillspector_mock() {
  local body="$1"
  cat >"${MOCK_BIN}/skillspector" <<EOF
#!/usr/bin/env bash
set -euo pipefail
# Parse --output PATH
out=""
while [[ \$# -gt 0 ]]; do
  case "\$1" in
    --output) shift; out="\$1" ;;
  esac
  shift || true
done
if [[ -z "\${out}" ]]; then
  echo "mock skillspector: missing --output" >&2
  exit 2
fi
cat >"\${out}" <<'JSON'
${body}
JSON
EOF
  chmod +x "${MOCK_BIN}/skillspector"
}

# --- unknown argument ---
assert_exit 1 'unknown argument rejected' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --bogus

# --- missing skillspector, soft mode ---
rm -f "${MOCK_BIN}/skillspector"
assert_exit 0 'missing skillspector soft-pass' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh"
assert_grep "${TEST_DIR}/err.log" 'NOT scanned' 'soft mode warns unscanned'

# --- missing skillspector, --force ---
assert_exit 1 'missing skillspector force-fail' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force

# --- valid empty issues ---
write_skillspector_mock '{
  "issues": [],
  "suppressed_count": 0,
  "risk_assessment": {"score": 0, "severity": "LOW", "recommendation": "SAFE"}
}'
assert_exit 0 'clean scan passes' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force
assert_grep "${TEST_DIR}/out.log" 'Security scan passed' 'clean scan message'

# --- {} missing issues key (fail closed) ---
write_skillspector_mock '{}'
assert_exit 1 'empty object force-fail' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force
assert_grep "${TEST_DIR}/err.log" "missing required key 'issues'" 'schema error for {}'

# --- issues not a list ---
write_skillspector_mock '{"issues": "nope"}'
assert_exit 1 'non-list issues force-fail' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force

# --- HIGH finding in issues ---
write_skillspector_mock '{
  "issues": [
    {
      "id": "AST4",
      "severity": "HIGH",
      "pattern": "subprocess shell=True",
      "location": {"file": "skills/demo/SKILL.md", "start_line": 10}
    }
  ],
  "suppressed_count": 0,
  "risk_assessment": {"score": 80, "severity": "HIGH", "recommendation": "DO_NOT_INSTALL"}
}'
assert_exit 1 'HIGH issue fails gate' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force
assert_grep "${TEST_DIR}/out.log" 'HIGH: AST4' 'prints HIGH finding'

# --- risk_assessment HIGH with empty issues (defense in depth) ---
write_skillspector_mock '{
  "issues": [],
  "suppressed_count": 0,
  "risk_assessment": {"score": 90, "severity": "CRITICAL", "recommendation": "DO_NOT_INSTALL"}
}'
assert_exit 1 'risk_assessment CRITICAL fails with empty issues' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force
assert_grep "${TEST_DIR}/out.log" 'risk_assessment.severity' 'mentions risk_assessment'

# --- invalid JSON ---
write_skillspector_mock 'not-json'
assert_exit 1 'invalid JSON force-fail' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh" --force

# --- soft mode tolerates incomplete scan ---
write_skillspector_mock '{}'
assert_exit 0 'empty object soft-pass' \
  bash "${FAKE_REPO}/scripts/skillspector-scan.sh"
assert_grep "${TEST_DIR}/err.log" 'Skipping severity filtering' 'soft mode skips filter'

printf '\nskillspector-scan tests: %s passed, %s failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
