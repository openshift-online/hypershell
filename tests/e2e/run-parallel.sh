#!/usr/bin/env bash
# run-parallel.sh - run several e2e suites concurrently against one cluster and
# report each result.
#
# Usage:
#   bash tests/e2e/run-parallel.sh NAME=COMMAND [NAME=COMMAND ...]
#
# Each COMMAND runs under `bash -c`, with its output streamed live using a
# `[NAME]` prefix and simultaneously captured to a separate log. When every
# suite has exited, the logs are replayed one after another (one collapsible
# group each under GitHub Actions) followed by a one-line result per suite. Set
# E2E_PARALLEL_LOG_DIR to preserve copies after the runner exits. The exit status
# is non-zero if any suite failed. While suites run, a heartbeat line is printed
# every E2E_PARALLEL_HEARTBEAT seconds (default 60) so a long run is not silent.
#
# The first NAME is the primary suite (e2e-openshell.sh). It is the only suite
# that disturbs the shared control plane (the area 12g scale-down), so this
# runner exports E2E_DISRUPTIVE_GATE_FILE to every suite and creates that file
# once every suite other than the primary has exited. The primary waits for it
# before the disturbance (see e2e_wait_disruptive_gate in lib.sh). A single-suite
# run opens the gate immediately.
#
# Written for bash 3.2 (macOS /bin/bash): no wait -n, mapfile or associative arrays.
set -uo pipefail

if (($# < 1)); then
  echo "usage: $0 NAME=COMMAND [NAME=COMMAND ...]" >&2
  exit 2
fi

names=()
cmds=()
for spec in "$@"; do
  name="${spec%%=*}"
  cmd="${spec#*=}"
  if [[ "$spec" != *=* || -z "$name" || -z "$cmd" || ! "$name" =~ ^[A-Za-z0-9_.-]+$ ]]; then
    echo "run-parallel: invalid suite '${spec}' (want NAME=COMMAND, NAME of [A-Za-z0-9_.-])" >&2
    exit 2
  fi
  names+=("$name")
  cmds+=("$cmd")
done

workdir="$(mktemp -d "${TMPDIR:-/tmp}/hypershell-e2e-parallel.XXXXXX")"
gate="${workdir}/disruptive-gate"
export E2E_DISRUPTIVE_GATE_FILE="$gate"
heartbeat="${E2E_PARALLEL_HEARTBEAT:-60}"
log_dir="${E2E_PARALLEL_LOG_DIR:-}"

pids=()
stop_children() {
  # bash < 4.4 treats an empty array as unset under `set -u`; a signal can land
  # before the first suite has started.
  ((${#pids[@]})) || return 0
  local pid
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
}
persist_logs() {
  [[ -n "$log_dir" ]] || return 0
  if ! mkdir -p "$log_dir"; then
    echo "[run-parallel] warning: could not create log directory ${log_dir}" >&2
    return 0
  fi

  local i log
  for i in "${!names[@]}"; do
    log="${workdir}/${names[$i]}.log"
    [[ -f "$log" ]] && cp "$log" "${log_dir}/${names[$i]}.log"
  done
}
cleanup() {
  persist_logs
  rm -rf "$workdir"
}
trap 'stop_children; exit 130' INT TERM
trap cleanup EXIT

start=$(date +%s)
for i in "${!names[@]}"; do
  (
    # `sed -u` keeps suite output live through the pipeline on both GNU and BSD
    # sed. Capture PIPESTATUS before the subshell exits so a suite failure is
    # not hidden by successful tee/sed processes.
    bash -c "${cmds[$i]}" 2>&1 | tee "${workdir}/${names[$i]}.log" | sed -u "s/^/[${names[$i]}] /"
    statuses=("${PIPESTATUS[@]}")
    ((statuses[1] == 0 && statuses[2] == 0)) || exit 1
    exit "${statuses[0]}"
  ) &
  pids+=("$!")
  echo "[run-parallel] started ${names[$i]} (pid ${pids[$i]}; live prefix [${names[$i]}])"
done
((${#names[@]} == 1)) && : >"$gate"

ends=()
for i in "${!names[@]}"; do ends+=(""); done
last_beat=$start
while :; do
  running=()
  others_running=0
  for i in "${!names[@]}"; do
    if [[ -z "${ends[$i]}" ]]; then
      if kill -0 "${pids[$i]}" 2>/dev/null; then
        running+=("${names[$i]}")
        ((i > 0)) && others_running=$((others_running + 1))
      else
        ends[i]=$(date +%s)
      fi
    fi
  done
  if ((others_running == 0)) && [[ ! -e "$gate" ]]; then
    : >"$gate"
    echo "[run-parallel] gate opened: suites other than ${names[0]} have finished"
  fi
  ((${#running[@]} == 0)) && break
  now=$(date +%s)
  if ((now - last_beat >= heartbeat)); then
    echo "[run-parallel] $((now - start))s elapsed; still running: ${running[*]}"
    last_beat=$now
  fi
  sleep 1
done

failed=0
summary=()
for i in "${!names[@]}"; do
  wait "${pids[$i]}"
  rc=$?
  ((rc != 0)) && failed=1
  secs=$((ends[i] - start))
  # A passing suite's log collapses under GitHub Actions; a failing one stays
  # expanded so the failure is visible without opening a group.
  grouped=0
  [[ "${GITHUB_ACTIONS:-}" == "true" && $rc -eq 0 ]] && grouped=1
  if ((grouped)); then
    echo "::group::${names[$i]} (exit ${rc}, ${secs}s)"
  else
    echo ""
    echo "================ ${names[$i]} (exit ${rc}, ${secs}s) ================"
  fi
  cat "${workdir}/${names[$i]}.log"
  ((grouped)) && echo "::endgroup::"
  summary+=("${names[$i]}: exit ${rc} after ${secs}s")
done

echo ""
echo "[run-parallel] results ($(($(date +%s) - start))s total):"
for line in "${summary[@]}"; do
  echo "[run-parallel]   ${line}"
done
exit "$failed"
