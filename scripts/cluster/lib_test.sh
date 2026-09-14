#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

assert_ok() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected success)\n' "${label}"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected failure)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

# Exercise the destructive-path ownership predicate with a minimal OpenShift
# seam. The driver exits on an unsafe namespace, so this wrapper is a subshell.
verify_openshift_namespace_fixture() (
  # shellcheck source=drivers/openshift.sh
  source "${SCRIPT_DIR}/drivers/openshift.sh"
  namespace_exists() { return 0; }
  namespace_label_value() {
    case "$2" in
      "${OWNED_LABEL}") printf '%s' "${FIXTURE_OWNED:-}" ;;
      "${ENV_LABEL}") printf '%s' "${FIXTURE_ENV_ID:-}" ;;
    esac
  }
  OPENSHIFT_ENVIRONMENT_ID="${FIXTURE_EXPECTED_ENV_ID:-}"
  verify_owned_namespace fixture
)

# --- RFC 1123 / sanitization ---
assert_eq "kube-admin" "$(sanitize_dns_label 'kube:admin')" "sanitize kube:admin"
assert_eq "user-redhat-com" "$(sanitize_dns_label 'user@redhat.com')" "sanitize email"
assert_eq "alice" "$(sanitize_dns_label 'Alice')" "sanitize mixed case"
assert_ok "valid short label" validate_rfc1123_label "alice" 54
assert_ok "54-char label" validate_rfc1123_label "$(printf 'a%.0s' {1..54})" 54
assert_fail "55-char platform label" validate_rfc1123_label "$(printf 'a%.0s' {1..55})" 54
assert_fail "uppercase rejected" validate_rfc1123_label "Alice" 54
assert_fail "underscore rejected" validate_rfc1123_label "alice_dev" 54
assert_eq "tok" "$(printf '%s' '{"access_token":"tok","expires_in":60}' | json_string_field access_token)" "json_string_field access_token"
assert_eq "abc-id" "$(printf '%s' '[{"id":"abc-id","clientId":"hypershell-frontend"}]' | json_first_id)" "json_first_id"
# Presenters emit id before name. The old grep ("name" then "id" in one object)
# misses this shape and re-POSTs a second dev-gateway on every openshift-seed.
_api_list='{"kind":"GatewayList","page":1,"size":100,"total":1,"items":[{"id":"2FhMpQzXBzABC","kind":"Gateway","href":"/api/hypershell/v1/gateways/2FhMpQzXBzABC","created_at":"2026-09-14T00:00:00Z","updated_at":"2026-09-14T00:00:00Z","name":"dev-gateway","cluster_id":"c1","release_id":"r1"}]}'
assert_eq "2FhMpQzXBzABC" "$(printf '%s' "${_api_list}" | json_named_id dev-gateway)" \
  "json_named_id finds id-before-name list items"
assert_eq "id-default" "$(printf '%s' '{"items":[{"name":"other","id":"id-other"},{"name":"dev-gateway","id":"id-default"}]}' | json_named_id dev-gateway)" \
  "json_named_id finds name-before-id list items"
assert_eq "" "$(printf '%s' "${_api_list}" | json_named_id missing-gateway)" \
  "json_named_id is empty when the name is absent"
assert_eq "" "$(printf '%s' 'not-json' | json_named_id dev-gateway)" \
  "json_named_id is empty on invalid JSON"
_pretty_list='{
  "items": [
    {
      "id": "pretty-id",
      "name": "dev-gateway"
    }
  ]
}'
assert_eq "pretty-id" "$(printf '%s' "${_pretty_list}" | json_named_id dev-gateway)" \
  "json_named_id finds pretty-printed list items"
assert_ok "internal registry svc:port is cluster-local" \
  registry_host_is_cluster_local 'image-registry.openshift-image-registry.svc:5000'
assert_ok "cluster.local registry is cluster-local" \
  registry_host_is_cluster_local 'image-registry.openshift-image-registry.svc.cluster.local:5000'
assert_fail "quay.io is not cluster-local" registry_host_is_cluster_local 'quay.io'
assert_fail "apps route is not cluster-local" \
  registry_host_is_cluster_local 'default-route-openshift-image-registry.apps.example.com'
(
  unset SWAP_REGISTRY IMAGE_REGISTRY
  assert_fail "unset SWAP_REGISTRY is an error" require_swap_registry
)
SWAP_REGISTRY='quay.io/example' \
  assert_ok "set SWAP_REGISTRY org prefix is accepted" require_swap_registry
SWAP_REGISTRY='quay.io' \
  assert_fail "SWAP_REGISTRY without org is refused" require_swap_registry
SWAP_REGISTRY='image-registry.openshift-image-registry.svc:5000' \
  assert_fail "cluster-local SWAP_REGISTRY is refused" require_swap_registry
assert_eq "hypershell-api-server" "$(swap_default_repository api-server)" "default api-server repo"
assert_eq "hypershell-controller" "$(swap_default_repository control-plane)" "default control-plane repo"
assert_eq "hypershell-web-console" "$(swap_default_repository web-console)" "default web-console repo"
assert_eq "hypershell-api-server" "$(unset SWAP_REPOSITORY; swap_repository_for_component api-server)" \
  "api-server repo without override"
