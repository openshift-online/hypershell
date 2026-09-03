#!/usr/bin/env bash
# OpenShift lifecycle driver. Deploys into an ephemeral namespace group on an
# existing cluster; it never creates or destroys the cluster.
set -euo pipefail

OWNED_LABEL="hypershell.redhat.io/owned"
ENV_LABEL="hypershell.redhat.io/environment"
MANAGED_LABEL="app.kubernetes.io/managed-by"
MANAGED_VALUE="hypershell-lifecycle"
PART_OF_LABEL="app.kubernetes.io/part-of"
PART_OF_VALUE="hypershell"

oc_cli() {
  oc "$@"
}

require_openshift_cluster() {
  if ! command -v oc >/dev/null 2>&1; then
    error "oc is not installed. Install the OpenShift CLI and retry."
    error "Provide an OpenShift cluster target before running 'make openshift-up'."
    exit 1
  fi
  if ! oc_cli whoami >/dev/null 2>&1; then
    error "No reachable OpenShift cluster in the current kubeconfig context."
    error "Log in with 'oc login' (or set KUBECONFIG) and retry. 'make openshift-up' does not create a cluster."
    exit 1
  fi
  if ! oc_cli api-resources --api-group=route.openshift.io --no-headers 2>/dev/null | grep -q .; then
    error "The current kubeconfig context is not an OpenShift cluster (route.openshift.io is missing)."
    error "Provide an OpenShift cluster target before running 'make openshift-up'."
    exit 1
  fi
}

resolve_openshift_namespace() {
  # Default: the current oc project. OPENSHIFT_NAMESPACE is an override.
  if [[ -n "${OPENSHIFT_NAMESPACE:-}" ]]; then
    return 0
  fi
  local project
  project="$(oc_cli project -q 2>/dev/null || true)"
  if [[ -z "${project}" ]]; then
    error "OPENSHIFT_NAMESPACE is unset and no oc project is selected."
    error "Run 'oc project <name>' or set OPENSHIFT_NAMESPACE to a unique RFC 1123 DNS label (max 54 characters)."
    exit 1
  fi
  OPENSHIFT_NAMESPACE="${project}"
  info "OPENSHIFT_NAMESPACE unset; using oc project '${OPENSHIFT_NAMESPACE}'"
}

validate_namespace_group() {
  validate_rfc1123_label "${OPENSHIFT_NAMESPACE}" 54 || exit 1
  OPENSHIFT_KEYCLOAK_NAMESPACE="$(keycloak_namespace_for "${OPENSHIFT_NAMESPACE}")"
  validate_rfc1123_label "${OPENSHIFT_KEYCLOAK_NAMESPACE}" 63 || exit 1
  if is_reserved_cluster_namespace "${OPENSHIFT_NAMESPACE}"; then
    error "Namespace '${OPENSHIFT_NAMESPACE}' is a reserved cluster namespace. Choose a different project."
    exit 1
  fi
  if is_reserved_cluster_namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}"; then
    error "Derived Keycloak namespace '${OPENSHIFT_KEYCLOAK_NAMESPACE}' is reserved. Choose a different platform name."
    exit 1
  fi
}

namespace_exists() {
  local out
  if ! out="$(oc_cli get project "$1" --ignore-not-found -o name 2>/dev/null)"; then
    error "checking for project $1 failed"
    exit 1
  fi
  [[ -n "${out}" ]]
}

current_project() {
  oc_cli project -q 2>/dev/null || true
}

use_project() {
  local ns="$1"
  local current
  current="$(current_project)"
  if [[ "${current}" == "${ns}" ]]; then
    return 0
  fi
  info "Using oc project ${ns}"
  if oc_cli project "${ns}" >/dev/null; then
    return 0
  fi
  error "Cannot switch to project ${ns}"
  exit 1
}

namespace_label_value() {
  oc_cli get namespace "$1" -o go-template="{{if .metadata.labels}}{{index .metadata.labels \"$2\"}}{{end}}" 2>/dev/null || true
}

namespace_is_owned() {
  [[ "$(namespace_label_value "$1" "${OWNED_LABEL}")" == "true" ]]
}

stamp_namespace_labels() {
  local ns="$1"
  local env_id="$2"
  local err
  if err="$(oc_cli label namespace "${ns}" \
    "${OWNED_LABEL}=true" \
    "${ENV_LABEL}=${env_id}" \
    "${MANAGED_LABEL}=${MANAGED_VALUE}" \
    "${PART_OF_LABEL}=${PART_OF_VALUE}" \
    --overwrite 2>&1)"; then
    return 0
  fi
  if grep -qi 'forbidden' <<<"${err}"; then
    warn "Cannot label namespace ${ns} (no permission to patch namespaces). Continuing."
    return 0
  fi
  error "Failed to label namespace ${ns}: ${err}"
  exit 1
}

# Namespaces labeled as a different HyperShell environment are foreign.
# Unlabeled namespaces are the developer's chosen target, not foreign.
refuse_foreign_namespace() {
  local ns="$1"
  local expected_env="${2:-}"
  if ! namespace_exists "${ns}"; then
    return 0
  fi
  local owned env_id
  owned="$(namespace_label_value "${ns}" "${OWNED_LABEL}")"
  env_id="$(namespace_label_value "${ns}" "${ENV_LABEL}")"
  if [[ "${owned}" != "true" ]]; then
    return 0
  fi
  if [[ -z "${env_id}" ]]; then
    error "Namespace '${ns}' is missing the ${ENV_LABEL} identifier. Refusing to adopt it."
    exit 1
  fi
  if [[ -n "${expected_env}" && "${env_id}" != "${expected_env}" ]]; then
    error "Namespace '${ns}' belongs to environment '${env_id}', not '${expected_env}'. Refusing to adopt it."
    exit 1
  fi
  printf '%s' "${env_id}"
}

# Recover a previously applied environment id from workload labels when the
# namespace itself could not be labeled.
env_id_from_workloads() {
  local ns="$1"
  oc_cli get deploy -n "${ns}" -o go-template \
    "{{range .items}}{{if .metadata.labels}}{{with index .metadata.labels \"${ENV_LABEL}\"}}{{.}}{{println}}{{end}}{{end}}{{end}}" \
    2>/dev/null | awk 'NF{print; exit}' || true
}

ensure_project() {
  local ns="$1"
  local env_id="$2"
  if namespace_exists "${ns}"; then
    if namespace_is_owned "${ns}"; then
      refuse_foreign_namespace "${ns}" "${env_id}" >/dev/null
    fi
    stamp_namespace_labels "${ns}" "${env_id}"
    return 0
  fi
  info "Creating project ${ns} (oc new-project)"
  local err
  if err="$(oc_cli new-project "${ns}" \
    --display-name="HyperShell ${ns}" \
    --description="HyperShell local-dev" 2>&1)"; then
    stamp_namespace_labels "${ns}" "${env_id}"
    return 0
  fi
  if grep -qi 'already exists' <<<"${err}"; then
    use_project "${ns}"
    stamp_namespace_labels "${ns}" "${env_id}"
    return 0
  fi
  if grep -qi 'forbidden' <<<"${err}"; then
    error "Cannot create project ${ns} (forbidden)."
    error "OpenShift developers create projects with 'oc new-project', not 'oc create namespace'."
    error "Ask an administrator to grant self-provisioner, or to create ${ns}."
    exit 1
  fi
  error "Failed to create project ${ns}: ${err}"
  exit 1
}

