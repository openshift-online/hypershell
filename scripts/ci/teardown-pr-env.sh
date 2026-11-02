#!/usr/bin/env bash
# teardown-pr-env.sh - shared OpenShift environment teardown used by
# `make openshift-down` and the out-of-band PR-environment reaper
# (ephemeral-pr-environments.spec.md: Timebox and Reaping).
#
# Invoked per environment with OPENSHIFT_NAMESPACE set to the platform
# namespace. Deletes the namespace group (platform and -keycloak), that
# environment's cluster-scoped RBAC, and instance-managed gateway/database
# namespaces. Idempotent: missing namespaces and RBAC are ignored.
#
# This is the deletion engine. `cluster_down` in
# scripts/cluster/drivers/openshift.sh still performs local-dev safety checks
# (owned-namespace verification) before calling this script. The reaper uses
# pr_env_is_reapable instead of those checks.
#
# Environment:
#   OPENSHIFT_NAMESPACE   platform namespace (required)
#   PR_ENV_KUBECTL        kubectl/oc binary (default: oc)
#   PR_ENV_REAP_DRY_RUN   when "true", log the decisions but delete nothing
#   PR_ENV_DELETE_TIMEOUT_SECONDS
#                         deadline for confirmed Namespace deletion (default: 300)
#   PR_ENV_DELETE_POLL_SECONDS
#                         positive Namespace polling interval (default: 2)
#   PR_ENV_DELETE_STATUS_INTERVAL_SECONDS
#                         positive progress logging interval (default: 30)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${OPENSHIFT_NAMESPACE:?OPENSHIFT_NAMESPACE is required}"
KUBECTL="${PR_ENV_KUBECTL:-oc}"
DRY_RUN="${PR_ENV_REAP_DRY_RUN:-false}"
DELETE_TIMEOUT_SECONDS="${PR_ENV_DELETE_TIMEOUT_SECONDS:-300}"
DELETE_POLL_SECONDS="${PR_ENV_DELETE_POLL_SECONDS:-2}"
DELETE_STATUS_INTERVAL_SECONDS="${PR_ENV_DELETE_STATUS_INTERVAL_SECONDS:-30}"
platform_ns="${OPENSHIFT_NAMESPACE}"
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

require_non_negative_integer() {
  local name="$1" value="$2"
  if [[ ! "${value}" =~ ^[0-9]+$ ]]; then
    log "  ERROR ${name} must be a non-negative integer, got ${value}"
    exit 1
  fi
}

require_positive_integer() {
  local name="$1" value="$2"
  if [[ ! "${value}" =~ ^[1-9][0-9]*$ ]]; then
    log "  ERROR ${name} must be a positive integer, got ${value}"
    exit 1
  fi
}

require_non_negative_integer PR_ENV_DELETE_TIMEOUT_SECONDS "${DELETE_TIMEOUT_SECONDS}"
require_positive_integer PR_ENV_DELETE_POLL_SECONDS "${DELETE_POLL_SECONDS}"
require_positive_integer PR_ENV_DELETE_STATUS_INTERVAL_SECONDS "${DELETE_STATUS_INTERVAL_SECONDS}"

# namespace_lookup returns 0 when the Namespace exists, 1 when it is confirmed
# absent, and 2 for an API/authentication error. Treating every get failure as
# absence could report a successful teardown without confirming deletion.
NAMESPACE_LOOKUP_ERROR=""
namespace_lookup() {
  local ns="$1" err=""
  NAMESPACE_LOOKUP_ERROR=""
  if err="$("${KUBECTL}" get namespace "${ns}" -o name 2>&1)"; then
    return 0
  fi
  if grep -qiE 'not found|\(NotFound\)' <<<"${err}"; then
    return 1
  fi
  NAMESPACE_LOOKUP_ERROR="${err}"
  return 2
}

log_namespace_status() {
  local ns="$1" status=""
  if status="$("${KUBECTL}" get namespace "${ns}" \
    -o jsonpath='{.status.phase}{" deletionTimestamp="}{.metadata.deletionTimestamp}{" finalizers="}{.spec.finalizers}{" conditions="}{range .status.conditions[*]}{.type}{"="}{.status}{"/"}{.reason}{";"}{end}' \
    2>&1)"; then
    log "  waiting for namespace ${ns}: ${status:-present}"
  elif ! grep -qiE 'not found|\(NotFound\)' <<<"${status}"; then
    log "  WARNING could not read deletion status for namespace ${ns}: ${status}"
  fi
}

request_namespace_deletion() {
  local ns="$1" err=""
  # Keep the OpenShift Project deletion path, but do not use oc's built-in
  # waiter. In live CI runs the Namespace was already absent while
  # `oc delete project --wait=true` remained blocked until its 300s timeout.
  if [[ "$(basename "${KUBECTL}")" == "oc" ]]; then
    if err="$("${KUBECTL}" delete project "${ns}" --ignore-not-found --wait=false 2>&1)"; then
      log "  requested deletion of project ${ns}"
      return 0
    fi
    if grep -qiE 'not found|\(NotFound\)' <<<"${err}"; then
      return 0
    fi
    log "  WARNING project deletion request failed for ${ns}; trying Namespace API: ${err}"
  fi
  if err="$("${KUBECTL}" delete namespace "${ns}" --ignore-not-found --wait=false 2>&1)"; then
    log "  requested deletion of namespace ${ns}"
    return 0
  fi
  log "  WARNING failed to request deletion of namespace ${ns}: ${err}"
  return 1
}

