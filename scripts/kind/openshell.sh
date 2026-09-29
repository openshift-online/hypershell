#!/usr/bin/env bash
# openshell.sh - run the openshell CLI against a Kind gateway from a container
# on the cluster's own podman network, instead of installing a CLI binary on
# the host.
#
# The deployed gateway/supervisor images can carry a downstream suffix (e.g.
# v0.0.116-rhaiv.6) identifying a build ahead of the last tagged public
# OpenShell release; no public CLI release is guaranteed proto-compatible with
# it (see specs/platform/openshell-gateway-database.spec.md and
# scripts/install-openshell.sh). The exact matching CLI is published only as
# the container image named by OPENSHELL_CLI_IMAGE (OPENSHELL_VERSION), which
# is Linux-only and cannot run natively on macOS.
#
# Kind's own nodes and its Gateway API load balancer (cloud-provider-kind) are
# themselves podman containers on a network named "kind" (see
# KIND_EXPERIMENTAL_PROVIDER=podman in lib.sh). Attaching a CLI container to
# that same network lets it reach the cluster's real ingress by container IP -
# the same Envoy proxy a browser or a Linux CLI would hit - with no host
# port-forwarding involved and no macOS/Linux binary mismatch, since the CLI
# never leaves its (Linux) container.
#
# Usage:
#   scripts/kind/openshell.sh -g <gateway-name> <openshell-args...>
#   make kind-openshell ARGS="-g dev sandbox create --name mysand"
#
# <gateway-name> must already be registered on this host, i.e.
# ~/.config/openshell/gateways/<gateway-name>/ must exist (openshell gateway
# add, or make kind-seed).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

if [[ "$(basename "${CONTAINER_ENGINE}")" != "podman" ]]; then
  error "This wrapper needs podman (found: ${CONTAINER_ENGINE}) -- Kind's shared container network is podman-specific here."
  exit 1
fi

# --- Resolve the gateway name the same way the real CLI would: -g/--gateway,
# falling back to OPENSHELL_GATEWAY. ---
GATEWAY_NAME="${OPENSHELL_GATEWAY:-}"
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
  case "${args[$i]}" in
    -g|--gateway)
      GATEWAY_NAME="${args[$((i + 1))]:-}"
      ;;
    -g=*)
      GATEWAY_NAME="${args[$i]#-g=}"
      ;;
    --gateway=*)
      GATEWAY_NAME="${args[$i]#--gateway=}"
      ;;
  esac
done
if [[ -z "${GATEWAY_NAME}" ]]; then
  error "No gateway specified. Pass -g <name> (must already be registered: ~/.config/openshell/gateways/<name>/)."
  exit 1
fi

GATEWAY_CFG_DIR="${HOME}/.config/openshell/gateways/${GATEWAY_NAME}"
METADATA_FILE="${GATEWAY_CFG_DIR}/metadata.json"
if [[ ! -f "${METADATA_FILE}" ]]; then
  error "No registered gateway '${GATEWAY_NAME}' (missing ${METADATA_FILE})."
  error "Register it first (openshell gateway add ..., or make kind-seed)."
  exit 1
fi

