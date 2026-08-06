#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Building Container Images"

info "Building API server..."
${CONTAINER_ENGINE} build -t "${api_server_local}" \
  -f components/api-server/Dockerfile \
  --build-arg GIT_VERSION="${build_version}" \
  --build-arg BUILD_TIME="${build_time}" \
  components/api-server

info "Building control plane..."
${CONTAINER_ENGINE} build -t "${control_plane_local}" \
  -f components/control-plane/Dockerfile .

info "Building web console..."
${CONTAINER_ENGINE} build -t "${web_console_local}" \
  -f components/web-console/Dockerfile .

success "All images built"

info "Tagging images with registry refs..."
${CONTAINER_ENGINE} tag "${api_server_local}" "${api_server_ref}"
${CONTAINER_ENGINE} tag "${control_plane_local}" "${control_plane_ref}"
${CONTAINER_ENGINE} tag "${web_console_local}" "${web_console_ref}"

if cluster_exists; then
  info "Loading images into Kind cluster..."
  kind load docker-image "${api_server_ref}" --name "${KIND_CLUSTER_NAME}"
  kind load docker-image "${control_plane_ref}" --name "${KIND_CLUSTER_NAME}"
  kind load docker-image "${web_console_ref}" --name "${KIND_CLUSTER_NAME}"
  success "Images loaded into Kind"
fi
