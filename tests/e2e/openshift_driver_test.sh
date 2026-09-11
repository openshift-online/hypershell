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
    *"get route hypershell-web-console -n test-team"*) printf '%s' 'console-test.apps.example.com' ;;
    *"get route keycloak -n test-team-keycloak"*) printf '%s' 'sso-test.apps.example.com' ;;
    *"get deployment hypershell-controller -n test-team"*) printf '%s' 'gw.test.example.com' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.hostnames[0]}"*) printf '%s' 'gw-a.gw.test.example.com' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.parentRefs[0].name}"*) printf '%s' 'shared-gateway' ;;
    *"get grpcroute openshell-gateway -n tenant-a -o jsonpath={.spec.parentRefs[0].namespace}"*) printf '%s' 'openshift-ingress' ;;
    *"get gateway shared-gateway -n openshift-ingress"*) printf '%s\n' 'Programmed=True' ;;
    *"get grpcroute openshell-gateway -n tenant-a"*) printf '%s\n' 'Accepted=True' ;;
    *"set env deployment/hypershell-controller -n test-team -c controller GATEWAY_NAMESPACE_GC_INTERVAL=30s GATEWAY_NAMESPACE_GC_GRACE_PERIOD=30s"*) : ;;
    *"rollout status deployment/hypershell-controller -n test-team --timeout=120s"*) : ;;
    *"set env deployment/hypershell-controller -n test-team -c controller GATEWAY_NAMESPACE_GC_INTERVAL- GATEWAY_NAMESPACE_GC_GRACE_PERIOD-"*) : ;;
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

discover_console_host
assert_eq 'https://console-test.apps.example.com' "${_DISCOVER_CONSOLE_HOST}" 'Console Route discovery'

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

if configure_namespace_gc_timing >/dev/null; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: configure_namespace_gc_timing did not succeed'
fi
assert_eq '1' "${_GC_TIMING_PATCHED}" 'namespace GC timing marked patched'

if restore_namespace_gc_timing >/dev/null; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: restore_namespace_gc_timing did not succeed'
fi
assert_eq '' "${_GC_TIMING_PATCHED}" 'namespace GC timing patch cleared after restore'

# --- E2E_OIDC_GRANT dispatch (ephemeral-pr-environments.spec.md, PR-ENV-10) ---
# Replace the reachability stub with a token-endpoint stub that captures the POST
# body and returns a JSON access token so acquire_oidc_token parses successfully.
E2E_OIDC_ISSUER='https://sso-test.apps.example.com/realms/hypershell'
E2E_OIDC_USERNAME='admin'
E2E_OIDC_PASSWORD='admin'
E2E_OIDC_CLIENT_ID='hypershell-frontend'
E2E_OIDC_SA_CLIENT_ID='hypershell-e2e'
E2E_OIDC_SA_CLIENT_SECRET='s3cr3t'
# The production token request runs curl inside $(...), a subshell, so capture the
# request body through a file that survives the subshell rather than a variable.
CURL_CAPTURE="$(mktemp)"
curl() {
  printf '%s' "$*" >"${CURL_CAPTURE}"
  printf '%s' '{"access_token":"stub.jwt.token"}'
}
captured_curl_args() { printf ' %s ' "$(cat "${CURL_CAPTURE}")"; }

# Default grant is the resource-owner password grant against the seeded user.
E2E_OIDC_GRANT=password acquire_oidc_token >/dev/null
case "$(captured_curl_args)" in
  *' grant_type=password '*' client_id=hypershell-frontend '*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); printf 'FAIL: password grant args (got=%q)\n' "$(captured_curl_args)" ;;
esac

# Admin client_credentials path uses the hypershell-e2e service-account client.
E2E_OIDC_GRANT=client_credentials acquire_oidc_token >/dev/null
case "$(captured_curl_args)" in
  *' grant_type=client_credentials '*' client_id=hypershell-e2e '*' client_secret=s3cr3t '*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); printf 'FAIL: client_credentials admin args (got=%q)\n' "$(captured_curl_args)" ;;
esac

# Developer client_credentials path impersonates the principal via token exchange,
# scoped to the requested audience (here a per-gateway client id).
E2E_OIDC_GRANT=client_credentials acquire_oidc_token developer developer openshell-gw-1 >/dev/null
case "$(captured_curl_args)" in
  *'grant_type=urn:ietf:params:oauth:grant-type:token-exchange'*' requested_subject=developer '*' audience=openshell-gw-1 '*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); printf 'FAIL: token-exchange developer args (got=%q)\n' "$(captured_curl_args)" ;;
esac

# Admin + per-gateway client must also token-exchange. Straight client_credentials
# on hypershell-e2e never carries openshell-admin for that gateway client
# (e2e-testing.spec.md acquire_gateway_token_with_role).
E2E_OIDC_GRANT=client_credentials acquire_oidc_token admin admin openshell-gw-1 >/dev/null
case "$(captured_curl_args)" in
  *'grant_type=urn:ietf:params:oauth:grant-type:token-exchange'*' requested_subject=admin '*' audience=openshell-gw-1 '*) PASS=$((PASS + 1)) ;;
  *) FAIL=$((FAIL + 1)); printf 'FAIL: token-exchange admin gateway args (got=%q)\n' "$(captured_curl_args)" ;;
esac

# client_credentials without the service-account secret fails fast.
if (E2E_OIDC_GRANT=client_credentials E2E_OIDC_SA_CLIENT_SECRET='' acquire_oidc_token >/dev/null 2>&1); then
  FAIL=$((FAIL + 1)); echo 'FAIL: client_credentials without secret was accepted'
else
  PASS=$((PASS + 1))
fi

# An unrecognized grant is rejected rather than silently defaulting.
if (E2E_OIDC_GRANT=totp acquire_oidc_token >/dev/null 2>&1); then
  FAIL=$((FAIL + 1)); echo 'FAIL: unknown E2E_OIDC_GRANT was accepted'
else
  PASS=$((PASS + 1))
fi

printf 'OpenShift driver tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