assert_eq "custom-api" "$(SWAP_REPOSITORY=custom-api swap_repository_for_component api-server)" \
  "SWAP_REPOSITORY overrides repo name"
assert_eq "quay.io/alice/hypershell-api-server" \
  "$(SWAP_REGISTRY=quay.io/alice SWAP_REPOSITORY= swap_image_repository api-server)" \
  "swap image is org prefix plus default repo"
assert_eq "quay.io/alice/custom-api" \
  "$(SWAP_REGISTRY=quay.io/alice SWAP_REPOSITORY=custom-api swap_image_repository api-server)" \
  "swap image uses SWAP_REPOSITORY override"
assert_eq "amd64" "$(SWAP_PLATFORM=linux/amd64 swap_target_goarch)" "SWAP_PLATFORM linux/amd64"
assert_eq "amd64" "$(SWAP_ARCH=x86_64 SWAP_PLATFORM= swap_target_goarch)" "SWAP_ARCH x86_64"
assert_eq "arm64" "$(SWAP_PLATFORM=linux/arm64 swap_target_goarch)" "SWAP_PLATFORM linux/arm64"
SWAP_PLATFORM=linux/ppc64le assert_fail "unsupported SWAP_PLATFORM" swap_target_goarch
case "$(uname -m)" in
  x86_64) _expected_build_arch=amd64 ;;
  aarch64|arm64) _expected_build_arch=arm64 ;;
  *) _expected_build_arch="" ;;
esac
if [[ -n "${_expected_build_arch}" ]]; then
  assert_eq "${_expected_build_arch}" "$(swap_build_goarch)" "laptop BUILDARCH matches uname -m"
fi
unset SWAP_PLATFORM SWAP_ARCH
assert_eq "sha256:d3f6ac0a7627fee89b55f34745e09fc64d0073e807719a66f6b4534a96541eb6" \
  "$(printf '%s\n' \
    'Copying blob sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0' \
    'Copying config sha256:ad4686094d8f0186ec8249fc4917b71faa2c1030d7b5a025c29f26e19d95c156' \
    'Writing manifest to image destination' \
    '6014ae9-hypershell-e2e-test: digest: sha256:d3f6ac0a7627fee89b55f34745e09fc64d0073e807719a66f6b4534a96541eb6 size: 1927' \
    | registry_digest_from_push_log)" \
  "push log parser takes digest: line not blob/config SHAs"
assert_eq "" \
  "$(printf '%s\n' \
    'Copying blob sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0' \
    'Copying config sha256:ad4686094d8f0186ec8249fc4917b71faa2c1030d7b5a025c29f26e19d95c156' \
    'Writing manifest to image destination' \
    'Storing signatures' \
    | registry_digest_from_push_log)" \
  "podman progress without a digest: line is not a registry digest"
assert_eq "/from-pull" \
  "$(PULL_SECRET=/from-pull KIND_PULL_SECRET=/from-kind resolved_pull_secret_path)" \
  "PULL_SECRET wins over KIND_PULL_SECRET"
assert_eq "/from-kind" \
  "$(PULL_SECRET= KIND_PULL_SECRET=/from-kind resolved_pull_secret_path)" \
  "KIND_PULL_SECRET is the PULL_SECRET alias"
_pull_auth="$(printf '%s' 'alice:s3cret' | base64 | tr -d '\n')"
_pull_dc="$(printf '%s' "{\"auths\":{\"quay.io\":{\"auth\":\"${_pull_auth}\"}}}" | base64 | tr -d '\n')"
_pull_secret="$(mktemp)"
cat > "${_pull_secret}" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: pull-secret
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: ${_pull_dc}
type: kubernetes.io/dockerconfigjson
EOF
_pull_user="$(registry_auth_json_for_host "${_pull_secret}" quay.io | python3 -c 'import json,sys; print(json.load(sys.stdin)["username"])')"
_pull_pass="$(registry_auth_json_for_host "${_pull_secret}" quay.io | python3 -c 'import json,sys; print(json.load(sys.stdin)["password"])')"
assert_eq "alice" "${_pull_user}" "pull secret username for quay.io"
assert_eq "s3cret" "${_pull_pass}" "pull secret password for quay.io"
_pull_https="$(registry_auth_json_for_host "${_pull_secret}" quay.io | python3 -c 'import json,sys; print(json.load(sys.stdin)["username"])')"
assert_eq "alice" "${_pull_https}" "pull secret matches quay.io host"
rm -f "${_pull_secret}"
merged="$(printf '%s' '{"id":"x","redirectUris":["https://console.hypershell.localhost/*"]}' | keycloak_client_with_console_redirects 'console.apps.example.com')"
assert_eq '["https://console.apps.example.com/auth/callback", "https://console.apps.example.com"]' \
  "$(printf '%s' "${merged}" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["redirectUris"]))')" \
  "keycloak_client_with_console_redirects replaces Kind localhost URIs"
_kc_patch="$(keycloak_route_env_patch 'https://keycloak.apps.example.com' 'web-console.apps.example.com')"
assert_eq 'https://keycloak.apps.example.com' \
  "$(printf '%s' "${_kc_patch}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["spec"]["template"]["spec"]["containers"][0]["env"][0]["value"])')" \
  "keycloak_route_env_patch sets KC_HOSTNAME on the keycloak container"