ensure_namespace_group() {
  local platform_env keycloak_env env_id
  platform_env=""
  keycloak_env=""
  if namespace_exists "${OPENSHIFT_NAMESPACE}" && namespace_is_owned "${OPENSHIFT_NAMESPACE}"; then
    platform_env="$(refuse_foreign_namespace "${OPENSHIFT_NAMESPACE}")"
  fi
  if namespace_exists "${OPENSHIFT_KEYCLOAK_NAMESPACE}" && namespace_is_owned "${OPENSHIFT_KEYCLOAK_NAMESPACE}"; then
    keycloak_env="$(refuse_foreign_namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}" "${platform_env}")"
  fi
  if [[ -n "${platform_env}" && -n "${keycloak_env}" && "${platform_env}" != "${keycloak_env}" ]]; then
    error "Namespace group mismatch: ${OPENSHIFT_NAMESPACE}=${platform_env} vs ${OPENSHIFT_KEYCLOAK_NAMESPACE}=${keycloak_env}"
    exit 1
  fi
  env_id="${platform_env:-${keycloak_env}}"
  if [[ -z "${env_id}" ]]; then
    env_id="$(env_id_from_workloads "${OPENSHIFT_NAMESPACE}")"
  fi
  if [[ -z "${env_id}" ]]; then
    env_id="$(uuidgen | tr '[:upper:]' '[:lower:]')"
  fi
  OPENSHIFT_ENVIRONMENT_ID="${env_id}"
  ensure_project "${OPENSHIFT_NAMESPACE}" "${env_id}"
  ensure_project "${OPENSHIFT_KEYCLOAK_NAMESPACE}" "${env_id}"
  use_project "${OPENSHIFT_NAMESPACE}"
  success "Namespace group ${OPENSHIFT_NAMESPACE} + ${OPENSHIFT_KEYCLOAK_NAMESPACE} (environment ${env_id})"
}

discover_gateway_base_domain() {
  local gw_name="$1"
  local gw_ns="$2"
  local host=""
  host="$(oc_cli get "gateway.gateway.networking.k8s.io/${gw_name}" -n "${gw_ns}" \
    -o jsonpath='{.spec.listeners[?(@.name=="grpc")].hostname}' 2>/dev/null || true)"
  if [[ -z "${host}" ]]; then
    host="$(oc_cli get "gateway.gateway.networking.k8s.io/${gw_name}" -n "${gw_ns}" \
      -o jsonpath='{.spec.listeners[0].hostname}' 2>/dev/null || true)"
  fi
  if [[ -z "${host}" ]]; then
    error "Shared Gateway '${gw_ns}/${gw_name}' has no listener hostname."
    error "The gateway wildcard hostname is the base domain; 'make openshift-up' does not take GATEWAY_API_BASE_DOMAIN."
    exit 1
  fi
  GATEWAY_API_BASE_DOMAIN="$(gateway_base_domain_from_hostname "${host}")"
  if [[ -z "${GATEWAY_API_BASE_DOMAIN}" ]]; then
    error "Could not derive a base domain from Gateway listener hostname '${host}'."
    exit 1
  fi
}

check_infrastructure() {
  header "Infrastructure prerequisites"
  local gw_name="${GATEWAY_API_GATEWAY_NAME}"
  local gw_ns="${GATEWAY_API_GATEWAY_NAMESPACE}"
  if ! oc_cli get "gateway.gateway.networking.k8s.io/${gw_name}" -n "${gw_ns}" >/dev/null 2>&1; then
    error "Required shared Gateway '${gw_name}' was not found in namespace '${gw_ns}'."
    error "An administrator must provision the Gateway, GatewayClass, certificate issuer, and wildcard certificate first."
    error "See deploy/openshift/infrastructure/GATEWAY-SETUP.md. 'make openshift-up' will not create cluster infrastructure."
    exit 1
  fi
  local gw_class
  gw_class="$(oc_cli get "gateway.gateway.networking.k8s.io/${gw_name}" -n "${gw_ns}" -o jsonpath='{.spec.gatewayClassName}' 2>/dev/null || true)"
  if [[ -z "${gw_class}" ]]; then
    error "Shared Gateway '${gw_ns}/${gw_name}' has no gatewayClassName."
    exit 1
  fi
  # GatewayClass is cluster-scoped. Typical developers cannot GET it; a
  # Programmed Gateway is the proof the class is serving this cluster.
  local programmed
  programmed="$(oc_cli get "gateway.gateway.networking.k8s.io/${gw_name}" -n "${gw_ns}" \
    -o jsonpath='{range .status.conditions[?(@.type=="Programmed")]}{.status}{end}' 2>/dev/null || true)"
  if [[ "${programmed}" != "True" ]]; then
    error "Shared Gateway '${gw_ns}/${gw_name}' is not Programmed=True (status=${programmed:-unknown})."
    error "Fix the cluster infrastructure before deploying HyperShell."
    exit 1
  fi
  discover_gateway_base_domain "${gw_name}" "${gw_ns}"
  success "Shared Gateway ${gw_ns}/${gw_name} (GatewayClass ${gw_class}) is ready"
  info "Gateway base domain: ${GATEWAY_API_BASE_DOMAIN} (from ${gw_ns}/${gw_name} listener)"
}

warn_skipped_cluster_rbac() {
  local err="$1"
  OPENSHIFT_CLUSTER_RBAC_APPLIED=false
  warn "Could not apply cluster-scoped RBAC for this environment."
  warn "${err}"
  warn "Expected ClusterRoleBinding ${OPENSHIFT_NAMESPACE}-dev-hypershell-controller"
  warn "bound to this environment's ClusterRole or to ClusterRole hypershell-controller."
  warn "Gateways and sandboxes will not provision until this environment's controller is bound."
  warn "Do not apply unprefixed ClusterRoleBinding hypershell-controller; that belongs to stage."
}

# roleRef is immutable. Delete a leftover ClusterRoleBinding so the next apply
# can point at the intended ClusterRole (prefixed copy vs cluster-wide).
replace_clusterrolebinding_if_role_ref_differs() {
  local name="$1"
  local want_ref="$2"
  local current
  current="$(oc_cli get clusterrolebinding "${name}" -o jsonpath='{.roleRef.name}' 2>/dev/null || true)"
  if [[ -z "${current}" || "${current}" == "${want_ref}" ]]; then
    return 0
  fi
  info "Replacing ClusterRoleBinding ${name} (roleRef ${current} -> ${want_ref}; roleRef is immutable)"
  oc_cli delete clusterrolebinding "${name}"
}

apply_rendered_cluster_rbac() {
  local rendered err
  if ! rendered="$(render_openshift_manifests "$@")"; then
    return 1
  fi
  assert_expected_cluster_scoped "${rendered}"
  if ! err="$(oc_cli apply -f "${rendered}" 2>&1)"; then
    rm -f "${rendered}"
    LAST_CLUSTER_RBAC_ERR="${err}"
    return 1
  fi
  printf '%s\n' "${err}"
  rm -f "${rendered}"
  return 0
}

bind_existing_clusterrole() {
  local prefix="${OPENSHIFT_NAMESPACE}-dev-"
  local crb="${prefix}hypershell-controller"
  if ! oc_cli get clusterrole hypershell-controller >/dev/null 2>&1; then
    LAST_CLUSTER_RBAC_ERR="ClusterRole hypershell-controller was not found"
    return 1
  fi
  info "Binding this environment to ClusterRole hypershell-controller (${crb})"
  if ! replace_clusterrolebinding_if_role_ref_differs "${crb}" "hypershell-controller"; then
    LAST_CLUSTER_RBAC_ERR="Could not replace ClusterRoleBinding ${crb} so roleRef can change"
    return 1
  fi
  apply_rendered_cluster_rbac \
    --only-namespace __cluster__ \
    --include-cluster-scoped \
    --omit-kinds ClusterRole \
    --keep-role-refs \
    --omit-names "${prefix}hypershell-controller-scc-bind"
}

