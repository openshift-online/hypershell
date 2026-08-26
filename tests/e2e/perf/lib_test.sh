#!/usr/bin/env bash
# Unit tests for tests/e2e/perf/lib.sh and the E2E_MODE helper in lib.sh.
# No cluster required. No python.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib.sh
source "${SCRIPT_DIR}/../lib.sh"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

FAILS=0
pass_u() { green "  ✓ $1"; }
fail_u() { red "  ✗ $1"; FAILS=$((FAILS + 1)); }

echo ""
bold "perf-lib + E2E_MODE unit tests"
sep

# --- E2E_MODE ---

E2E_MODE=long
if e2e_step short && e2e_step long; then
  pass_u "long mode runs short and long steps"
else
  fail_u "long mode should run every step"
fi

E2E_MODE=short
if e2e_step short && ! e2e_step long; then
  pass_u "short mode runs short steps and skips long steps"
else
  fail_u "short mode step gating is wrong"
fi

E2E_MODE=medium
if (e2e_validate_mode) &>/dev/null; then
  fail_u "invalid E2E_MODE should fail"
else
  pass_u "invalid E2E_MODE fails fast"
fi
E2E_MODE=long

# --- Percentiles (nearest-rank: ceil(p/100*n) for 1..10 -> 5, 9, 10, 10) ---

json=$(perf_percentiles_json 1 2 3 4 5 6 7 8 9 10)
if [[ "$json" == '{"p50": 5, "p90": 9, "p99": 10, "max": 10}' ]]; then
  pass_u "percentiles: nearest-rank p50=5 p90=9 p99=10 max=10 for 1..10"
else
  fail_u "percentiles unexpected: ${json}"
fi

empty=$(perf_percentiles_json)
if [[ "$empty" == '{"p50": null, "p90": null, "p99": null, "max": null}' ]]; then
  pass_u "empty percentiles are null"
else
  fail_u "empty percentiles should be null: ${empty}"
fi

# --- JSON results (bash state, no merge-from-stdin) ---

tmp=$(mktemp -d)
results="${tmp}/kind-testrun.json"
E2E_INFRA_DRIVER=kind
PERF_STARTED_AT="2026-08-21T15:30:00Z"
E2E_PERF_GATEWAY_COUNT=20
E2E_PERF_BATCH_SIZE=5
E2E_PERF_CONCURRENCY=4
E2E_PERF_PROVISION_TIMEOUT=600
E2E_PERF_GATEWAY_PREFIX=perf-gw
E2E_PERF_CHECKPOINT=1
E2E_PERF_STOP_ON_CHECKPOINT_FAILURE=1
E2E_PERF_RUN_FUNCTIONAL=1
E2E_PERF_MIN_SUCCESS_RATE=""
E2E_PERF_MAX_PROVISION_P99=""
perf_results_init "$results"

PERF_P50=92 PERF_P90=140 PERF_P99=150 PERF_MAX=152
perf_results_add_checkpoint 5 "2026-08-21T15:33:10Z" short pass 34

need=(schema_version driver started_at finished_at config scale_up checkpoints functional slo result)
missing=()
for k in "${need[@]}"; do
  grep -q "\"${k}\"" "$results" || missing+=("$k")