assert_eq 'keycloak' \
  "$(printf '%s' "${_kc_patch}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["spec"]["template"]["spec"]["containers"][0]["name"])')" \
  "keycloak_route_env_patch targets container keycloak"
assert_eq 'web-console.apps.example.com' \
  "$(printf '%s' "${_kc_patch}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["spec"]["template"]["spec"]["initContainers"][0]["env"][0]["value"])')" \
  "keycloak_route_env_patch sets HYPERSHELL_CONSOLE_HOST on render-realm-config"
assert_eq 'render-realm-config' \
  "$(printf '%s' "${_kc_patch}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["spec"]["template"]["spec"]["initContainers"][0]["name"])')" \
  "keycloak_route_env_patch targets init container render-realm-config"
assert_eq 'HYPERSHELL_CONSOLE_HOST' \
  "$(printf '%s' "${_kc_patch}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["spec"]["template"]["spec"]["initContainers"][0]["env"][0]["name"])')" \
  "keycloak_route_env_patch names the console-host env var"
if grep -A14 'Setting Keycloak KC_HOSTNAME' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'keycloak_route_env_patch'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: openshift-up does not stamp Keycloak route env via keycloak_route_env_patch'
fi
if grep -A14 'Setting Keycloak KC_HOSTNAME' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q -- '--type=strategic'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: openshift-up does not strategic-merge patch Keycloak route env'
fi
if grep -A14 'Setting Keycloak KC_HOSTNAME' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -Eq 'replace -f|set env deployment/keycloak'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: openshift-up still replaces or set-envs the Keycloak Deployment'
else
  PASS=$((PASS + 1))
fi
if grep -A25 'Recycle Keycloak when GitHub OAuth secret changes' "${REPO_ROOT}/.github/workflows/pr-environment.yml" | grep -q 'already matches; skip recycle'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: PR env workflow recycles Keycloak even when the oauth-secret hash is unchanged'
fi
if grep 'Keycloak:' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'admin/admin'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: Keycloak banner still prints admin/admin outside the test-user branch'
else
  PASS=$((PASS + 1))
fi
if grep -A3 '^cluster_teardown()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'cluster_down'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_teardown is not an alias of cluster_down'
fi
if grep -A25 '^remove_project()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q -- '--wait=false'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: remove_project deletes projects without waiting'
else
  PASS=$((PASS + 1))
fi
if grep -E 'deletion started|removal started' "${SCRIPT_DIR}/drivers/openshift.sh" >/dev/null; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift down still reports deletion as only started'
else
  PASS=$((PASS + 1))
fi
if grep -A30 '^verify_owned_namespace()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q '"${owned}" != "true" || -z "${env_id}"'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift down does not refuse unlabeled or partially labeled namespaces'
fi
if grep -A30 '^verify_owned_namespace()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q 'Refusing to delete it'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift down ownership verification does not fail closed'
fi
FIXTURE_OWNED= FIXTURE_ENV_ID= \
  assert_fail "unlabeled project is refused by openshift-down" verify_openshift_namespace_fixture
FIXTURE_OWNED=true FIXTURE_ENV_ID= \
  assert_fail "partially labeled project is refused by openshift-down" verify_openshift_namespace_fixture
FIXTURE_OWNED=true FIXTURE_ENV_ID=environment-a FIXTURE_EXPECTED_ENV_ID=environment-b \
  assert_fail "foreign environment is refused by openshift-down" verify_openshift_namespace_fixture
FIXTURE_OWNED=true FIXTURE_ENV_ID=environment-a FIXTURE_EXPECTED_ENV_ID=environment-a \
  assert_ok "owned environment is accepted by openshift-down" verify_openshift_namespace_fixture
if grep -A50 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'OPENSHIFT_KEYCLOAK_NAMESPACE'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down does not remove the Keycloak namespace'
fi
if grep -A50 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'clusterrolebinding "${prefix}hypershell-controller"'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down does not delete this environment'\''s prefixed ClusterRoleBinding'
fi
if grep -A50 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'clusterrole "${prefix}hypershell-controller"'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down does not delete this environment'\''s prefixed ClusterRole'
fi
if grep -A50 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'OPENSHIFT_NAMESPACE}-dev-'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down does not use the -dev- cluster-scoped prefix'
fi
if grep -A80 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'delete_instance_managed_namespaces'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down does not delete instance-managed gateway namespaces'
fi
if grep -A20 '^delete_instance_managed_namespaces()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'empty instance identity'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: delete_instance_managed_namespaces does not refuse an empty instance'
fi
_selector_got="$(bash -c '
  # shellcheck source=lib.sh
  source "'"${SCRIPT_DIR}"'/lib.sh"
  # shellcheck source=drivers/openshift.sh
  source "'"${SCRIPT_DIR}"'/drivers/openshift.sh"
  instance_managed_namespace_selector alice
')"
assert_eq \
  'hypershell.redhat.io/managed=true,app.kubernetes.io/managed-by=hypershell-control-plane,hypershell.redhat.io/instance=alice' \
  "${_selector_got}" \
  "instance selector stamps managed + managed-by + instance"
if grep -A80 '^cluster_down()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'still reaping instance-managed leftovers'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_down returns early when the platform project is already gone'
fi
if grep -E 'delete clusterrole(binding)? "hypershell-controller' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift down deletes unprefixed cluster-scoped names (would hit stage)'
else
  PASS=$((PASS + 1))