apply_prefixed_cluster_rbac() {
  local prefix="${OPENSHIFT_NAMESPACE}-dev-"
  local crb="${prefix}hypershell-controller"
  # Apply the ClusterRole first. Applying it with the ClusterRoleBinding in one
  # shot can create a CRB that points at a ClusterRole that never exists, and
  # roleRef cannot be patched afterward.
  if ! apply_rendered_cluster_rbac \
    --only-namespace __cluster__ \
    --include-cluster-scoped \
    --only-kinds ClusterRole; then
    return 1
  fi
  if ! replace_clusterrolebinding_if_role_ref_differs "${crb}" "${crb}"; then
    LAST_CLUSTER_RBAC_ERR="Could not replace ClusterRoleBinding ${crb} so roleRef can change"
    return 1
  fi
  apply_rendered_cluster_rbac \
    --only-namespace __cluster__ \
    --include-cluster-scoped \
    --only-kinds ClusterRoleBinding
}

assert_expected_cluster_scoped() {
  local rendered="$1"
  local prefix="${OPENSHIFT_NAMESPACE}-dev-"
  local bad
  if ! bad="$(python3 -c '
from importlib.machinery import SourceFileLoader
from pathlib import Path
import sys
rw = SourceFileLoader("rewrite", sys.argv[1]).load_module()
bad = rw.unprefixed_cluster_scoped(Path(sys.argv[2]).read_text(), sys.argv[3])
if bad:
    print("\n".join(bad))
    raise SystemExit(1)
' "${CLUSTER_SCRIPT_DIR}/rewrite-namespaces.py" "${rendered}" "${prefix}")"; then
    error "Refusing cluster-scoped names that would collide with stage:"
    error "${bad}"
    exit 1
  fi
}

apply_sandbox_scc() {
  local err
  if err="$(oc_cli apply -n "${OPENSHIFT_NAMESPACE}" -f - 2>&1 <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: hypershell-sandbox-scc
  namespace: ${OPENSHIFT_NAMESPACE}
  labels:
    ${OWNED_LABEL}: "true"
    ${ENV_LABEL}: "${OPENSHIFT_ENVIRONMENT_ID}"
    ${MANAGED_LABEL}: "${MANAGED_VALUE}"
    ${PART_OF_LABEL}: "${PART_OF_VALUE}"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:openshift:scc:privileged
subjects:
  - kind: ServiceAccount
    name: openshell-gateway-sandbox
    namespace: ${OPENSHIFT_NAMESPACE}
EOF
)"; then
    printf '%s\n' "${err}"
    return 0
  fi
  if grep -qi 'forbidden' <<<"${err}"; then
    warn "Skipping privileged SCC RoleBinding (the current user cannot bind system:openshift:scc:privileged)."
    warn "Gateway sandboxes will not start until an administrator creates hypershell-sandbox-scc in ${OPENSHIFT_NAMESPACE}."
    return 0
  fi
  error "${err}"
  return 1
}

apply_cluster_rbac() {
  # Default: apply the overlay ClusterRole and ClusterRoleBinding, names prefixed
  # so they never replace stage's hypershell-controller. If ClusterRole create is
  # Forbidden (escalation prevention: rules the user does not hold), bind this
  # environment's service account to the existing cluster-wide ClusterRole.
  local prefix="${OPENSHIFT_NAMESPACE}-dev-"
  OPENSHIFT_CLUSTER_RBAC_APPLIED=true
  LAST_CLUSTER_RBAC_ERR=""

  info "Applying cluster-scoped RBAC from deploy/openshift (${prefix}*)..."
  if apply_prefixed_cluster_rbac; then
    apply_sandbox_scc
    return 0
  fi

  local err="${LAST_CLUSTER_RBAC_ERR}"
  if ! grep -qi 'forbidden' <<<"${err}"; then
    error "${err}"
    return 1
  fi

  warn "Could not create ClusterRole ${prefix}hypershell-controller."
  warn "${err}"
  warn "Falling back to existing ClusterRole hypershell-controller."
  if bind_existing_clusterrole; then
    success "Bound this environment to ClusterRole hypershell-controller"
    apply_sandbox_scc
    return 0
  fi
  warn_skipped_cluster_rbac "${LAST_CLUSTER_RBAC_ERR:-${err}}"
  apply_sandbox_scc || true
}

create_bootstrap_secrets() {
  local kc_svc="http://keycloak-service.${OPENSHIFT_KEYCLOAK_NAMESPACE}.svc.cluster.local:8080"
  local session
  session="$(openssl rand -hex 32)"

  oc_cli create secret generic hypershell-oidc-session \
    -n "${OPENSHIFT_NAMESPACE}" \
    --from-literal=SESSION_SECRET="${session}" \
    --dry-run=client -o yaml | oc_cli apply -f - >/dev/null

  oc_cli create secret generic hypershell-api-config \
    -n "${OPENSHIFT_NAMESPACE}" \
    --from-literal=api-service.issuerUrl="${kc_svc}/realms/hypershell" \
    --from-literal=api-service.clientId="hypershell-control-plane" \
    --from-literal=api-service.clientSecret="control-plane-secret" \
    --from-literal=api-service.jwkCertUrl="${kc_svc}/realms/hypershell/protocol/openid-connect/certs" \
    --dry-run=client -o yaml | oc_cli apply -f - >/dev/null

  oc_cli create secret generic hypershell-cp-oidc \
    -n "${OPENSHIFT_NAMESPACE}" \
    --from-literal=client-secret="control-plane-secret" \
    --dry-run=client -o yaml | oc_cli apply -f - >/dev/null

  oc_cli create secret generic hypershell-keycloak-admin \
    -n "${OPENSHIFT_NAMESPACE}" \
    --from-literal=server-url="${kc_svc}" \
    --from-literal=realm="hypershell" \
    --from-literal=client-id="hypershell-provisioner" \
    --from-literal=client-secret="provisioner-secret" \
    --dry-run=client -o yaml | oc_cli apply -f - >/dev/null
}

render_openshift_manifests() {
  local tmp rendered overlay_rel
  tmp="$(mktemp -d "${REPO_ROOT}/.openshift-render.XXXXXX")"
  overlay_rel="$(python3 -c 'import os, sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))' \
    "${REPO_ROOT}/deploy/openshift" "${tmp}")"
  cat > "${tmp}/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ${overlay_rel}
labels:
  - pairs:
      ${PART_OF_LABEL}: ${PART_OF_VALUE}
      ${OWNED_LABEL}: "true"
      ${ENV_LABEL}: "${OPENSHIFT_ENVIRONMENT_ID}"
      ${MANAGED_LABEL}: ${MANAGED_VALUE}
    includeSelectors: false
patches:
  - patch: |
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
                  - name: GATEWAY_API_GATEWAY_NAME
                    value: "${GATEWAY_API_GATEWAY_NAME}"
                  - name: GATEWAY_API_GATEWAY_NAMESPACE
                    value: "${GATEWAY_API_GATEWAY_NAMESPACE}"
                  - name: GATEWAY_API_BASE_DOMAIN
                    value: "${GATEWAY_API_BASE_DOMAIN}"
                  - name: GATEWAY_OIDC_ISSUER_URL
                    value: "https://keycloak.pending.invalid/realms/hypershell"
EOF
  rendered="$(mktemp)"
  if ! kustomize build --load-restrictor=LoadRestrictionsNone "${tmp}" \
    | python3 "${CLUSTER_SCRIPT_DIR}/rewrite-namespaces.py" \
      --platform-namespace "${OPENSHIFT_NAMESPACE}" \
      --keycloak-namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}" \
      --omit-namespaces \
      "$@" \
    > "${rendered}"; then
    rm -rf "${tmp}"
    rm -f "${rendered}"
    error "Failed to render OpenShift overlay"
    return 1
  fi
  rm -rf "${tmp}"
  if [[ ! -s "${rendered}" ]]; then
    rm -f "${rendered}"
    error "Rendered overlay is empty"
    return 1
  fi
  printf '%s' "${rendered}"
}

api_group_available() {
  oc_cli api-resources --api-group="$1" --no-headers 2>/dev/null | grep -q .
}

