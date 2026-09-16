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
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${OPENSHIFT_NAMESPACE:?OPENSHIFT_NAMESPACE is required}"
KUBECTL="${PR_ENV_KUBECTL:-oc}"
DRY_RUN="${PR_ENV_REAP_DRY_RUN:-false}"
platform_ns="${OPENSHIFT_NAMESPACE}"
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

namespace_exists() {
  "${KUBECTL}" get namespace "$1" >/dev/null 2>&1
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
  if ! namespace_exists "${ns}"; then
    log "  namespace ${ns} already gone"
    return 0
  fi
  # Prefer `oc delete project` when the binary is oc so OpenShift project
  # finalizers run the same way make openshift-down does; fall back to
  # namespace delete for plain kubectl.
  local err=""
  if [[ "$(basename "${KUBECTL}")" == "oc" ]]; then
    if err="$("${KUBECTL}" delete project "${ns}" --wait=true --timeout=300s 2>&1)"; then
      log "  deleted project ${ns}"
      return 0
    fi
    if grep -qi 'not found' <<<"${err}"; then
      return 0
    fi
  fi
  if "${KUBECTL}" delete namespace "${ns}" --ignore-not-found --wait=true --timeout=300s >/dev/null 2>&1; then
    log "  deleted namespace ${ns}"
    return 0
  fi
  log "  WARNING failed to delete namespace ${ns}"
  return 1
}

delete_instance_managed() {
  local instance="$1"
  if [[ -z "${instance}" ]]; then
    log "  WARNING refusing to delete instance-managed namespaces with an empty instance identity"
    return 1
  fi
  local selector names ns
  selector="${PR_ENV_CP_MANAGED_LABEL}=${PR_ENV_CP_MANAGED_VALUE},${PR_ENV_CP_MANAGED_BY_LABEL}=${PR_ENV_CP_MANAGED_BY_VALUE},${PR_ENV_CP_INSTANCE_LABEL}=${instance}"
  names="$("${KUBECTL}" get namespace -l "${selector}" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"
  if [[ -z "${names}" ]]; then
    log "  no instance-managed namespaces for ${instance}"
    return 0
  fi
  local failed=0
  while IFS= read -r ns; do
    [[ -n "${ns}" ]] || continue
    if [[ "${ns}" == "${instance}" || "${ns}" == "${instance}-keycloak" ]]; then
      continue
    fi
    log "  instance workload ${ns} (instance=${instance})"
    if ! delete_namespace "${ns}"; then
      failed=1
    fi
  done <<< "${names}"
  return "${failed}"
}

log "teardown ${platform_ns} (keycloak ${keycloak_ns}, dry_run=${DRY_RUN})"
delete_cluster_rbac "${platform_ns}"
failed=0
if ! delete_namespace "${keycloak_ns}"; then
  failed=1
fi
if ! delete_namespace "${platform_ns}"; then
  failed=1
fi
if ! delete_instance_managed "${platform_ns}"; then
  failed=1
fi
if [[ "${failed}" -ne 0 ]]; then
  log "teardown failed for ${platform_ns}"
  exit 1
fi
log "teardown complete for ${platform_ns}"
