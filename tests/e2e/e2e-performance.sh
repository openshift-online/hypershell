#!/usr/bin/env bash
# e2e-performance.sh - infrastructure-agnostic performance harness.
#
# Provisions a fleet of gateways in batches, runs the e2e suite in short mode
# after each batch (checkpoint), then runs the suite in long mode as a
# functional gate. Reuses the e2e driver interface: no kubectl/oc/kind
# commands appear in this file.
#
# The infrastructure driver is auto-detected from the current KUBECONFIG
# context, the same as e2e-openshell.sh. Set E2E_INFRA_DRIVER to override.
#
# Usage:
#   bash tests/e2e/e2e-performance.sh
#   make e2e-performance
#   OPENSHIFT_NAMESPACE=my-env E2E_INFRA_DRIVER=openshift \
#     make e2e-performance   # override detection
#
# See specs/platform/e2e-testing.spec.md "Performance Testing".
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
# shellcheck source=perf/lib.sh
source "${SCRIPT_DIR}/perf/lib.sh"

# --- Performance defaults ---

: "${E2E_PERF_GATEWAY_COUNT:=5}"
: "${E2E_PERF_BATCH_SIZE:=5}"
: "${E2E_PERF_CHECKPOINT:=1}"
: "${E2E_PERF_STOP_ON_CHECKPOINT_FAILURE:=1}"
: "${E2E_PERF_CONCURRENCY:=4}"
: "${E2E_PERF_PROGRESS_INTERVAL:=10}"
: "${E2E_PERF_GATEWAY_PREFIX:=perf-gw}"
: "${E2E_PERF_PROVISION_TIMEOUT:=180}"
: "${E2E_PERF_RUN_FUNCTIONAL:=1}"
: "${E2E_PERF_FUNCTIONAL_GATEWAY_NAME:=perf-e2e-gw}"
: "${E2E_PERF_RESULTS_DIR:=perf-results}"
: "${E2E_PERF_CSV:=0}"
# Optional SLO thresholds: unset means no gate.
: "${E2E_PERF_MIN_SUCCESS_RATE:=}"
: "${E2E_PERF_MAX_PROVISION_P99:=}"

# Checkpoints disabled: provision the full fleet in one pass, no mini tests.
if [[ "${E2E_PERF_CHECKPOINT}" != "1" ]]; then
  E2E_PERF_BATCH_SIZE="${E2E_PERF_GATEWAY_COUNT}"
fi

DB_PROVIDER="${DATABASE_PROVIDER:-deployment}"
E2E_HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

# --- Driver selection ---

e2e_select_infra_driver

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "$DRIVER_FILE" ]]; then
  e2e_die_unknown_driver "Unknown driver '${E2E_INFRA_DRIVER}'. Driver file not found: ${DRIVER_FILE}"
fi

# shellcheck source=drivers/kind.sh
source "$DRIVER_FILE"

REQUIRED_FUNCTIONS=(
  discover_api_host discover_gateway_endpoint get_cluster_domain get_cli_binary
  wait_for_gateway_route acquire_oidc_token api_curl acquire_gateway_token_with_role
)
for fn in "${REQUIRED_FUNCTIONS[@]}"; do
  if ! declare -f "$fn" >/dev/null 2>&1; then
    red "ERROR: Driver '${E2E_INFRA_DRIVER}' does not implement required function: ${fn}"
    exit 1
  fi
done

CLI=$(get_cli_binary)

# --- Run state ---

