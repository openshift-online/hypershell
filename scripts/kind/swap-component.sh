#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

ACTION="${1:-}"
COMPONENT="${2:-}"

if [[ -z "${ACTION}" ]] || [[ -z "${COMPONENT}" ]]; then
  error "Usage: swap-component.sh up|down <api-server|control-plane|web-console>"
  exit 1
fi

# Map component name to deployment name, container name(s), image vars
case "${COMPONENT}" in
  api-server)
    DEPLOYMENT="hypershell-api-server"
    CONTAINERS="api-server migrate"
    LOCAL_IMAGE="${api_server_local}"
    BASELINE_IMAGE="${api_server_ref}"
    DOCKERFILE="components/api-server/Dockerfile"
    BUILD_CONTEXT="components/api-server"
    BUILD_ARGS="--build-arg GIT_VERSION=${build_version} --build-arg BUILD_TIME=${build_time}"
    ;;
  control-plane)
    DEPLOYMENT="hypershell-controller"
    CONTAINERS="controller"
    LOCAL_IMAGE="${control_plane_local}"
    BASELINE_IMAGE="${control_plane_ref}"
    DOCKERFILE="components/control-plane/Dockerfile"
    BUILD_CONTEXT="."
    BUILD_ARGS=""
    ;;
  web-console)
    DEPLOYMENT="hypershell-web-console"
    CONTAINERS="web-console"
    LOCAL_IMAGE="${web_console_local}"
    BASELINE_IMAGE="${web_console_ref}"
    DOCKERFILE="components/web-console/Dockerfile"
    BUILD_CONTEXT="."
    BUILD_ARGS=""
    ;;
  *)
    error "Unknown component: ${COMPONENT}"
    error "Valid components: api-server, control-plane, web-console"
    exit 1
    ;;
esac

swap_up() {
  # Web console hot reload mode
  if [[ "${COMPONENT}" == "web-console" ]] && [[ "${KIND_HOT_RELOAD:-true}" == "true" ]]; then
    header "Web Console (hot reload)"
    info "Mounting ${KIND_HOST_MOUNT_PATH:-$(pwd)}/components/web-console into pod..."
    kubectl patch deployment "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --type=json -p='[
      {"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "node:22-slim"},
      {"op": "replace", "path": "/spec/template/spec/containers/0/command", "value": ["sh", "-c", "cd /mnt/host/components/web-console && npm run dev -- --host 0.0.0.0 --port 8080"]},
      {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts", "value": [{"name": "host-src", "mountPath": "/mnt/host"}]},
      {"op": "add", "path": "/spec/template/spec/volumes", "value": [{"name": "host-src", "hostPath": {"path": "/mnt/host"}}]},
      {"op": "replace", "path": "/spec/template/spec/containers/0/env", "value": [{"name": "PORT", "value": "8080"}, {"name": "HOST", "value": "0.0.0.0"}]}
    ]'
    info "Waiting for web console (hot reload)..."
    kubectl wait --for=condition=available "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
    track_swap "${COMPONENT}"
    echo ""
    success "Web console running in hot reload mode."
    info "File changes in ${KIND_HOST_MOUNT_PATH:-$(pwd)}/components/web-console are reflected immediately."
    return
  fi

  if [[ "${COMPONENT}" == "web-console" ]] || [[ "${COMPONENT}" == "api-server" ]] || [[ "${COMPONENT}" == "control-plane" ]]; then
    if [[ "${COMPONENT}" != "web-console" ]] && [[ "${KIND_HOT_RELOAD:-true}" == "true" ]]; then
      info "Hot reload is not supported for ${COMPONENT}, using rebuild-and-replace"
    fi
  fi

  header "Swap ${COMPONENT} (up)"

  info "Building ${COMPONENT} from working tree..."
  # shellcheck disable=SC2086
  ${CONTAINER_ENGINE} build -t "${LOCAL_IMAGE}" \
    -f "${DOCKERFILE}" ${BUILD_ARGS} "${BUILD_CONTEXT}"

  info "Loading image into Kind..."
  local tar_file="/tmp/hypershell-${COMPONENT}-dev.tar"
  rm -f "${tar_file}"
  ${CONTAINER_ENGINE} save -o "${tar_file}" "${LOCAL_IMAGE}"
  kind load image-archive "${tar_file}" --name "${KIND_CLUSTER_NAME}"
  rm -f "${tar_file}"

  info "Replacing ${COMPONENT} deployment..."
  local set_image_args=""
  for container in ${CONTAINERS}; do
    set_image_args="${set_image_args} ${container}=${LOCAL_IMAGE}"
  done
  # shellcheck disable=SC2086
  kubectl set image "deployment/${DEPLOYMENT}" ${set_image_args} -n "${KIND_NAMESPACE}"
  kubectl rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}"

  info "Waiting for ${COMPONENT}..."
  kubectl wait --for=condition=available "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
  track_swap "${COMPONENT}"
  success "${COMPONENT} swapped to local build."
}

swap_down() {
  header "Swap ${COMPONENT} (down)"

  if ! is_swapped "${COMPONENT}"; then
    warn "${COMPONENT} is already running the baseline image."
    return
  fi

  info "Reverting ${COMPONENT} to baseline image..."

  if [[ "${COMPONENT}" == "web-console" ]]; then
    kubectl apply -f deploy/kind/web-console.yaml
  else
    local set_image_args=""
    for container in ${CONTAINERS}; do
      set_image_args="${set_image_args} ${container}=${BASELINE_IMAGE}"
    done
    # shellcheck disable=SC2086
    kubectl set image "deployment/${DEPLOYMENT}" ${set_image_args} -n "${KIND_NAMESPACE}"
  fi

  kubectl rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}"
  info "Waiting for ${COMPONENT}..."
  kubectl wait --for=condition=available "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
  clear_swap "${COMPONENT}"
  success "${COMPONENT} reverted to baseline."
}

case "${ACTION}" in
  up)   swap_up   ;;
  down) swap_down ;;
  *)
    error "Unknown action: ${ACTION}"
    error "Valid actions: up, down"
    exit 1
    ;;
esac