fi
if grep -A40 '^bind_existing_clusterrole()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'get clusterrole hypershell-controller' \
  && grep -A40 '^bind_existing_clusterrole()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'keep-role-refs'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: bind_existing_clusterrole does not look up ClusterRole hypershell-controller'
fi
if grep -q 'OPENSHIFT_USE_EXISTING_CLUSTERROLE' "${SCRIPT_DIR}/drivers/openshift.sh" "${REPO_ROOT}/Makefile"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OPENSHIFT_USE_EXISTING_CLUSTERROLE is still present; fallback bind replaced it'
else
  PASS=$((PASS + 1))
fi
if grep -A50 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'Applying cluster-scoped RBAC from deploy/openshift'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: default apply_cluster_rbac does not apply overlay ClusterRole/ClusterRoleBinding'
fi
if grep -A80 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'Falling back to existing ClusterRole hypershell-controller' \
  && grep -A80 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'bind_existing_clusterrole'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: apply_cluster_rbac does not fall back to ClusterRole hypershell-controller'
fi
if grep -A20 '^replace_clusterrolebinding_if_role_ref_differs()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'roleRef is immutable'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: leftover ClusterRoleBinding roleRef is not replaced before fallback bind'
fi
if grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'skip_seed'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_up does not honor SKIP_SEED'
fi
if grep -q 'extract_named_id' "${SCRIPT_DIR}/drivers/openshift.sh" \
  || grep -Fq '[^}]*"id"' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seed still greps name-then-id (misses API list JSON)'
else
  PASS=$((PASS + 1))
fi
if grep -q 'json_named_id local-openshift' "${SCRIPT_DIR}/drivers/openshift.sh" \
  && grep -q 'json_named_id dev-release' "${SCRIPT_DIR}/drivers/openshift.sh" \
  && grep -q 'json_named_id openshell-db' "${SCRIPT_DIR}/drivers/openshift.sh" \
  && grep -q 'json_named_id dev-gateway' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seed_via_api does not look up existing resources with json_named_id'
fi
if grep -Fq '[^}]*"id"' "${REPO_ROOT}/scripts/kind/seed.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: Kind seed still greps name-then-id (misses API list JSON)'
else
  PASS=$((PASS + 1))
fi
if grep -q 'json_named_id local-kind' "${REPO_ROOT}/scripts/kind/seed.sh" \
  && grep -q 'json_named_id dev-gateway' "${REPO_ROOT}/scripts/kind/seed.sh"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: Kind seed does not look up existing resources with json_named_id'
fi
if grep -B2 'db_provider="\$(effective_database_provider)"' "${SCRIPT_DIR}/drivers/openshift.sh" >/dev/null \
  && grep -A20 'Creating ManagedDatabase' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'provider='; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seed still hardcodes ManagedDatabase provider=cnpg'
fi
if grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'cutover_database_provider'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_up does not reconcile the database provider on cutover'
fi
if grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'TARGET_DB_PROVIDER="\$(effective_database_provider)"' \
  && grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'cutover_database_provider "\${TARGET_DB_PROVIDER}"'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_up still passes effective_database_provider via command substitution in argument position'
fi
if grep -A20 '^effective_database_provider()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'exit 1'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift effective_database_provider still uses exit 1 (ineffective inside command substitution)'
else
  PASS=$((PASS + 1))
fi
if grep -A25 '^cutover_database_provider()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'empty target'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cutover_database_provider does not guard against an empty target'
fi
if grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'restart_after_database_cutover'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_up does not restart components after a database cutover'
fi
if grep -A20 '^effective_database_provider()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'DATABASE_PROVIDER'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift effective_database_provider does not honor DATABASE_PROVIDER override'
fi
if awk '/^cutover_database_provider\(\)/,/^}/' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'delete secret hypershell-db-app'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cutover_database_provider does not clear the provider-shaped Secret'
fi
if grep -A30 '^wait_for_deployments()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'is_openshift_swapped'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: wait_for_deployments skips swapped components'
else
  PASS=$((PASS + 1))
fi
if grep -A30 '^wait_for_deployments()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'wait_for_named_rollout hypershell-api-server' \
  && grep -A30 '^wait_for_deployments()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'wait_for_named_rollout hypershell-controller' \
  && grep -A30 '^wait_for_deployments()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'wait_for_named_rollout hypershell-web-console'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: wait_for_deployments does not wait for platform rollouts'
fi
if grep -qE 'wait_for_oidc_token|wait_for_api_openapi|wait_for_api_healthcheck' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: wait_for_deployments still probes Routes for HTTP readiness'
else
  PASS=$((PASS + 1))
fi
if grep -A20 'containerPort: 9443' "${SCRIPT_DIR}/../../deploy/base/controller.yaml" | grep -q 'readinessProbe'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: control plane Deployment has no readiness probe on the provisioner port'
fi
if grep -A15 'Automatic seeding incomplete' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'seed_strict'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seeding does not honor SEED_STRICT'
fi
if grep -q 'fail_required_cluster_rbac' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift still fails the whole up when cluster RBAC cannot be applied'
else
  PASS=$((PASS + 1))
