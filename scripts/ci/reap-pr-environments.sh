#!/usr/bin/env bash
# reap-pr-environments.sh - out-of-band reaper for ephemeral pull-request
# environments (ephemeral-pr-environments.spec.md, Timebox and Reaping).
#
# Runs independently of any CI run (as a CronJob on the target cluster, see
# deploy/e2e/reaper), so an abandoned pull request's environment is reclaimed
# even when no further CI runs for that pull request. It deletes a namespace when
# and only when pr_env_is_reapable is true: prefixed hypershell-ci-pr-, owned by
# HyperShell, environment id pr-<number>, and past its
# hypershell.redhat.io/expires-at. Because every deploying run refreshes that
# annotation, an actively worked pull request is never reaped mid-flight; a
# namespace with no activity for the timebox falls past its expiry and is removed.
#
# The reaper matches the platform and the -keycloak namespaces independently
# (both carry the same labels and prefix), so one pass removes the whole group.
# Gateway and ManagedDatabase namespaces are siblings labeled
# hypershell.redhat.io/instance=<platform ns>; they are reaped with the platform
# project and again if that project is already gone.
#
# Environment:
#   PR_ENV_KUBECTL        kubectl/oc binary (default: kubectl)
#   PR_ENV_REAP_DRY_RUN   when "true", log the decisions but delete nothing
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

KUBECTL="${PR_ENV_KUBECTL:-kubectl}"
DRY_RUN="${PR_ENV_REAP_DRY_RUN:-false}"

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

# List candidate namespaces (owned by HyperShell) as tab-separated
# name<TAB>owned<TAB>environment<TAB>expires-at. Narrowed by the owned label;
# the full predicate still runs per row. `if` guards protect against namespaces
# with no labels or no annotations (index on a nil map errors in go-template).
list_owned_namespaces() {
  "${KUBECTL}" get namespaces \
    -l "${PR_ENV_OWNED_LABEL}=true" \
    -o go-template='{{range .items}}{{.metadata.name}}{{"\t"}}{{if .metadata.labels}}{{index .metadata.labels "hypershell.redhat.io/owned"}}{{end}}{{"\t"}}{{if .metadata.labels}}{{index .metadata.labels "hypershell.redhat.io/environment"}}{{end}}{{"\t"}}{{if .metadata.annotations}}{{index .metadata.annotations "hypershell.redhat.io/expires-at"}}{{end}}{{"\n"}}{{end}}'
}

# name<TAB>instance for namespaces the control plane stamped.
list_control_plane_managed_namespaces() {
  "${KUBECTL}" get namespaces \
    -l "${PR_ENV_CP_MANAGED_LABEL}=${PR_ENV_CP_MANAGED_VALUE},${PR_ENV_CP_MANAGED_BY_LABEL}=${PR_ENV_CP_MANAGED_BY_VALUE}" \
    -o go-template='{{range .items}}{{.metadata.name}}{{"\t"}}{{if .metadata.labels}}{{index .metadata.labels "hypershell.redhat.io/instance"}}{{end}}{{"\n"}}{{end}}'
}

platform_namespace_exists() {
  "${KUBECTL}" get namespace "$1" >/dev/null 2>&1
}

# Delete this environment's cluster-scoped RBAC, mirroring cluster_down in
# scripts/cluster/drivers/openshift.sh. Only the platform namespace owns the
# ${ns}-dev-* cluster RBAC; the -keycloak namespace has none.
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

reap_namespace() {
  local ns="$1"
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "  DRY-RUN would delete namespace ${ns}"
    return 0
  fi
  # --wait=false: do not block the reaper on finalizers; the delete is recorded
  # and the namespace terminates asynchronously.
  if "${KUBECTL}" delete namespace "${ns}" --ignore-not-found --wait=false >/dev/null 2>&1; then
    log "  deleted namespace ${ns}"
  else
    log "  WARNING failed to delete namespace ${ns}"
    return 1
  fi
}

# Delete sibling gateway/database namespaces this platform instance stamped.
reap_instance_workloads() {
  local instance="$1"
  local rows name inst
  if [[ -z "${instance}" ]]; then
    log "  WARNING refusing to reap instance workloads with an empty instance identity"
    return 1
  fi
  if ! rows="$(list_control_plane_managed_namespaces)"; then
    log "  WARNING could not list instance-managed namespaces for ${instance}"
    return 1
  fi
  while IFS=$'\t' read -r name inst; do
    [[ -n "${name}" ]] || continue
    [[ "${inst}" == "${instance}" ]] || continue
    if [[ "${name}" == "${instance}" || "${name}" == "${instance}-keycloak" ]]; then
      continue
    fi
    log "  instance workload ${name} (instance=${instance})"
    reap_namespace "${name}" || true
  done <<< "${rows}"
}

reap_leftover_instance_workloads() {
  local rows name inst exists
  if ! rows="$(list_control_plane_managed_namespaces)"; then
    log "ERROR: could not list control-plane managed namespaces via ${KUBECTL}"
    return 1
  fi
  while IFS=$'\t' read -r name inst; do
    [[ -n "${name}" ]] || continue
    exists="false"
    if [[ -n "${inst}" ]] && platform_namespace_exists "${inst}"; then
      exists="true"
    fi
    if pr_env_should_reap_instance_workload "${name}" "${inst}" "${exists}"; then
      log "REAP leftover ${name} (instance=${inst}, platform absent)"
      if reap_namespace "${name}"; then
        reaped=$((reaped + 1))
      else
        failed=$((failed + 1))
      fi
    fi
  done <<< "${rows}"
}

main() {
  local now considered=0 reaped=0 retained=0 failed=0
  now="$(pr_env_now_epoch)"
  log "pr-env reaper: scanning owned namespaces (dry_run=${DRY_RUN})"

  local rows
  if ! rows="$(list_owned_namespaces)"; then
    log "ERROR: could not list namespaces via ${KUBECTL}"
    return 1
  fi

  local name owned env_id expires
  while IFS=$'\t' read -r name owned env_id expires; do
    [[ -n "${name}" ]] || continue
    considered=$((considered + 1))
    if pr_env_is_reapable "${name}" "${owned}" "${env_id}" "${expires}" "${now}"; then
      log "REAP ${name} (env=${env_id}, expired at ${expires})"
      if reap_namespace "${name}"; then
        # Only the platform namespace owns cluster RBAC and instance-stamped
        # gateway/database namespaces; skip the -keycloak half.
        if [[ "${name}" != *-keycloak ]]; then
          delete_cluster_rbac "${name}"
          reap_instance_workloads "${name}"
        fi
        reaped=$((reaped + 1))
      else
        failed=$((failed + 1))
      fi
    else
      retained=$((retained + 1))
    fi
  done <<< "${rows}"

  reap_leftover_instance_workloads

  log "pr-env reaper: considered=${considered} reaped=${reaped} retained=${retained} failed=${failed}"
  [[ "${failed}" -eq 0 ]]
}

main "$@"
