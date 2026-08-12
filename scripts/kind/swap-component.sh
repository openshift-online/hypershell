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
    BUILD_ARGS=(--build-arg "GIT_VERSION=${build_version}" --build-arg "BUILD_TIME=${build_time}")
    ;;
  control-plane)
    DEPLOYMENT="hypershell-controller"
    CONTAINERS="controller"
    LOCAL_IMAGE="${control_plane_local}"
    BASELINE_IMAGE="${control_plane_ref}"
    DOCKERFILE="components/control-plane/Dockerfile"
    BUILD_CONTEXT="."
    BUILD_ARGS=()
    ;;
  web-console)
    DEPLOYMENT="hypershell-web-console"
    CONTAINERS="web-console"
    LOCAL_IMAGE="${web_console_local}"
    BASELINE_IMAGE="${web_console_ref}"
    DOCKERFILE="components/web-console/Dockerfile"
    BUILD_CONTEXT="."
    BUILD_ARGS=()
    ;;
  *)
    error "Unknown component: ${COMPONENT}"
    error "Valid components: api-server, control-plane, web-console"
    exit 1
    ;;
esac

swap_up() {
  # Web console hot reload mode: run Vite dev server on the host,
  # redirect the in-cluster Service so the Gateway routes to it.
  if [[ "${COMPONENT}" == "web-console" ]] && [[ "${KIND_HOT_RELOAD:-true}" == "true" ]]; then
    header "Web Console (hot reload)"

    DEV_PORT=5173

    # Discover which container engine owns the existing Kind node. This may
    # differ from CONTAINER_ENGINE when both Docker and Podman are installed.
    KIND_ENGINE=""
    for engine in "${CONTAINER_ENGINE}" docker podman; do
      if command -v "${engine}" >/dev/null 2>&1 && \
          "${engine}" inspect "${KIND_CLUSTER_NAME}-control-plane" >/dev/null 2>&1; then
        KIND_ENGINE="${engine}"
        break
      fi
    done

    # Discover a host IP reachable from inside the Kind cluster.
    # Candidate IPs are tested by running a connectivity check from the
    # Kind control-plane node.  In rootless Podman the Kind network bridge
    # gateway is often unreachable from pods, so we try multiple candidates.
    candidate_ips=()
    if [[ -n "${KIND_ENGINE}" ]]; then
      # Kind network bridge gateway (works with Docker, sometimes Podman)
      gw=$("${KIND_ENGINE}" inspect "${KIND_CLUSTER_NAME}-control-plane" \
        -f '{{.NetworkSettings.Networks.kind.Gateway}}' 2>/dev/null || true)
      [[ -n "${gw}" ]] && candidate_ips+=("${gw}")
      # Docker/Podman bridge gateway (172.17.0.1 etc.)
      for net in bridge podman; do
        bgw=$("${KIND_ENGINE}" inspect "${KIND_CLUSTER_NAME}-control-plane" \
          -f "{{.NetworkSettings.Networks.${net}.Gateway}}" 2>/dev/null || true)
        [[ -n "${bgw}" ]] && candidate_ips+=("${bgw}")
      done
    fi
    # Host-routable aliases (macOS/Windows VMs)
    LOOKUP_ENGINE="${KIND_ENGINE:-${CONTAINER_ENGINE}}"
    for host_alias in host.docker.internal host.containers.internal; do
      alias_ip=$("${LOOKUP_ENGINE}" run --rm alpine getent hosts "${host_alias}" 2>/dev/null | awk '{print $1}' || true)
      [[ -n "${alias_ip}" ]] && candidate_ips+=("${alias_ip}")
    done

    HOST_IP=""
    for candidate in "${candidate_ips[@]}"; do
      if "${KIND_ENGINE:-${CONTAINER_ENGINE}}" exec "${KIND_CLUSTER_NAME}-control-plane" \
          sh -c "cat < /dev/tcp/${candidate}/${DEV_PORT}" >/dev/null 2>&1; then
        HOST_IP="${candidate}"
        break
      fi
    done
    # If nothing is listening yet (dev server not started), pick first candidate
    # that is at least routable (TCP connect to a common port).
    if [[ -z "${HOST_IP}" ]]; then
      for candidate in "${candidate_ips[@]}"; do
        if "${KIND_ENGINE:-${CONTAINER_ENGINE}}" exec "${KIND_CLUSTER_NAME}-control-plane" \
            sh -c "cat < /dev/tcp/${candidate}/22 || cat < /dev/tcp/${candidate}/5173" >/dev/null 2>&1; then
          HOST_IP="${candidate}"
          break
        fi
      done
    fi
    # Last resort: use first candidate and hope for the best
    if [[ -z "${HOST_IP}" ]] && [[ ${#candidate_ips[@]} -gt 0 ]]; then
      HOST_IP="${candidate_ips[0]}"
    fi
    if [[ -z "${HOST_IP}" ]]; then
      error "Could not determine host IP reachable from Kind cluster"
      exit 1
    fi

    # Pull OIDC env vars from the deployment so the local dev server has them.
    DEPLOY_ENV=$(kube get deployment "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" \
      -o jsonpath='{range .spec.template.spec.containers[0].env[*]}{.name}={.value}{"\n"}{end}' 2>/dev/null || true)
    for var in OIDC_ISSUER OIDC_CLIENT_ID OIDC_REDIRECT_URI OIDC_POST_LOGOUT_REDIRECT_URI NODE_TLS_REJECT_UNAUTHORIZED; do
      val=$(echo "${DEPLOY_ENV}" | grep "^${var}=" | head -1 | cut -d= -f2- || true)
      if [[ -n "${val}" ]]; then
        export "${var}=${val}"
      fi
    done
    # SESSION_SECRET is stored in a K8s Secret, not inline.
    if echo "${DEPLOY_ENV}" | grep -q "^SESSION_SECRET=" 2>/dev/null; then
      SECRET_VAL=$(kube get secret hypershell-oidc-session -n "${KIND_NAMESPACE}" \
        -o jsonpath='{.data.session-secret}' 2>/dev/null | base64 -d || true)
      if [[ -n "${SECRET_VAL}" ]]; then
        export SESSION_SECRET="${SECRET_VAL}"
      fi
    fi
    if [[ -n "${OIDC_ISSUER:-}" ]]; then
      info "OIDC env vars loaded from deployment"
    fi

    info "Scaling down in-cluster web console..."
    kube scale deployment "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --replicas=0

    info "Redirecting Service → host dev server (${HOST_IP}:${DEV_PORT})..."
    kube patch service "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --type=json \
      -p='[{"op": "remove", "path": "/spec/selector"}]' 2>/dev/null || true
    kube apply -f - <<EOF
apiVersion: v1
kind: Endpoints
metadata:
  name: ${DEPLOYMENT}
  namespace: ${KIND_NAMESPACE}
subsets:
  - addresses:
      - ip: ${HOST_IP}
    ports:
      - name: http
        port: ${DEV_PORT}
EOF

    info "Port-forwarding API server to localhost:8000..."
    kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
    API_PF_PID=$!

    track_swap "${COMPONENT}"

    _cleaned=false
    cleanup_hot_reload() {
      ${_cleaned} && return
      _cleaned=true
      echo ""
      info "Stopping hot reload..."
      kill "${API_PF_PID}" 2>/dev/null || true
      wait "${API_PF_PID}" 2>/dev/null || true
      info "Restoring in-cluster web console..."
      kube delete endpoints "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" 2>/dev/null || true
      kube apply -f "${REPO_ROOT}/deploy/kind/web-console.yaml" || true
      kube rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" || true
      info "Waiting for web console to become available..."
      kube wait --for=condition=available "deployment/${DEPLOYMENT}" \
        -n "${KIND_NAMESPACE}" --timeout=120s || true
      clear_swap "${COMPONENT}" 2>/dev/null || true
      success "Web console reverted to baseline"
    }
    trap cleanup_hot_reload EXIT
    # Let pnpm receive SIGINT from the terminal; bash stays alive to run cleanup.
    trap : INT TERM HUP

    if lsof -i ":${DEV_PORT}" >/dev/null 2>&1; then
      error "Port ${DEV_PORT} is already in use. Kill the process and try again:"
      error "  kill \$(lsof -ti :${DEV_PORT})"
      exit 1
    fi

    echo ""
    success "Web Console: https://${CONSOLE_HOSTNAME}"
    info "To use rebuild-and-replace instead: KIND_HOT_RELOAD=false make kind-web-console-up"
    info "Starting dev server (Ctrl+C to stop and revert)..."
    echo ""

    (cd "${REPO_ROOT}" && pnpm install --frozen-lockfile && \
      info "Building workspace dependencies (sdk → domain-probes → gateway-management-ui)..." && \
      pnpm --filter @openshift-online/hypershell-sdk build && \
      pnpm --filter @openshift-online/hypershell-domain-probes build && \
      pnpm --filter @openshift-online/hypershell-gateway-management-ui build && \
      DEV_SERVER_HOST=0.0.0.0 pnpm --filter @openshift-online/hypershell-web-console dev) || true
    exit 0
  fi

  header "Swap ${COMPONENT} (up)"

  info "Building ${COMPONENT} from working tree..."
  ${CONTAINER_ENGINE} build -t "${LOCAL_IMAGE}" \
    -f "${DOCKERFILE}" ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"} "${BUILD_CONTEXT}"

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
  kube set image "deployment/${DEPLOYMENT}" ${set_image_args} -n "${KIND_NAMESPACE}"
  kube rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}"

  info "Waiting for ${COMPONENT}..."
  kube wait --for=condition=available "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
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
    kube delete endpoints "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" 2>/dev/null || true
    kube apply -f deploy/kind/web-console.yaml
  else
    local set_image_args=""
    for container in ${CONTAINERS}; do
      set_image_args="${set_image_args} ${container}=${BASELINE_IMAGE}"
    done
    # shellcheck disable=SC2086
    kube set image "deployment/${DEPLOYMENT}" ${set_image_args} -n "${KIND_NAMESPACE}"
  fi

  kube rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}"
  info "Waiting for ${COMPONENT}..."
  kube wait --for=condition=available "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
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
