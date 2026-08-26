#!/usr/bin/env bash
# Performance-test utilities. Sourced by e2e-performance.sh.
#
# Timing, nearest-rank percentiles, bounded concurrency, and schema_version=1
# JSON written from bash state. No python, no jq.

# --- Timing ---

# Milliseconds since epoch. Prefers bash 5 EPOCHREALTIME; falls back to
# whole seconds on bash 3.2 (macOS /bin/bash).
perf_now_ms() {
  if [[ "${EPOCHREALTIME:-}" == *.* ]]; then
    local sec frac
    sec="${EPOCHREALTIME%%.*}"
    frac="${EPOCHREALTIME#*.}000"
    frac="${frac:0:3}"
    echo $((10#$sec * 1000 + 10#$frac))
    return
  fi
  echo $(( $(date +%s) * 1000 ))
}

# Seconds elapsed since a perf_now_ms timestamp (3 decimal places).
perf_elapsed_s() {
  local start_ms="${1:?start_ms required}"
  local elapsed=$(( $(perf_now_ms) - start_ms ))
  if (( elapsed < 0 )); then elapsed=0; fi
  perf_ms_to_s "$elapsed"
}

# --- Decimal helpers (integer thousandths; no bc/python) ---

# "12.3" -> 12300 (milli-units). Empty/null -> empty.
perf_to_milli() {
  local n="${1:-}" w f
  [[ -z "$n" || "$n" == "null" ]] && { echo ""; return; }
  [[ "$n" == .* ]] && n="0${n}"
  w="${n%%.*}"
  [[ -z "$w" ]] && w=0
  if [[ "$n" == *.* ]]; then
    f="${n#*.}000"
    f="${f:0:3}"
  else
    f="000"
  fi
  echo $((10#$w * 1000 + 10#$f))
}

perf_ms_to_s() {
  local ms="${1:-0}"
  local w=$((ms / 1000)) f=$((ms % 1000))
  if (( f == 0 )); then
    echo "$w"
    return
  fi
  local out
  out=$(printf '%d.%03d' "$w" "$f")
  out="${out%"${out##*[!0]}"}"
  echo "${out%.}"
}

perf_num_or_null() {
  [[ -z "${1:-}" || "$1" == "null" ]] && echo null || echo "$1"
}

perf_ge() { (( $(perf_to_milli "$1") >= $(perf_to_milli "$2") )); }
perf_le() { (( $(perf_to_milli "$1") <= $(perf_to_milli "$2") )); }

# 1 decimal percent: 100 * num / den
perf_pct_1() {
  local num="${1:-0}" den="${2:-0}"
  if (( den == 0 )); then echo 0; return; fi
  local t=$((num * 1000 / den))
  printf '%d.%d' $((t / 10)) $((t % 10))
}

# Gateways per minute, 2 decimals. wall_s is a seconds string.
perf_throughput() {
  local provisioned="${1:-0}" wall_s="${2:-0}"
  local wall_ms
  wall_ms=$(perf_to_milli "$wall_s")
  if [[ -z "$wall_ms" ]] || (( wall_ms == 0 )); then echo 0; return; fi
  # per_min * 100 = provisioned * 60 * 100 * 1000 / wall_ms
  local x=$((provisioned * 6000000 / wall_ms))
  printf '%d.%02d' $((x / 100)) $((x % 100))
}

# --- Percentiles (nearest-rank, NIST: rank = ceil(p/100 * n)) ---
#
# Sets PERF_P50 PERF_P90 PERF_P99 PERF_MAX (each a number or "null").
# Usage: perf_percentiles [n...]

perf_percentiles() {
  PERF_P50=null PERF_P90=null PERF_P99=null PERF_MAX=null
  (( $# == 0 )) && return 0
  local ms_list=() v ms
  for v in "$@"; do
    ms=$(perf_to_milli "$v")
    [[ -n "$ms" ]] && ms_list+=("$ms")
  done
  local n=${#ms_list[@]}
  (( n == 0 )) && return 0
  local sorted=()
  local IFS=$'\n'
  # shellcheck disable=SC2207
  sorted=($(printf '%s\n' "${ms_list[@]}" | sort -n))
  unset IFS

  _perf_rank() {
    local p="$1" n="$2"
    local rank=$(( (p * n + 99) / 100 ))
    (( rank < 1 )) && rank=1
    (( rank > n )) && rank=n
    echo "$rank"
  }

  PERF_P50=$(perf_ms_to_s "${sorted[$(( $(_perf_rank 50 "$n") - 1 ))]}")
  PERF_P90=$(perf_ms_to_s "${sorted[$(( $(_perf_rank 90 "$n") - 1 ))]}")
  PERF_P99=$(perf_ms_to_s "${sorted[$(( $(_perf_rank 99 "$n") - 1 ))]}")
  PERF_MAX=$(perf_ms_to_s "${sorted[$((n - 1))]}")
}

perf_percentiles_json() {
  perf_percentiles "$@"
  printf '{"p50": %s, "p90": %s, "p99": %s, "max": %s}' \
    "$(perf_num_or_null "$PERF_P50")" \
    "$(perf_num_or_null "$PERF_P90")" \
    "$(perf_num_or_null "$PERF_P99")" \
    "$(perf_num_or_null "$PERF_MAX")"
}

# --- Bounded concurrency ---
#
# PID-array pool that does not rely on `wait -n` or `$(jobs -p)` (both are
# unportable / subshell-broken). Callers must run these in the same shell.

PERF_BG_PIDS=()

perf_interrupt() {
  local signal="${1:?signal required}" code="${2:?exit code required}"
  echo ""
  dim "  Received ${signal}; stopping the run and starting teardown..."
  exit "$code"
}

perf_install_signal_traps() {
  trap 'perf_interrupt INT 130' INT
  trap 'perf_interrupt TERM 143' TERM
}

perf_wait_for_slot() {
  local limit="${1:?concurrency required}"
  while true; do
    local alive=0 pid
    local new_pids=()
    for pid in "${PERF_BG_PIDS[@]+"${PERF_BG_PIDS[@]}"}"; do
      if kill -0 "$pid" 2>/dev/null; then
        new_pids+=("$pid")
        alive=$((alive + 1))
      else
        wait "$pid" 2>/dev/null || true
      fi
    done
    PERF_BG_PIDS=("${new_pids[@]+"${new_pids[@]}"}")
    if (( alive < limit )); then
      return 0
    fi
    if declare -f perf_progress_tick >/dev/null 2>&1; then
      perf_progress_tick
    fi
    sleep 0.2
  done
}

perf_bg() {
  local limit="${1:?concurrency required}"
  shift
  perf_wait_for_slot "$limit"
  "$@" &
  PERF_BG_PIDS+=("$!")
}

perf_wait_all() {
  while ((${#PERF_BG_PIDS[@]} > 0)); do
    local pid alive=0
    local new_pids=()
    for pid in "${PERF_BG_PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        new_pids+=("$pid")
        alive=$((alive + 1))
      else
        wait "$pid" 2>/dev/null || true
      fi
    done
    PERF_BG_PIDS=("${new_pids[@]+"${new_pids[@]}"}")
    if declare -f perf_progress_tick >/dev/null 2>&1; then
      perf_progress_tick
    fi
    if ((alive > 0)); then
      sleep 0.2
    fi
  done
}

# Stop active provisioning/deletion workers before teardown starts. This is
# idempotent and intentionally best-effort: EXIT cleanup must continue even if
# a worker has already exited between the liveness check and kill.
perf_cancel_all() {
  local pid
  for pid in "${PERF_BG_PIDS[@]+"${PERF_BG_PIDS[@]}"}"; do
    kill -TERM "$pid" 2>/dev/null || true
  done
  for pid in "${PERF_BG_PIDS[@]+"${PERF_BG_PIDS[@]}"}"; do
    wait "$pid" 2>/dev/null || true
  done
  PERF_BG_PIDS=()
}

# Reap a namespace owned by the performance run without waiting for Kubernetes
# finalizers. The caller performs a bounded, concurrent completion wait.
perf_reap_namespace() {
  local cli="${1:?CLI required}" namespace="${2:?namespace required}"
  "$cli" delete namespace "$namespace" --ignore-not-found --wait=false >/dev/null
}

# --- Results JSON (bash state -> file; never parse-merge) ---

PERF_RES_PATH=""
PERF_RES_DRIVER=""
PERF_RES_STARTED_AT=""
PERF_RES_FINISHED_AT="null"
PERF_RES_GATEWAY_COUNT=0
PERF_RES_BATCH_SIZE=0
PERF_RES_CONCURRENCY=0
PERF_RES_PROVISION_TIMEOUT=0
PERF_RES_GATEWAY_PREFIX=""
PERF_RES_CHECKPOINT=true
PERF_RES_STOP_ON_FAIL=true
PERF_RES_RUN_FUNCTIONAL=true
PERF_RES_MIN_SUCCESS_RATE="null"
PERF_RES_MAX_PROVISION_P99="null"
PERF_RES_REQUESTED=0
PERF_RES_PROVISIONED=0
PERF_RES_FAILED=0
PERF_RES_SUCCESS_RATE="null"
PERF_RES_WALL="null"
PERF_RES_THROUGHPUT="null"
PERF_RES_CREATE_JSON='{"p50": null, "p90": null, "p99": null, "max": null}'
PERF_RES_TTR_JSON='{"p50": null, "p90": null, "p99": null, "max": null}'
PERF_RES_STOPPED_EARLY=false
PERF_RES_BREAKING_SCALE="null"
PERF_RES_CHECKPOINTS=()
PERF_RES_FUNC_RAN=false
PERF_RES_FUNC_PASSED="null"
PERF_RES_FUNC_GW="null"
PERF_RES_SLO_PASSED=true
PERF_RES_RESULT="null"

perf_json_str() {
  local s="${1:-}"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '"%s"' "$s"
}

perf_json_str_or_null() {
  [[ -z "${1:-}" || "$1" == "null" ]] && echo null || perf_json_str "$1"
}

perf_json_bool() {
  case "${1:-}" in
    1|true|True) echo true ;;
    *) echo false ;;
  esac
}

perf_results_write() {
  local path="${PERF_RES_PATH:?results path not set}"
  local dir
  dir="$(dirname "$path")"
  mkdir -p "$dir"

  local cps="" i n
  n=${#PERF_RES_CHECKPOINTS[@]}
  if (( n == 0 )); then
    cps='[]'
  else
    cps=$'[\n'
    for ((i = 0; i < n; i++)); do
      if (( i < n - 1 )); then
        cps+="    ${PERF_RES_CHECKPOINTS[i]},"$'\n'
      else
        cps+="    ${PERF_RES_CHECKPOINTS[i]}"$'\n'
      fi
    done
    cps+='  ]'
  fi

  local func_gw
  func_gw=$(perf_json_str_or_null "${PERF_RES_FUNC_GW}")

  cat > "$path" <<EOF
{
  "schema_version": "1",
  "driver": $(perf_json_str "$PERF_RES_DRIVER"),
  "started_at": $(perf_json_str "$PERF_RES_STARTED_AT"),
  "finished_at": $(perf_json_str_or_null "$PERF_RES_FINISHED_AT"),
  "config": {
    "gateway_count": ${PERF_RES_GATEWAY_COUNT},
    "batch_size": ${PERF_RES_BATCH_SIZE},
    "concurrency": ${PERF_RES_CONCURRENCY},
    "provision_timeout": ${PERF_RES_PROVISION_TIMEOUT},
    "gateway_prefix": $(perf_json_str "$PERF_RES_GATEWAY_PREFIX"),
    "checkpoint": ${PERF_RES_CHECKPOINT},
    "stop_on_checkpoint_failure": ${PERF_RES_STOP_ON_FAIL},
    "run_functional": ${PERF_RES_RUN_FUNCTIONAL},
    "min_success_rate": ${PERF_RES_MIN_SUCCESS_RATE},
    "max_provision_p99": ${PERF_RES_MAX_PROVISION_P99}
  },
  "scale_up": {
    "requested": ${PERF_RES_REQUESTED},
    "provisioned": ${PERF_RES_PROVISIONED},
    "failed": ${PERF_RES_FAILED},
    "success_rate": ${PERF_RES_SUCCESS_RATE},
    "wall_clock_seconds": ${PERF_RES_WALL},
    "throughput_per_min": ${PERF_RES_THROUGHPUT},
    "create_latency_seconds": ${PERF_RES_CREATE_JSON},
    "time_to_running_seconds": ${PERF_RES_TTR_JSON},
    "stopped_early": ${PERF_RES_STOPPED_EARLY},
    "breaking_scale": ${PERF_RES_BREAKING_SCALE}
  },
  "checkpoints": ${cps},
  "functional": {
    "ran": ${PERF_RES_FUNC_RAN},
    "passed": ${PERF_RES_FUNC_PASSED},
    "gateway_name": ${func_gw}
  },
  "slo": {
    "min_success_rate": ${PERF_RES_MIN_SUCCESS_RATE},
    "max_provision_p99": ${PERF_RES_MAX_PROVISION_P99},
    "passed": ${PERF_RES_SLO_PASSED}
  },
  "result": $(perf_json_str_or_null "$PERF_RES_RESULT")
}
EOF
}

perf_results_init() {
  PERF_RES_PATH="${1:?results path required}"
  PERF_RES_DRIVER="${E2E_INFRA_DRIVER}"
  PERF_RES_STARTED_AT="${PERF_STARTED_AT}"
  PERF_RES_FINISHED_AT="null"
  PERF_RES_GATEWAY_COUNT="${E2E_PERF_GATEWAY_COUNT}"
  PERF_RES_BATCH_SIZE="${E2E_PERF_BATCH_SIZE}"
  PERF_RES_CONCURRENCY="${E2E_PERF_CONCURRENCY}"
  PERF_RES_PROVISION_TIMEOUT="${E2E_PERF_PROVISION_TIMEOUT}"
  PERF_RES_GATEWAY_PREFIX="${E2E_PERF_GATEWAY_PREFIX}"
  PERF_RES_CHECKPOINT=$(perf_json_bool "${E2E_PERF_CHECKPOINT}")
  PERF_RES_STOP_ON_FAIL=$(perf_json_bool "${E2E_PERF_STOP_ON_CHECKPOINT_FAILURE}")
  PERF_RES_RUN_FUNCTIONAL=$(perf_json_bool "${E2E_PERF_RUN_FUNCTIONAL}")
  PERF_RES_MIN_SUCCESS_RATE=$(perf_num_or_null "${E2E_PERF_MIN_SUCCESS_RATE:-}")
  PERF_RES_MAX_PROVISION_P99=$(perf_num_or_null "${E2E_PERF_MAX_PROVISION_P99:-}")
  PERF_RES_REQUESTED="${E2E_PERF_GATEWAY_COUNT}"
  PERF_RES_PROVISIONED=0
  PERF_RES_FAILED=0
  PERF_RES_SUCCESS_RATE="null"
  PERF_RES_WALL="null"
  PERF_RES_THROUGHPUT="null"
  PERF_RES_CREATE_JSON='{"p50": null, "p90": null, "p99": null, "max": null}'
  PERF_RES_TTR_JSON='{"p50": null, "p90": null, "p99": null, "max": null}'
  PERF_RES_STOPPED_EARLY=false
  PERF_RES_BREAKING_SCALE="null"
  PERF_RES_CHECKPOINTS=()
  PERF_RES_FUNC_RAN=false
  PERF_RES_FUNC_PASSED="null"
  PERF_RES_FUNC_GW="null"
  PERF_RES_SLO_PASSED=true
  PERF_RES_RESULT="null"
  perf_results_write
}

perf_results_add_checkpoint() {
  local count="$1" at="$2" mode="$3" mini="$4" mini_s="$5"
  local p50="${PERF_P50}" p90="${PERF_P90}" p99="${PERF_P99}" mx="${PERF_MAX}"
  local obj
  obj=$(printf '{"gateways_running": %s, "at": %s, "batch_time_to_running_seconds": {"p50": %s, "p90": %s, "p99": %s, "max": %s}, "mode": %s, "mini_test": %s, "mini_test_seconds": %s}' \
    "$count" \
    "$(perf_json_str "$at")" \
    "$(perf_num_or_null "$p50")" \
    "$(perf_num_or_null "$p90")" \
    "$(perf_num_or_null "$p99")" \
    "$(perf_num_or_null "$mx")" \
    "$(perf_json_str "$mode")" \
    "$(perf_json_str "$mini")" \
    "$(perf_num_or_null "$mini_s")")
  PERF_RES_CHECKPOINTS+=("$obj")
  perf_results_write
}

perf_results_point_latest() {
  local path="${1:-$PERF_RES_PATH}"
  local dir
  dir="$(dirname "$path")"
  cp "$path" "${dir}/latest.json"
}

perf_csv_append() {
  local csv_path="${1:?csv path required}"
  local new=0
  [[ -f "$csv_path" ]] || new=1
  mkdir -p "$(dirname "$csv_path")"
  if [[ "$new" == "1" ]]; then
    printf '%s\n' 'started_at,driver,gateway_count,success_rate,p99,throughput_per_min,result' > "$csv_path"
  fi
  local p99=""
  [[ "$PERF_RES_TTR_JSON" == *'"p99": '* ]] && p99="${PERF_RES_TTR_JSON#*\"p99\": }"
  p99="${p99%%,*}"
  p99="${p99%%\}*}"
  p99="${p99# }"
  [[ "$p99" == "null" ]] && p99=""
  printf '%s,%s,%s,%s,%s,%s,%s\n' \
    "$PERF_RES_STARTED_AT" \
    "$PERF_RES_DRIVER" \
    "$PERF_RES_GATEWAY_COUNT" \
    "${PERF_RES_SUCCESS_RATE}" \
    "$p99" \
    "${PERF_RES_THROUGHPUT}" \
    "${PERF_RES_RESULT}" >> "$csv_path"
}

# --- Read our pretty-printed schema_version=1 JSON (report / tests) ---

perf_json_unquote() {
  local v="$1"
  v="${v#"${v%%[![:space:]]*}"}"
  v="${v%"${v##*[![:space:]]}"}"
  v="${v%,}"
  v="${v%"${v##*[![:space:]]}"}"
  [[ "$v" == "null" ]] && { echo ""; return; }
  if [[ "$v" == \"* ]]; then
    v="${v#\"}"
    v="${v%\"}"
  fi
  echo "$v"
}

# First occurrence of "key": value in a file.
perf_json_first() {
  local file="$1" key="$2" line
  [[ -f "$file" ]] || { echo ""; return; }
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == *"\"${key}\":"* ]]; then
      perf_json_unquote "${line#*:}"
      return 0
    fi
  done < "$file"
  echo ""
}

# Value of key inside the first JSON object named block.
perf_json_in() {
  local file="$1" block="$2" key="$3"
  local seen=0 depth=0 line
  [[ -f "$file" ]] || { echo ""; return; }
  while IFS= read -r line || [[ -n "$line" ]]; do
    if (( seen == 0 )); then
      if [[ "$line" == *"\"${block}\":"* ]]; then
        seen=1
        if [[ "$line" == *"{"* ]]; then depth=1; fi
      fi
      continue
    fi
    if [[ "$line" == *"\"${key}\":"* ]]; then
      perf_json_unquote "${line#*:}"
      return 0
    fi
    [[ "$line" == *"{"* ]] && depth=$((depth + 1))
    if [[ "$line" == *"}"* ]]; then
      depth=$((depth - 1))
      (( depth <= 0 )) && { echo ""; return 0; }
    fi
  done < "$file"
  echo ""
}

# Value of inner_key from the first line that contains "object_key": {...}.
# Used for compact objects such as time_to_running_seconds.
perf_json_object_field() {
  local file="$1" obj="$2" key="$3" line rest
  [[ -f "$file" ]] || { echo ""; return; }
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == *"\"${obj}\":"* ]]; then
      rest="${line#*\"${key}\":}"
      perf_json_unquote "${rest%%,*}"
      return 0
    fi
  done < "$file"
  echo ""
}

# Print checkpoint rows: count<TAB>p99<TAB>mini_s<TAB>result
perf_json_checkpoints() {
  local file="$1" line
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == *"\"gateways_running\":"* ]] || continue
    local count p99 mini result rest
    count=$(perf_json_unquote "${line#*\"gateways_running\":}")
    count="${count%%,*}"
    count=$(perf_json_unquote "$count")
    rest="${line#*\"p99\":}"
    p99=$(perf_json_unquote "${rest%%,*}")
    rest="${line#*\"mini_test_seconds\":}"
    mini=$(perf_json_unquote "${rest%%,*}")
    rest="${line#*\"mini_test\":}"
    result=$(perf_json_unquote "${rest%%,*}")
    printf '%s\t%s\t%s\t%s\n' "$count" "$p99" "$mini" "$result"
  done < "$file"
}

perf_print_summary() {
  local dash='-'
  _n() { [[ -z "$1" || "$1" == "null" ]] && echo "$dash" || echo "$1"; }
  local cl_p50 cl_p90 cl_p99 cl_max ttr_p50 ttr_p90 ttr_p99 ttr_max
  cl_p50=$(perf_json_unquote "${PERF_RES_CREATE_JSON#*\"p50\": }"); cl_p50="${cl_p50%%,*}"
  cl_p90=$(perf_json_unquote "${PERF_RES_CREATE_JSON#*\"p90\": }"); cl_p90="${cl_p90%%,*}"
  cl_p99=$(perf_json_unquote "${PERF_RES_CREATE_JSON#*\"p99\": }"); cl_p99="${cl_p99%%,*}"
  cl_max=$(perf_json_unquote "${PERF_RES_CREATE_JSON#*\"max\": }"); cl_max="${cl_max%%\}*}"
  ttr_p50=$(perf_json_unquote "${PERF_RES_TTR_JSON#*\"p50\": }"); ttr_p50="${ttr_p50%%,*}"
  ttr_p90=$(perf_json_unquote "${PERF_RES_TTR_JSON#*\"p90\": }"); ttr_p90="${ttr_p90%%,*}"
  ttr_p99=$(perf_json_unquote "${PERF_RES_TTR_JSON#*\"p99\": }"); ttr_p99="${ttr_p99%%,*}"
  ttr_max=$(perf_json_unquote "${PERF_RES_TTR_JSON#*\"max\": }"); ttr_max="${ttr_max%%\}*}"

  printf '  %-22s  %s\n' driver "$PERF_RES_DRIVER"
  printf '  %-22s  %s\n' result "$(_n "$PERF_RES_RESULT")"
  printf '  %-22s  %s\n' requested "$PERF_RES_REQUESTED"
  printf '  %-22s  %s\n' provisioned "$PERF_RES_PROVISIONED"
  printf '  %-22s  %s\n' failed "$PERF_RES_FAILED"
  printf '  %-22s  %s\n' 'success rate %' "$(_n "$PERF_RES_SUCCESS_RATE")"
  printf '  %-22s  %s\n' 'wall clock s' "$(_n "$PERF_RES_WALL")"
  printf '  %-22s  %s\n' 'throughput /min' "$(_n "$PERF_RES_THROUGHPUT")"
  printf '  %-22s  %s\n' 'create p50/p90/p99/max' "$(_n "$cl_p50") / $(_n "$cl_p90") / $(_n "$cl_p99") / $(_n "$cl_max")"
  printf '  %-22s  %s\n' 'ttr p50/p90/p99/max' "$(_n "$ttr_p50") / $(_n "$ttr_p90") / $(_n "$ttr_p99") / $(_n "$ttr_max")"
  printf '  %-22s  %s\n' 'stopped early' "$PERF_RES_STOPPED_EARLY"
  printf '  %-22s  %s\n' 'breaking scale' "$(_n "$PERF_RES_BREAKING_SCALE")"
  if (( ${#PERF_RES_CHECKPOINTS[@]} > 0 )); then
    echo ""
    echo "  checkpoints"
    printf '    %6s  %8s  %6s  %8s  %s\n' count 'batch p99' mode 'mini s' result
    local line count p99 mini mode result
    for line in "${PERF_RES_CHECKPOINTS[@]}"; do
      count=$(perf_json_unquote "${line#*\"gateways_running\":}")
      count="${count%%,*}"
      count=$(perf_json_unquote "$count")
      p99=$(perf_json_unquote "${line#*\"p99\": }"); p99="${p99%%,*}"
      mini=$(perf_json_unquote "${line#*\"mini_test_seconds\": }"); mini="${mini%%\}*}"
      mode=$(perf_json_unquote "${line#*\"mode\": }"); mode="${mode%%,*}"
      result=$(perf_json_unquote "${line#*\"mini_test\": }"); result="${result%%,*}"
      printf '    %6s  %8s  %6s  %8s  %s\n' "$count" "$(_n "$p99")" "$mode" "$(_n "$mini")" "$result"
    done
  fi
  echo ""
  echo "  results file: ${PERF_RES_PATH}"
}

# --- Diagnostics grouping ---

perf_diag_begin() {
  local title="$1"
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::group::${title}"
  else
    echo ""
    bold "${title}"
  fi
}

perf_diag_end() {
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::endgroup::"
  fi
}

# --- Gateway provision / delete (shared with background workers) ---

# Provision one named gateway (reuse-or-create). Writes a tab-separated record:
# status  create_s  running_s  id  namespace  name
perf_provision_one() {
  local name="${1:?gateway name required}"
  local record="${2:?record path required}"
  local start_ms create_s running_s status="fail"
  local id="" ns="" phase=""
  start_ms=$(perf_now_ms)
  create_s="null"
  running_s="null"
  printf '%s\t%s\n' "$name" "authenticating" > "${record}.state"

  acquire_oidc_token 2>/dev/null || true
  printf '%s\t%s\n' "$name" "checking" > "${record}.state"
  e2e_lookup_gateway_by_name "$name"
  if [[ -n "${_GW_ID}" ]]; then
    id="${_GW_ID}"
    ns="${_GW_NAMESPACE}"
    phase="${_GW_PHASE}"
    create_s="0"
    if [[ "$phase" == "Running" ]]; then
      running_s=$(perf_elapsed_s "$start_ms")
      status="ok"
    else
      printf '%s\t%s\t%s\t%s\n' "$name" "waiting:${phase:-unknown}" "$id" "$ns" > "${record}.state"
      if phase=$(e2e_wait_gateway_running "$id" "${E2E_PERF_PROVISION_TIMEOUT:-600}"); then
        running_s=$(perf_elapsed_s "$start_ms")
        status="ok"
      else
        running_s=$(perf_elapsed_s "$start_ms")
        status="fail"
      fi
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$status" "$create_s" "$running_s" "$id" "$ns" "$name" > "$record"
    return 0
  fi

  local body resp post_start
  printf '%s\t%s\n' "$name" "creating" > "${record}.state"
  body=$(e2e_gateway_create_body "$name")
  post_start=$(perf_now_ms)
  resp=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" -d "${body}" 2>/dev/null || true)
  create_s=$(perf_elapsed_s "$post_start")
  e2e_parse_gateway_response "$resp"
  if [[ "$_CREATE_KIND" != "OK" || -z "$_CREATE_ID" ]]; then
    running_s=$(perf_elapsed_s "$start_ms")
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "fail" "$create_s" "$running_s" "" "" "$name" > "$record"
    return 0
  fi
  id="$_CREATE_ID"
  ns="$_CREATE_NAMESPACE"
  printf '%s\t%s\t%s\t%s\n' "$name" "waiting:Provisioning" "$id" "$ns" > "${record}.state"
  if phase=$(e2e_wait_gateway_running "$id" "${E2E_PERF_PROVISION_TIMEOUT:-600}"); then
    running_s=$(perf_elapsed_s "$start_ms")
    status="ok"
  else
    running_s=$(perf_elapsed_s "$start_ms")
    status="fail"
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$status" "$create_s" "$running_s" "$id" "$ns" "$name" > "$record"
}

perf_delete_gateway_by_name() {
  local name="${1:?gateway name required}"
  local out="${2:?out path required}"
  : > "$out"
  if ! acquire_oidc_token 2>/dev/null; then
    printf 'error\t\t\t%s\tauth\n' "$name" >> "$out"
    return 1
  fi

  # A prior interrupted/retried run may have created duplicate names. Repeat
  # exact lookup + deletion until none remain, with a defensive upper bound.
  local deleted=0 attempt code
  for ((attempt = 1; attempt <= 20; attempt++)); do
    e2e_lookup_gateway_by_name "$name"
    if [[ -z "${_GW_ID}" ]]; then
      if ((deleted == 0)); then
        printf 'missing\t\t\t%s\t\n' "$name" >> "$out"
      fi
      return 0
    fi
    code=$(api_curl -o /dev/null -w '%{http_code}' -X DELETE \
      "${API_HOST}/api/hypershell/v1/gateways/${_GW_ID}" 2>/dev/null || true)
    if [[ "$code" != "200" && "$code" != "202" && "$code" != "204" ]]; then
      printf 'error\t%s\t%s\t%s\thttp-%s\n' \
        "${_GW_ID}" "${_GW_NAMESPACE}" "$name" "${code:-no-response}" >> "$out"
      return 1
    fi
    printf 'deleted\t%s\t%s\t%s\t\n' \
      "${_GW_ID}" "${_GW_NAMESPACE}" "$name" >> "$out"
    deleted=$((deleted + 1))
    sleep 1
  done
  printf 'error\t\t\t%s\tduplicate-limit\n' "$name" >> "$out"
  return 1
}