done
if [[ ${#missing[@]} -eq 0 ]] && grep -q '"schema_version": "1"' "$results" && grep -q '"gateways_running": 5' "$results"; then
  pass_u "results JSON has documented schema_version=1 keys and incremental checkpoints"
else
  fail_u "results JSON missing keys: ${missing[*]:-schema/checkpoint}"
fi

PERF_RES_RESULT=pass
PERF_RES_FINISHED_AT="2026-08-21T15:41:12Z"
perf_results_write
if grep -q '"result": "pass"' "$results" && grep -q '"gateways_running": 5' "$results"; then
  pass_u "results rewrite keeps checkpoints"
else
  fail_u "results rewrite dropped checkpoints or result"
fi

# --- Report ---

mkdir -p "${tmp}/hist"
cp "$results" "${tmp}/hist/kind-20260821T153000Z.json"
PERF_RES_STARTED_AT="2026-08-21T16:00:00Z"
PERF_RES_RESULT=fail
PERF_RES_PATH="${tmp}/hist/kind-20260821T160000Z.json"
perf_results_write

report=$(E2E_PERF_RESULTS_DIR="${tmp}/hist" E2E_PERF_REPORT_LIMIT=10 bash "${SCRIPT_DIR}/../../../scripts/perf-report.sh")
if echo "$report" | grep -q 'kind' && echo "$report" | grep -q 'fail' && echo "$report" | grep -q 'pass'; then
  pass_u "report tabulates recent runs"
else
  fail_u "report output missing expected rows: ${report:0:200}"
fi

ckpt=$(E2E_PERF_RESULTS_DIR="${tmp}/hist" E2E_PERF_REPORT_RUN=20260821T153000Z bash "${SCRIPT_DIR}/../../../scripts/perf-report.sh")
if echo "$ckpt" | grep -q 'batch p99' && echo "$ckpt" | grep -q '150'; then
  pass_u "report renders a single run's checkpoints"
else
  fail_u "checkpoint report unexpected: ${ckpt:0:200}"
fi

# --- Bounded concurrency ---

PERF_BG_PIDS=()
PROGRESS_TICKS=0
perf_progress_tick() { PROGRESS_TICKS=$((PROGRESS_TICKS + 1)); }
active_file="${tmp}/active"
max_file="${tmp}/max"
lock_dir="${tmp}/lock"
echo 0 > "$active_file"
echo 0 > "$max_file"
worker() {
  while ! mkdir "$lock_dir" 2>/dev/null; do sleep 0.01; done
  local n=$(( $(cat "$active_file") + 1 ))
  echo "$n" > "$active_file"
  if (( n > $(cat "$max_file") )); then
    echo "$n" > "$max_file"
  fi
  rmdir "$lock_dir"
  sleep 0.3
  while ! mkdir "$lock_dir" 2>/dev/null; do sleep 0.01; done
  n=$(( $(cat "$active_file") - 1 ))
  echo "$n" > "$active_file"
  rmdir "$lock_dir"
}
for _ in 1 2 3 4 5 6; do
  perf_bg 2 worker
done
perf_wait_all
max_seen=$(cat "$max_file")
if [[ "$max_seen" -le 2 && "$max_seen" -ge 1 ]]; then
  pass_u "bounded concurrency never exceeded 2 (saw ${max_seen})"
else
  fail_u "bounded concurrency max=${max_seen}, expected <=2"
fi
if [[ "$PROGRESS_TICKS" -gt 0 ]]; then
  pass_u "bounded concurrency invokes progress heartbeats"
else
  fail_u "bounded concurrency should invoke progress heartbeats while waiting"
fi
unset -f perf_progress_tick

# --- Interrupted worker cleanup ---

PERF_BG_PIDS=()
perf_bg 2 sleep 30
cancel_pid="${PERF_BG_PIDS[0]}"
perf_cancel_all
if ! kill -0 "$cancel_pid" 2>/dev/null && [[ ${#PERF_BG_PIDS[@]} -eq 0 ]]; then
  pass_u "worker cancellation stops and reaps active jobs"
else
  fail_u "worker cancellation left an active job or tracked PID"
fi

REAP_ARGS=""
fake_reap_cli() { REAP_ARGS="$*"; }
perf_reap_namespace fake_reap_cli openshell-test-run
if [[ "$REAP_ARGS" == "delete namespace openshell-test-run --ignore-not-found --wait=false" ]]; then
  pass_u "performance cleanup directly reaps tracked namespaces"
else
  fail_u "namespace reap command unexpected: ${REAP_ARGS}"
fi
unset -f fake_reap_cli

signal_cleanup="${tmp}/signal-cleanup"
signal_continued="${tmp}/signal-continued"
set +e
PERF_TEST_LIB="${SCRIPT_DIR}/../lib.sh" PERF_PERF_LIB="${SCRIPT_DIR}/lib.sh" \
  PERF_SIGNAL_CLEANUP="$signal_cleanup" PERF_SIGNAL_CONTINUED="$signal_continued" \
  bash -c '
    source "$PERF_TEST_LIB"
    source "$PERF_PERF_LIB"
    trap '\''printf cleanup > "$PERF_SIGNAL_CLEANUP"'\'' EXIT
    perf_install_signal_traps
    sleep 30 & sleeper=$!
    (sleep 0.1; kill -INT $$ "$sleeper" 2>/dev/null || true) &
    set +e
    wait "$sleeper"
    printf continued > "$PERF_SIGNAL_CONTINUED"
  ' >/dev/null 2>&1
signal_rc=$?
set -e
if [[ "$signal_rc" -eq 130 && -f "$signal_cleanup" && ! -f "$signal_continued" ]]; then
  pass_u "SIGINT exits through cleanup even while errexit is disabled"
else
  fail_u "SIGINT was swallowed (rc=${signal_rc}, cleanup=$([[ -f "$signal_cleanup" ]] && echo yes || echo no), continued=$([[ -f "$signal_continued" ]] && echo yes || echo no))"
fi

# --- Driver-not-set message (harness + suite share e2e_die_unknown_driver) ---

if e2e_list_available_drivers | grep -qx kind; then
  pass_u "kind driver is listed"
else
  fail_u "kind driver should be listed in tests/e2e/drivers"
fi

# --- Harness is infra-agnostic ---

hits=$(grep -nE '\b(kubectl|oc)\b' "${SCRIPT_DIR}/../e2e-performance.sh" | grep -v '#' || true)
if [[ -n "$hits" ]]; then
  fail_u "e2e-performance.sh contains kubectl/oc: ${hits}"
else
  pass_u "e2e-performance.sh has no kubectl/oc commands"
fi

# --- Performance layer is bash-only ---

py_hits=$(grep -n 'python3' \
  "${SCRIPT_DIR}/lib.sh" \
  "${SCRIPT_DIR}/../e2e-performance.sh" \
  "${SCRIPT_DIR}/../../../scripts/perf-report.sh" || true)
if [[ -n "$py_hits" ]]; then
  fail_u "performance layer still calls python3: ${py_hits}"
else
  pass_u "performance layer has no python3"
fi

rm -rf "$tmp"

echo ""
if [[ "$FAILS" -gt 0 ]]; then
  red "${FAILS} test(s) failed"
  exit 1
fi
green "All unit tests passed"
exit 0
