#!/usr/bin/env bash
# stamp-pr-env.sh - stamp the per-PR namespace group with the ownership labels
# and the timebox annotation after a successful `make openshift-up`
# (ephemeral-pr-environments.spec.md: Per-Pull-Request Environment Identity,
# Timebox and Reaping).
#
# `make openshift-up` assigns an opaque environment id and does NOT write the
# timebox; this script overwrites the environment id with pr-<number> so the
# reaper can attribute the group to its pull request, and stamps
# hypershell.redhat.io/expires-at <timebox> in the future. It fails closed: if
# labeling or annotating either namespace fails, the whole workflow must fail and
# NOT leave an unlabeled or un-timeboxed environment (the local-dev
# warn-and-continue path does not apply to CI).
#
# Environment:
#   PR_NUMBER             pull-request number (required)
#   PR_ENV_TIMEBOX_DAYS   timebox in days (default 3)
#   PR_ENV_KUBECTL        kubectl/oc binary (default: oc)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

KUBECTL="${PR_ENV_KUBECTL:-oc}"
: "${PR_NUMBER:?PR_NUMBER is required}"

platform_ns="$(pr_env_namespace "${PR_NUMBER}")"
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"
env_id="$(pr_env_environment_id "${PR_NUMBER}")"
expires_at="$(pr_env_expires_at "${PR_ENV_TIMEBOX_DAYS:-3}")"

stamp_namespace() {
  local ns="$1"
  echo "Stamping ${ns} (environment=${env_id}, expires-at=${expires_at})"
  "${KUBECTL}" label namespace "${ns}" \
    "${PR_ENV_OWNED_LABEL}=true" \
    "${PR_ENV_ENVIRONMENT_LABEL}=${env_id}" \
    "${PR_ENV_MANAGED_LABEL}=${PR_ENV_MANAGED_VALUE}" \
    "${PR_ENV_PART_OF_LABEL}=${PR_ENV_PART_OF_VALUE}" \
    --overwrite
  "${KUBECTL}" annotate namespace "${ns}" \
    "${PR_ENV_EXPIRES_ANNOTATION}=${expires_at}" \
    --overwrite
}

stamp_namespace "${platform_ns}"
stamp_namespace "${keycloak_ns}"

# Publish the resolved facts for later workflow steps (comment, summary).
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "platform_namespace=${platform_ns}"
    echo "keycloak_namespace=${keycloak_ns}"
    echo "environment_id=${env_id}"
    echo "expires_at=${expires_at}"
  } >> "${GITHUB_OUTPUT}"
fi

echo "Stamped namespace group ${platform_ns} + ${keycloak_ns}"