fi
assert_eq "alice-keycloak" "$(keycloak_namespace_for alice)" "keycloak namespace suffix"
assert_ok "derived keycloak ns fits 63" validate_rfc1123_label "$(keycloak_namespace_for "$(printf 'a%.0s' {1..54})")" 63
assert_eq "openshell.example.com" "$(gateway_base_domain_from_hostname '*.openshell.example.com')" "strip wildcard listener hostname"
assert_eq "openshell.example.com" "$(gateway_base_domain_from_hostname 'openshell.example.com')" "passthrough non-wildcard hostname"
assert_fail "alice is not reserved" is_reserved_cluster_namespace alice
assert_fail "hypershell-e2e-test is not reserved" is_reserved_cluster_namespace hypershell-e2e-test
assert_ok "default is reserved" is_reserved_cluster_namespace default
assert_ok "openshift-ingress is reserved" is_reserved_cluster_namespace openshift-ingress
assert_ok "kube-system is reserved" is_reserved_cluster_namespace kube-system

# --- Driver loading ---
CLUSTER_DRIVER=kind
assert_ok "kind driver loads" load_cluster_driver
declare -F cluster_up >/dev/null && PASS=$((PASS + 1)) || { FAIL=$((FAIL + 1)); echo 'FAIL: kind cluster_up missing'; }

CLUSTER_DRIVER=openshift
# Re-source a fresh shell function table by loading again in a subshell
assert_ok "openshift driver loads" bash -c '
  source "'"${SCRIPT_DIR}"'/lib.sh"
  CLUSTER_DRIVER=openshift
  load_cluster_driver
  declare -F cluster_up cluster_down cluster_teardown cluster_status cluster_seed component_swap component_revert >/dev/null
'

CLUSTER_DRIVER=not-a-driver
assert_fail "unknown driver rejected" load_cluster_driver

# --- Swap ledger ---
OPENSHIFT_NAMESPACE=test-alice
swap_root="$(mktemp -d)"
# Point the ledger at a temp dir by overriding REPO_ROOT via a subshell helper
ledger_test() {
  local tmp="$1"
  (
    # shellcheck source=lib.sh
    source "${SCRIPT_DIR}/lib.sh"
    REPO_ROOT="${tmp}"
    OPENSHIFT_NAMESPACE=test-alice
    track_openshift_swap api-server "image.example/test-alice/hypershell-api-server@sha256:abc"
    is_openshift_swapped api-server
    [[ "$(openshift_swap_image api-server)" == "image.example/test-alice/hypershell-api-server@sha256:abc" ]]
    ! is_openshift_swapped control-plane
    track_openshift_swap api-server "image.example/test-alice/hypershell-api-server@sha256:def"
    [[ "$(openshift_swap_image api-server)" == "image.example/test-alice/hypershell-api-server@sha256:def" ]]
    clear_openshift_swap api-server
    ! is_openshift_swapped api-server
  )
}
assert_ok "openshift swap ledger" ledger_test "${swap_root}"
rm -rf "${swap_root}"

# --- Namespace rewriter ---
fixture="$(mktemp)"
cat > "${fixture}" <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: hypershell-system
---
apiVersion: v1
kind: Namespace
metadata:
  name: keycloak
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
  namespace: keycloak
spec:
  template:
    spec:
      containers:
        - name: keycloak
          env:
            - name: KC_HOSTNAME
              value: https://keycloak.hypershell.localhost
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak-service
  namespace: keycloak
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hypershell-controller
  namespace: hypershell-system
spec:
  template:
    spec:
      containers:
        - name: controller
          env:
            - name: HYPERSHELL_NAMESPACE
              value: hypershell-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hypershell-controller
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hypershell-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: hypershell-controller
subjects:
  - kind: ServiceAccount
    name: hypershell-controller
    namespace: hypershell-system
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: sidecar-ref
  namespace: hypershell-system
data:
  image: quay.io/example/hypershell-system-sidecar:1
  dns: hypershell-api-server.hypershell-system.svc.cluster.local
EOF

rewritten="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak < "${fixture}")"
rm -f "${fixture}"

if printf '%s' "${rewritten}" | grep -qE '(^|[^A-Za-z0-9-])hypershell-system([^A-Za-z0-9-]|$)'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: leftover hypershell-system token after rewrite'
else
  PASS=$((PASS + 1))
fi
if printf '%s' "${rewritten}" | grep -q 'quay.io/example/hypershell-system-sidecar:1'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: hyphenated image name containing hypershell-system was rewritten'
fi
if printf '%s' "${rewritten}" | grep -q 'hypershell-api-server.alice.svc.cluster.local'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: in-cluster DNS hypershell-system was not rewritten'
fi
printf '%s' "${rewritten}" | grep -q $'kind: Namespace\nmetadata:\n  name: alice$' \
  || printf '%s' "${rewritten}" | grep -A2 'kind: Namespace' | grep -q 'name: alice'
if printf '%s' "${rewritten}" | grep -q 'name: alice$'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: platform namespace name not rewritten to alice'
fi
if printf '%s' "${rewritten}" | grep -q 'name: alice-keycloak'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: keycloak namespace name not rewritten'
fi
if printf '%s' "${rewritten}" | grep -q 'namespace: alice-keycloak'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: keycloak resource namespace not rewritten'
fi
if printf '%s' "${rewritten}" | grep -q 'name: keycloak-service'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: keycloak-service name was rewritten'
fi
if printf '%s' "${rewritten}" | grep -q 'name: alice-dev-hypershell-controller$'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRoleBinding name not prefixed with alice-dev-'
fi
if printf '%s' "${rewritten}" | grep -q 'controllerroleRef'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRoleBinding name was glued onto roleRef'
else
  PASS=$((PASS + 1))
