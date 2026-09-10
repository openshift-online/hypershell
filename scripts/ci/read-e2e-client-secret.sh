#!/usr/bin/env bash
# read-e2e-client-secret.sh - read the hypershell-e2e confidential client secret
# from the deployed per-PR Keycloak namespace (ephemeral-pr-environments.spec.md:
# Automated E2E Authentication).
#
# The e2e suite authenticates through the hypershell-e2e client (client
# credentials for the admin path, token exchange for the developer path), never
# through a brokered GitHub user's password grant. The client secret is per-PR:
# it is the same value the workflow put in the hypershell-github-oauth Secret in
# the Keycloak namespace, which the realm import substitutes into the
# hypershell-e2e client via the ${HYPERSHELL_E2E_CLIENT_SECRET} placeholder. This
# script prints that secret to stdout so the workflow can mask it and export
# E2E_OIDC_SA_CLIENT_SECRET; it must never be echoed into logs, the pull-request
# comment, or a public artifact.
#
# Environment:
#   PR_NUMBER                 pull-request number (required)
#   PR_ENV_E2E_SECRET_NAME    Secret name (default: hypershell-github-oauth)
#   PR_ENV_E2E_SECRET_KEY     Secret data key (default: e2e-client-secret)
#   PR_ENV_KUBECTL            kubectl/oc binary (default: oc)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

KUBECTL="${PR_ENV_KUBECTL:-oc}"
: "${PR_NUMBER:?PR_NUMBER is required}"
secret_name="${PR_ENV_E2E_SECRET_NAME:-hypershell-github-oauth}"
secret_key="${PR_ENV_E2E_SECRET_KEY:-e2e-client-secret}"

platform_ns="$(pr_env_namespace "${PR_NUMBER}")"
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"

encoded="$("${KUBECTL}" get secret "${secret_name}" -n "${keycloak_ns}" \
  -o "jsonpath={.data.${secret_key}}" 2>/dev/null || true)"

if [[ -z "${encoded}" ]]; then
  {
    echo "ERROR: could not read ${secret_key} from Secret ${secret_name} in ${keycloak_ns}."
    echo "The workflow's 'Provision GitHub OAuth secret' step must create this Secret"
    echo "(with the e2e-client-secret key) before Keycloak boots and e2e runs."
  } >&2
  exit 1
fi

printf '%s' "${encoded}" | base64 -d
