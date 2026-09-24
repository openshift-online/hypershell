#!/usr/bin/env bash
# Ensure the per-PR hypershell-e2e client secret exists in the Keycloak
# namespace. This secret is generated per environment and is not in AWS
# Secrets Manager (ephemeral-pr-environments.spec.md, ephemeral-ci-secrets.spec.md).
#
# Organization name and allowlist are non-secret configuration copied from
# workflow vars into this same object so Keycloak and the BFF can read them
# without putting them in the ESO-owned OAuth Secret.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

KUBECTL="${PR_ENV_KUBECTL:-oc}"
: "${PR_NUMBER:?PR_NUMBER is required}"
: "${PR_ENV_GITHUB_ORG:=openshift-online}"
: "${PR_ENV_E2E_CLIENT_SECRET_NAME:=hypershell-e2e-client}"

platform_ns="$(pr_env_namespace "${PR_NUMBER}")"
keycloak_ns="$(pr_env_keycloak_namespace "${platform_ns}")"

existing="$("${KUBECTL}" get secret "${PR_ENV_E2E_CLIENT_SECRET_NAME}" -n "${keycloak_ns}" \
  -o jsonpath='{.data.e2e-client-secret}' 2>/dev/null || true)"
if [[ -n "${existing}" ]]; then
  e2e_secret="$(printf '%s' "${existing}" | base64 -d)"
else
  e2e_secret="$(openssl rand -hex 24)"
fi

if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
  echo "::add-mask::${e2e_secret}"
fi

"${KUBECTL}" create secret generic "${PR_ENV_E2E_CLIENT_SECRET_NAME}" -n "${keycloak_ns}" \
  --from-literal=e2e-client-enabled=true \
  --from-literal=e2e-client-secret="${e2e_secret}" \
  --from-literal=org="${PR_ENV_GITHUB_ORG}" \
  --from-literal=allowlist="${PR_ENV_GITHUB_ALLOWLIST:-}" \
  --dry-run=client -o yaml | "${KUBECTL}" apply -f - >/dev/null

echo "Ensured ${PR_ENV_E2E_CLIENT_SECRET_NAME} in ${keycloak_ns}"
unset e2e_secret existing