fi
# Per-environment ClusterRole/ClusterRoleBinding: ${ns}-dev-${name}
if printf '%s' "${rewritten}" | grep -q 'kind: ClusterRole' \
  && printf '%s' "${rewritten}" | awk '/kind: ClusterRole$/{p=1} p&&/^  name:/{print; exit}' | grep -q 'name: alice-dev-hypershell-controller'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRole name should be alice-dev-hypershell-controller'
fi
if printf '%s' "${rewritten}" | grep -A4 'roleRef:' | grep -q 'name: alice-dev-hypershell-controller'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRoleBinding roleRef.name should be alice-dev-hypershell-controller'
fi
if printf '%s' "${rewritten}" | awk '/kind: ClusterRole$/{p=1} p&&/^  name:/{print; exit}' | grep -qx '  name: hypershell-controller'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRole metadata.name was left as hypershell-controller (would collide with stage)'
else
  PASS=$((PASS + 1))
fi
if printf '%s' "${rewritten}" | grep -q 'value: alice'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: HYPERSHELL_NAMESPACE value not rewritten'
fi

omit_fixture="$(mktemp)"
cat > "${omit_fixture}" <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: hypershell-system
---
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: hypershell-system
EOF
omitted="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces < "${omit_fixture}")"
rm -f "${omit_fixture}"
if printf '%s' "${omitted}" | grep -q 'kind: Namespace'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --omit-namespaces still contains Namespace'
else
  PASS=$((PASS + 1))
fi
if printf '%s' "${omitted}" | grep -q 'kind: Service'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --omit-namespaces dropped non-Namespace resources'
fi

only_kc="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --only-namespace alice-keycloak <<<"${rewritten}")"
if printf '%s' "${only_kc}" | grep -q 'namespace: alice-keycloak'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --only-namespace alice-keycloak dropped Keycloak resources'
fi
if printf '%s' "${only_kc}" | grep -q 'namespace: alice$'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --only-namespace alice-keycloak kept platform resources'
else
  PASS=$((PASS + 1))
fi
if printf '%s' "${only_kc}" | grep -q 'kind: ClusterRole'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --only-namespace alice-keycloak kept cluster-scoped resources'
else
  PASS=$((PASS + 1))
fi

only_plat="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --only-namespace alice \
  --include-cluster-scoped <<<"${rewritten}")"
if printf '%s' "${only_plat}" | grep -q 'kind: ClusterRole'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --include-cluster-scoped dropped ClusterRole'
fi
# ClusterRoleBinding prefix must not glue metadata.name onto roleRef
crb_fixture="$(mktemp)"
cat > "${crb_fixture}" <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: hypershell
  name: hypershell-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: hypershell-controller
subjects:
- kind: ServiceAccount
  name: hypershell-controller
  namespace: hypershell-system
EOF
crb_out="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --only-namespace __cluster__ \
  --include-cluster-scoped < "${crb_fixture}")"
rm -f "${crb_fixture}"
if printf '%s' "${crb_out}" | grep -qx '  name: alice-dev-hypershell-controller'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: kustomize-style ClusterRoleBinding metadata.name not prefixed on its own line'
  printf '%s\n' "${crb_out}"
fi
if printf '%s' "${crb_out}" | grep -qx 'roleRef:'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: roleRef was not kept as its own line'
fi
if printf '%s' "${crb_out}" | grep -A3 '^roleRef:' | grep -qx '  name: alice-dev-hypershell-controller'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRoleBinding roleRef.name should be alice-dev-hypershell-controller'
  printf '%s\n' "${crb_out}"
fi
keep_ref_out="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --keep-role-refs \
  --only-namespace __cluster__ \
  --include-cluster-scoped \
  --omit-kinds ClusterRole <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hypershell-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: hypershell-controller
subjects:
- kind: ServiceAccount
  name: hypershell-controller
  namespace: hypershell-system
EOF
)"
if printf '%s' "${keep_ref_out}" | grep -qx '  name: alice-dev-hypershell-controller' \
  && printf '%s' "${keep_ref_out}" | grep -A3 '^roleRef:' | grep -qx '  name: hypershell-controller'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --keep-role-refs should prefix ClusterRoleBinding name but leave roleRef hypershell-controller'
  printf '%s\n' "${keep_ref_out}"
fi

sys_crb="$(mktemp)"
cat > "${sys_crb}" <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hypershell-sandbox-scc
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:openshift:scc:privileged
subjects:
- kind: ServiceAccount
  name: openshell-gateway-sandbox
  namespace: hypershell-system
EOF
sys_out="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --only-namespace __cluster__ \
  --include-cluster-scoped < "${sys_crb}")"
rm -f "${sys_crb}"
if printf '%s' "${sys_out}" | grep -qx '  name: alice-dev-hypershell-sandbox-scc'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: ClusterRoleBinding to a built-in ClusterRole was not name-prefixed'
fi
if printf '%s' "${sys_out}" | grep -A3 '^roleRef:' | grep -qx '  name: system:openshift:scc:privileged'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: built-in system: ClusterRole roleRef was renamed'
  printf '%s\n' "${sys_out}"