cnpg_available() {
  api_group_available postgresql.cnpg.io
}

apply_rendered_overlay() {
  local rendered="$1"
  local prune_args=(
    --prune
    -l "${ENV_LABEL}=${OPENSHIFT_ENVIRONMENT_ID}"
    --prune-allowlist=core/v1/ConfigMap
    --prune-allowlist=core/v1/Service
    --prune-allowlist=core/v1/ServiceAccount
    --prune-allowlist=apps/v1/Deployment
    --prune-allowlist=networking.k8s.io/v1/NetworkPolicy
    --prune-allowlist=route.openshift.io/v1/Route
    --prune-allowlist=rbac.authorization.k8s.io/v1/RoleBinding
  )
  if api_group_available cert-manager.io; then
    prune_args+=(
      --prune-allowlist=cert-manager.io/v1/Certificate
      --prune-allowlist=cert-manager.io/v1/Issuer
    )
  fi
  if cnpg_available; then
    prune_args+=(--prune-allowlist=postgresql.cnpg.io/v1/Cluster)
  fi
  # Secrets are omitted from prune: bootstrap OIDC secrets are not in the overlay
  # and must survive reconcile. A reconcile that drops a swapped Deployment's
  # image is restored by restore_swaps_after_reconcile.
  if [[ ! -s "${rendered}" ]]; then
    error "Rendered overlay is empty"
    rm -f "${rendered}"
    exit 1
  fi
  if ! oc_cli apply -f "${rendered}" "${prune_args[@]}"; then
    warn "Apply with prune failed; applying without prune"
    oc_cli apply -f "${rendered}"
  fi
  rm -f "${rendered}"
}

apply_postgres_fallback() {
  info "CNPG operator is not installed; deploying bundled PostgreSQL Deployment"
  local rendered
  rendered="$(mktemp)"
  if ! python3 "${CLUSTER_SCRIPT_DIR}/rewrite-namespaces.py" \
    --platform-namespace "${OPENSHIFT_NAMESPACE}" \
    --keycloak-namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}" \
    --omit-namespaces \
    --only-namespace "${OPENSHIFT_NAMESPACE}" \
    --strip-openshift-uids \
    < "${REPO_ROOT}/deploy/base/postgres.yaml" \
    > "${rendered}"; then
    rm -f "${rendered}"
    error "Failed to render PostgreSQL fallback"
    return 1
  fi
  oc_cli apply -f "${rendered}"
  rm -f "${rendered}"
}

configure_postgres_fallback_ssl() {
  # migrate is an initContainer; omit -c so oc sets both init and app containers.
  oc_cli set env deployment/hypershell-api-server -n "${OPENSHIFT_NAMESPACE}" \
    DB_SSLMODE=disable >/dev/null
}

developer_omit_kinds() {
  local kinds="ClusterRole,ClusterRoleBinding"
  if ! cnpg_available; then
    kinds+=",Cluster"
  fi
  if ! api_group_available cert-manager.io; then
    kinds+=",Certificate,Issuer"
  fi
  printf '%s' "${kinds}"
}

apply_overlay() {
  header "Deploying Components"
  local rendered omit_kinds
  omit_kinds="$(developer_omit_kinds)"

  info "Applying Keycloak in project ${OPENSHIFT_KEYCLOAK_NAMESPACE}..."
  use_project "${OPENSHIFT_KEYCLOAK_NAMESPACE}"
  if ! rendered="$(render_openshift_manifests --only-namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}")"; then
    exit 1
  fi
  apply_rendered_overlay "${rendered}"

  info "Applying HyperShell in project ${OPENSHIFT_NAMESPACE}..."
  use_project "${OPENSHIFT_NAMESPACE}"
  if ! cnpg_available; then
    apply_postgres_fallback
  fi
  if ! rendered="$(render_openshift_manifests \
    --only-namespace "${OPENSHIFT_NAMESPACE}" \
    --omit-kinds "${omit_kinds}" \
    --omit-names hypershell-sandbox-scc)"; then
    exit 1
  fi
  apply_rendered_overlay "${rendered}"
  if ! cnpg_available; then
    configure_postgres_fallback_ssl
  fi

  success "Overlay applied"
}

restore_swaps_after_reconcile() {
  local component image
  for component in api-server control-plane web-console; do
    if is_openshift_swapped "${component}"; then
      image="$(openshift_swap_image "${component}")"
      if [[ -z "${image}" ]]; then
        warn "Swap state for ${component} is empty; clearing stale entry"
        clear_openshift_swap "${component}"
        continue
      fi
      info "Preserving ${component} working-tree image ${image}"
      component_spec "${component}"
      local args=()
      local c
      for c in ${CONTAINERS}; do
        args+=("${c}=${image}")
      done
      oc_cli set image "deployment/${DEPLOYMENT}" "${args[@]}" -n "${OPENSHIFT_NAMESPACE}"
    fi
  done
}

wait_for_route_host() {
  local name="$1"
  local ns="$2"
  local host=""
  local i
  for i in $(seq 1 30); do
    host="$(oc_cli get route "${name}" -n "${ns}" -o jsonpath='{.spec.host}' 2>/dev/null || true)"
    if [[ -n "${host}" ]]; then
      printf '%s' "${host}"
      return 0
    fi
    sleep 2
  done
  return 1
}

wait_for_named_rollout() {
  local deployment="$1"
  local ns="$2"
  local timeout="${3:-180s}"
  info "Waiting for ${deployment}..."
  if ! oc_cli rollout status "deployment/${deployment}" -n "${ns}" --timeout="${timeout}"; then
    error "${deployment} did not become ready in ${ns}"
    oc_cli describe "deploy/${deployment}" -n "${ns}" | tail -30 || true
    exit 1
  fi
  success "${deployment} ready"
}

wait_for_keycloak() {
  wait_for_named_rollout keycloak "${OPENSHIFT_KEYCLOAK_NAMESPACE}"
}

configure_oidc_from_routes() {
  header "OIDC"
  wait_for_keycloak
  info "Waiting for Keycloak Route..."
  local kc_host api_host console_host
  if ! kc_host="$(wait_for_route_host keycloak "${OPENSHIFT_KEYCLOAK_NAMESPACE}")"; then
    error "Keycloak Route host was not assigned in ${OPENSHIFT_KEYCLOAK_NAMESPACE}"
    exit 1
  fi
  if ! api_host="$(wait_for_route_host hypershell-api "${OPENSHIFT_NAMESPACE}")"; then
    error "API Route host was not assigned in ${OPENSHIFT_NAMESPACE}"
    exit 1
  fi
  if ! console_host="$(wait_for_route_host hypershell-web-console "${OPENSHIFT_NAMESPACE}")"; then
    error "Web console Route host was not assigned in ${OPENSHIFT_NAMESPACE}"
    exit 1
  fi
  OPENSHIFT_KEYCLOAK_HOST="${kc_host}"
  OPENSHIFT_API_HOST="${api_host}"
  OPENSHIFT_CONSOLE_HOST="${console_host}"
  OPENSHIFT_KC_HOSTNAME="https://${kc_host}"
  OPENSHIFT_OIDC_ISSUER="https://${kc_host}/realms/hypershell"

  info "Setting Keycloak KC_HOSTNAME=${OPENSHIFT_KC_HOSTNAME}"
  oc_cli set env deployment/keycloak -n "${OPENSHIFT_KEYCLOAK_NAMESPACE}" \
    "KC_HOSTNAME=${OPENSHIFT_KC_HOSTNAME}" >/dev/null

  info "Configuring web console OIDC"
  oc_cli set env deployment/hypershell-web-console -n "${OPENSHIFT_NAMESPACE}" -c web-console \
    "OIDC_ISSUER=${OPENSHIFT_OIDC_ISSUER}" \
    OIDC_CLIENT_ID=hypershell-frontend \
    "OIDC_REDIRECT_URI=https://${console_host}/auth/callback" \
    "OIDC_POST_LOGOUT_REDIRECT_URI=https://${console_host}" >/dev/null
  oc_cli set env deployment/hypershell-web-console -n "${OPENSHIFT_NAMESPACE}" -c web-console \
    --from=secret/hypershell-oidc-session >/dev/null

  info "Configuring control plane public gateway issuer"
  oc_cli set env deployment/hypershell-controller -n "${OPENSHIFT_NAMESPACE}" -c controller \
    "GATEWAY_OIDC_ISSUER_URL=${OPENSHIFT_OIDC_ISSUER}" >/dev/null

  success "Routes: api=${api_host} console=${console_host} keycloak=${kc_host}"
}

