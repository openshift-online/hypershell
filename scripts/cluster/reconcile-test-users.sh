#!/usr/bin/env bash
# Reconcile or de-seed the three Keycloak test-tier principals
# (admin, developer, platform-admin) as ephemeral-test-credentials.spec.md
# defines. Source this file; do not execute it.
#
# Environment:
#   KEYCLOAK_BASE_URL              Keycloak origin, no trailing slash (required)
#   KC_BOOTSTRAP_ADMIN_USERNAME    Master-realm admin (default: admin)
#   KC_BOOTSTRAP_ADMIN_PASSWORD    Master-realm admin password (default: admin)
#   KEYCLOAK_CURL_INSECURE         Set to true to pass curl -k (Kind)
#   TEST_USER_PASSWORD_SOURCE      static | secret (default: static)
#   TEST_USER_SECRET_NAMESPACE     Keycloak namespace holding the ESO Secret
#   TEST_USER_SECRET_NAME          default: hypershell-e2e-test-users
#   TEST_USER_OC                   oc/kubectl binary (default: oc)
#   TEST_USER_FAIL_CLOSED          true to exit non-zero on failure (CI-owned)

: "${KC_BOOTSTRAP_ADMIN_USERNAME:=admin}"
: "${KC_BOOTSTRAP_ADMIN_PASSWORD:=admin}"
: "${TEST_USER_PASSWORD_SOURCE:=static}"
: "${TEST_USER_SECRET_NAME:=hypershell-e2e-test-users}"
: "${TEST_USER_OC:=oc}"

TEST_TIER_USERNAMES=(admin developer platform-admin)

_test_user_curl() {
  local -a args=(-sS -m 15)
  if [[ "${KEYCLOAK_CURL_INSECURE:-}" == "true" ]]; then
    args+=(-k)
  fi
  curl "${args[@]}" "$@"
}

_test_user_roles() {
  case "$1" in
    admin) printf '%s\n' hypershell-admins hypershell-users gateway:creator platform:admin ;;
    developer) printf '%s\n' hypershell-users ;;
    platform-admin) printf '%s\n' hypershell-users platform:admin ;;
    *) return 1 ;;
  esac
}

_test_user_profile() {
  case "$1" in
    admin) printf '%s\t%s\t%s' Admin User admin@hypershell.local ;;
    developer) printf '%s\t%s\t%s' Developer User developer@hypershell.local ;;
    platform-admin) printf '%s\t%s\t%s' Platform Administrator platform-admin@hypershell.local ;;
    *) return 1 ;;
  esac
}

keycloak_admin_token() {
  local base="${KEYCLOAK_BASE_URL:?KEYCLOAK_BASE_URL is required}"
  local resp
  resp="$(_test_user_curl -X POST "${base%/}/realms/master/protocol/openid-connect/token" \
    -d "grant_type=password" \
    -d "client_id=admin-cli" \
    -d "username=${KC_BOOTSTRAP_ADMIN_USERNAME}" \
    -d "password=${KC_BOOTSTRAP_ADMIN_PASSWORD}" 2>/dev/null || true)"
  printf '%s' "${resp}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true
}

_read_secret_key() {
  local ns="$1" name="$2" key="$3"
  "${TEST_USER_OC}" get secret "${name}" -n "${ns}" \
    -o "jsonpath={.data.${key}}" 2>/dev/null | base64 -d 2>/dev/null || true
}

# Sets TEST_USER_PASSWORD_<name> for each principal. Does not print values.
load_test_user_passwords() {
  local username password
  unset TEST_USER_PASSWORD_admin TEST_USER_PASSWORD_developer TEST_USER_PASSWORD_platform_admin
  case "${TEST_USER_PASSWORD_SOURCE}" in
    static)
      for username in "${TEST_TIER_USERNAMES[@]}"; do
        printf -v "TEST_USER_PASSWORD_${username//-/_}" '%s' "${username}"
      done
      ;;
    secret)
      if [[ -z "${TEST_USER_SECRET_NAMESPACE:-}" ]]; then
        echo "TEST_USER_SECRET_NAMESPACE is required when TEST_USER_PASSWORD_SOURCE=secret" >&2
        return 1
      fi
      for username in "${TEST_TIER_USERNAMES[@]}"; do
        password="$(_read_secret_key "${TEST_USER_SECRET_NAMESPACE}" "${TEST_USER_SECRET_NAME}" "${username}")"
        if [[ -z "${password}" ]]; then
          echo "Secret ${TEST_USER_SECRET_NAME} in ${TEST_USER_SECRET_NAMESPACE} is missing key ${username}" >&2
          return 1
        fi
        if [[ "${password}" == "${username}" ]]; then
          echo "Secret ${TEST_USER_SECRET_NAME} key ${username} equals the username; refusing guessable password" >&2
          return 1
        fi
        if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
          echo "::add-mask::${password}"
        fi
        printf -v "TEST_USER_PASSWORD_${username//-/_}" '%s' "${password}"
      done
      ;;
    *)
      echo "Unknown TEST_USER_PASSWORD_SOURCE '${TEST_USER_PASSWORD_SOURCE}' (valid: static, secret)" >&2
      return 1
      ;;
  esac
}

_password_for() {
  local var="TEST_USER_PASSWORD_${1//-/_}"
  printf '%s' "${!var:-}"
}

_lookup_user_id() {
  local token="$1" username="$2"
  local base="${KEYCLOAK_BASE_URL%/}"
  _test_user_curl -H "Authorization: Bearer ${token}" \
    "${base}/admin/realms/hypershell/users?username=${username}&exact=true" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true
}