PERF_STARTED_AT="$(e2e_utc_now)"
PERF_STAMP="$(e2e_utc_stamp)"
PERF_RESULTS_DIR="${E2E_PERF_RESULTS_DIR}"
if [[ "${PERF_RESULTS_DIR}" != /* ]]; then
  PERF_RESULTS_DIR="${REPO_ROOT}/${PERF_RESULTS_DIR}"
fi
mkdir -p "${PERF_RESULTS_DIR}"
PERF_RESULTS_FILE="${PERF_RESULTS_DIR}/${E2E_INFRA_DRIVER}-${PERF_STAMP}.json"

PERF_RUN_RESULT="pass"
PERF_STOPPED_EARLY=false
PERF_BREAKING_SCALE=""
PERF_FUNCTIONAL_RAN=false
PERF_FUNCTIONAL_PASSED=""
PERF_SCALE_START_MS=""
PERF_SCALE_WALL_S=""

# Per-gateway records: tab-separated status/create_s/running_s/id/namespace/name
PERF_RECORD_DIR=$(mktemp -d)

CANARY_NAME="${E2E_PERF_GATEWAY_PREFIX}-canary"
CANARY_ID=""
CANARY_NS=""

# Track every gateway this run created or reused so teardown can delete them.
PERF_TRACKED_IDS=()
PERF_TRACKED_NS=()
PERF_TRACKED_NAMES=()

perf_track() {
  local id="${1:-}" ns="${2:-}" name="${3:-}"
  [[ -z "$id" ]] && return
  PERF_TRACKED_IDS+=("$id")
  PERF_TRACKED_NS+=("$ns")
  PERF_TRACKED_NAMES+=("$name")
}

# Export worker-needed env so bash -c children inherit it. Do not splice
# secrets into the command string (they would appear in argv / ps).
perf_export_child_env() {
  export API_HOST="${API_HOST:-}"
  export E2E_INFRA_DRIVER="${E2E_INFRA_DRIVER:-}"
  export E2E_OIDC_ISSUER="${E2E_OIDC_ISSUER:-}"
  export E2E_OIDC_CLIENT_ID="${E2E_OIDC_CLIENT_ID:-}"
  export E2E_OIDC_USERNAME="${E2E_OIDC_USERNAME:-}"
  export E2E_OIDC_PASSWORD="${E2E_OIDC_PASSWORD:-}"
  export E2E_CLUSTER_ID="${E2E_CLUSTER_ID:-}"
  export E2E_RELEASE_ID="${E2E_RELEASE_ID:-}"
  export E2E_DATABASE_ID="${E2E_DATABASE_ID:-}"
  export E2E_PERF_PROVISION_TIMEOUT="${E2E_PERF_PROVISION_TIMEOUT:-}"
  export E2E_HS_NAMESPACE="${E2E_HS_NAMESPACE:-}"
  if [[ -n "${OPENSHIFT_NAMESPACE:-}" ]]; then
    export OPENSHIFT_NAMESPACE
  fi
}

# --- Cleanup trap ---

perf_cleanup() {
  local exit_code=$?
  trap - EXIT
  # Do not let a second terminal signal interrupt cleanup halfway through and
  # strand live Gateway records. Worker cancellation happens before temp state
  # is removed so no process can recreate a record beneath us.
  trap '' INT TERM
  perf_cancel_all

  # Harvest namespace ownership before removing worker state. Completed batch
  # records, in-flight state, and the parent tracker cover every point after a
  # Gateway create response has supplied its namespace.
  local namespaces=() namespace_names=()
  local i f state_name state_stage state_id state_ns
  for ((i = 0; i < ${#PERF_TRACKED_NS[@]}; i++)); do
    [[ -z "${PERF_TRACKED_NS[$i]}" ]] && continue
    namespaces+=("${PERF_TRACKED_NS[$i]}")
    namespace_names+=("${PERF_TRACKED_NAMES[$i]}")
  done
  for f in "${PERF_RECORD_DIR}"/*.state; do
    [[ -f "$f" ]] || continue
    IFS=$'\t' read -r state_name state_stage state_id state_ns < "$f" || true
    [[ -z "${state_ns:-}" ]] && continue
    namespaces+=("$state_ns")
    namespace_names+=("$state_name")
  done
  local record_status record_create record_running record_id record_ns record_name
  for f in "${PERF_RECORD_DIR}"/*; do
    [[ -f "$f" && "$f" != *.state ]] || continue
    IFS=$'\t' read -r record_status record_create record_running record_id record_ns record_name < "$f" || true
    [[ -z "${record_ns:-}" ]] && continue
    namespaces+=("$record_ns")
    namespace_names+=("$record_name")
  done
  rm -rf "${PERF_RECORD_DIR}"
  if [[ "${E2E_SKIP_CLEANUP}" == "1" ]]; then
    dim "  E2E_SKIP_CLEANUP=1: keeping perf fleet for inspection"
    exit "$exit_code"
  fi
  echo ""
  bold "Teardown"
  sep
  dim "  Deleting performance gateways through the HyperShell API..."
  perf_export_child_env
  local name
  local to_delete=()
  for ((i = 1; i <= E2E_PERF_GATEWAY_COUNT; i++)); do
    to_delete+=("${E2E_PERF_GATEWAY_PREFIX}-${i}")
  done
  to_delete+=("${CANARY_NAME}" "${E2E_PERF_FUNCTIONAL_GATEWAY_NAME}")

  local del_dir
  del_dir=$(mktemp -d)
  PERF_BG_PIDS=()
  for name in "${to_delete[@]}"; do
    perf_bg "${E2E_PERF_CONCURRENCY}" bash -c '
      set -euo pipefail
      source "'"${SCRIPT_DIR}"'/lib.sh"
      source "'"${SCRIPT_DIR}"'/perf/lib.sh"
      source "'"${DRIVER_FILE}"'"
      perf_delete_gateway_by_name "$1" "$2"
    ' _ "$name" "${del_dir}/${name}"
  done
  perf_wait_all

  local delete_errors=0 deleted=0
  local result id ns detail
  for f in "${del_dir}"/*; do
    [[ -f "$f" ]] || continue
    while IFS=$'\t' read -r result id ns name detail; do
      case "$result" in
        deleted)
          deleted=$((deleted + 1))
          namespaces+=("$ns")
          namespace_names+=("$name")
          dim "    delete accepted: ${name} (${id})"
          ;;
        error)
          delete_errors=$((delete_errors + 1))
          red "    delete failed: ${name} (${detail})"
          if [[ -n "${ns:-}" ]]; then
            namespaces+=("$ns")
            namespace_names+=("$name")
          fi
          ;;
      esac
    done < "$f"
  done

  if ((delete_errors > 0)); then
    red "  ${delete_errors} gateway deletion(s) failed; direct namespace reap will proceed, but live API records may recreate them"
  fi

  local unique_namespaces=() unique_namespace_names=() candidate existing seen
  for ((i = 0; i < ${#namespaces[@]}; i++)); do
    candidate="${namespaces[$i]}"
    [[ -z "$candidate" ]] && continue
    seen=false
    for existing in "${unique_namespaces[@]+"${unique_namespaces[@]}"}"; do
      if [[ "$existing" == "$candidate" ]]; then
        seen=true
        break
      fi
    done
    if [[ "$seen" == "false" ]]; then
      unique_namespaces+=("$candidate")
      unique_namespace_names+=("${namespace_names[$i]}")
    fi
  done
  namespaces=("${unique_namespaces[@]+"${unique_namespaces[@]}"}")
  namespace_names=("${unique_namespace_names[@]+"${unique_namespace_names[@]}"}")

  local leftover=${#namespaces[@]}
  if ((leftover > 0)); then
    dim "  Reaping ${leftover} performance namespace(s) directly..."
    local reap_errors=0
    for ((i = 0; i < ${#namespaces[@]}; i++)); do
      if ! perf_reap_namespace "$CLI" "${namespaces[$i]}"; then
        reap_errors=$((reap_errors + 1))
        red "    namespace delete failed: ${namespace_names[$i]} (${namespaces[$i]})"
      fi
    done
    if ((reap_errors > 0)); then
      red "  ${reap_errors} direct namespace deletion(s) failed"
    fi
    dim "  Waiting for namespace deletion concurrently (global timeout: ${E2E_GC_TIMEOUT}s)..."
    local deadline=$(($(date +%s) + E2E_GC_TIMEOUT))
    local last_report=0 now i
    while ((leftover > 0 && $(date +%s) < deadline)); do
      leftover=0
      for ((i = 0; i < ${#namespaces[@]}; i++)); do
        [[ -z "${namespaces[$i]}" ]] && continue
        if "$CLI" get namespace "${namespaces[$i]}" &>/dev/null; then
          leftover=$((leftover + 1))
        else
          dim "    reaped: ${namespace_names[$i]} (${namespaces[$i]})"
          namespaces[$i]=""
        fi
      done
      now=$(date +%s)
      if ((leftover > 0 && now - last_report >= 10)); then
        dim "    cleanup progress: ${leftover} namespace(s) remaining, elapsed=$((E2E_GC_TIMEOUT - (deadline - now)))s"
        last_report=$now
      fi
      if ((leftover > 0)); then
        sleep 5
      fi
    done
  fi
  rm -rf "$del_dir"
  if ((leftover > 0)); then
    red "  ${leftover} performance namespace(s) not deleted within the global ${E2E_GC_TIMEOUT}s timeout"
  fi
  exit "$exit_code"
}
trap perf_cleanup EXIT
perf_install_signal_traps

# --- Helpers ---

perf_collect_batch_metrics() {
  local dir="${1:?batch dir required}"
  PERF_BATCH_OK=0
  PERF_BATCH_FAIL=0
  PERF_BATCH_CREATE=()
  PERF_BATCH_RUNNING=()
  local f status create_s running_s id ns name
  for f in "${dir}"/*; do
    [[ -f "$f" && "$f" != *.state ]] || continue
    IFS=$'\t' read -r status create_s running_s id ns name <<< "$(cat "$f")"
    if [[ "$status" == "ok" ]]; then
      PERF_BATCH_OK=$((PERF_BATCH_OK + 1))
    else
      PERF_BATCH_FAIL=$((PERF_BATCH_FAIL + 1))
    fi
    [[ "$create_s" != "null" && -n "$create_s" ]] && PERF_BATCH_CREATE+=("$create_s")
    [[ "$running_s" != "null" && -n "$running_s" ]] && PERF_BATCH_RUNNING+=("$running_s")
    [[ -n "$id" ]] && perf_track "$id" "$ns" "$name"
  done
}

# Called by the bounded worker pool while it is waiting for a free slot and
# while it drains the active workers. Record files appear atomically when a
# gateway finishes; state files expose the current stage before that happens.
perf_progress_tick() {
  [[ -z "${PERF_PROGRESS_DIR:-}" ]] && return 0
  local now elapsed force="${1:-0}"
  now=$(date +%s)
  if [[ "$force" != "1" && $((now - PERF_PROGRESS_LAST_S)) -lt E2E_PERF_PROGRESS_INTERVAL ]]; then
    return 0
  fi
  PERF_PROGRESS_LAST_S="$now"
  elapsed=$((now - PERF_PROGRESS_START_S))

  local complete=0 ok=0 failed=0 f status
  local authenticating=0 checking=0 creating=0 waiting=0
  for f in "${PERF_PROGRESS_DIR}"/*; do
    [[ -f "$f" && "$f" != *.state ]] || continue
    complete=$((complete + 1))
    IFS=$'\t' read -r status _ < "$f" || true
    if [[ "$status" == "ok" ]]; then ok=$((ok + 1)); else failed=$((failed + 1)); fi
  done
  local name stage
  for f in "${PERF_PROGRESS_DIR}"/*.state; do
    [[ -f "$f" && ! -f "${f%.state}" ]] || continue
    IFS=$'\t' read -r name stage < "$f" || true
    case "$stage" in
      authenticating) authenticating=$((authenticating + 1)) ;;
      checking) checking=$((checking + 1)) ;;
      creating) creating=$((creating + 1)) ;;
      waiting:*) waiting=$((waiting + 1)) ;;
    esac
  done
  dim "    progress: complete=${complete}/${PERF_PROGRESS_TOTAL} running=${ok} failed=${failed} waiting=${waiting} creating=${creating} checking=${checking} auth=${authenticating} elapsed=${elapsed}s"
}

perf_run_mini_test() {
  local start_ms
  start_ms=$(perf_now_ms)
  # Prefix the child env only - do not export into the harness, so the EXIT
  # trap still tears the canary and fleet down. Suite output goes to the
  # terminal; rc/duration are stored in PERF_MINI_RC / PERF_MINI_S.
  set +e
  E2E_GATEWAY_NAME="${CANARY_NAME}" \
    E2E_SKIP_CLEANUP=1 \
    E2E_MODE=short \
    E2E_PAUSE=0 \
    E2E_INFRA_DRIVER="${E2E_INFRA_DRIVER}" \
    E2E_CLUSTER_ID="${E2E_CLUSTER_ID}" \
    E2E_RELEASE_ID="${E2E_RELEASE_ID}" \
    E2E_DATABASE_ID="${E2E_DATABASE_ID:-}" \
    bash "${SCRIPT_DIR}/e2e-openshell.sh"
  PERF_MINI_RC=$?
  set -e
  PERF_MINI_S=$(perf_elapsed_s "$start_ms")
}

perf_collect_diagnostics() {
  perf_diag_begin "Performance failure diagnostics"
  echo ""
  bold "Pending pods"
  "$CLI" get pods -A --field-selector=status.phase=Pending -o wide 2>/dev/null \
    | while IFS= read -r line; do dim "  $line"; done || true
  echo ""
  bold "Node capacity"
  "$CLI" get nodes -o custom-columns=NAME:.metadata.name,CPU:.status.capacity.cpu,MEM:.status.capacity.memory,ALLOC_CPU:.status.allocatable.cpu,ALLOC_MEM:.status.allocatable.memory 2>/dev/null \
    | while IFS= read -r line; do dim "  $line"; done || true
  echo ""
  bold "Perf gateway phases"
  acquire_oidc_token 2>/dev/null || true
  local i name
  for ((i = 1; i <= E2E_PERF_GATEWAY_COUNT; i++)); do
    name="${E2E_PERF_GATEWAY_PREFIX}-${i}"
    e2e_lookup_gateway_by_name "$name"
    dim "  ${name}: id=${_GW_ID:-<none>} phase=${_GW_PHASE:-<none>} ns=${_GW_NAMESPACE:-<none>}"
  done
  e2e_lookup_gateway_by_name "$CANARY_NAME"
  dim "  ${CANARY_NAME}: id=${_GW_ID:-<none>} phase=${_GW_PHASE:-<none>}"
  echo ""
  bold "Control-plane logs"
  e2e_dump_namespace_gc_logs "${E2E_HS_NAMESPACE}" "$CLI"
  "$CLI" logs -l app=hypershell-controller -n "${E2E_HS_NAMESPACE}" --tail=80 2>/dev/null \
    | tail -40 | while IFS= read -r line; do dim "    $line"; done || true
  perf_diag_end
}

# --- Preflight ---

echo ""
bold "HyperShell Performance Test"
sep
echo ""

if ! discover_api_host; then
  red "ERROR: Could not discover HyperShell API host"
  exit 1
fi
API_HOST="${_DISCOVER_API_HOST}"

if ! acquire_oidc_token; then
  red "ERROR: Could not acquire OIDC token"
  exit 1
fi

if ! e2e_discover_seed_ids; then
  red "ERROR: Seeded cluster/release not found. Run make kind-up (or the OpenShift deploy) first."
  exit 1
fi
perf_export_child_env

if [[ "${DB_PROVIDER}" == "cnpg" && -z "${E2E_DATABASE_ID}" ]]; then
  red "ERROR: No ManagedDatabase found (required for DATABASE_PROVIDER=cnpg)"
  exit 1
fi

dim "  Driver:            ${E2E_INFRA_DRIVER}"
dim "  HyperShell API:    ${API_HOST}"
dim "  Gateway count:     ${E2E_PERF_GATEWAY_COUNT}"
dim "  Batch size:        ${E2E_PERF_BATCH_SIZE}"
dim "  Concurrency:       ${E2E_PERF_CONCURRENCY}"
dim "  Prefix:            ${E2E_PERF_GATEWAY_PREFIX}"
dim "  Checkpoint:        ${E2E_PERF_CHECKPOINT}"
dim "  Functional:        ${E2E_PERF_RUN_FUNCTIONAL}"
dim "  Results:           ${PERF_RESULTS_FILE}"
dim "  cluster_id:        ${E2E_CLUSTER_ID}"
dim "  release_id:        ${E2E_RELEASE_ID}"
[[ -n "${E2E_DATABASE_ID}" ]] && dim "  database_id:       ${E2E_DATABASE_ID}"
echo ""
sep

perf_results_init "${PERF_RESULTS_FILE}"

# --- Canary ---

if [[ "${E2E_PERF_CHECKPOINT}" == "1" ]]; then
  echo ""
  bold "Preflight: canary gateway ${CANARY_NAME}"
  echo ""
  local_record="${PERF_RECORD_DIR}/canary"
  perf_provision_one "$CANARY_NAME" "$local_record"
  IFS=$'\t' read -r status create_s running_s CANARY_ID CANARY_NS _ <<< "$(cat "$local_record")"
  if [[ "$status" != "ok" || -z "$CANARY_ID" ]]; then
    red "ERROR: Canary gateway ${CANARY_NAME} did not reach Running"
    PERF_RUN_RESULT="fail"
    PERF_RES_RESULT="fail"
    PERF_RES_FINISHED_AT="$(e2e_utc_now)"
    perf_results_write
    perf_collect_diagnostics
    exit 1
  fi
  pass "Canary running: ${CANARY_NAME} (${CANARY_ID})"
  perf_track "$CANARY_ID" "$CANARY_NS" "$CANARY_NAME"

  GW_KC_CLIENT_ID="${CANARY_NAME}-${CANARY_ID}"
  show_cmd "# grant canary OIDC role once (acquire_gateway_token_with_role)"
  if acquire_gateway_token_with_role "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" "$GW_KC_CLIENT_ID" openshell-admin; then
    pass "Canary OIDC role granted (openshell-admin on ${GW_KC_CLIENT_ID})"
  else
    fail_test "Failed to grant canary OIDC role; checkpoints may fail"
  fi
fi

# --- Scale-up ---

echo ""
bold "Scale-up"
echo ""

PERF_SCALE_START_MS=$(perf_now_ms)
TOTAL_OK=0
TOTAL_FAIL=0
ALL_CREATE=()
ALL_RUNNING=()

start_index=1
while (( start_index <= E2E_PERF_GATEWAY_COUNT )); do
  batch_end=$((start_index + E2E_PERF_BATCH_SIZE - 1))
  if (( batch_end > E2E_PERF_GATEWAY_COUNT )); then
    batch_end=$E2E_PERF_GATEWAY_COUNT
  fi
  batch_dir="${PERF_RECORD_DIR}/batch-${start_index}-${batch_end}"
  mkdir -p "$batch_dir"

  bold "  Batch ${start_index}–${batch_end}"
  PERF_PROGRESS_DIR="$batch_dir"
  PERF_PROGRESS_TOTAL=$((batch_end - start_index + 1))
  PERF_PROGRESS_START_S=$(date +%s)
  PERF_PROGRESS_LAST_S=$((PERF_PROGRESS_START_S - E2E_PERF_PROGRESS_INTERVAL))
  perf_progress_tick 1
  PERF_BG_PIDS=()
  local_i=""
  for ((local_i = start_index; local_i <= batch_end; local_i++)); do
    name="${E2E_PERF_GATEWAY_PREFIX}-${local_i}"
    rec="${batch_dir}/${local_i}"
    perf_bg "${E2E_PERF_CONCURRENCY}" bash -c '
      set -euo pipefail
      source "'"${SCRIPT_DIR}"'/lib.sh"
      source "'"${SCRIPT_DIR}"'/perf/lib.sh"
      source "'"${DRIVER_FILE}"'"
      perf_provision_one "$1" "$2"
    ' _ "$name" "$rec"
  done
  perf_wait_all
  perf_progress_tick 1
  PERF_PROGRESS_DIR=""

  perf_collect_batch_metrics "${batch_dir}"
  TOTAL_OK=$((TOTAL_OK + PERF_BATCH_OK))
  TOTAL_FAIL=$((TOTAL_FAIL + PERF_BATCH_FAIL))
  ALL_CREATE+=("${PERF_BATCH_CREATE[@]+"${PERF_BATCH_CREATE[@]}"}")
  ALL_RUNNING+=("${PERF_BATCH_RUNNING[@]+"${PERF_BATCH_RUNNING[@]}"}")

  dim "    provisioned=${PERF_BATCH_OK} failed=${PERF_BATCH_FAIL} (cumulative running=${TOTAL_OK})"

  if [[ "${E2E_PERF_CHECKPOINT}" == "1" ]]; then
    echo ""
    dim "  Checkpoint mini test (E2E_MODE=short, gateway=${CANARY_NAME})..."
    perf_run_mini_test
    if [[ "$PERF_MINI_RC" == "0" ]]; then
      mini_result="pass"
      pass "Checkpoint at ${TOTAL_OK} gateways passed (${PERF_MINI_S}s)"
    else
      mini_result="fail"
      fail_test "Checkpoint at ${TOTAL_OK} gateways failed (${PERF_MINI_S}s)"
    fi

    perf_percentiles "${PERF_BATCH_RUNNING[@]+"${PERF_BATCH_RUNNING[@]}"}"
    perf_results_add_checkpoint "${TOTAL_OK}" "$(e2e_utc_now)" "short" "$mini_result" "$PERF_MINI_S"

    if [[ "$mini_result" == "fail" ]]; then
      PERF_RUN_RESULT="fail"
      if [[ "${E2E_PERF_STOP_ON_CHECKPOINT_FAILURE}" == "1" ]]; then
        PERF_STOPPED_EARLY=true
        PERF_BREAKING_SCALE="${TOTAL_OK}"
        PERF_RES_PROVISIONED="${TOTAL_OK}"
        PERF_RES_FAILED="${TOTAL_FAIL}"
        PERF_RES_STOPPED_EARLY=true
        PERF_RES_BREAKING_SCALE="${TOTAL_OK}"
        perf_results_write
        perf_collect_diagnostics
        break
      fi
    fi
  fi

  start_index=$((batch_end + 1))
done

PERF_SCALE_WALL_S=$(perf_elapsed_s "$PERF_SCALE_START_MS")

# --- Scale-up metrics ---

REQUESTED="${E2E_PERF_GATEWAY_COUNT}"
if [[ "$PERF_STOPPED_EARLY" == "true" ]]; then
  REQUESTED="${PERF_BREAKING_SCALE}"
fi
PROVISIONED="${TOTAL_OK}"
FAILED="${TOTAL_FAIL}"
if [[ $((PROVISIONED + FAILED)) -gt 0 ]]; then
  SUCCESS_RATE=$(perf_pct_1 "$PROVISIONED" $((PROVISIONED + FAILED)))
else
  SUCCESS_RATE=0
fi
THROUGHPUT=$(perf_throughput "$PROVISIONED" "$PERF_SCALE_WALL_S")

CREATE_JSON=$(perf_percentiles_json "${ALL_CREATE[@]+"${ALL_CREATE[@]}"}")
TTR_JSON=$(perf_percentiles_json "${ALL_RUNNING[@]+"${ALL_RUNNING[@]}"}")
TTR_P99="$PERF_P99"

PERF_RES_REQUESTED="${E2E_PERF_GATEWAY_COUNT}"
PERF_RES_PROVISIONED="$PROVISIONED"
PERF_RES_FAILED="$FAILED"
PERF_RES_SUCCESS_RATE="$SUCCESS_RATE"
PERF_RES_WALL="$PERF_SCALE_WALL_S"
PERF_RES_THROUGHPUT="$THROUGHPUT"
PERF_RES_CREATE_JSON="$CREATE_JSON"
PERF_RES_TTR_JSON="$TTR_JSON"
PERF_RES_STOPPED_EARLY=$( [[ "$PERF_STOPPED_EARLY" == "true" ]] && echo true || echo false )
PERF_RES_BREAKING_SCALE=$(perf_num_or_null "${PERF_BREAKING_SCALE}")
perf_results_write

if [[ "$FAILED" -gt 0 ]]; then
  perf_collect_diagnostics
fi

# --- Functional ---

if [[ "${E2E_PERF_RUN_FUNCTIONAL}" == "1" && "$PERF_STOPPED_EARLY" != "true" ]]; then
  echo ""
  bold "Functional check (E2E_MODE=long, gateway=${E2E_PERF_FUNCTIONAL_GATEWAY_NAME})"
  echo ""
  PERF_FUNCTIONAL_RAN=true
  set +e
  E2E_GATEWAY_NAME="${E2E_PERF_FUNCTIONAL_GATEWAY_NAME}" \
    E2E_MODE=long \
    E2E_PAUSE=0 \
    E2E_INFRA_DRIVER="${E2E_INFRA_DRIVER}" \
    E2E_CLUSTER_ID="${E2E_CLUSTER_ID}" \
    E2E_RELEASE_ID="${E2E_RELEASE_ID}" \
    E2E_DATABASE_ID="${E2E_DATABASE_ID:-}" \
    bash "${SCRIPT_DIR}/e2e-openshell.sh"
  func_rc=$?
  set -e
  if [[ "$func_rc" == "0" ]]; then
    PERF_FUNCTIONAL_PASSED=true
    pass "Functional suite passed under load"
  else
    PERF_FUNCTIONAL_PASSED=false
    PERF_RUN_RESULT="fail"
    fail_test "Functional suite failed under load"
    perf_collect_diagnostics
  fi
  PERF_RES_FUNC_RAN=true
  PERF_RES_FUNC_PASSED=$( [[ "$PERF_FUNCTIONAL_PASSED" == "true" ]] && echo true || echo false )
  PERF_RES_FUNC_GW="${E2E_PERF_FUNCTIONAL_GATEWAY_NAME}"
  perf_results_write
else
  PERF_RES_FUNC_RAN=false
  PERF_RES_FUNC_PASSED="null"
  PERF_RES_FUNC_GW="null"
  perf_results_write
fi

# --- SLO ---

SLO_PASSED=true
if [[ -n "${E2E_PERF_MIN_SUCCESS_RATE}" ]]; then
  if perf_ge "$SUCCESS_RATE" "$E2E_PERF_MIN_SUCCESS_RATE"; then
    :
  else
    SLO_PASSED=false
    PERF_RUN_RESULT="fail"
    fail_test "Success rate ${SUCCESS_RATE}% is below E2E_PERF_MIN_SUCCESS_RATE=${E2E_PERF_MIN_SUCCESS_RATE}"
  fi
fi
if [[ -n "${E2E_PERF_MAX_PROVISION_P99}" && -n "${TTR_P99}" && "${TTR_P99}" != "null" ]]; then
  if perf_le "$TTR_P99" "$E2E_PERF_MAX_PROVISION_P99"; then
    :
  else
    SLO_PASSED=false
    PERF_RUN_RESULT="fail"
    fail_test "p99 ${TTR_P99}s is above E2E_PERF_MAX_PROVISION_P99=${E2E_PERF_MAX_PROVISION_P99}"
  fi
fi

PERF_RES_SLO_PASSED=$( [[ "$SLO_PASSED" == "true" ]] && echo true || echo false )
PERF_RES_FINISHED_AT="$(e2e_utc_now)"
PERF_RES_RESULT="$PERF_RUN_RESULT"
perf_results_write

perf_results_point_latest "${PERF_RESULTS_FILE}"

if [[ "${E2E_PERF_CSV}" == "1" ]]; then
  perf_csv_append "${PERF_RESULTS_DIR}/history.csv" "${PERF_RESULTS_FILE}"
fi

# --- Report ---

echo ""
bold "Performance summary"
sep
perf_print_summary

# Reached the summary without a fatal abort; print_results (lib.sh) notes when
# this was never set, which does not apply to this script's own cleanup path.
E2E_COMPLETED=1
print_results

if [[ "${PERF_RUN_RESULT}" != "pass" ]]; then
  exit 1
fi
