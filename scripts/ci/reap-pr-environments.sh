#!/usr/bin/env bash
# reap-pr-environments.sh - out-of-band reaper for ephemeral pull-request
# environments (ephemeral-pr-environments.spec.md, Timebox and Reaping).
#
# Runs independently of any CI run (as a CronJob on the target cluster, see
# deploy/e2e/reaper), so a crashed in-run teardown or a quiet retained pull
# request is still reclaimed. It identifies a namespace group when
# pr_env_is_reapable is true: prefixed hypershell-ci-pr-, owned by HyperShell,
# environment id pr-<number>, and past its hypershell.redhat.io/expires-at.
# Deletion uses the same teardown path as `make openshift-down`
# (scripts/ci/teardown-pr-env.sh) per expired environment.
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
TEARDOWN="${SCRIPT_DIR}/teardown-pr-env.sh"

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

# Platform namespace for a reapable owned name (strip the -keycloak suffix).
platform_for() {
  local name="$1"
  if [[ "${name}" == *-keycloak ]]; then
    printf '%s' "${name%-keycloak}"
  else
    printf '%s' "${name}"
  fi
}

teardown_env() {
  local ns="$1"
  if [[ "${DRY_RUN}" == "true" ]]; then
    log "  DRY-RUN would teardown ${ns} via ${TEARDOWN}"
    return 0
  fi
  if OPENSHIFT_NAMESPACE="${ns}" PR_ENV_KUBECTL="${KUBECTL}" PR_ENV_REAP_DRY_RUN="${DRY_RUN}" \
    bash "${TEARDOWN}"; then
    log "  teardown ${ns} succeeded"
    return 0
  fi
  log "  WARNING teardown ${ns} failed"
  return 1
}

main() {
  local now considered=0 reaped=0 retained=0 failed=0
  local seen=" "
  now="$(pr_env_now_epoch)"
  log "pr-env reaper: scanning owned namespaces (dry_run=${DRY_RUN})"

  local rows
  if ! rows="$(list_owned_namespaces)"; then
    log "ERROR: could not list namespaces via ${KUBECTL}"
    return 1
  fi

  local name owned env_id expires platform
  while IFS=$'\t' read -r name owned env_id expires; do
    [[ -n "${name}" ]] || continue
    considered=$((considered + 1))
    if pr_env_is_reapable "${name}" "${owned}" "${env_id}" "${expires}" "${now}"; then
      platform="$(platform_for "${name}")"
      if [[ "${seen}" == *" ${platform} "* ]]; then
        log "REAP ${name} (env=${env_id}) already queued as ${platform}"
        continue
      fi
      seen="${seen}${platform} "
      log "REAP ${name} (env=${env_id}, expired at ${expires}) via openshift-down teardown"
      if teardown_env "${platform}"; then
        reaped=$((reaped + 1))
      else
        failed=$((failed + 1))
      fi
    else
      retained=$((retained + 1))
    fi
  done <<< "${rows}"

  local inst exists
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
      if [[ "${seen}" == *" ${inst} "* ]]; then
        continue
      fi
      seen="${seen}${inst} "
      log "REAP leftover ${name} (instance=${inst}, platform absent) via openshift-down teardown"
      if teardown_env "${inst}"; then
        reaped=$((reaped + 1))
      else
        failed=$((failed + 1))
      fi
    fi
  done <<< "${rows}"

  log "pr-env reaper: considered=${considered} reaped=${reaped} retained=${retained} failed=${failed}"
  [[ "${failed}" -eq 0 ]]
}

main "$@"