wait_for_deployments() {
  header "Readiness"
  # Use `rollout status`, not `wait --for=condition=available`. With replicas=1
  # and the default rolling update the Deployment stays Available throughout a
  # rollout (the old pod keeps serving until the new one is Ready), so
  # `wait --for=condition=available` returns immediately after Route-derived
  # `oc set env` and swap restore -- while a rollout is still in flight.
  wait_for_keycloak

  if [[ -n "$(oc_cli get cluster.postgresql.cnpg.io hypershell-db -n "${OPENSHIFT_NAMESPACE}" --ignore-not-found -o name 2>/dev/null || true)" ]]; then
    info "Waiting for CNPG cluster..."
    oc_cli wait --for=condition=Ready cluster/hypershell-db -n "${OPENSHIFT_NAMESPACE}" --timeout=300s \
      || warn "CNPG cluster not Ready yet; API server will retry connections"
  elif oc_cli get deployment/hypershell-postgres -n "${OPENSHIFT_NAMESPACE}" >/dev/null 2>&1; then
    wait_for_named_rollout hypershell-postgres "${OPENSHIFT_NAMESPACE}" 120s
  fi

  wait_for_named_rollout hypershell-api-server "${OPENSHIFT_NAMESPACE}"
  wait_for_named_rollout hypershell-controller "${OPENSHIFT_NAMESPACE}"
  wait_for_named_rollout hypershell-web-console "${OPENSHIFT_NAMESPACE}"
}

# Talk to OpenShift Routes from the developer machine. The API server image has
# no curl, so oc exec cannot reach Keycloak or the API. -k matches kind-up:
# cluster default certs are not always in the local trust store.
openshift_curl() {
  curl -sS -k -m 15 "$@"
}

add_keycloak_redirect_uri() {
  if ! command -v curl >/dev/null 2>&1; then
    warn "curl is required on this machine to register the console redirect URI in Keycloak"
    return 0
  fi
  local callback="https://${OPENSHIFT_CONSOLE_HOST}/auth/callback"
  local kc="${OPENSHIFT_KC_HOSTNAME}"
  local token client_json client_id full updated http
  token="$(openshift_curl -X POST "${kc}/realms/master/protocol/openid-connect/token" \
    -d grant_type=password -d client_id=admin-cli \
    -d username=admin -d password=admin \
    | json_string_field access_token || true)"
  if [[ -z "${token}" ]]; then
    warn "Could not obtain Keycloak admin token to add console redirect URI"
    return 0
  fi
  client_json="$(openshift_curl -H "Authorization: Bearer ${token}" \
    "${kc}/admin/realms/hypershell/clients?clientId=hypershell-frontend" || true)"
  client_id="$(printf '%s' "${client_json}" | json_first_id || true)"
  if [[ -z "${client_id}" ]]; then
    warn "Could not find hypershell-frontend client to add redirect URI"
    return 0
  fi
  full="$(openshift_curl -H "Authorization: Bearer ${token}" \
    "${kc}/admin/realms/hypershell/clients/${client_id}" || true)"
  if [[ -z "${full}" ]]; then
    warn "Could not fetch hypershell-frontend client representation"
    return 0
  fi
  if printf '%s' "${full}" | grep -Fq "${callback}"; then
    info "Keycloak redirect URI already includes ${callback}"
    return 0
  fi
  info "Setting Keycloak redirect URIs for ${OPENSHIFT_CONSOLE_HOST}"
  updated="$(printf '%s' "${full}" | keycloak_client_with_console_redirects "${OPENSHIFT_CONSOLE_HOST}")"
  http="$(openshift_curl -o /dev/null -w '%{http_code}' -X PUT \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    "${kc}/admin/realms/hypershell/clients/${client_id}" \
    -d "${updated}" || true)"
  if [[ "${http}" != "204" && "${http}" != "200" ]]; then
    warn "Failed to update Keycloak redirect URIs (HTTP ${http:-none})"
    return 0
  fi
  success "Keycloak redirect URI ${callback}"
}

