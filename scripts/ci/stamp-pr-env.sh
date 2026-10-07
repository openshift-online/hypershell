#!/usr/bin/env bash
# stamp-pr-env.sh - stamp the namespace group (platform + keycloak) with the
# ownership labels and the timebox annotation after a successful
# `make openshift-up` (ephemeral-pr-environments.spec.md: Per-Pull-Request
# Environment Identity, Timebox and Reaping).
#
# `make openshift-up` assigns an opaque environment id and does NOT write the
# timebox; this script overwrites the environment id with the scheme
# pr_env_is_reapable expects (pr-<number>, main-<sha>, or mq-<sha>, matching
# resolve-openshift-namespace.sh) and stamps hypershell.redhat.io/expires-at.
# A retained pull request gets PR_ENV_RETAINED_MAX_HOURS (default 72); an
# unretained pull request gets PR_ENV_UNRETAINED_MAX_HOURS (default 24); a
# push-to-main or merge-queue deploy -- never retained, never reviewed the way
# a pull request is -- gets the much shorter PR_ENV_MAIN_MQ_MAX_HOURS (default
# 4) since the reaper is only a backstop for a crashed in-run teardown, not a
# lifetime. It fails closed: if labeling or annotating either namespace fails,
# the whole workflow must fail and NOT leave an unlabeled environment (the
# local-dev warn-and-continue path does not apply to CI).
#
# Environment:
#   PR_NUMBER                    pull-request number; empty on push to main
#                                or a merge-queue entry
#   GITHUB_EVENT_NAME            GitHub Actions default; selects main vs
#                                merge-queue naming when PR_NUMBER is empty
#   GITHUB_SHA                   full commit SHA; required when PR_NUMBER is
#                                empty
#   PR_ENV_RETAINED              "true" to stamp the retained max lifetime
#                                (pull request only)
#   PR_ENV_RETAINED_MAX_HOURS    retained PR max lifetime in hours (default 72)
#   PR_ENV_UNRETAINED_MAX_HOURS  unretained PR max lifetime in hours (default 24)
#   PR_ENV_MAIN_MQ_MAX_HOURS     main/merge-queue max lifetime in hours (default 4)
#   PR_ENV_EXPIRES_AT            optional RFC 3339 UTC expiry; when set, used
#                                as-is so the access comment and the namespace
#                                annotation share one timestamp
#   PR_ENV_KUBECTL               kubectl/oc binary (default: oc)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

KUBECTL="${PR_ENV_KUBECTL:-oc}"

if [[ -n "${PR_NUMBER:-}" ]]; then
  platform_ns="$(pr_env_namespace "${PR_NUMBER}")"
  env_id="$(pr_env_environment_id "${PR_NUMBER}")"
  if [[ -n "${PR_ENV_EXPIRES_AT:-}" ]]; then
    expires_at="${PR_ENV_EXPIRES_AT}"
  else
    expires_at="$(pr_env_inactivity_expires_at "${PR_ENV_RETAINED:-false}")"
  fi
elif [[ "${GITHUB_EVENT_NAME:-}" == "merge_group" ]]; then
  : "${GITHUB_SHA:?GITHUB_SHA is required when PR_NUMBER is empty}"
  platform_ns="$(pr_env_merge_queue_namespace "${GITHUB_SHA}")"
  env_id="$(pr_env_merge_queue_environment_id "${GITHUB_SHA}")"
  expires_at="${PR_ENV_EXPIRES_AT:-$(pr_env_main_mq_expires_at)}"
else
  : "${GITHUB_SHA:?GITHUB_SHA is required when PR_NUMBER is empty}"
  platform_ns="$(pr_env_main_namespace "${GITHUB_SHA}")"
  env_id="$(pr_env_main_environment_id "${GITHUB_SHA}")"
  expires_at="${PR_ENV_EXPIRES_AT:-$(pr_env_main_mq_expires_at)}"
fi
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"

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
"${KUBECTL}" label namespace "${keycloak_ns}" \
  "${PR_ENV_CI_KEYCLOAK_LABEL}=${PR_ENV_CI_KEYCLOAK_VALUE}" \
  --overwrite

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
