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
: "${GATEWAY_IMAGE:=quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0@sha256:a80b79e514826e8d57ea137749cf18a6e7f3d92e26bfefe005f3a9c4a55b8bdd}"
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

# True when a registry hostname only resolves inside the cluster. Laptop
# podman/docker cannot push to these names.
registry_host_is_cluster_local() {
  local host="${1:-}"
  host="${host#https://}"
  host="${host#http://}"
  host="${host%%/*}"
  case "${host}" in
    *.svc|*.svc:*|*.cluster.local|*.cluster.local:*) return 0 ;;
  esac
  return 1
}

# SWAP_REGISTRY is the laptop-push org prefix for OpenShift component swaps
# (for example quay.io/<org>). IMAGE_REGISTRY is the baseline/pull path and is
# never used as the swap destination. Repo names default to hypershell-api-server,
# hypershell-controller, and hypershell-web-console; SWAP_REPOSITORY overrides
# the repo name for the current swap.
require_swap_registry() {
  if [[ -z "${SWAP_REGISTRY:-}" ]]; then
    error "SWAP_REGISTRY is unset. Set it to a registry org prefix this cluster can pull (for example quay.io/<org>)."
    error "IMAGE_REGISTRY is only for baseline images; swaps do not push there."
    return 1
  fi
  local prefix="${SWAP_REGISTRY#https://}"
  prefix="${prefix#http://}"
  prefix="${prefix%/}"
  if registry_host_is_cluster_local "${prefix}"; then
    error "SWAP_REGISTRY=${SWAP_REGISTRY} is only reachable inside the cluster."
    error "Set SWAP_REGISTRY to a Quay org prefix this cluster can pull (for example quay.io/<org>)."
    return 1
  fi
  if [[ "${prefix}" != */* ]]; then
    error "SWAP_REGISTRY must be a registry org prefix (for example quay.io/<org>), not '${SWAP_REGISTRY}'."
    return 1
  fi
}

# GOARCH for OpenShift swap images. Laptop architecture is not used: ROSA and
# most OpenShift nodes are amd64, while developer laptops may be arm64.
# SWAP_PLATFORM (linux/amd64 or linux/arm64) wins; otherwise the first node's
# architecture is used.
swap_target_goarch() {
  local raw="${SWAP_PLATFORM:-${SWAP_ARCH:-}}"
  if [[ -z "${raw}" ]]; then
    raw="$(oc get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}' 2>/dev/null || true)"
  fi
  case "${raw}" in
    linux/amd64|amd64|x86_64) printf 'amd64' ;;
    linux/arm64|arm64|aarch64) printf 'arm64' ;;
    '')
      error "Could not detect OpenShift node architecture. Set SWAP_PLATFORM=linux/amd64 (or linux/arm64) to match the cluster nodes."
      return 1
      ;;
    *)
      error "Unsupported swap architecture '${raw}'. Set SWAP_PLATFORM=linux/amd64 or linux/arm64."
      return 1
      ;;
  esac
}

swap_default_repository() {
  case "${1:-}" in
    api-server) printf 'hypershell-api-server' ;;
    control-plane) printf 'hypershell-controller' ;;
    web-console) printf 'hypershell-web-console' ;;
    *)
      error "Unknown component: ${1:-}"
      return 1
      ;;
  esac
}

swap_repository_for_component() {
  if [[ -n "${SWAP_REPOSITORY:-}" ]]; then
    printf '%s' "${SWAP_REPOSITORY}"
    return 0
  fi
  swap_default_repository "$1"
}

swap_image_repository() {
  local prefix repo_name
  require_swap_registry || return 1
  prefix="${SWAP_REGISTRY#https://}"
  prefix="${prefix#http://}"
  prefix="${prefix%/}"
  repo_name="$(swap_repository_for_component "$1")" || return 1
  printf '%s/%s' "${prefix}" "${repo_name}"
}

# Registry manifest digest from a docker-style push log (`digest: sha256:...`).
# Ignore unanchored sha256 values: podman progress prints blob and config
# digests that are not the digest the registry stores for the tag.
registry_digest_from_push_log() {
  sed -nE 's/.*digest: (sha256:[0-9a-f]{64}).*/\1/p' | tail -1
}

# PULL_SECRET is the canonical path to a kubernetes.io/dockerconfigjson Secret
# YAML (or a raw Docker config JSON). KIND_PULL_SECRET remains an alias.
resolved_pull_secret_path() {
  printf '%s' "${PULL_SECRET:-${KIND_PULL_SECRET:-}}"
}

# Print {"username":"...","password":"..."} for registry host from a pull secret.
registry_auth_json_for_host() {
  local file="${1:-}" host="${2:-}"
  if [[ -z "${file}" || ! -f "${file}" ]]; then
    error "Pull secret file is missing: ${file:-'(empty path)'}"
    return 1
  fi
  python3 - "${file}" "${host}" <<'PY'
import base64, json, re, sys

path, host = sys.argv[1], sys.argv[2]
text = open(path, encoding="utf-8").read()

def load_dockerconfig(raw: str):
    raw = raw.strip()
    try:
        data = json.loads(raw)
    except json.JSONDecodeError:
        data = None
    if isinstance(data, dict) and "auths" in data:
        return data
    if isinstance(data, dict):
        b64 = (data.get("data") or {}).get(".dockerconfigjson")
        if b64:
            return json.loads(base64.b64decode(b64))
    match = re.search(
        r"^\s*\.dockerconfigjson:\s*[\"']?([A-Za-z0-9+/=]+)[\"']?\s*$",
        raw,
        re.M,
    )
    if not match:
        raise SystemExit("no dockerconfigjson in pull secret")
    blob = match.group(1)
    decoded = base64.b64decode(blob)
    try:
        return json.loads(decoded.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SystemExit(
            "pull secret .dockerconfigjson is not JSON; the YAML value may be truncated"
        ) from exc

def credentials(entry: dict):
    user, password = entry.get("username"), entry.get("password")
    if user and password:
        return user, password
    auth = entry.get("auth")
    if not auth:
        return None
    decoded = base64.b64decode(auth).decode("utf-8")
    user, password = decoded.split(":", 1)
    return user, password

cfg = load_dockerconfig(text)
auths = cfg.get("auths") or {}
candidates = [
    host,
    f"https://{host}",
    f"http://{host}",
    f"https://{host}/v1/",
    f"https://{host}/v2/",
]
entry = None
for key in candidates:
    if key in auths:
        entry = auths[key]
        break
if entry is None:
    for key, value in auths.items():
        if host in key:
            entry = value
            break
if entry is None:
    raise SystemExit(f"pull secret has no auth for {host}")
pair = credentials(entry)
if not pair:
    raise SystemExit(f"pull secret auth for {host} has no username/password")
json.dump({"username": pair[0], "password": pair[1]}, sys.stdout)
PY
}

login_registry_with_pull_secret() {
  local host="$1"
  local file
  file="$(resolved_pull_secret_path)"
  if [[ -z "${file}" ]]; then
    error "PULL_SECRET is unset. Set it to a kubernetes.io/dockerconfigjson Secret YAML (KIND_PULL_SECRET is still accepted)."
    return 1
  fi
  local auth_json user password
  if ! auth_json="$(registry_auth_json_for_host "${file}" "${host}")"; then
    error "Could not read ${host} credentials from ${file}."
    return 1
  fi
  user="$(printf '%s' "${auth_json}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["username"])')"
  password="$(printf '%s' "${auth_json}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["password"])')"
  info "Logging in to ${host} with PULL_SECRET"
  if ! printf '%s' "${password}" | ${CONTAINER_ENGINE} login --username "${user}" --password-stdin "${host}" >/dev/null; then
    error "Failed to log in to ${host} with credentials from ${file}."
    return 1
  fi
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
  for fn in cluster_up cluster_down cluster_teardown cluster_status cluster_seed component_swap component_revert; do
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
# Spec: oidc-integration Identity Provider Client Security: no wildcards.
keycloak_client_with_console_redirects() {
  local console_host="$1"
  python3 -c 'import json,sys
host=sys.argv[1]
doc=json.load(sys.stdin)
doc["redirectUris"]=[f"https://{host}/auth/callback", f"https://{host}"]
json.dump(doc, sys.stdout)
' "${console_host}"
}

# SKIP_SEED and SEED_STRICT apply to Kind and OpenShift. KIND_* names remain aliases.
skip_seed() {
  case "${SKIP_SEED:-${KIND_SKIP_SEED:-}}" in
    true|TRUE|1|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

seed_strict() {
  case "${SEED_STRICT:-${KIND_SEED_STRICT:-}}" in
    true|TRUE|1|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}
