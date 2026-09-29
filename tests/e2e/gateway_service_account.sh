#!/usr/bin/env bash
# Opt-in public-API machine flow. Existing identity-token paths remain default.
: "${E2E_GATEWAY_AUTH:=identity}"
GATEWAY_SA_DIR=""

e2e_validate_gateway_auth() {
  case "$E2E_GATEWAY_AUTH" in
    identity) ;;
    service_account)
      if [[ "$E2E_MODE" != "short" ]]; then
        red "ERROR: E2E_GATEWAY_AUTH=service_account requires E2E_MODE=short"
        return 1
      fi
      ;;
    *) red "ERROR: E2E_GATEWAY_AUTH must be identity or service_account"; return 1 ;;
  esac
}

gateway_service_account() (
  set +x
  export API_HOST GW_ID GW_KC_CLIENT_ID E2E_OIDC_ISSUER E2E_INFRA_DRIVER
  export GW_CONFIG_DIR
  export E2E_GATEWAY_API_TOKEN="${_OIDC_ACCESS_TOKEN:-}"
  exec python3 "${SCRIPT_DIR}/gateway_service_account.py" "$1" "${GATEWAY_SA_DIR}/credential.json" "${@:2}"
)

# Renew short-lived gateway tokens before CLI calls. Only the one-gateway token
# is written to the CLI config; the management credential never reaches it.
install_gateway_service_account_cli() {
  export GW_ID GW_KC_CLIENT_ID E2E_OIDC_ISSUER E2E_INFRA_DRIVER GW_CONFIG_DIR
  # Both layers exec, preserving the background CLI PID for teardown signals.
  # No credentials are embedded in the wrapper or passed on its command line.
  {
    printf '#!/usr/bin/env bash\nset -euo pipefail\nexec python3 '
    printf '%q ' "${SCRIPT_DIR}/gateway_service_account.py" run "${GATEWAY_SA_DIR}/credential.json" "${OPENSHELL_BIN}"
    printf '"$@"\n'
  } > "${GATEWAY_SA_DIR}/openshell"
  chmod 700 "${GATEWAY_SA_DIR}/openshell"
  OPENSHELL_BIN="${GATEWAY_SA_DIR}/openshell"
}
