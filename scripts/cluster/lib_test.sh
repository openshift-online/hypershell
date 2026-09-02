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
merged="$(printf '%s' '{"id":"x","redirectUris":["https://console.hypershell.localhost/*"]}' | keycloak_client_with_console_redirects 'console.apps.example.com')"
assert_eq '["https://console.apps.example.com/auth/callback", "https://console.apps.example.com"]' \
  "$(printf '%s' "${merged}" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["redirectUris"]))')" \
  "keycloak_client_with_console_redirects replaces Kind localhost URIs"
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
if grep -E 'delete clusterrole(binding)? "hypershell-controller' "${SCRIPT_DIR}/drivers/openshift.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift down deletes unprefixed cluster-scoped names (would hit stage)'
else
  PASS=$((PASS + 1))
fi
if grep -q 'OPENSHIFT_USE_EXISTING_CLUSTERROLE' "${SCRIPT_DIR}/drivers/openshift.sh" \
  && grep -A50 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'get clusterrole hypershell-controller' \
  && grep -A50 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'keep-role-refs'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OPENSHIFT_USE_EXISTING_CLUSTERROLE does not look up ClusterRole hypershell-controller'
fi
if grep -A50 '^apply_cluster_rbac()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'Applying cluster-scoped RBAC from deploy/openshift'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: default apply_cluster_rbac does not apply overlay ClusterRole/ClusterRoleBinding'
fi
if grep -A20 '^cluster_up()' "${SCRIPT_DIR}/drivers/openshift.sh" | grep -q 'skip_seed'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: OpenShift cluster_up does not honor SKIP_SEED'
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
  else
    FAIL=$((FAIL + 1))
    echo 'FAIL: rewritten overlay render'
  fi
  rm -f "${os_out}"
else
  warn "kustomize not installed; skipping overlay render checks"
fi

printf 'OpenShift lifecycle tests: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