fi
omit_cr="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --omit-kinds ClusterRole,ClusterRoleBinding \
  --only-namespace alice \
  --include-cluster-scoped <<<"${rewritten}")"
if printf '%s' "${omit_cr}" | grep -q 'kind: ClusterRole'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --omit-kinds kept ClusterRole'
else
  PASS=$((PASS + 1))
fi

only_cr="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --only-namespace __cluster__ \
  --include-cluster-scoped \
  --only-kinds ClusterRole <<<"${rewritten}")"
if printf '%s' "${only_cr}" | grep -q 'kind: ClusterRole'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --only-kinds ClusterRole dropped ClusterRole'
fi
if printf '%s' "${only_cr}" | grep -q 'kind: ClusterRoleBinding'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --only-kinds ClusterRole kept ClusterRoleBinding'
else
  PASS=$((PASS + 1))
fi

stripped="$(python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
  --platform-namespace alice \
  --keycloak-namespace alice-keycloak \
  --omit-namespaces \
  --strip-openshift-uids \
  < "${REPO_ROOT}/deploy/base/postgres.yaml")"
if printf '%s' "${stripped}" | grep -q 'runAsUser\|fsGroup'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: --strip-openshift-uids left runAsUser or fsGroup'
else
  PASS=$((PASS + 1))
fi
if printf '%s' "${stripped}" | grep -q 'runAsNonRoot: true'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: --strip-openshift-uids dropped runAsNonRoot'
fi

# --- Overlay still renders ---
if command -v kustomize >/dev/null 2>&1; then
  if kustomize build --load-restrictor=LoadRestrictionsNone "${REPO_ROOT}/deploy/openshift" >/dev/null; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo 'FAIL: kustomize build deploy/openshift'
  fi
  if kustomize build "${REPO_ROOT}/deploy/hub" >/dev/null; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo 'FAIL: kustomize build deploy/hub'
  fi
  os_out="$(mktemp)"
  if kustomize build --load-restrictor=LoadRestrictionsNone "${REPO_ROOT}/deploy/openshift" \
    | python3 "${SCRIPT_DIR}/rewrite-namespaces.py" \
      --platform-namespace alice \
      --keycloak-namespace alice-keycloak \
    > "${os_out}"; then
    if grep -q 'kind: Route' "${os_out}"; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: rewritten overlay missing Route'
    fi
    if grep -q 'name: alice-keycloak' "${os_out}"; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: rewritten overlay missing alice-keycloak namespace'
    fi
    kc_np="$(awk '/name: keycloak-allow-platform/,/^---$/' "${os_out}")"
    if printf '%s' "${kc_np}" | grep -q 'namespace: alice-keycloak' \
      && printf '%s' "${kc_np}" | grep -q 'kubernetes.io/metadata.name: alice'; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: rewritten overlay missing Keycloak allow-from-platform NetworkPolicy'
    fi
    if ! grep -q hypershell-system "${os_out}"; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: rewritten overlay still contains hypershell-system'
    fi
    unprefixed="$(python3 -c '
from importlib.machinery import SourceFileLoader
from pathlib import Path
import sys
rw = SourceFileLoader("rewrite", sys.argv[1]).load_module()
bad = rw.unprefixed_cluster_scoped(Path(sys.argv[2]).read_text(), "alice-dev-")
if bad:
    print("\n".join(bad))
    raise SystemExit(1)
' "${SCRIPT_DIR}/rewrite-namespaces.py" "${os_out}")" || true
    if [[ -z "${unprefixed}" ]]; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo "FAIL: rewritten overlay has unprefixed cluster-scoped names:"
      printf '%s\n' "${unprefixed}"
    fi
    if grep -q 'name: system:openshift:scc:privileged' "${os_out}"; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: namespaced SCC RoleBinding lost built-in system: roleRef'
    fi
    if grep -A1 'name: API_ENV' "${os_out}" | grep -q 'development_oidc'; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: OpenShift overlay does not set API_ENV=development_oidc'
    fi
    if grep -q 'name: HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR' "${os_out}" \
      && grep -q 'hypershell-controller.$(POD_NAMESPACE).svc.cluster.local:9443' "${os_out}" \
      && grep -B8 'name: HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR' "${os_out}" \
        | grep -q 'fieldPath: metadata.namespace'; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: OpenShift overlay dropped HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR FQDN'
    fi
    if grep -q 'hypershell-controller.hypershell-system.svc.cluster.local:9443' "${os_out}"; then
      FAIL=$((FAIL + 1))
      echo 'FAIL: provisioner addr still hardcodes hypershell-system instead of $(POD_NAMESPACE)'
    else
      PASS=$((PASS + 1))
    fi
    if grep -A1 'name: GATEWAY_API_HTTP_LISTENER_NAME' "${os_out}" | grep -q 'grpc'; then
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
      echo 'FAIL: OpenShift overlay does not set GATEWAY_API_HTTP_LISTENER_NAME=grpc'
    fi
  else
    FAIL=$((FAIL + 1))
    echo 'FAIL: rewritten overlay render'
  fi
  rm -f "${os_out}"
else
  warn "kustomize not installed; skipping overlay render checks"
fi

if grep -q 'API_ENV=development_oidc' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift driver still sets API_ENV imperatively; it belongs in deploy/openshift'
else
  PASS=$((PASS + 1))
