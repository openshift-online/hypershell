#!/usr/bin/env bash
# perf-report.sh - tabulate recent performance runs from perf-results/*.json.
#
# Depends only on bash (no python, no jq). Reads schema_version=1 files
# written by tests/e2e/perf/lib.sh.
#
# Usage:
#   make e2e-performance-report
#   bash scripts/perf-report.sh
#   bash scripts/perf-report.sh kind-20260821T153000Z
#   E2E_PERF_REPORT_RUN=kind-20260821T153000Z bash scripts/perf-report.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# shellcheck source=../tests/e2e/perf/lib.sh
source "${REPO_ROOT}/tests/e2e/perf/lib.sh"

: "${E2E_PERF_RESULTS_DIR:=perf-results}"
: "${E2E_PERF_REPORT_LIMIT:=10}"
: "${E2E_PERF_REPORT_RUN:=}"

RESULTS_DIR="${E2E_PERF_RESULTS_DIR}"
if [[ "${RESULTS_DIR}" != /* ]]; then
  RESULTS_DIR="${REPO_ROOT}/${RESULTS_DIR}"
fi

RUN_SELECTOR="${E2E_PERF_REPORT_RUN:-${1:-}}"

_perf_dash() {
  [[ -z "${1:-}" || "$1" == "null" ]] && echo '-' || echo "$1"
}

_perf_compact_stamp() {
  local s="${1:-}"
  s="${s//-/}"
  s="${s//:/}"
  echo "$s"
}

# Print matching JSON paths, most recent started_at first.
_perf_list_runs() {
  local f base sv started
  [[ -d "$RESULTS_DIR" ]] || return 0
  local rows=()
  for f in "$RESULTS_DIR"/*.json; do
    [[ -f "$f" ]] || continue
    base="$(basename "$f")"
    [[ "$base" == "latest.json" ]] && continue
    sv="$(perf_json_first "$f" schema_version)"
    [[ "$sv" == "1" ]] || continue
    started="$(perf_json_first "$f" started_at)"
    rows+=("${started}"$'\t'"$f")
  done
  if (( ${#rows[@]} == 0 )); then
    return 0
  fi
  printf '%s\n' "${rows[@]}" | sort -r | while IFS=$'\t' read -r _ path; do
    printf '%s\n' "$path"
  done
}

_perf_find_run() {
  local selector="$1"
  if [[ -f "$selector" ]]; then
    printf '%s\n' "$selector"
    return 0
  fi
  if [[ -f "${RESULTS_DIR}/${selector}" ]]; then
    printf '%s\n' "${RESULTS_DIR}/${selector}"
    return 0
  fi
  if [[ -f "${RESULTS_DIR}/${selector}.json" ]]; then
    printf '%s\n' "${RESULTS_DIR}/${selector}.json"
    return 0
  fi
  local f base started compact
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    base="$(basename "$f" .json)"
    started="$(perf_json_first "$f" started_at)"
    compact="$(_perf_compact_stamp "$started")"
    if [[ "$base" == "$selector" || "$base" == *"$selector"* || "$started" == "$selector" || "$compact" == "$selector" ]]; then
      printf '%s\n' "$f"
      return 0
    fi
  done < <(_perf_list_runs)
  return 1
}

_perf_print_table() {
  local -a headers=("$@")
  local -a rows=()
  local line
  while IFS= read -r line; do
    [[ -n "$line" ]] && rows+=("$line")
  done

  local -a widths=()
  local i cell
  for ((i = 0; i < ${#headers[@]}; i++)); do
    widths[i]=${#headers[i]}
  done
  for line in "${rows[@]+"${rows[@]}"}"; do
    IFS=$'\t' read -r -a cells <<< "$line"
    for ((i = 0; i < ${#headers[@]}; i++)); do
      cell="${cells[i]:-}"
      if (( ${#cell} > widths[i] )); then
        widths[i]=${#cell}
      fi
    done
  done

  local fmt="" sep=""
  for ((i = 0; i < ${#headers[@]}; i++)); do
    fmt+="%-${widths[i]}s"
    sep+=$(printf '%*s' "${widths[i]}" '' | tr ' ' '-')
    if (( i < ${#headers[@]} - 1 )); then
      fmt+="  "
      sep+="  "
    fi
  done
  # shellcheck disable=SC2059
  printf "${fmt}\n" "${headers[@]}"
  printf '%s\n' "$sep"
  for line in "${rows[@]+"${rows[@]}"}"; do
    IFS=$'\t' read -r -a cells <<< "$line"
    # shellcheck disable=SC2059
    printf "${fmt}\n" "${cells[@]}"
  done
}

_perf_print_recent() {
  local limit="${E2E_PERF_REPORT_LIMIT:-10}"
  local f n=0
  local -a lines=()
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    n=$((n + 1))
    (( n > limit )) && break
    lines+=("$(_perf_dash "$(perf_json_first "$f" started_at)")"$'\t'"$(_perf_dash "$(perf_json_first "$f" driver)")"$'\t'"$(_perf_dash "$(perf_json_first "$f" gateway_count)")"$'\t'"$(_perf_dash "$(perf_json_first "$f" success_rate)")"$'\t'"$(_perf_dash "$(perf_json_object_field "$f" time_to_running_seconds avg)")"$'\t'"$(_perf_dash "$(perf_json_object_field "$f" time_to_running_seconds p99)")"$'\t'"$(_perf_dash "$(perf_json_first "$f" throughput_per_min)")"$'\t'"$(_perf_dash "$(perf_json_first "$f" result)")")
  done < <(_perf_list_runs)

  if (( ${#lines[@]} == 0 )); then
    echo "No performance results in ${RESULTS_DIR}"
    echo "Run \`make e2e-performance\` to produce a history file."
    return 0
  fi
  printf '%s\n' "${lines[@]}" | _perf_print_table timestamp driver count 'success%' avg p99 'tput/min' result
}

_perf_print_checkpoints() {
  local file="$1"
  echo "Run: $(basename "$file")"
  echo "  driver=$(_perf_dash "$(perf_json_first "$file" driver)")  result=$(_perf_dash "$(perf_json_first "$file" result)")  started=$(_perf_dash "$(perf_json_first "$file" started_at)")"
  local cps
  cps="$(perf_json_checkpoints "$file")"
  if [[ -z "$cps" ]]; then
    echo "  (no checkpoints)"
    return 0
  fi
  echo ""
  printf '%s\n' "$cps" | _perf_print_table count 'batch avg' 'batch p99' 'mini s' result
}

if [[ -n "$RUN_SELECTOR" ]]; then
  RUN_FILE=""
  if ! RUN_FILE="$(_perf_find_run "$RUN_SELECTOR")"; then
    echo "No run matching '${RUN_SELECTOR}' in ${RESULTS_DIR}" >&2
    exit 1
  fi
  _perf_print_checkpoints "$RUN_FILE"
else
  _perf_print_recent
fi