_assign_realm_role() {
  local token="$1" user_id="$2" role="$3"
  local base="${KEYCLOAK_BASE_URL%/}"
  local role_json role_id role_name code encoded
  encoded="$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "${role}")"
  role_json="$(_test_user_curl -H "Authorization: Bearer ${token}" \
    "${base}/admin/realms/hypershell/roles/${encoded}" 2>/dev/null || true)"
  role_id="$(printf '%s' "${role_json}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)"
  role_name="$(printf '%s' "${role_json}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)"
  if [[ -z "${role_id}" || -z "${role_name}" ]]; then
    echo "Keycloak realm role not found: ${role}" >&2
    return 1
  fi
  code="$(_test_user_curl -o /dev/null -w '%{http_code}' -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    "${base}/admin/realms/hypershell/users/${user_id}/role-mappings/realm" \
    -d "[{\"id\":\"${role_id}\",\"name\":\"${role_name}\"}]" 2>/dev/null || true)"
  if [[ "${code}" != "204" && "${code}" != "200" ]]; then
    echo "Failed to assign realm role ${role} (HTTP ${code})" >&2
    return 1
  fi
}

_create_or_reset_user() {
  local token="$1" username="$2" password="$3"
  local base="${KEYCLOAK_BASE_URL%/}"
  local user_id first last email profile rest code
  profile="$(_test_user_profile "${username}")"
  first="${profile%%$'\t'*}"
  rest="${profile#*$'\t'}"
  last="${rest%%$'\t'*}"
  email="${rest#*$'\t'}"

  user_id="$(_lookup_user_id "${token}" "${username}")"
  if [[ -z "${user_id}" ]]; then
    code="$(_test_user_curl -o /dev/null -w '%{http_code}' -X POST \
      -H "Authorization: Bearer ${token}" \
      -H "Content-Type: application/json" \
      "${base}/admin/realms/hypershell/users" \
      -d "$(python3 -c 'import json,sys; print(json.dumps({
        "username": sys.argv[1],
        "firstName": sys.argv[2],
        "lastName": sys.argv[3],
        "email": sys.argv[4],
        "emailVerified": True,
        "enabled": True,
        "credentials": [{"type": "password", "value": sys.argv[5], "temporary": False}],
      }))' "${username}" "${first}" "${last}" "${email}" "${password}")" 2>/dev/null || true)"
    if [[ "${code}" != "201" && "${code}" != "204" ]]; then
      echo "Failed to create Keycloak user ${username} (HTTP ${code})" >&2
      return 1
    fi
    user_id="$(_lookup_user_id "${token}" "${username}")"
  else
    code="$(_test_user_curl -o /dev/null -w '%{http_code}' -X PUT \
      -H "Authorization: Bearer ${token}" \
      -H "Content-Type: application/json" \
      "${base}/admin/realms/hypershell/users/${user_id}/reset-password" \
      -d "$(python3 -c 'import json,sys; print(json.dumps({
        "type": "password", "value": sys.argv[1], "temporary": False
      }))' "${password}")" 2>/dev/null || true)"
    if [[ "${code}" != "204" && "${code}" != "200" ]]; then
      echo "Failed to reset password for Keycloak user ${username} (HTTP ${code})" >&2
      return 1
    fi
  fi
  if [[ -z "${user_id}" ]]; then
    echo "Keycloak user ${username} was not found after create" >&2
    return 1
  fi
  local role
  while IFS= read -r role; do
    [[ -z "${role}" ]] && continue
    _assign_realm_role "${token}" "${user_id}" "${role}" || return 1
  done < <(_test_user_roles "${username}")
}

keycloak_reconcile_test_users() {
  local token="" username password
  local i
  if [[ -z "${KEYCLOAK_BASE_URL:-}" ]]; then
    echo "KEYCLOAK_BASE_URL is required" >&2
    return 1
  fi
  load_test_user_passwords || return 1
  for i in $(seq 1 30); do
    token="$(keycloak_admin_token)"
    if [[ -n "${token}" ]]; then
      break
    fi
    sleep 2
  done
  if [[ -z "${token}" ]]; then
    echo "Could not obtain Keycloak master admin token" >&2
    return 1
  fi
  for username in "${TEST_TIER_USERNAMES[@]}"; do
    password="$(_password_for "${username}")"
    if [[ -z "${password}" ]]; then
      echo "No password loaded for ${username}" >&2
      return 1
    fi
    _create_or_reset_user "${token}" "${username}" "${password}" || return 1
  done
}

# Idempotent: a missing user is success.
keycloak_de_seed_test_users() {
  local token username user_id code
  if [[ -z "${KEYCLOAK_BASE_URL:-}" ]]; then
    echo "KEYCLOAK_BASE_URL is required" >&2
    return 1
  fi
  token="$(keycloak_admin_token)"
  if [[ -z "${token}" ]]; then
    echo "Could not obtain Keycloak master admin token to de-seed test users" >&2
    return 1
  fi
  for username in "${TEST_TIER_USERNAMES[@]}"; do
    user_id="$(_lookup_user_id "${token}" "${username}")"
    if [[ -z "${user_id}" ]]; then
      continue
    fi
    code="$(_test_user_curl -o /dev/null -w '%{http_code}' -X DELETE \
      -H "Authorization: Bearer ${token}" \
      "${KEYCLOAK_BASE_URL%/}/admin/realms/hypershell/users/${user_id}" 2>/dev/null || true)"
    if [[ "${code}" != "204" && "${code}" != "200" && "${code}" != "404" ]]; then
      echo "Failed to delete Keycloak user ${username} (HTTP ${code})" >&2
      return 1
    fi
  done
}