seed_via_api() {
  header "Gateway Provisioning"
  local kc_token_url="${OPENSHIFT_OIDC_ISSUER}/protocol/openid-connect/token"
  local api="https://${OPENSHIFT_API_HOST}"
  local token="" resp=""
  local i
  if ! command -v curl >/dev/null 2>&1; then
    warn "curl is required on this machine to seed the API; skip automatic seeding"
    if seed_strict; then
      error "Platform seeding failed and SEED_STRICT=true - failing"
      return 1
    fi
    return 0
  fi
  info "Obtaining API token from Keycloak Route..."
  for i in $(seq 1 30); do
    resp="$(openshift_curl -X POST "${kc_token_url}" \
      -d grant_type=password -d client_id=hypershell-frontend \
      -d username=admin -d password=admin || true)"
    token="$(printf '%s' "${resp}" | json_string_field access_token || true)"
    if [[ -n "${token}" ]]; then
      break
    fi
    sleep 2
  done
  if [[ -z "${token}" ]]; then
    warn "Could not obtain API token; skip automatic seeding"
    if seed_strict; then
      error "Platform seeding failed and SEED_STRICT=true - failing"
      return 1
    fi
    return 0
  fi
  success "API token obtained"

  api_exec() {
    local method="$1" path="$2" data="${3:-}"
    if [[ -n "${data}" ]]; then
      openshift_curl -w "\n%{http_code}" -X "${method}" "${api}${path}" \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "${data}" || true
    else
      openshift_curl -w "\n%{http_code}" -X "${method}" "${api}${path}" \
        -H "Authorization: Bearer ${token}" || true
    fi
  }

  extract_named_id() {
    local resp="$1" name="$2"
    printf '%s' "${resp}" | grep -o "\"name\":\"${name}\"[^}]*\"id\":\"[^\"]*\"" \
      | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true
  }

  extract_id() {
    local resp="$1"
    if echo "${resp}" | grep -q '"kind":"Error"'; then
      echo ""
      return
    fi
    echo "${resp}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || true
  }

  local seed_failed="" CLUSTER_ID="" RELEASE_ID="" DATABASE_ID="" GATEWAY_ID=""
  local raw http body

  raw="$(api_exec GET /api/hypershell/v1/managed_clusters)"
  http="$(printf '%s' "${raw}" | tail -1)"
  body="$(printf '%s' "${raw}" | sed '$d')"
  if [[ "${http}" == "200" ]]; then
    CLUSTER_ID="$(extract_named_id "${body}" local-openshift)"
  fi
  if [[ -z "${CLUSTER_ID}" ]]; then
    info "Creating ManagedCluster..."
    raw="$(api_exec POST /api/hypershell/v1/managed_clusters \
      "{\"name\":\"local-openshift\",\"provider\":\"openshift\",\"kubeconfig_secret\":\"openshift-kubeconfig\"}")"
    http="$(printf '%s' "${raw}" | tail -1)"
    body="$(printf '%s' "${raw}" | sed '$d')"
    CLUSTER_ID="$(extract_id "${body}")"
    if [[ -z "${CLUSTER_ID}" ]]; then
      warn "ManagedCluster creation failed (HTTP ${http}): ${body:-no response}"
      seed_failed=true
    else
      success "ManagedCluster created: ${CLUSTER_ID}"
    fi
  else
    success "local-openshift ManagedCluster already exists: ${CLUSTER_ID}"
  fi

  if [[ -z "${seed_failed}" ]]; then
    raw="$(api_exec GET /api/hypershell/v1/gateway_releases)"
    http="$(printf '%s' "${raw}" | tail -1)"
    body="$(printf '%s' "${raw}" | sed '$d')"
    if [[ "${http}" == "200" ]]; then
      RELEASE_ID="$(extract_named_id "${body}" dev-release)"
    fi
    if [[ -z "${RELEASE_ID}" ]]; then
      info "Creating GatewayRelease..."
      raw="$(api_exec POST /api/hypershell/v1/gateway_releases \
        "{\"name\":\"dev-release\",\"image\":\"${GATEWAY_IMAGE}\"}")"
      http="$(printf '%s' "${raw}" | tail -1)"
      body="$(printf '%s' "${raw}" | sed '$d')"
      RELEASE_ID="$(extract_id "${body}")"
      if [[ -z "${RELEASE_ID}" ]]; then
        warn "GatewayRelease creation failed (HTTP ${http}): ${body:-no response}"
        seed_failed=true
      else
        success "GatewayRelease created: ${RELEASE_ID}"
      fi
    else
      success "dev-release GatewayRelease already exists: ${RELEASE_ID}"
    fi
  fi

  if [[ -z "${seed_failed}" ]]; then
    local db_provider="cnpg"
    if ! cnpg_available; then
      db_provider="deployment"
    fi
    raw="$(api_exec GET /api/hypershell/v1/managed_databases)"
    http="$(printf '%s' "${raw}" | tail -1)"
    body="$(printf '%s' "${raw}" | sed '$d')"
    if [[ "${http}" == "200" ]]; then
      DATABASE_ID="$(extract_named_id "${body}" openshell-db)"
    fi
    if [[ -n "${DATABASE_ID}" ]] && ! cnpg_available \
      && printf '%s' "${body}" | grep -Fq '"provider":"cnpg"'; then
      warn "openshell-db ManagedDatabase ${DATABASE_ID} has provider=cnpg, but this cluster has no CNPG operator"
      warn "Gateways using it will not reconcile. Run make openshift-down then make openshift-up, or delete that ManagedDatabase and make openshift-seed"
      seed_failed=true
    fi
    if [[ -z "${seed_failed}" && -z "${DATABASE_ID}" ]]; then
      info "Creating ManagedDatabase (provider=${db_provider})..."
      raw="$(api_exec POST /api/hypershell/v1/managed_databases \
        "{\"name\":\"openshell-db\",\"provider\":\"${db_provider}\"}")"
      http="$(printf '%s' "${raw}" | tail -1)"
      body="$(printf '%s' "${raw}" | sed '$d')"
      if [[ "${http}" != "201" && "${http}" != "200" ]]; then
        warn "ManagedDatabase creation failed (HTTP ${http}): ${body:-no response}"
        seed_failed=true
      else
        DATABASE_ID="$(extract_id "${body}")"
        success "ManagedDatabase created: ${DATABASE_ID} (provider=${db_provider})"
      fi
    elif [[ -z "${seed_failed}" ]]; then
      success "openshell-db ManagedDatabase already exists: ${DATABASE_ID}"
    fi
  fi

  if [[ -z "${seed_failed}" ]]; then
    raw="$(api_exec GET /api/hypershell/v1/gateways)"
    http="$(printf '%s' "${raw}" | tail -1)"
    body="$(printf '%s' "${raw}" | sed '$d')"
    if [[ "${http}" == "200" ]]; then
      GATEWAY_ID="$(extract_named_id "${body}" dev-gateway)"
    fi
    if [[ -z "${GATEWAY_ID}" ]]; then
      info "Creating Gateway with OIDC..."
      local oidc
      oidc="{\\\"issuer\\\":\\\"${OPENSHIFT_OIDC_ISSUER}\\\",\\\"audience\\\":\\\"hypershell-frontend\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}"
      raw="$(api_exec POST /api/hypershell/v1/gateways \
        "{\"name\":\"dev-gateway\",\"cluster_id\":\"${CLUSTER_ID}\",\"release_id\":\"${RELEASE_ID}\",\"database_id\":\"${DATABASE_ID}\",\"oidc\":\"${oidc}\",\"route\":\"{\\\"enabled\\\":true}\"}")"
      http="$(printf '%s' "${raw}" | tail -1)"
      body="$(printf '%s' "${raw}" | sed '$d')"
      GATEWAY_ID="$(extract_id "${body}")"
      if [[ -z "${GATEWAY_ID}" ]]; then
        warn "Gateway creation failed (HTTP ${http}): ${body:-no response}"
      else
        success "Gateway created: ${GATEWAY_ID}"
      fi
    else
      success "dev-gateway already exists: ${GATEWAY_ID}"
    fi
  fi

  if [[ -n "${seed_failed}" ]]; then
    warn "Automatic seeding incomplete - create resources manually after the API server is ready"
    warn "Hint: If your local branch has schema / contract changes (e.g. dropped 'fleet_id'), the baseline API server image in the registry might reject your seed request."
    warn "      To resolve this, swap in your working-tree API server build first, then run 'make openshift-seed':"
    warn "      1. SKIP_SEED=true make openshift-up"
    warn "      2. make openshift-api-server-up"
    warn "      3. make openshift-seed"
    if seed_strict; then
      error "Platform seeding failed and SEED_STRICT=true - failing"
      return 1
    fi
  fi
}

print_banner() {
  header "HyperShell is running on OpenShift"
  echo ""
  info "Namespace:     ${OPENSHIFT_NAMESPACE} (Keycloak: ${OPENSHIFT_KEYCLOAK_NAMESPACE})"
  info "HTTP API:      https://${OPENSHIFT_API_HOST}"
  info "Web Console:   https://${OPENSHIFT_CONSOLE_HOST}"
  info "Keycloak:      ${OPENSHIFT_KC_HOSTNAME} (admin/admin)"
  info "OIDC Issuer:   ${OPENSHIFT_OIDC_ISSUER}"
  info "Login:         https://${OPENSHIFT_CONSOLE_HOST}/auth/login"
  info "Test users:    admin/admin (admins + users), developer/developer (users only)"
  echo ""
  info "API Server Logs:    oc logs -f -l app=hypershell-api-server -n ${OPENSHIFT_NAMESPACE}"
  info "Control Plane Logs: oc logs -f -l app=hypershell-controller -n ${OPENSHIFT_NAMESPACE}"
  info "Web Console Logs:   oc logs -f -l app=hypershell-web-console -n ${OPENSHIFT_NAMESPACE}"
  if [[ "${OPENSHIFT_CLUSTER_RBAC_APPLIED:-true}" != "true" ]]; then
    echo ""
    warn "This environment's controller is not bound to a ClusterRole."
    warn "Gateways/sandboxes need ClusterRoleBinding ${OPENSHIFT_NAMESPACE}-dev-hypershell-controller."
  fi
}

cluster_up() {
  header "HyperShell OpenShift Development Environment"
  echo ""
  require_openshift_cluster
  resolve_openshift_namespace
  OPENSHIFT_ENTRY_PROJECT="$(current_project)"
  trap 'if [[ -n "${OPENSHIFT_ENTRY_PROJECT:-}" ]]; then oc_cli project "${OPENSHIFT_ENTRY_PROJECT}" >/dev/null 2>&1 || true; fi' EXIT
  validate_namespace_group
  check_infrastructure
  ensure_namespace_group
  create_bootstrap_secrets
  apply_cluster_rbac
  apply_overlay
  restore_swaps_after_reconcile
  configure_oidc_from_routes
  wait_for_deployments
  add_keycloak_redirect_uri || true
  if skip_seed; then
    info "SKIP_SEED=true - skipping platform seeding"
  else
    seed_via_api
  fi
  echo ""
  print_banner
}

