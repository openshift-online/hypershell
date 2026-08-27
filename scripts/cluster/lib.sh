#!/usr/bin/env bash
# Shared seams for the cluster lifecycle driver model.
# Kind and OpenShift drivers implement cluster_up, cluster_down, cluster_teardown,
# cluster_status, component_swap, and component_revert.
set -euo pipefail

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  BOLD='\033[1m'
  BLUE='\033[0;34m'
  CYAN='\033[0;36m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RED='\033[0;31m'
  NC='\033[0m'
else
  BOLD='' BLUE='' CYAN='' GREEN='' YELLOW='' RED='' NC=''
fi

header()  { printf "${BOLD}${BLUE}==> %s${NC}\n" "$*"; }
info()    { printf "${CYAN}    %s${NC}\n" "$*"; }
success() { printf "${GREEN}    %s${NC}\n" "$*"; }
warn()    { printf "${YELLOW}    %s${NC}\n" "$*"; }
error()   { printf "${RED}ERROR: %s${NC}\n" "$*" >&2; }

CLUSTER_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${CLUSTER_SCRIPT_DIR}/../.." && pwd)"

: "${CONTAINER_ENGINE:=$(command -v podman 2>/dev/null || echo docker)}"
: "${IMAGE_REGISTRY:=quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main}"
: "${IMAGE_TAG:=latest}"
: "${api_server_ref:=${IMAGE_REGISTRY}/hypershell-api-server-main:${IMAGE_TAG}}"
: "${control_plane_ref:=${IMAGE_REGISTRY}/hypershell-control-plane-main:${IMAGE_TAG}}"
: "${web_console_ref:=${IMAGE_REGISTRY}/hypershell-web-console-main:${IMAGE_TAG}}"
: "${api_server_local:=localhost/hypershell:dev}"
: "${control_plane_local:=localhost/hypershell-controller:dev}"
: "${web_console_local:=localhost/hypershell-web-console:dev}"
: "${build_version:=$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
: "${build_time:=$(date -u '+%Y-%m-%d %H:%M:%S UTC')}"
: "${GATEWAY_IMAGE:=ghcr.io/nvidia/openshell/gateway:0.0.109}"
: "${GATEWAY_API_GATEWAY_NAME:=openshell-grpc-gateway}"
: "${GATEWAY_API_GATEWAY_NAMESPACE:=openshift-ingress}"

# RFC 1123 DNS label: [a-z0-9]([a-z0-9-]*[a-z0-9])? up to 63 characters.
# OpenShift platform namespaces are capped at 54 so "${ns}-keycloak" stays <= 63.
RFC1123_LABEL_RE='^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'

validate_rfc1123_label() {
  local name="$1"
  local max="${2:-63}"
  if [[ -z "${name}" ]]; then
    error "Name is empty (must be an RFC 1123 DNS label of at most ${max} characters)"
    return 1
  fi
  if (( ${#name} > max )); then
    error "Name '${name}' is ${#name} characters; maximum is ${max}"
    return 1
  fi
  if [[ ! "${name}" =~ ${RFC1123_LABEL_RE} ]]; then
    error "Name '${name}' is not a valid RFC 1123 DNS label (lowercase alphanumeric and '-', must start and end with alphanumeric)"
    return 1
  fi
}

# Map an identity (oc whoami, email, kube:admin) to an RFC 1123 DNS label.
sanitize_dns_label() {
  local raw="$1"
  local max="${2:-54}"
  local out
  out=$(printf '%s' "${raw}" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9-]+/-/g; s/-+/-/g; s/^-+//; s/-+$//')
  if [[ -z "${out}" ]]; then
    printf ''
    return 1
  fi
  if [[ ! "${out:0:1}" =~ [a-z0-9] ]]; then
    out="ns-${out}"
  fi
  if (( ${#out} > max )); then
    out="${out:0:${max}}"
    out="${out%-}"
  fi
  if [[ ! "${out: -1}" =~ [a-z0-9] ]]; then
    out="${out%-}x"
  fi
  printf '%s' "${out}"
}

keycloak_namespace_for() {
  printf '%s-keycloak' "$1"
}

# Cluster-scoped / system namespaces that local-dev must never claim.
is_reserved_cluster_namespace() {
  case "$1" in
    default|openshift|kube-system|kube-public|kube-node-lease) return 0 ;;
    kube-*|openshift-*) return 0 ;;
  esac
  return 1
}

# Strip a Gateway listener hostname (e.g. *.openshell.example.com) down to the
# tenant base domain the control plane expects (openshell.example.com).
gateway_base_domain_from_hostname() {
  local host="$1"
  host="${host#\*.}"
  printf '%s' "${host}"
}

load_cluster_driver() {
  local driver="${CLUSTER_DRIVER:-}"
  if [[ -z "${driver}" ]]; then
    error "CLUSTER_DRIVER is not set. Use 'make kind-up' or 'make openshift-up'."
    return 1
  fi
  local path="${CLUSTER_SCRIPT_DIR}/drivers/${driver}.sh"
  if [[ ! -f "${path}" ]]; then
    error "Unknown cluster driver '${driver}'. Expected ${path}"
    error "Available drivers: kind, openshift"
    return 1
  fi
  # shellcheck source=/dev/null
  source "${path}"
  local fn
  for fn in cluster_up cluster_down cluster_teardown cluster_status component_swap component_revert; do
    if ! declare -F "${fn}" >/dev/null; then
      error "Driver '${driver}' does not implement ${fn}"
      return 1
    fi
  done
}

# Component lookup used by both drivers' swap paths.
# Sets: DEPLOYMENT CONTAINERS LOCAL_IMAGE BASELINE_IMAGE DOCKERFILE BUILD_CONTEXT BUILD_ARGS
component_spec() {
  local component="$1"
  BUILD_ARGS=()
  case "${component}" in
    api-server)
      DEPLOYMENT="hypershell-api-server"
      CONTAINERS="api-server migrate"
      LOCAL_IMAGE="${api_server_local}"
      BASELINE_IMAGE="${api_server_ref}"
      DOCKERFILE="components/api-server/Dockerfile"
      BUILD_CONTEXT="components/api-server"
      BUILD_ARGS=(--build-arg "GIT_VERSION=${build_version}" --build-arg "BUILD_TIME=${build_time}")
      ;;
    control-plane)
      DEPLOYMENT="hypershell-controller"
      CONTAINERS="controller"
      LOCAL_IMAGE="${control_plane_local}"
      BASELINE_IMAGE="${control_plane_ref}"
      DOCKERFILE="components/control-plane/Dockerfile"
      BUILD_CONTEXT="."
      ;;
    web-console)
      DEPLOYMENT="hypershell-web-console"
      CONTAINERS="web-console"
      LOCAL_IMAGE="${web_console_local}"
      BASELINE_IMAGE="${web_console_ref}"
      DOCKERFILE="components/web-console/Dockerfile"
      BUILD_CONTEXT="."
      ;;
    *)
      error "Unknown component: ${component}"
      error "Valid components: api-server, control-plane, web-console"
      return 1
      ;;
  esac
}

# Per-namespace OpenShift swap ledger. Kind keeps using scripts/kind/.kind-swaps.
openshift_swap_dir() {
  printf '%s/.openshift-swaps' "${REPO_ROOT}"
}

openshift_swap_file() {
  local ns="${1:-${OPENSHIFT_NAMESPACE:-}}"
  printf '%s/%s' "$(openshift_swap_dir)" "${ns}"
}

track_openshift_swap() {
  local component="$1"
  local image="$2"
  local file
  file="$(openshift_swap_file)"
  mkdir -p "$(openshift_swap_dir)"
  touch "${file}"
  if grep -q "^${component}[[:space:]]" "${file}" 2>/dev/null; then
    local tmp
    tmp="$(mktemp)"
    sed "/^${component}[[:space:]]/d" "${file}" > "${tmp}"
    mv "${tmp}" "${file}"
  fi
  printf '%s\t%s\n' "${component}" "${image}" >> "${file}"
}

clear_openshift_swap() {
  local component="$1"
  local file
  file="$(openshift_swap_file)"
  if [[ -f "${file}" ]]; then
    local tmp
    tmp="$(mktemp)"
    sed "/^${component}[[:space:]]/d" "${file}" > "${tmp}"
    mv "${tmp}" "${file}"
    if [[ ! -s "${file}" ]]; then
      rm -f "${file}"
    fi
  fi
}

is_openshift_swapped() {
  local component="$1"
  local file
  file="$(openshift_swap_file)"
  [[ -f "${file}" ]] && grep -q "^${component}[[:space:]]" "${file}"
}

openshift_swap_image() {
  local component="$1"
  local file
  file="$(openshift_swap_file)"
  [[ -f "${file}" ]] || return 0
  awk -F '\t' -v c="${component}" '$1 == c { print $2; exit }' "${file}"
}

clear_all_openshift_swaps() {
  rm -f "$(openshift_swap_file)"
}

json_string_field() {
  python3 -c 'import json,sys
doc=json.load(sys.stdin)
val=doc.get(sys.argv[1],"")
print("" if val is None else val)
' "$1"
}

json_first_id() {
  python3 -c 'import json,sys
docs=json.load(sys.stdin)
print(docs[0]["id"] if docs else "")
'
}

# Restrict a Keycloak client representation to this console origin.
# Spec: oidc-integration Identity Provider Client Security — no wildcards.
keycloak_client_with_console_redirects() {
  local console_host="$1"
  python3 -c 'import json,sys
host=sys.argv[1]
doc=json.load(sys.stdin)
doc["redirectUris"]=[f"https://{host}/auth/callback", f"https://{host}"]
json.dump(doc, sys.stdout)
' "${console_host}"
}
