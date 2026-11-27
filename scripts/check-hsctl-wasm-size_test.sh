#!/usr/bin/env bash
# Unit test for scripts/check-hsctl-wasm-size.sh: the size check passes within
# the budget, fails over it, and reports the measured and allowed sizes
# (specs/platform/hsctl-browser-build.spec.md, CLI-WASM-05.02).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS=0
FAIL=0

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

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

# Random bytes do not compress, so the gzip size is close to the input size.
head -c 4096 /dev/urandom >"${tmp}/hsctl.wasm"
measured="$(gzip -9 -c "${tmp}/hsctl.wasm" | wc -c | tr -d ' ')"

out="$(HSCTL_WASM_MAX_GZIP_BYTES=$((measured + 1)) sh "${SCRIPT_DIR}/check-hsctl-wasm-size.sh" "${tmp}/hsctl.wasm" 2>&1)" && rc=0 || rc=$?
check "within budget exits 0 (got ${rc}: ${out})" test "${rc}" -eq 0
check "within budget reports the measured size" grep -q "${measured} bytes" <<<"${out}"

# Verifies: CLI-WASM-05.02
out="$(HSCTL_WASM_MAX_GZIP_BYTES=$((measured - 1)) sh "${SCRIPT_DIR}/check-hsctl-wasm-size.sh" "${tmp}/hsctl.wasm" 2>&1)" && rc=0 || rc=$?
check "over budget exits non-zero" test "${rc}" -ne 0
check "over budget reports measured and allowed sizes (${out})" \
  grep -q "${measured} bytes (gzip -9), over the allowed $((measured - 1)) bytes" <<<"${out}"

out="$(sh "${SCRIPT_DIR}/check-hsctl-wasm-size.sh" "${tmp}/hsctl.wasm" 2>&1)" && rc=0 || rc=$?
check "default budget is 8 MiB (${out})" grep -q "allowed 8388608 bytes" <<<"${out}"

out="$(sh "${SCRIPT_DIR}/check-hsctl-wasm-size.sh" "${tmp}/missing.wasm" 2>&1)" && rc=0 || rc=$?
check "missing module exits non-zero" test "${rc}" -ne 0
check "missing module names the build script" grep -q "build-hsctl-wasm.sh" <<<"${out}"

echo "check-hsctl-wasm-size: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