cluster_seed() {
  header "Seeding HyperShell on OpenShift"
  echo ""
  require_openshift_cluster
  resolve_openshift_namespace
  OPENSHIFT_ENTRY_PROJECT="$(current_project)"
  trap 'if [[ -n "${OPENSHIFT_ENTRY_PROJECT:-}" ]]; then oc_cli project "${OPENSHIFT_ENTRY_PROJECT}" >/dev/null 2>&1 || true; fi' EXIT
  validate_namespace_group
  configure_oidc_from_routes
  seed_via_api
}

verify_owned_namespace() {
  local ns="$1"
  if ! namespace_exists "${ns}"; then
    warn "Namespace ${ns} does not exist"
    return 1
  fi
  local owned env_id
  owned="$(namespace_label_value "${ns}" "${OWNED_LABEL}")"
  env_id="$(namespace_label_value "${ns}" "${ENV_LABEL}")"
  if [[ "${owned}" == "true" && -n "${env_id}" && -n "${OPENSHIFT_ENVIRONMENT_ID:-}" && "${env_id}" != "${OPENSHIFT_ENVIRONMENT_ID}" ]]; then
    error "Namespace '${ns}' belongs to environment '${env_id}', not '${OPENSHIFT_ENVIRONMENT_ID}'. Refusing to delete it."
    exit 1
  fi
  if [[ "${owned}" == "true" && -n "${env_id}" ]]; then
    OPENSHIFT_ENVIRONMENT_ID="${env_id}"
  fi
}

delete_hypershell_resources() {
  local ns="$1"
  info "Removing HyperShell resources from ${ns} (keeping the project)"
  oc_cli delete deploy,statefulset,job,cronjob,pod,svc,cm,secret,pvc,sa,role,rolebinding,route,networkpolicy \
    -n "${ns}" \
    -l "${MANAGED_LABEL}=${MANAGED_VALUE}" \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  oc_cli delete deploy,svc,cm,secret,pvc,sa \
    -n "${ns}" \
    -l app.kubernetes.io/name=hypershell \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  oc_cli delete issuer,certificate \
    -n "${ns}" \
    -l "${MANAGED_LABEL}=${MANAGED_VALUE}" \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  oc_cli delete cluster.postgresql.cnpg.io \
    -n "${ns}" \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  oc_cli delete deploy,svc,secret \
    -n "${ns}" \
    hypershell-postgres hypershell-db-app \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  oc_cli delete secret \
    -n "${ns}" \
    hypershell-oidc-session hypershell-api-config hypershell-cp-oidc hypershell-keycloak-admin \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
  # Bundled Keycloak is unlabeled (deploy/base/keycloak). Delete it by name so a
  # forbidden project-delete still clears the companion -keycloak namespace.
  oc_cli delete deploy,svc,cm,secret,route,networkpolicy \
    -n "${ns}" \
    keycloak keycloak-service keycloak-realm \
    keycloak-hypershell-theme keycloak-hypershell-theme-assets \
    keycloak-allow-platform \
    --ignore-not-found --wait=true --timeout=180s >/dev/null 2>&1 || true
}

wait_until_project_gone() {
  local ns="$1"
  local timeout="${2:-300}"
  local elapsed=0
  while namespace_exists "${ns}"; do
    if (( elapsed >= timeout )); then
      error "Timed out after ${timeout}s waiting for project ${ns} to be deleted"
      return 1
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
}

remove_project() {
  local ns="$1"
  local err
  if ! namespace_exists "${ns}"; then
    info "Project ${ns} does not exist"
    return 0
  fi
  info "Deleting project ${ns}"
  if err="$(oc_cli delete project "${ns}" --wait=true --timeout=300s 2>&1)"; then
    wait_until_project_gone "${ns}" 60 || return 1
    success "Project ${ns} deleted"
    return 0
  fi
  if grep -qi 'not found' <<<"${err}"; then
    return 0
  fi
  if grep -qi 'forbidden' <<<"${err}"; then
    warn "Cannot delete project ${ns} (forbidden)."
    delete_hypershell_resources "${ns}"
    return 0
  fi
  error "Failed to delete project ${ns}: ${err}"
  return 1
}

cluster_down() {
  header "Removing OpenShift environment"
  require_openshift_cluster
  resolve_openshift_namespace
  validate_namespace_group

  local ok_platform=false ok_keycloak=false
  if namespace_exists "${OPENSHIFT_NAMESPACE}"; then
    verify_owned_namespace "${OPENSHIFT_NAMESPACE}"
    ok_platform=true
  fi
  if namespace_exists "${OPENSHIFT_KEYCLOAK_NAMESPACE}"; then
    verify_owned_namespace "${OPENSHIFT_KEYCLOAK_NAMESPACE}"
    ok_keycloak=true
  fi
  if [[ "${ok_platform}" != "true" && "${ok_keycloak}" != "true" ]]; then
    warn "No namespace group found for ${OPENSHIFT_NAMESPACE} / ${OPENSHIFT_KEYCLOAK_NAMESPACE}"
    clear_all_openshift_swaps
    return 0
  fi

  info "Deleting this environment's cluster-scoped RBAC..."
  local prefix="${OPENSHIFT_NAMESPACE}-dev-"
  oc_cli delete clusterrolebinding "${prefix}hypershell-controller-scc-bind" --ignore-not-found >/dev/null 2>&1 || true
  oc_cli delete clusterrolebinding "${prefix}hypershell-controller" --ignore-not-found >/dev/null 2>&1 || true
  oc_cli delete clusterrole "${prefix}hypershell-controller-scc-bind" --ignore-not-found >/dev/null 2>&1 || true
  oc_cli delete clusterrole "${prefix}hypershell-controller" --ignore-not-found >/dev/null 2>&1 || true

  info "Removing namespace group ${OPENSHIFT_NAMESPACE} and ${OPENSHIFT_KEYCLOAK_NAMESPACE}"
  remove_project "${OPENSHIFT_KEYCLOAK_NAMESPACE}"
  remove_project "${OPENSHIFT_NAMESPACE}"
  clear_all_openshift_swaps
  success "Environment ${OPENSHIFT_NAMESPACE} (and ${OPENSHIFT_KEYCLOAK_NAMESPACE}) removed"
}

# OpenShift has no cluster to destroy. Keep the Kind-shaped target as an alias.
cluster_teardown() {
  cluster_down
}