wait_for_namespace_gone() {
  local ns="$1"
  local started_at="${SECONDS}" next_status_at="${SECONDS}" lookup_rc=0
  while true; do
    lookup_rc=0
    namespace_lookup "${ns}" || lookup_rc=$?
    if [[ "${lookup_rc}" -eq 1 ]]; then
      log "  deleted namespace ${ns}"
      return 0
    fi

    if (( SECONDS >= next_status_at )); then
      if [[ "${lookup_rc}" -eq 2 ]]; then
        log "  WARNING Namespace lookup failed for ${ns}: ${NAMESPACE_LOOKUP_ERROR}"
      else
        log_namespace_status "${ns}"
      fi
      next_status_at=$((SECONDS + DELETE_STATUS_INTERVAL_SECONDS))
    fi

    if (( SECONDS - started_at >= DELETE_TIMEOUT_SECONDS )); then
      log "  WARNING timed out after ${DELETE_TIMEOUT_SECONDS}s waiting for namespace ${ns} deletion"
      return 1
    fi
    sleep "${DELETE_POLL_SECONDS}"
  done
}

delete_cluster_rbac() {
  local ns="$1"
  local prefix="${ns}-dev-"
  local kind name
  for kind in clusterrolebinding clusterrole; do
    for name in "${prefix}hypershell-controller-scc-bind" "${prefix}hypershell-controller"; do
      if [[ "${DRY_RUN}" == "true" ]]; then
        log "  DRY-RUN would delete ${kind}/${name}"
      else
        "${KUBECTL}" delete "${kind}" "${name}" --ignore-not-found >/dev/null 2>&1 || true
      fi
    done
  done
}

delete_namespace() {
  local ns="$1"
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "  DRY-RUN would delete namespace ${ns}"
    return 0
  fi
  local lookup_rc=0
  namespace_lookup "${ns}" || lookup_rc=$?
  if [[ "${lookup_rc}" -eq 1 ]]; then
    log "  namespace ${ns} already gone"
    return 0
  fi
  if [[ "${lookup_rc}" -eq 2 ]]; then
    log "  WARNING could not verify namespace ${ns} before deletion: ${NAMESPACE_LOOKUP_ERROR}"
    return 1
  fi
  request_namespace_deletion "${ns}" || return 1
  wait_for_namespace_gone "${ns}"
}

# delete_namespaces_parallel deletes every given namespace concurrently and waits
# for all of them, returning non-zero if any delete failed. Namespace deletion is
# finalizer-bound and the namespaces are mutually independent, so issuing each
# nonblocking request and polling in parallel bounds wall-clock by the slowest
# namespace instead of their sum. Backgrounded deletes do not trip errexit; each
# exit code is collected with wait.
delete_namespaces_parallel() {
  local ns rc=0
  local pids=()
  for ns in "$@"; do
    [[ -n "${ns}" ]] || continue
    delete_namespace "${ns}" &
    pids+=("$!")
  done
  if [[ "${#pids[@]}" -gt 0 ]]; then
    local pid
    for pid in "${pids[@]}"; do
      if ! wait "${pid}"; then
        rc=1
      fi
    done
  fi
  return "${rc}"
}

delete_instance_managed() {
  local instance="$1"
  if [[ -z "${instance}" ]]; then
    log "  WARNING refusing to delete instance-managed namespaces with an empty instance identity"
    return 1
  fi
  local selector names ns
  local to_delete=()
  selector="${PR_ENV_CP_MANAGED_LABEL}=${PR_ENV_CP_MANAGED_VALUE},${PR_ENV_CP_MANAGED_BY_LABEL}=${PR_ENV_CP_MANAGED_BY_VALUE},${PR_ENV_CP_INSTANCE_LABEL}=${instance}"
  if ! names="$("${KUBECTL}" get namespace -l "${selector}" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>&1)"; then
    log "  WARNING failed to list instance-managed namespaces for ${instance}: ${names}"
    return 1
  fi
  if [[ -z "${names}" ]]; then
    log "  no instance-managed namespaces for ${instance}"
    return 0
  fi
  while IFS= read -r ns; do
    [[ -n "${ns}" ]] || continue
    if [[ "${ns}" == "${instance}" || "${ns}" == "${instance}-keycloak" ]]; then
      continue
    fi
    log "  instance workload ${ns} (instance=${instance})"
    to_delete+=("${ns}")
  done <<< "${names}"
  [[ "${#to_delete[@]}" -gt 0 ]] || return 0
  delete_namespaces_parallel "${to_delete[@]}"
}

log "teardown ${platform_ns} (keycloak ${keycloak_ns}, dry_run=${DRY_RUN})"
delete_cluster_rbac "${platform_ns}"
failed=0
# Tier 1: the control-plane namespace group (platform + its companion keycloak).
# Delete it BEFORE the instance-managed gateway namespaces so the control plane
# cannot recreate a gateway namespace from a still-live Gateway record while the
# teardown is in flight. The two group namespaces are independent, so delete them
# concurrently (ephemeral-pr-environments.spec.md CICD-PREN-02).
if ! delete_namespaces_parallel "${keycloak_ns}" "${platform_ns}"; then
  failed=1
fi
# Tier 2: instance-managed gateway/database namespaces, now that the control plane
# is gone. They are mutually independent, so delete them concurrently too.
if ! delete_instance_managed "${platform_ns}"; then
  failed=1
fi
if [[ "${failed}" -ne 0 ]]; then
  log "teardown failed for ${platform_ns}"
  exit 1
fi
log "teardown complete for ${platform_ns}"
