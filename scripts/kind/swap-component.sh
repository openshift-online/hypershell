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

# restore_web_console_from_overlay: re-apply the web-console's Kind-overlay
# resources (base + the OIDC env patch from deploy/kind) rather than the bare
# deploy/base manifest. The base manifest carries no OIDC_ISSUER/OIDC_CLIENT_ID/
# SESSION_SECRET, so applying it strips the OIDC env off the running deployment
# and breaks the next dev swap: the hot-reload /api proxy then forwards without a
# bearer token and the OIDC-enforcing API server returns 401. Render the overlay
# to a temp dir and apply ONLY the web-console's own resources, so a concurrently
# swapped api-server or control-plane is never reset to its baseline image.
restore_web_console_from_overlay() {
  local out applied=false f
  out="$(mktemp -d)"
  if kustomize build "${REPO_ROOT}/deploy/kind" -o "${out}" 2>/dev/null; then
    for f in "${out}"/*hypershell-web-console*.yaml; do
      [[ -e "${f}" ]] || continue
      kube apply -f "${f}" && applied=true
    done
  fi
  rm -rf "${out}"
  if [[ "${applied}" == "true" ]]; then
    return 0
  fi
  warn "Could not render web-console from the deploy/kind overlay; applying the base manifest (no OIDC env)."
  kube apply -f "${REPO_ROOT}/deploy/base/web-console.yaml"
}

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

    # Discover a host IP reachable from inside pods.
    # On Linux, the docker0/podman0 bridge is reliably reachable from pods
    # even in rootless Podman (the Kind network gateway is not).
    # On macOS/Windows, use the special host.docker.internal alias.
    HOST_IP=""
    case "$(uname -s)" in
      Linux)
        for iface in docker0 podman0 cni-podman0; do
          HOST_IP=$(ip -4 addr show "${iface}" 2>/dev/null | grep -oP 'inet \K[0-9.]+' || true)
          if [[ -n "${HOST_IP}" ]]; then break; fi
        done
        ;;
      Darwin)
        LOOKUP_ENGINE="${KIND_ENGINE:-${CONTAINER_ENGINE}}"
        HOST_IP=$("${LOOKUP_ENGINE}" run --rm alpine getent hosts host.docker.internal 2>/dev/null | awk '{print $1}' || true)
        ;;
    esac
    # Fallback: Kind network bridge gateway
    if [[ -z "${HOST_IP}" ]] && [[ -n "${KIND_ENGINE}" ]]; then
      HOST_IP=$("${KIND_ENGINE}" inspect "${KIND_CLUSTER_NAME}-control-plane" \
        -f '{{.NetworkSettings.Networks.kind.Gateway}}' 2>/dev/null || true)
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
    # Dev identity for the Vite /api proxy's Keycloak token minting. The proxy
    # mints a bearer token as this user (resource owner password grant) so the
    # hot-reload console can reach the OIDC-enforcing API server. Override to
    # test other roles, e.g.:
    #   KIND_DEV_USER=developer KIND_DEV_PASSWORD=developer make kind-web-console-up
    export KIND_DEV_USER="${KIND_DEV_USER:-admin}"
    export KIND_DEV_PASSWORD="${KIND_DEV_PASSWORD:-admin}"
    if [[ -n "${OIDC_ISSUER:-}" ]]; then
      info "OIDC env vars loaded from deployment"
      info "Dev API requests authenticate as '${KIND_DEV_USER}' (KIND_DEV_USER/KIND_DEV_PASSWORD to change)"
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

    # The console's /api proxy targets localhost:8000, so the API server must be
    # running before (and while) we forward. If a prior `kind-up` parked it at
    # zero replicas, forwarding to the endpoint-less service yields ECONNREFUSED;
    # forwarding while its pod is mid-rollout yields "socket hang up".
    api_replicas="$(kube get deployment hypershell-api-server -n "${KIND_NAMESPACE}" \
      -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)"
    if [[ "${api_replicas:-0}" -lt 1 ]]; then
      warn "API server is scaled to ${api_replicas:-0} replicas -- the console cannot load data."
      warn "Start it first in another terminal: make kind-api-server-up"
    else
      info "Waiting for API server to be ready before forwarding..."
      kube rollout status deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s || \
        warn "API server did not become ready; data may fail to load until it does."
    fi

    # Self-healing port-forward: a single `kubectl port-forward` dies when its
    # target pod is replaced (rollout, restart, eviction), silently breaking the
    # console's /api proxy. Run it in a loop that reconnects until asked to stop.
    info "Port-forwarding API server to localhost:8000 (self-healing)..."
    API_PF_STOPFILE="/tmp/hypershell-api-pf-$$.stop"
    rm -f "${API_PF_STOPFILE}"
    (
      while [[ ! -f "${API_PF_STOPFILE}" ]]; do
        kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 || true
        [[ -f "${API_PF_STOPFILE}" ]] && break
        sleep 1
      done
    ) &
    API_PF_PID=$!

    track_swap "${COMPONENT}"

    _cleaned=false
    cleanup_hot_reload() {
      ${_cleaned} && return
      _cleaned=true
      echo ""
      info "Stopping hot reload..."
      # Signal the reconnect loop to stop, kill it, then reap any port-forward
      # child it may have orphaned (killing the loop's shell does not stop the
      # kubectl process it spawned).
      touch "${API_PF_STOPFILE}" 2>/dev/null || true
      kill "${API_PF_PID}" 2>/dev/null || true
      wait "${API_PF_PID}" 2>/dev/null || true
      pkill -f "port-forward svc/hypershell-api-server -n ${KIND_NAMESPACE} 8000:8000" 2>/dev/null || true
      rm -f "${API_PF_STOPFILE}" 2>/dev/null || true
      info "Restoring in-cluster web console..."
      kube delete endpoints "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" 2>/dev/null || true
      restore_web_console_from_overlay || true
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

  # A prior `kind-up` may have parked this deployment at zero replicas (it scales
  # swapped components down). `set image`/`rollout restart` do not restore the
  # replica count, and `wait --for=condition=available` passes instantly on a
  # zero-replica deployment -- yielding a false success with no running pods.
  # Ensure at least one replica before rolling out.
  local desired_replicas
  desired_replicas="$(kube get deployment "${DEPLOYMENT}" -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)"
  if [[ "${desired_replicas:-0}" -lt 1 ]]; then
    info "Scaling ${COMPONENT} up from ${desired_replicas:-0} to 1 replica..."
    kube scale "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --replicas=1
  fi

  kube rollout restart "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}"

  info "Waiting for ${COMPONENT}..."
  # `rollout status` requires updated replicas to actually become available, so
  # it fails (rather than passing trivially) if no pods come up.
  kube rollout status "deployment/${DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=120s
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
    restore_web_console_from_overlay
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