GW_HOST=$(python3 -c "
import json, urllib.parse
with open('${METADATA_FILE}') as f:
    meta = json.load(f)
print(urllib.parse.urlparse(meta['gateway_endpoint']).hostname or '')
" 2>/dev/null || true)
if [[ -z "${GW_HOST}" ]]; then
  error "Could not read a gateway_endpoint hostname from ${METADATA_FILE}"
  exit 1
fi

# The gateway workload's namespace is embedded in its route hostname
# (gw-<namespace>.gw.localhost - see openshell-gateway-routing.spec.md).
GW_NAMESPACE="${GW_HOST#gw-}"
GW_NAMESPACE="${GW_NAMESPACE%.gw.localhost}"

GW_DEPLOYED_IMAGE=$(kube get deployment openshell-gateway -n "${GW_NAMESPACE}" \
  -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
if [[ -z "${GW_DEPLOYED_IMAGE}" ]]; then
  error "Could not read the deployed gateway image in namespace ${GW_NAMESPACE}."
  error "Is '${GATEWAY_NAME}' really a gateway on this cluster?"
  exit 1
fi
CLI_IMAGE="${OPENSHELL_CLI_IMAGE}:${GW_DEPLOYED_IMAGE##*:}"

info "Gateway ${GATEWAY_NAME} -> namespace ${GW_NAMESPACE}, CLI image ${CLI_IMAGE}"

# --- The cluster's ingress load-balancer IP, on Kind's own podman network.
# Same lookup up.sh uses to patch in-cluster CoreDNS (patch_cluster_coredns);
# reused here because it is already the project's own way to find this
# address, not because of anything specific to this script. ---
GW_ADDR=$(kube get gateway hypershell-gw -n "${KIND_NAMESPACE}" \
  -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
if [[ -z "${GW_ADDR}" ]]; then
  error "Networking Gateway hypershell-gw has no address yet. Is cloud-provider-kind running? (make kind-up)"
  exit 1
fi

# --- Point this one run at the in-cluster-network endpoint (port 443, the
# Envoy proxy's real listener) instead of the host-mapped kind-fix-ports
# endpoint, which is not reachable from inside a container. Do this in a
# scratch copy of the gateway's config directory, not the real one: the
# registered gateway_endpoint stays usable from the Mac itself, and this
# script never writes back a refreshed token or other CLI state into it. ---
SCRATCH_BASE="${HOME}/.cache/hypershell-kind-openshell"
mkdir -p "${SCRATCH_BASE}"
SCRATCH_DIR="$(mktemp -d "${SCRATCH_BASE}/run.XXXXXX")"
trap 'rm -rf "${SCRATCH_DIR}"' EXIT
mkdir -p "${SCRATCH_DIR}/gateways"
cp -R "${GATEWAY_CFG_DIR}" "${SCRATCH_DIR}/gateways/${GATEWAY_NAME}"
python3 -c "
import json
path = '${SCRATCH_DIR}/gateways/${GATEWAY_NAME}/metadata.json'
with open(path) as f:
    meta = json.load(f)
meta['gateway_endpoint'] = 'https://${GW_HOST}:443'
meta['gateway_insecure'] = True
with open(path, 'w') as f:
    json.dump(meta, f)
"

# --- Mint a fresh OIDC token rather than reusing whatever is stored. Access
# tokens for this dev realm expire in minutes, and a wrapper someone reruns
# throughout a session is exactly where a stale one becomes annoying rather
# than a one-off. Local dev seeds a fixed admin/admin user (see
# reconcile_keycloak_seed_users in lib.sh); override via
# OPENSHELL_DEV_OIDC_USERNAME/PASSWORD for a different registered user. ---
OIDC_ISSUER=$(python3 -c "
import json
with open('${METADATA_FILE}') as f:
    print(json.load(f).get('oidc_issuer', ''))
")
OIDC_CLIENT_ID=$(python3 -c "
import json
with open('${METADATA_FILE}') as f:
    print(json.load(f).get('oidc_client_id', ''))
")
if [[ -n "${OIDC_ISSUER}" && -n "${OIDC_CLIENT_ID}" ]]; then
  TOKEN_RESPONSE=$(curl -sk --ipv4 -X POST "${OIDC_ISSUER}/protocol/openid-connect/token" \
    -d "grant_type=password" \
    -d "client_id=${OIDC_CLIENT_ID}" \
    -d "username=${OPENSHELL_DEV_OIDC_USERNAME:-admin}" \
    -d "password=${OPENSHELL_DEV_OIDC_PASSWORD:-admin}" 2>/dev/null || true)
  ACCESS_TOKEN=$(echo "${TOKEN_RESPONSE}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)
  if [[ -n "${ACCESS_TOKEN}" ]]; then
    python3 -c "
import json
path = '${SCRATCH_DIR}/gateways/${GATEWAY_NAME}/oidc_token.json'
with open(path, 'w') as f:
    json.dump({'access_token': '''${ACCESS_TOKEN}''', 'issuer': '${OIDC_ISSUER}', 'client_id': '${OIDC_CLIENT_ID}'}, f)
"
  else
    warn "Could not mint a fresh OIDC token; reusing the stored one (may be expired)."
  fi
fi

TTY_FLAGS=()
if [[ -t 0 && -t 1 ]]; then
  TTY_FLAGS=(-it)
fi

exec "${CONTAINER_ENGINE}" run --rm ${TTY_FLAGS[@]+"${TTY_FLAGS[@]}"} \
  --network kind \
  --add-host "${GW_HOST}:${GW_ADDR}" \
  --add-host "${KEYCLOAK_HOSTNAME}:${GW_ADDR}" \
  -v "${SCRATCH_DIR}:/home/cli/.config/openshell:Z" \
  -e HOME=/home/cli \
  -e OPENSHELL_GATEWAY_INSECURE=true \
  "${CLI_IMAGE}" "${args[@]}"
