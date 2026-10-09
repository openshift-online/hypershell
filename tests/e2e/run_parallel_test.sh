#!/usr/bin/env bash
# Check run-parallel.sh and the disruptive-gate helper without a cluster.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="${SCRIPT_DIR}/run-parallel.sh"

PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "$want" == "$got" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "$label" "$want" "$got"
  fi
}

assert_contains() {
  local needle="$1" haystack="$2" label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (missing %q)\n' "$label" "$needle"
  fi
}

run() {
  local rc=0
  OUT="$(E2E_PARALLEL_HEARTBEAT=1000 E2E_PARALLEL_LOG_DIR="${TMP}/logs" bash "$RUNNER" "$@" 2>&1)" || rc=$?
  RC=$rc
}

# Both suites pass: exit 0, each log is replayed, both results reported.
run "a=echo from-a" "b=echo from-b"
assert_eq 0 "$RC" 'all suites passing exits 0'
assert_contains 'from-a' "$OUT" 'first suite log is replayed'
assert_contains 'from-b' "$OUT" 'second suite log is replayed'
assert_contains '[a] from-a' "$OUT" 'first suite log streams with a prefix'
assert_contains '[b] from-b' "$OUT" 'second suite log streams with a prefix'
assert_contains 'a: exit 0' "$OUT" 'first suite result is reported'
assert_contains 'b: exit 0' "$OUT" 'second suite result is reported'
assert_contains 'from-a' "$(<"${TMP}/logs/a.log")" 'first suite log is retained'
assert_contains 'from-b' "$(<"${TMP}/logs/b.log")" 'second suite log is retained'

# One suite fails: the runner fails and names it, but the other still ran.
run "ok=echo fine" "bad=echo broke; exit 3"
assert_eq 1 "$RC" 'a failing suite fails the runner'
assert_contains 'bad: exit 3' "$OUT" 'failing suite exit code is reported'
assert_contains 'ok: exit 0' "$OUT" 'passing suite is still reported'
assert_contains 'broke' "$OUT" 'failing suite log is replayed'

# Invalid specs are rejected before anything runs.
run "no-equals-sign"
assert_eq 2 "$RC" 'a spec without NAME=COMMAND is rejected'
run "bad name=echo x"
assert_eq 2 "$RC" 'a suite name with spaces is rejected'
RC=0
bash "$RUNNER" >/dev/null 2>&1 || RC=$?
assert_eq 2 "$RC" 'no suites is a usage error'

# The gate stays closed for the primary suite until the others have exited.
primary="bash -c 'source ${SCRIPT_DIR}/lib.sh; e2e_wait_disruptive_gate; test -e ${TMP}/other-done'"
other="sleep 2; touch ${TMP}/other-done"
run "primary=${primary}" "other=${other}"
assert_eq 0 "$RC" 'primary does not pass the gate before the other suite exits'
assert_contains 'gate opened' "$OUT" 'runner reports the gate opening'

# A single suite is not held back.
rm -f "${TMP}/other-done"
primary_alone="bash -c 'source ${SCRIPT_DIR}/lib.sh; e2e_wait_disruptive_gate; echo through'"
run "solo=${primary_alone}"
assert_eq 0 "$RC" 'a lone suite passes the gate immediately'
assert_contains 'through' "$OUT" 'a lone suite reaches the code after the gate'

# Standalone run (no runner): the helper is a no-op.
RC=0
(
  # shellcheck source=lib.sh
  source "${SCRIPT_DIR}/lib.sh"
  unset E2E_DISRUPTIVE_GATE_FILE
  e2e_wait_disruptive_gate
) >/dev/null 2>&1 || RC=$?
assert_eq 0 "$RC" 'gate helper returns at once when no runner set the gate file'

# A gate that never opens times out and proceeds instead of hanging.
RC=0
start=$(date +%s)
(
  # shellcheck source=lib.sh
  source "${SCRIPT_DIR}/lib.sh"
  E2E_DISRUPTIVE_GATE_FILE="${TMP}/never" E2E_DISRUPTIVE_GATE_TIMEOUT=1 e2e_wait_disruptive_gate
) >/dev/null 2>&1 || RC=$?
elapsed=$(($(date +%s) - start))
assert_eq 0 "$RC" 'gate helper proceeds after its timeout'
if ((elapsed > 10)); then
  FAIL=$((FAIL + 1))
  printf 'FAIL: gate helper took %ss with a 1s timeout\n' "$elapsed"
else
  PASS=$((PASS + 1))
fi

echo "run-parallel tests: ${PASS} passed, ${FAIL} failed"
((FAIL == 0))