cluster_status() {
  header "OpenShift"
  if ! command -v oc >/dev/null 2>&1 || ! oc_cli whoami >/dev/null 2>&1; then
    warn "No reachable OpenShift cluster in the current kubeconfig context"
    return 0
  fi
  info "User:    $(oc_cli whoami 2>/dev/null || echo unknown)"
  info "Server:  $(oc_cli whoami --show-server 2>/dev/null || echo unknown)"
  echo ""

  resolve_openshift_namespace
  validate_namespace_group

  header "Namespaces"
  oc_cli get namespace "${OPENSHIFT_NAMESPACE}" "${OPENSHIFT_KEYCLOAK_NAMESPACE}" 2>/dev/null \
    || warn "Namespace group not found"
  echo ""

  header "Pods (${OPENSHIFT_NAMESPACE})"
  oc_cli get pods -n "${OPENSHIFT_NAMESPACE}" -o wide 2>/dev/null || warn "Namespace not found"
  echo ""
  header "Pods (${OPENSHIFT_KEYCLOAK_NAMESPACE})"
  oc_cli get pods -n "${OPENSHIFT_KEYCLOAK_NAMESPACE}" -o wide 2>/dev/null || warn "Namespace not found"
  echo ""

  header "Services"
  oc_cli get svc -n "${OPENSHIFT_NAMESPACE}" 2>/dev/null || true
  oc_cli get svc -n "${OPENSHIFT_KEYCLOAK_NAMESPACE}" 2>/dev/null || true
  echo ""

  header "Routes"
  oc_cli get route -n "${OPENSHIFT_NAMESPACE}" 2>/dev/null || true
  oc_cli get route -n "${OPENSHIFT_KEYCLOAK_NAMESPACE}" 2>/dev/null || true
  echo ""

  header "Gateway"
  oc_cli get gateway "${GATEWAY_API_GATEWAY_NAME}" -n "${GATEWAY_API_GATEWAY_NAMESPACE}" 2>/dev/null \
    || warn "Shared Gateway ${GATEWAY_API_GATEWAY_NAMESPACE}/${GATEWAY_API_GATEWAY_NAME} not found"
  oc_cli get grpcroutes -n "${OPENSHIFT_NAMESPACE}" 2>/dev/null || true
  echo ""

  header "Component Swap Status"
  local file
  file="$(openshift_swap_file)"
  if [[ -f "${file}" ]] && [[ -s "${file}" ]]; then
    info "Swapped components:"
    local comp image
    while IFS=$'\t' read -r comp image; do
      [[ -n "${comp}" ]] || continue
      info "  - ${comp} (working-tree ${image})"
    done < "${file}"
    info "Baseline components:"
    for comp in api-server control-plane web-console; do
      if ! is_openshift_swapped "${comp}"; then
        info "  - ${comp} (registry image)"
      fi
    done
  else
    info "All components running baseline (registry) images"
  fi
}

swap_registry_prefix() {
  require_swap_registry || exit 1
  local prefix="${SWAP_REGISTRY#https://}"
  prefix="${prefix#http://}"
  printf '%s' "${prefix%/}"
}

swap_registry_host() {
  local prefix
  prefix="$(swap_registry_prefix)"
  printf '%s' "${prefix%%/*}"
}

login_swap_registry() {
  local prefix host
  prefix="$(swap_registry_prefix)"
  OPENSHIFT_PUSH_REGISTRY="${prefix}"
  OPENSHIFT_PULL_REGISTRY="${prefix}"
  host="$(swap_registry_host)"
  info "SWAP_REGISTRY=${prefix} (host ${host})"
  if [[ -n "${SWAP_REPOSITORY:-}" ]]; then
    info "SWAP_REPOSITORY=${SWAP_REPOSITORY}"
  fi
  if [[ -n "$(resolved_pull_secret_path)" ]]; then
    login_registry_with_pull_secret "${host}" || exit 1
    return 0
  fi
  if ${CONTAINER_ENGINE} login --get-login "${host}" >/dev/null 2>&1; then
    return 0
  fi
  error "Not logged in to ${host}."
  error "Set PULL_SECRET to a kubernetes.io/dockerconfigjson Secret YAML (KIND_PULL_SECRET is still accepted), or run: ${CONTAINER_ENGINE} login ${host}"
  exit 1
}

push_component_image() {
  local component="$1"
  component_spec "${component}"
  local commit
  commit="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  local tag="${commit}-${OPENSHIFT_NAMESPACE}"
  tag="$(printf '%s' "${tag}" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9._-]+/-/g')"
  local repo
  repo="$(swap_image_repository "${component}")" || exit 1
  local push_ref="${repo}:${tag}"
  local target_arch
  target_arch="$(swap_target_goarch)" || exit 1

  info "Building ${component} from working tree for linux/${target_arch}..."
  ${CONTAINER_ENGINE} build --platform "linux/${target_arch}" -t "${LOCAL_IMAGE}" \
    -f "${REPO_ROOT}/${DOCKERFILE}" ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"} \
    --build-arg "TARGETARCH=${target_arch}" \
    --build-arg "TARGETOS=linux" \
    "${REPO_ROOT}/${BUILD_CONTEXT}"
  ${CONTAINER_ENGINE} tag "${LOCAL_IMAGE}" "${push_ref}"
  info "Pushing ${push_ref}..."
  # Pin the registry manifest digest from this push, not `inspect` of the
  # local image. Local digests (and the blob/config SHAs in podman progress)
  # are not the digest Quay stores, so a cluster pull by that digest fails
  # with manifest unknown. `podman push --digestfile` writes the digest the
  # registry accepted. Docker prints `digest: sha256:...` on stdout instead.
  local digestfile push_log digest=""
  digestfile="$(mktemp)"
  push_log="$(mktemp)"
  if ${CONTAINER_ENGINE} push --help 2>&1 | grep -q -- '--digestfile'; then
    ${CONTAINER_ENGINE} push --digestfile="${digestfile}" "${push_ref}"
    digest="$(tr -d '[:space:]' < "${digestfile}")"
  else
    ${CONTAINER_ENGINE} push "${push_ref}" >"${push_log}" 2>&1 || {
      cat "${push_log}"
      rm -f "${digestfile}" "${push_log}"
      exit 1
    }
    cat "${push_log}"
    digest="$(registry_digest_from_push_log < "${push_log}")"
  fi
  rm -f "${digestfile}" "${push_log}"
  if [[ "${digest}" == sha256:* ]]; then
    OPENSHIFT_SWAPPED_IMAGE="${repo}@${digest}"
  else
    OPENSHIFT_SWAPPED_IMAGE="${push_ref}"
  fi
}

rollout_component_image() {
  local component="$1"
  local image="$2"
  component_spec "${component}"
  local args=() c
  for c in ${CONTAINERS}; do
    args+=("${c}=${image}")
  done
  oc_cli set image "deployment/${DEPLOYMENT}" "${args[@]}" -n "${OPENSHIFT_NAMESPACE}"
  local desired
  desired="$(oc_cli get deployment "${DEPLOYMENT}" -n "${OPENSHIFT_NAMESPACE}" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)"
  if [[ "${desired:-0}" -lt 1 ]]; then
    oc_cli scale "deployment/${DEPLOYMENT}" -n "${OPENSHIFT_NAMESPACE}" --replicas=1
  fi
  oc_cli rollout restart "deployment/${DEPLOYMENT}" -n "${OPENSHIFT_NAMESPACE}"
  oc_cli rollout status "deployment/${DEPLOYMENT}" -n "${OPENSHIFT_NAMESPACE}" --timeout=180s
}

component_swap() {
  local component="$1"
  header "Swap ${component} (up)"
  require_openshift_cluster
  resolve_openshift_namespace
  validate_namespace_group
  if ! namespace_exists "${OPENSHIFT_NAMESPACE}"; then
    error "No OpenShift environment in namespace '${OPENSHIFT_NAMESPACE}'. Run 'make openshift-up' first."
    exit 1
  fi
  refuse_foreign_namespace "${OPENSHIFT_NAMESPACE}" >/dev/null
  login_swap_registry
  push_component_image "${component}"
  info "Rolling out ${component} to ${OPENSHIFT_SWAPPED_IMAGE}"
  rollout_component_image "${component}" "${OPENSHIFT_SWAPPED_IMAGE}"
  track_openshift_swap "${component}" "${OPENSHIFT_SWAPPED_IMAGE}"
  success "${component} swapped to working-tree image ${OPENSHIFT_SWAPPED_IMAGE}"
}

component_revert() {
  local component="$1"
  header "Swap ${component} (down)"
  require_openshift_cluster
  resolve_openshift_namespace
  validate_namespace_group
  if ! is_openshift_swapped "${component}"; then
    warn "${component} is already running the baseline image."
    return 0
  fi
  component_spec "${component}"
  info "Reverting ${component} to baseline image ${BASELINE_IMAGE}..."
  rollout_component_image "${component}" "${BASELINE_IMAGE}"
  clear_openshift_swap "${component}"
  success "${component} reverted to baseline."
}