fi
if grep -A80 '^login_swap_registry()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'oc registry'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap login still uses oc registry; it should push to SWAP_REGISTRY'
else
  PASS=$((PASS + 1))
fi
if grep -q 'start-build' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap still uses oc start-build; it should push to SWAP_REGISTRY from the laptop'
else
  PASS=$((PASS + 1))
fi
if grep -A30 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q 'swap_image_repository'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap push ref does not use swap_image_repository (SWAP_REGISTRY/org + default repo)'
fi
if grep -A80 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q -- '--digestfile'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap push does not record the registry digest via --digestfile'
fi
if grep -A80 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q "inspect -f '{{.Digest}}'"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap still pins inspect of the local image; that digest is not on the registry'
else
  PASS=$((PASS + 1))
fi
if grep -A80 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q 'TARGETARCH'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap build does not pass TARGETARCH for the cluster node architecture'
fi
if grep -A80 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q 'BUILDARCH'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap build does not pass BUILDARCH for the laptop architecture'
fi
if grep -A80 '^push_component_image()' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q -- '--platform'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap build does not pass --platform linux/<arch>'
fi
if grep -q 'AS go-amd64' "${REPO_ROOT}/components/api-server/Dockerfile" \
  && grep -q 'AS go-arm64' "${REPO_ROOT}/components/api-server/Dockerfile" \
  && grep -q 'AS static-amd64' "${REPO_ROOT}/components/api-server/Dockerfile" \
  && grep -q 'AS static-arm64' "${REPO_ROOT}/components/api-server/Dockerfile" \
  && grep -q 'AS go-amd64' "${REPO_ROOT}/components/control-plane/Dockerfile" \
  && grep -q 'AS go-arm64' "${REPO_ROOT}/components/control-plane/Dockerfile" \
  && grep -q 'AS build-amd64' "${REPO_ROOT}/components/web-console/Dockerfile" \
  && grep -q 'AS build-arm64' "${REPO_ROOT}/components/web-console/Dockerfile" \
  && grep -q 'AS runtime-amd64' "${REPO_ROOT}/components/web-console/Dockerfile" \
  && grep -q 'AS runtime-arm64' "${REPO_ROOT}/components/web-console/Dockerfile"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap Dockerfiles do not pin HI bases per amd64 and arm64'
fi
if grep -q 'GOARCH="${TARGETARCH' "${REPO_ROOT}/components/api-server/Dockerfile" \
  && grep -q 'GOARCH="${TARGETARCH' "${REPO_ROOT}/components/control-plane/Dockerfile"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: Go Dockerfiles do not honor TARGETARCH for OpenShift swap cross-compile'
fi
if grep -q 'build-${BUILDARCH}' "${REPO_ROOT}/components/web-console/Dockerfile" \
  && grep -q 'AS bundle' "${REPO_ROOT}/components/web-console/Dockerfile" \
  && grep -q 'build-${TARGETARCH}' "${REPO_ROOT}/components/web-console/Dockerfile"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: web-console Dockerfile does not compile on BUILDARCH and bundle native addons on TARGETARCH'
fi
if grep -q 'OPENSHIFT_IMAGE_REGISTRY' "${SCRIPT_DIR}/drivers/openshift.sh" "${SCRIPT_DIR}/lib.sh" "${REPO_ROOT}/Makefile"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OPENSHIFT_IMAGE_REGISTRY is still present; swaps use SWAP_REGISTRY'
else
  PASS=$((PASS + 1))
fi
if grep -q 'SWAP_REGISTRY:-${IMAGE_REGISTRY}' "${SCRIPT_DIR}/drivers/openshift.sh" "${SCRIPT_DIR}/lib.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: SWAP_REGISTRY still falls back to IMAGE_REGISTRY'
else
  PASS=$((PASS + 1))
fi
if grep -A40 '^login_swap_registry()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'login_registry_with_pull_secret'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: swap login does not use PULL_SECRET via login_registry_with_pull_secret'
fi
if grep -q 'PULL_SECRET:-${KIND_PULL_SECRET' "${REPO_ROOT}/scripts/kind/up.sh"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: kind-up does not accept PULL_SECRET with KIND_PULL_SECRET alias'
fi
# Keycloak has no persistent storage (start-dev on in-memory H2), so a Keycloak
# pod restart discards dev-gateway's OIDC client while its row survives in
# PostgreSQL, permanently sticking it in "Keycloak client is missing" (the
# reconciler never auto-recreates a missing client). Until Keycloak gets
# durable storage, seed_via_api must delete and recreate dev-gateway on every
# run instead of reusing whatever it finds.
if awk '/^seed_via_api\(\)/,0' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -A20 'json_named_id dev-gateway' | grep -q 'api_exec DELETE "/api/hypershell/v1/gateways/\${GATEWAY_ID}"'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seed_via_api does not delete an existing dev-gateway before recreating it'
fi
if awk '/^seed_via_api\(\)/,0' "${SCRIPT_DIR}/drivers/openshift.sh" \
  | grep -q 'dev-gateway already exists'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift seed_via_api still reuses an existing dev-gateway instead of recreating it'
else
  PASS=$((PASS + 1))
fi

printf 'OpenShift lifecycle tests: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
