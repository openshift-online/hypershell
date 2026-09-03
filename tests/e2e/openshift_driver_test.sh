#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
# shellcheck source=drivers/openshift.sh
source "${SCRIPT_DIR}/drivers/openshift.sh"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "$want" == "$got" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "$label" "$want" "$got"
  fi
}

OPENSHIFT_NAMESPACE=test-team

oc() {
  local args="$*"
  case "$args" in
    "project -q") printf '%s' "${OC_PROJECT:-}" ;;
    *"get route hypershell-api -n test-team"*) printf '%s' 'api-test.apps.example.com' ;;
    *"get route keycloak -n test-team-keycloak"*) printf '%s' 'sso-test.apps.example.com' ;;
    *"get deployment hypershell-controller -n test-team"*) printf '%s' 'gw.test.example.com' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.hostnames[0]}"*) printf '%s' 'gw-a.gw.test.example.com' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.parentRefs[0].name}"*) printf '%s' 'shared-gateway' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.parentRefs[0].namespace}"*) printf '%s' 'openshift-ingress' ;;
    *"get gateway shared-gateway -n openshift-ingress"*) printf '%s\n' 'Programmed=True' ;;
    *"get grpcroute openshell-gateway -n tenant-a"*) printf '%s\n' 'Accepted=True' ;;
    *) return 1 ;;
  esac
}

CURL_ARGS=""
curl() {
  CURL_ARGS="$*"
  printf '%s' '401'
}

discover_api_host
assert_eq 'https://api-test.apps.example.com' "${_DISCOVER_API_HOST}" 'API Route discovery'
assert_eq 'https://sso-test.apps.example.com/realms/hypershell' "${E2E_OIDC_ISSUER}" 'Keycloak issuer discovery'
assert_eq 'test-team-keycloak' "${E2E_KEYCLOAK_NAMESPACE}" 'Keycloak namespace derivation'
assert_eq 'gw.test.example.com' "$(get_cluster_domain)" 'configured gateway domain'
assert_eq 'oc' "$(get_cli_binary)" 'OpenShift CLI'

discover_gateway_endpoint gw-a tenant-a
assert_eq 'https://gw-a.gw.test.example.com:443' "${_DISCOVER_GW_ENDPOINT}" 'Gateway API endpoint discovery'

E2E_PROVISION_TIMEOUT=1
wait_for_gateway_route gw-a tenant-a
PASS=$((PASS + 1))

if [[ " ${CURL_ARGS} " == *' -k '* || " ${CURL_ARGS} " == *' --insecure '* ]]; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift curl disabled TLS verification'
else
  PASS=$((PASS + 1))
fi

resolved_namespace="$(unset OPENSHIFT_NAMESPACE; OC_PROJECT=current-project; _openshift_require_config >/dev/null; printf '%s' "${OPENSHIFT_NAMESPACE}")"
assert_eq 'current-project' "${resolved_namespace}" 'current oc project selects OpenShift E2E namespace'

if (unset OPENSHIFT_NAMESPACE; OC_PROJECT=; _openshift_require_config >/dev/null 2>&1); then
  FAIL=$((FAIL + 1))
  echo 'FAIL: missing OPENSHIFT_NAMESPACE and oc project were accepted'
else
  PASS=$((PASS + 1))
fi

printf 'OpenShift driver tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
