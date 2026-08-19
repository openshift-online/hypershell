#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "HyperShell Local Development Environment"
echo ""

# --- Early sudo acquisition ---
# Several steps need elevated privileges (cloud-provider-kind, DNS resolver,
# port forwarding). Prompt once now so the rest of the script runs unattended.
if [[ "${KIND_NO_SUDO:-}" == "true" ]]; then
  warn "KIND_NO_SUDO=true - will use kubectl port-forward if cloud-provider-kind needs sudo"
  HAVE_SUDO=false
else
  info "This script may use sudo for cloud-provider-kind, DNS resolver, and port forwarding."
  info "Set KIND_NO_SUDO=true to skip sudo (services will use kubectl port-forward instead)."
  if sudo -v 2>/dev/null; then
    HAVE_SUDO=true
    success "sudo credentials cached"
    # Keep the sudo timestamp fresh for the whole run. make kind-up spends
    # several minutes building images and waiting on rollouts before it reaches
    # the pfctl port-forward and DNS resolver steps; without this the default
    # 5-minute sudo timeout expires first, those sudo calls fail silently (they
    # are guarded with `|| warn`), and port forwarding is left unconfigured.
    # The loop refreshes every 50s and exits on its own once this script ($$)
    # is gone, so no EXIT trap (which the seeding step below rebinds) is needed.
    ( while kill -0 "$$" 2>/dev/null; do sudo -n -v 2>/dev/null || exit; sleep 50; done ) &
  else
    warn "sudo unavailable - will use kubectl port-forward as fallback"
    HAVE_SUDO=false
  fi
fi
echo ""

# --- Build cloud-provider-kind from fork ---
# The fork (squizzi/cloud-provider-kind branch hypershell) adds BackendTLSPolicy
# support (TLS re-encryption) and HTTP/2 protocol options for GRPCRoute backends.
# Build once into ./bin/ and prepend to PATH so up.sh always finds it.
info "Ensuring cloud-provider-kind is built from fork..."
make -C "${REPO_ROOT}" kind-prereqs
export PATH="${REPO_ROOT}/bin:${PATH}"

# --- Cluster creation (idempotent) ---
header "Cluster"
if cluster_exists; then
  warn "Cluster '${KIND_CLUSTER_NAME}' already exists, reusing"
else
  info "Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
  rendered=$(mktemp /tmp/kind-config-XXXXXX)
  sed "s|__KIND_HOST_MOUNT_PATH__|${KIND_HOST_MOUNT_PATH:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}|g" \
    "${KIND_CONFIG}" > "${rendered}"
  kind create cluster --name "${KIND_CLUSTER_NAME}" --config "${rendered}"
  rm -f "${rendered}"
  success "Cluster created"
fi

# --- Set kubectl context and wait for API server ---
kube cluster-info >/dev/null 2>&1 || \
  kubectl config use-context "$(kctx)"

info "Waiting for Kubernetes API server..."
for i in $(seq 1 15); do
  if kube get nodes >/dev/null 2>&1; then break; fi
  info "API server not ready, retrying (${i}/15)..."
  sleep 2
done
echo ""

# --- Verify and start cloud-provider-kind ---
header "cloud-provider-kind"
CPK_RUNNING=false

# Returns 0 if a running cloud-provider-kind can still enumerate the kind
# cluster, 1 if it is stuck. A stale runtime connection makes the daemon spin
# on "failed to list clusters" so it never assigns LoadBalancer/Gateway
# addresses - reusing such an instance silently breaks networking.
cpk_healthy() {
  # The daemon lists clusters via the same container engine we use here; if the
  # CLI cannot, neither can the daemon.
  ${CONTAINER_ENGINE} ps -a --filter label=io.x-k8s.kind.cluster >/dev/null 2>&1 || return 1
  # A recent, unrecovered list failure at the tail of the log is the signature.
  if [[ -f "${CPK_LOG}" ]] && tail -n 5 "${CPK_LOG}" 2>/dev/null | grep -q "failed to list clusters"; then
    return 1
  fi
  return 0
}

start_cpk() {
  # Remove orphaned proxy containers before launching. A fresh daemon binds its
  # xDS server to a new port; proxies left over from a prior daemon keep
  # pointing at the dead xDS port and serve stale config (e.g. old Service
  # ClusterIPs), which surfaces as 503s through the Gateway. Deleting them forces
  # a clean rebuild against the current cluster.
  local stale
  stale=$(${CONTAINER_ENGINE} ps -aq --filter "name=kindccm" 2>/dev/null || true)
  if [[ -n "${stale}" ]]; then
    info "Removing stale cloud-provider-kind proxy containers..."
    # shellcheck disable=SC2086
    ${CONTAINER_ENGINE} rm -f ${stale} >/dev/null 2>&1 || true
  fi

  info "Starting cloud-provider-kind..."
  if nohup cloud-provider-kind --enable-lb-port-mapping >"${CPK_LOG}" 2>&1 &
     sleep 2 && pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
    CPK_RUNNING=true
    record_cpk_sha "$(cpk_expected_sha)"
    success "cloud-provider-kind started (without sudo)"
  elif [[ "${HAVE_SUDO}" == "true" ]]; then
    info "Retrying with sudo..."
    sudo -E nohup cloud-provider-kind --enable-lb-port-mapping >"${CPK_LOG}" 2>&1 &
    sleep 2
    if pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
      CPK_RUNNING=true
      record_cpk_sha "$(cpk_expected_sha)"
      success "cloud-provider-kind started (with sudo)"
    else
      error "cloud-provider-kind failed to start - check ${CPK_LOG}"
      exit 1
    fi
  else
    warn "cloud-provider-kind requires sudo on this system - will use kubectl port-forward instead"
  fi
}

if ! command -v cloud-provider-kind >/dev/null 2>&1; then
  if [[ "${HAVE_SUDO}" == "true" ]]; then
    error "cloud-provider-kind not found in PATH"
    info "Install via: make kind-prereqs"
    exit 1
  else
    warn "cloud-provider-kind not found - will use kubectl port-forward instead"
  fi
elif pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
  # A cloud-provider-kind is already running. Restarting it republishes the
  # gateway LoadBalancer on NEW random host ports (docker `--publish 443/tcp`
  # with no fixed host port), which invalidates the pfctl rules, cluster
  # CoreDNS, and in-cluster DNAT pinned to the old ports and breaks access on
  # https://localhost:443. So restart only when we must: the pinned build has
  # changed (the running commit differs from the SHA `make kind-prereqs` just
  # built), the daemon is wedged (cannot list clusters), or the user forces it
  # with KIND_RESTART_CPK=true. Otherwise reuse the instance and keep its ports
  # stable. A missing/unknown running-marker counts as a mismatch, biasing
  # toward a restart so the pinned build is guaranteed.
  EXPECTED_SHA="$(cpk_expected_sha)"
  RUNNING_SHA="$(cpk_running_sha)"
  needs_restart=true
  if [[ "${KIND_RESTART_CPK:-}" == "true" ]]; then
    info "KIND_RESTART_CPK=true - restarting cloud-provider-kind..."
  elif [[ "${RUNNING_SHA}" != "${EXPECTED_SHA}" ]]; then
    info "cloud-provider-kind is ${RUNNING_SHA:-unknown}, pinned build is ${EXPECTED_SHA:-unknown} - restarting to pick it up..."
  elif ! cpk_healthy; then
    warn "cloud-provider-kind already running but unhealthy (cannot list clusters) - restarting"
    info "  See ${CPK_LOG} for the underlying error"
  else
    needs_restart=false
  fi
  if [[ "${needs_restart}" == "true" ]]; then
    pkill -f "cloud-provider-kind" 2>/dev/null || true
    [[ "${HAVE_SUDO}" == "true" ]] && sudo pkill -f "cloud-provider-kind" 2>/dev/null || true
    sleep 2
    start_cpk
  else
    info "Reusing cloud-provider-kind (rev ${RUNNING_SHA:-unknown}) - up to date, keeps LB ports stable"
    info "Set KIND_RESTART_CPK=true to force a restart."
    CPK_RUNNING=true
  fi
else
  start_cpk
fi
echo ""

# --- Install infrastructure prerequisites via kustomize ---
header "Infrastructure"
# Kubernetes 1.33+ may pre-install Gateway API CRDs whose storedVersions
# contain API versions the experimental bundle no longer serves (e.g. v1 for
# TCPRoute/UDPRoute).  Delete them first so the apply can re-create them
# with the correct spec.versions.
for crd in tcproutes.gateway.networking.k8s.io udproutes.gateway.networking.k8s.io; do
  kube delete crd "$crd" --ignore-not-found 2>/dev/null || true
done
for crd in tcproutes.gateway.networking.k8s.io udproutes.gateway.networking.k8s.io; do
  kube wait --for=delete crd/"$crd" --timeout=30s 2>/dev/null || true
done
info "Installing CRDs and controllers (cert-manager, Gateway API, Agent Sandbox)..."
kustomize build --load-restrictor=LoadRestrictionsNone deploy/kind/infrastructure | \
  kube apply --server-side --force-conflicts -f -
info "Waiting for cert-manager..."
kube wait --for=condition=available deployment/cert-manager -n cert-manager --timeout=120s
kube wait --for=condition=available deployment/cert-manager-webhook -n cert-manager --timeout=120s
info "Waiting for agent-sandbox controller..."
kube wait --for=condition=available deployment/agent-sandbox-controller -n agent-sandbox-system --timeout=120s
success "Infrastructure ready"
echo ""

# --- Build and load local images (offline mode) ---
FORCE_ROLLOUT=""
if [[ "${LOCAL_IMAGES:-}" == "true" ]]; then
  header "Local Images"
  info "Building baseline images from origin/main..."
  "${SCRIPT_DIR}/build-images.sh"
  FORCE_ROLLOUT=true
  echo ""
fi

# --- Apply pull secret (if configured) ---
if [[ -n "${KIND_PULL_SECRET:-}" ]]; then
  header "Pull Secret"
  kube create namespace "${KIND_NAMESPACE}" --dry-run=client -o yaml | \
    kube apply -f -
  info "Applying pull secret from ${KIND_PULL_SECRET}..."
  kube apply -f "${KIND_PULL_SECRET}" -n "${KIND_NAMESPACE}"
  SECRET_NAME=$(kube get -f "${KIND_PULL_SECRET}" -n "${KIND_NAMESPACE}" -o jsonpath='{.metadata.name}')
  if [[ -n "${SECRET_NAME}" ]]; then
    info "Waiting for default ServiceAccount in ${KIND_NAMESPACE}..."
    for i in $(seq 1 30); do
      if kube get serviceaccount default -n "${KIND_NAMESPACE}" >/dev/null 2>&1; then break; fi
      sleep 1
    done
    info "Patching default ServiceAccount with imagePullSecrets..."
    kube patch serviceaccount default -n "${KIND_NAMESPACE}" \
      -p "{\"imagePullSecrets\":[{\"name\":\"${SECRET_NAME}\"}]}"
  fi
  echo ""
fi

# --- OIDC session secret (must exist before kustomize apply) ---
header "OIDC Secrets"
kube create namespace "${KIND_NAMESPACE}" --dry-run=client -o yaml | kube apply -f -
info "Creating OIDC session secret..."
SESSION_SECRET=$(openssl rand -hex 32)
kube create secret generic hypershell-oidc-session \
  -n "${KIND_NAMESPACE}" \
  --from-literal=session-secret="${SESSION_SECRET}" \
  --dry-run=client -o yaml | kube apply -f -
success "OIDC session secret created"
echo ""

# --- Deploy all components via kustomize ---
header "Deploying Components"
info "Applying Kind manifests via kustomize..."
kustomize build deploy/kind | kube apply -f -

info "Waiting for PostgreSQL..."
kube wait --for=condition=available deployment/hypershell-postgres -n "${KIND_NAMESPACE}" --timeout=300s
success "PostgreSQL ready"

if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Waiting for Keycloak..."
  kube wait --for=condition=available deployment/keycloak -n keycloak --timeout=180s
  success "Keycloak ready"
fi

# --- Jaeger (optional, for OTel trace inspection) ---
# Deploys an all-in-one Jaeger v2 for local trace inspection alongside the API
# server observability work (HYPERSHELL-26). The web console browser and BFF
# export over OTLP/HTTP (4318) because browsers cannot speak OTLP gRPC; the API
# server uses gRPC (4317).
# Renders deploy/kind/jaeger.yaml into the selected namespace with sed, the same
# portable substitution used for the Kind cluster config. Using sed instead of
# GNU envsubst keeps bring-up working on stock macOS, where envsubst is absent.
render_jaeger() {
  sed "s|__KIND_NAMESPACE__|${KIND_NAMESPACE}|g" deploy/kind/jaeger.yaml
}

# Reports whether the named deployment exists, distinguishing a genuine NotFound
# from an API, auth, or authorization error. --ignore-not-found makes kubectl
# exit 0 with empty output when the resource is absent and nonzero for every
# other failure, so absence is read from an empty successful result rather than
# by matching error text: a client-side failure such as "kubectl: command not
# found" no longer masquerades as absence. Any nonzero exit propagates and
# aborts, since reading a swallowed lookup error as "absent" would silently skip
# the tracing-disable reconciliation and leave the BFF exporting to a dead
# collector. Stderr flows to the terminal so a real failure stays diagnosable.
deployment_exists() {
  local name="$1" out
  if ! out=$(kube get "deployment/${name}" -n "${KIND_NAMESPACE}" \
    --ignore-not-found -o name); then
    error "checking for deployment/${name} failed"
    exit 1
  fi
  [[ -n "${out}" ]]
}

# Reports 0 when the web console BFF still carries an OTLP exporter endpoint, so
# the disabled-state reconciliation can verify it actually removed the endpoint
# rather than trusting that the unset command had any effect. A lookup failure is
# propagated rather than read as "endpoint absent", which would let a silent API
# error masquerade as a successful disable.
bff_otel_endpoint_set() {
  local names
  if ! names=$(kube get deployment/hypershell-web-console -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="web-console")].env[*].name}' \
    2>&1); then
    error "verifying OTLP endpoint removal: ${names}"
    exit 1
  fi
  tr ' ' '\n' <<<"${names}" | grep -qx "OTEL_EXPORTER_OTLP_ENDPOINT"
}

# Reports 0 when the API server still carries an OTLP exporter endpoint, so the
# disabled-state reconciliation can verify it actually removed the endpoint
# rather than trusting that the unset command had any effect. A lookup failure is
# propagated rather than read as "endpoint absent", which would let a silent API
# error masquerade as a successful disable.
api_server_otel_endpoint_set() {
  local names
  if ! names=$(kube get deployment/hypershell-api-server -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="api-server")].env[*].name}' \
    2>&1); then
    error "verifying OTLP endpoint removal: ${names}"
    exit 1
  fi
  tr ' ' '\n' <<<"${names}" | grep -qx "OTEL_EXPORTER_OTLP_ENDPOINT"
}

if [[ "${KIND_JAEGER:-}" == "true" ]]; then
  header "Jaeger"
  info "Deploying Jaeger..."
  render_jaeger | kube apply -f -
  info "Patching web console BFF with OTEL_EXPORTER_OTLP_ENDPOINT..."
  kube set env deployment/hypershell-web-console -c web-console -n "${KIND_NAMESPACE}" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://jaeger.${KIND_NAMESPACE}.svc.cluster.local:4318"
  # The API server exports over OTLP/gRPC (4318 is OTLP/HTTP for the browser and
  # BFF; 4317 is OTLP/gRPC reserved for the API server). Setting the endpoint on
  # the Deployment opts the API server into tracing; a swapped-in working-tree
  # image keeps this env, so browser -> BFF -> API traces join in Jaeger.
  # Jaeger ingests traces only; its OTLP endpoint has no metrics service, so the
  # API server's metric exporter would log a periodic "Unimplemented" upload
  # error. Turn metrics off in the dev cluster (OTEL_METRICS_EXPORTER=none) while
  # keeping trace export on; production points at a full collector that accepts
  # both.
  info "Patching API server with OTEL_EXPORTER_OTLP_ENDPOINT..."
  kube set env deployment/hypershell-api-server -c api-server -n "${KIND_NAMESPACE}" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://jaeger.${KIND_NAMESPACE}.svc.cluster.local:4317" \
    OTEL_METRICS_EXPORTER="none"
  info "Waiting for Jaeger..."
  kube wait --for=condition=available deployment/jaeger -n "${KIND_NAMESPACE}" --timeout=120s
  success "Jaeger ready"
  echo ""
else
  # Reconcile the disabled state, do not create-or-skip: a cluster brought up
  # once with KIND_JAEGER=true keeps the Jaeger workload and the BFF exporter
  # endpoint until they are removed. On a reused cluster with tracing turned
  # off, tear Jaeger down and unset the endpoint so the BFF stops exporting to a
  # collector that is no longer there. Both steps are idempotent on a cluster
  # that never had Jaeger, but a failure other than absence must surface rather
  # than leave the BFF exporting to a collector that is gone.
  info "KIND_JAEGER not enabled - ensuring Jaeger is removed and tracing is off..."
  # --ignore-not-found tolerates the resources being absent; any other kubectl
  # failure propagates through the pipe (pipefail) and aborts the run.
  render_jaeger | kube delete --ignore-not-found -f -
  # Unset the exporter endpoint only when the deployment exists; on a cluster
  # that has it, removing an already-absent variable is a no-op, then verify the
  # variable is actually gone so a silent failure cannot leave tracing enabled.
  # deployment_exists tolerates only a true NotFound; an API, auth, or
  # authorization error aborts rather than being mistaken for absence.
  if deployment_exists hypershell-web-console; then
    kube set env deployment/hypershell-web-console -c web-console -n "${KIND_NAMESPACE}" \
      OTEL_EXPORTER_OTLP_ENDPOINT-
    if bff_otel_endpoint_set; then
      error "OTEL_EXPORTER_OTLP_ENDPOINT is still set after disabling tracing"
      exit 1
    fi
  fi
  if deployment_exists hypershell-api-server; then
    kube set env deployment/hypershell-api-server -c api-server -n "${KIND_NAMESPACE}" \
      OTEL_EXPORTER_OTLP_ENDPOINT- OTEL_METRICS_EXPORTER-
    if api_server_otel_endpoint_set; then
      error "OTEL_EXPORTER_OTLP_ENDPOINT is still set after disabling tracing"
      exit 1
    fi
  fi
  echo ""
fi

# --- Gateway trusted CA (self-signed CA for OIDC over HTTPS) ---
# The gateway pod validates OIDC tokens against the canonical HTTPS issuer
# (https://keycloak.hypershell.localhost). That endpoint is served by the
# gateway LB using the *.hypershell.localhost cert signed by Kind's self-signed
# cert-manager CA, which the gateway does not trust out of the box. Publish the
# CA as the gateway-trusted-ca ConfigMap in the control-plane namespace BEFORE
# restarting the control plane so the reconciler can apply it when provisioning
# gateways. The reconciler copies it into each gateway's namespace and mounts it
# as SSL_CERT_FILE so OIDC discovery over HTTPS succeeds
# (see specs/platform/openshell-gateway-tls.spec.md).
header "Gateway Trusted CA"
info "Waiting for hypershell-https-tls certificate to be issued..."
CA_PEM=""
for _ in $(seq 1 30); do
  CA_PEM=$(kube get secret hypershell-https-tls -n "${KIND_NAMESPACE}" \
    -o go-template='{{index .data "ca.crt" | base64decode}}' 2>/dev/null || true)
  if [[ -n "${CA_PEM}" ]]; then break; fi
  sleep 2
done
if [[ -n "${CA_PEM}" ]]; then
  printf '%s' "${CA_PEM}" | kube create configmap gateway-trusted-ca \
    -n "${KIND_NAMESPACE}" --from-file=ca-bundle.crt=/dev/stdin \
    --dry-run=client -o yaml | kube apply -f -
  success "gateway-trusted-ca ConfigMap published"
else
  warn "hypershell-https-tls has no ca.crt yet - gateway OIDC over HTTPS may fail"
fi
echo ""

# The API server enforces JWT and loads Keycloak's JWKS at startup. If it
# started before Keycloak was serving keys it is stuck in CrashLoopBackoff;
# restart it now that Keycloak is ready so a fresh pod (with no backoff delay)
# comes up on the first try instead of waiting out the backoff timer.
if ! is_swapped api-server; then
  info "Restarting API server now that Keycloak serves JWKS..."
  kube rollout restart deployment/hypershell-api-server -n "${KIND_NAMESPACE}"
fi

# The controller's gRPC watch streams must connect to a running API server.
# With simultaneous deployment the controller may start before the API server
# is ready, fail the first connection, and sit in a 16s backoff -- missing
# any gateway events created during that window.  Wait for the API server
# first, then restart the controller so it connects immediately.
if ! is_swapped api-server; then
  info "Waiting for API server..."
  kube wait --for=condition=available deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
fi
if ! is_swapped control-plane; then
  info "Restarting control plane to establish watch streams..."
  kube rollout restart deployment/hypershell-controller -n "${KIND_NAMESPACE}"
  kube wait --for=condition=available deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s
fi

if is_swapped web-console; then
  warn "Web console is swapped -- scaling to zero (runs locally via npm)"
  kube scale deployment/hypershell-web-console -n "${KIND_NAMESPACE}" --replicas=0
fi

local_registry="${IMAGE_REGISTRY:-quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main}"
_api_img="${API_SERVER_IMAGE:-}"
_cp_img="${CONTROL_PLANE_IMAGE:-}"
_wc_img="${WEB_CONSOLE_IMAGE:-}"
if [[ "${IMAGE_TAG:-latest}" != "latest" ]]; then
  : "${_api_img:=${local_registry}/hypershell-api-server-main:${IMAGE_TAG}}"
  : "${_cp_img:=${local_registry}/hypershell-control-plane-main:${IMAGE_TAG}}"
  : "${_wc_img:=${local_registry}/hypershell-web-console-main:${IMAGE_TAG}}"
fi

if [[ -n "${_api_img}" || -n "${_cp_img}" || -n "${_wc_img}" ]]; then
  info "Overriding component images..."
  if [[ -n "${_api_img}" ]] && ! is_swapped api-server; then
    info "  api-server  -> ${_api_img}"
    kube set image "deployment/hypershell-api-server" \
      "api-server=${_api_img}" \
      "migrate=${_api_img}" \
      -n "${KIND_NAMESPACE}"
  fi
  if [[ -n "${_cp_img}" ]] && ! is_swapped control-plane; then
    info "  controller  -> ${_cp_img}"
    kube set image "deployment/hypershell-controller" \
      "controller=${_cp_img}" \
      -n "${KIND_NAMESPACE}"
  fi
  if [[ -n "${_wc_img}" ]] && ! is_swapped web-console; then
    info "  web-console -> ${_wc_img}"
    kube set image "deployment/hypershell-web-console" \
      "web-console=${_wc_img}" \
      -n "${KIND_NAMESPACE}"
  fi
fi


if [[ "${FORCE_ROLLOUT}" == "true" ]]; then
  info "Rolling out non-swapped deployments to pick up rebuilt images..."
  for pair in "hypershell-api-server:api-server" \
              "hypershell-controller:control-plane" \
              "hypershell-web-console:web-console"; do
    dep="${pair%%:*}"
    comp="${pair##*:}"
    if ! is_swapped "${comp}"; then
      kube rollout restart "deployment/${dep}" -n "${KIND_NAMESPACE}"
    fi
  done
fi
echo ""

# --- Gateway address discovery ---
header "TLS & Networking"

GATEWAY_PORT=""
if [[ "${CPK_RUNNING}" == "true" ]]; then
  info "Waiting for networking Gateway to get an address..."
  for i in $(seq 1 30); do
    GW_ADDR=$(kube get gateway hypershell-gw -n "${KIND_NAMESPACE}" \
      -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
    if [[ -n "${GW_ADDR}" ]]; then break; fi
    if (( i % 5 == 0 )); then
      info "Gateway not ready yet (${i}/30)... is cloud-provider-kind running?"
    fi
    sleep 2
  done

  if [[ -n "${GW_ADDR}" ]]; then
    success "Networking Gateway address: ${GW_ADDR}"

    patch_cluster_coredns "${GW_ADDR}"

    # cloud-provider-kind exposes Gateways via Docker proxy containers.
    # On macOS the container IPs are not routable, so --enable-lb-port-mapping
    # publishes an ephemeral host port.  Discover it for the banner URLs.
    info "Discovering Gateway proxy port..."
    for j in $(seq 1 15); do
      PROXY_CONTAINER=$(${CONTAINER_ENGINE} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
      if [[ -n "${PROXY_CONTAINER}" ]]; then
        GATEWAY_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 443 2>/dev/null | head -1 | cut -d: -f2)
        if [[ -n "${GATEWAY_PORT}" ]]; then break; fi
      fi
      sleep 2
    done

    if [[ -n "${GATEWAY_PORT}" ]]; then
      success "Gateway HTTPS on host port ${GATEWAY_PORT}"
      # Flush any stale rules from a previous run (which may have pinned a
      # different ephemeral port) before installing the current mapping, so the
      # port-forward always reflects the live proxy container. Matches the
      # stop-then-start sequence in port-forward.sh (make kind-fix-ports).
      stop_port_forward
      start_port_forward "${GATEWAY_PORT}"
    else
      warn "Could not discover Gateway proxy port - check '${CONTAINER_ENGINE} ps --filter name=kindccm-gw'"
    fi
  else
    warn "Gateway has no address after 60s - cloud-provider-kind is not assigning addresses"
    if [[ -f "${CPK_LOG}" ]]; then
      warn "Recent cloud-provider-kind log (${CPK_LOG}):"
      tail -n 10 "${CPK_LOG}" 2>/dev/null | sed 's/^/      /' || true
    fi
  fi
else
  info "Skipping Gateway address discovery (no cloud-provider-kind)"
  info "Services will be accessible via kubectl port-forward"
fi
echo ""

# --- OIDC port-suffix overrides (only when port forwarding failed) ---
PORT_SUFFIX=""
if [[ -z "${PORT_FORWARD_ACTIVE:-}" ]] && [[ -n "${GATEWAY_PORT:-}" ]]; then
  PORT_SUFFIX=":${GATEWAY_PORT}"
fi

if [[ -n "${GATEWAY_PORT:-}" ]] && [[ -n "${GW_ADDR:-}" ]] && [[ -z "${PORT_FORWARD_ACTIVE:-}" ]]; then
  info "Routing in-cluster port ${GATEWAY_PORT} to gateway port 443..."
  ${CONTAINER_ENGINE} exec "${KIND_CLUSTER_NAME}-control-plane" \
    iptables -t nat -C PREROUTING -p tcp -d "${GW_ADDR}" --dport "${GATEWAY_PORT}" \
      -j DNAT --to-destination "${GW_ADDR}:443" 2>/dev/null || \
  ${CONTAINER_ENGINE} exec "${KIND_CLUSTER_NAME}-control-plane" \
    iptables -t nat -A PREROUTING -p tcp -d "${GW_ADDR}" --dport "${GATEWAY_PORT}" \
      -j DNAT --to-destination "${GW_ADDR}:443"
  success "In-cluster OIDC routing: ${GW_ADDR}:${GATEWAY_PORT} -> ${GW_ADDR}:443"
fi

if [[ -n "${PORT_SUFFIX}" ]]; then
  warn "Port forwarding not active - overriding OIDC URLs with port suffix ${PORT_SUFFIX}"
  warn "Caveat: gateway OIDC validation expects the canonical issuer"
  warn "  https://${KEYCLOAK_HOSTNAME} that the gateway is seeded with. On this"
  warn "  fallback path Keycloak mints tokens with a port-suffixed issuer, which"
  warn "  will not match, so gateway token validation will fail. Use port"
  warn "  forwarding (the default) for end-to-end gateway OIDC."

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    kube set env deployment/keycloak -n keycloak \
      KC_HOSTNAME="https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX}"
    kube rollout restart deployment/keycloak -n keycloak
    kube wait --for=condition=available deployment/keycloak -n keycloak --timeout=120s
  fi

  if ! is_swapped web-console; then
    kube set env deployment/hypershell-web-console -n "${KIND_NAMESPACE}" -c web-console \
      OIDC_ISSUER="https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX}/realms/hypershell" \
      OIDC_REDIRECT_URI="https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}/auth/callback"
  fi
fi
echo ""

# --- Wait for readiness ---
header "Readiness"
# Use `rollout status`, not `wait --for=condition=available`. With replicas=1
# and the default rolling update the Deployment stays Available throughout a
# rollout (the old pod keeps serving until the new one is Ready), so
# `wait --for=condition=available` returns immediately after the earlier
# `rollout restart`s -- while a rollout is still in flight. The API-server
# port-forward below would then bind to a pod that is being terminated, and the
# seed requests would fail with curl (52) "empty reply from server" (HTTP 000).
# `rollout status` blocks until the new ReplicaSet is fully rolled out and the
# old pods are gone, so the Service endpoints are stable before we port-forward.
info "Waiting for API server..."
kube rollout status deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
success "API server ready"

info "Waiting for control plane..."
kube rollout status deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s
success "Control plane ready"

info "Waiting for web console..."
kube rollout status deployment/hypershell-web-console -n "${KIND_NAMESPACE}" --timeout=120s
success "Web console ready"
echo ""

# --- Seed Gateway via REST API ---
header "Gateway Provisioning"
API_URL="http://localhost:8000"
info "Port-forwarding to API server..."
kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
PF_PID=$!
cleanup_pf() { kill "${PF_PID}" 2>/dev/null || true; wait "${PF_PID}" 2>/dev/null || true; }
trap cleanup_pf EXIT

# `port-forward` accepts a local TCP connection before it has confirmed the pod
# is serving, so a fixed `sleep` races the REST server coming up. Poll until the
# API answers with *any* HTTP status -- a 401/403 without a token still proves
# the server responded (curl exits 0). An empty reply / dead forward makes curl
# exit non-zero (HTTP 000), so tear the forward down and re-establish it before
# retrying.
info "Waiting for API server to answer through the port-forward..."
api_reachable=""
for _ in $(seq 1 30); do
  if curl -s -o /dev/null -m 3 "${API_URL}/api/hypershell/v1/fleets" 2>/dev/null; then
    api_reachable=true
    break
  fi
  kill "${PF_PID}" 2>/dev/null || true
  wait "${PF_PID}" 2>/dev/null || true
  kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
  PF_PID=$!
  sleep 2
done
if [[ -z "${api_reachable}" ]]; then
  warn "API server did not answer through the port-forward; seeding may fail"
fi

# Obtain a Bearer token from Keycloak for API calls.
API_AUTH_HEADER=""
info "Obtaining API token from Keycloak..."
# Use the Gateway-routed Keycloak URL instead of port-forwarding.
# Keycloak is accessible via HTTPRoute at keycloak.hypershell.localhost.
#
# Seed with the admin resource-owner (password) token, NOT the control-plane
# client-credentials token. The kind overlay enables RBAC_ENFORCE=true, and the
# HTTP authz middleware (unlike the gRPC interceptor) has no service-account
# bypass -- every write requires the caller's JWT to carry the `gateway:creator`
# realm role. The `hypershell-control-plane` client holds no such role, so its
# token 403s on `POST /fleets` onward and (because seeding is non-fatal) would
# leave the cluster with no seeded resources behind a scroll-past warning. The
# `admin` user has `gateway:creator`, and `hypershell-frontend` permits the
# password grant (publicClient + directAccessGrantsEnabled), so this token is
# authorized to create the platform resources below.
#
# Poll rather than fetching once. On a fresh `kind-up` the gateway LB has an
# address (waited on above) and Keycloak is Available, but the gateway's
# Keycloak route/listener may not be accepting on :443 yet -- a single curl
# then fails with (7) "Couldn't connect to server", the token is empty, and
# seeding proceeds unauthenticated (HTTP 401). Re-running `kind-up` "fixes" it
# only because everything is warm by then. Retry until Keycloak answers with a
# token (or we time out) so the first run seeds successfully. Mirrors the
# API-server port-forward readiness loop above.
KC_TOKEN_URL="https://${KEYCLOAK_HOSTNAME}/realms/hypershell/protocol/openid-connect/token"
API_TOKEN=""
TOKEN_RESP=""
for _ in $(seq 1 30); do
  TOKEN_RESP=$(curl -sSk -m 5 -X POST "${KC_TOKEN_URL}" \
    -d "grant_type=password" \
    -d "client_id=hypershell-frontend" \
    -d "username=admin" \
    -d "password=admin" 2>&1 || true)
  API_TOKEN=$(echo "${TOKEN_RESP}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4 || true)
  if [[ -n "${API_TOKEN}" ]]; then break; fi
  sleep 2
done
if [[ -n "${API_TOKEN}" ]]; then
  API_AUTH_HEADER="Authorization: Bearer ${API_TOKEN}"
  success "API token obtained"
else
  warn "Could not obtain API token: ${TOKEN_RESP:0:200}"
fi

# Helper: POST a JSON resource; prints the response body on success or failure.
api_post() {
  local url="$1" data="$2"
  local auth_args=()
  if [[ -n "${API_AUTH_HEADER}" ]]; then
    auth_args=(-H "${API_AUTH_HEADER}")
  fi
  curl -sS -w "\n%{http_code}" -X POST "${url}" \
    -H "Content-Type: application/json" \
    ${auth_args[@]+"${auth_args[@]}"} \
    -d "${data}" 2>&1 || true
}

extract_id() {
  local id
  id=$(echo "$1" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
  echo "${id}"
}

seed_failed=""

info "Creating default Fleet..."
FLEET_RAW=$(api_post "${API_URL}/api/hypershell/v1/fleets" \
  '{"name":"default","description":"Local development fleet"}')
FLEET_HTTP=$(echo "${FLEET_RAW}" | tail -1)
FLEET_RESP=$(echo "${FLEET_RAW}" | sed '$d')
FLEET_ID=$(extract_id "${FLEET_RESP}")

if [[ -z "${FLEET_ID}" ]]; then
  warn "Fleet creation failed (HTTP ${FLEET_HTTP}): ${FLEET_RESP:-no response}"
  seed_failed=true
fi

if [[ -z "${seed_failed}" ]]; then
  success "Fleet created: ${FLEET_ID}"

  info "Creating ManagedCluster..."
  MC_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_clusters" \
    "{\"name\":\"local-kind\",\"fleet_id\":\"${FLEET_ID}\",\"provider\":\"kind\",\"kubeconfig_secret\":\"kind-kubeconfig\"}")
  MC_HTTP=$(echo "${MC_RAW}" | tail -1)
  MC_RESP=$(echo "${MC_RAW}" | sed '$d')
  CLUSTER_ID=$(extract_id "${MC_RESP}")

  if [[ -z "${CLUSTER_ID}" ]]; then
    warn "ManagedCluster creation failed (HTTP ${MC_HTTP}): ${MC_RESP:-no response}"
    seed_failed=true
  else
    success "ManagedCluster created: ${CLUSTER_ID}"
  fi
fi

if [[ -z "${seed_failed}" ]]; then
  info "Creating GatewayRelease..."
  GR_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateway_releases" \
    "{\"name\":\"dev-release\",\"fleet_id\":\"${FLEET_ID}\",\"image\":\"${GATEWAY_IMAGE}\"}")
  GR_HTTP=$(echo "${GR_RAW}" | tail -1)
  GR_RESP=$(echo "${GR_RAW}" | sed '$d')
  RELEASE_ID=$(extract_id "${GR_RESP}")

  if [[ -z "${RELEASE_ID}" ]]; then
    warn "GatewayRelease creation failed (HTTP ${GR_HTTP}): ${GR_RESP:-no response}"
    seed_failed=true
  else
    success "GatewayRelease created: ${RELEASE_ID}"
  fi
fi

if [[ -z "${seed_failed}" ]]; then
  info "Creating ManagedDatabase..."
  MD_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_databases" \
    "{\"name\":\"local-postgres\",\"fleet_id\":\"${FLEET_ID}\",\"provider\":\"local\"}")
  MD_HTTP=$(echo "${MD_RAW}" | tail -1)
  MD_RESP=$(echo "${MD_RAW}" | sed '$d')
  DATABASE_ID=$(extract_id "${MD_RESP}")

  if [[ -z "${DATABASE_ID}" ]]; then
    warn "ManagedDatabase creation failed (HTTP ${MD_HTTP}): ${MD_RESP:-no response}"
    seed_failed=true
  else
    success "ManagedDatabase created: ${DATABASE_ID}"
  fi
fi

if [[ -z "${seed_failed}" ]]; then
  info "Creating Gateway with OIDC..."
  OIDC_JSON="{\\\"issuer\\\":\\\"${KEYCLOAK_OIDC_ISSUER}\\\",\\\"audience\\\":\\\"${KEYCLOAK_OIDC_AUDIENCE}\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}"
  # namespace is server-derived (BeforeCreate sets openshell-<hex> from the ksuid);
  # sending it is rejected as an unknown field (ErrorMalformedRequest / id 17).
  GW_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateways" \
    "{\"name\":\"dev-gateway\",\"fleet_id\":\"${FLEET_ID}\",\"cluster_id\":\"${CLUSTER_ID}\",\"release_id\":\"${RELEASE_ID}\",\"database_id\":\"${DATABASE_ID}\",\"oidc\":\"${OIDC_JSON}\"}")
  GW_HTTP=$(echo "${GW_RAW}" | tail -1)
  GW_RESP=$(echo "${GW_RAW}" | sed '$d')
  GATEWAY_ID=$(extract_id "${GW_RESP}")

  if [[ -z "${GATEWAY_ID}" ]]; then
    warn "Gateway creation failed (HTTP ${GW_HTTP}): ${GW_RESP:-no response}"
  else
    success "Gateway created with OIDC: ${GATEWAY_ID}"
  fi
fi

if [[ -n "${seed_failed}" ]]; then
  warn "Automatic seeding incomplete - create resources manually after API server is ready"
fi

cleanup_pf
trap - EXIT
echo ""

# --- kubectl port-forward (no cloud-provider-kind fallback) ---
if [[ "${CPK_RUNNING}" == "false" ]]; then
  header "kubectl Port Forwarding"
  start_kubectl_port_forwards
  echo ""
fi

# --- DNS resolution ---
header "DNS"
start_dns
setup_resolver
success "DNS configured - *.hypershell.localhost resolves to 127.0.0.1"
echo ""

# --- Summary banner ---
header "HyperShell is running!"
echo ""

if [[ "${CPK_RUNNING}" == "true" ]]; then
  PORT_SUFFIX=""
  if [[ -z "${PORT_FORWARD_ACTIVE:-}" ]] && [[ -n "${GATEWAY_PORT:-}" ]]; then
    PORT_SUFFIX=":${GATEWAY_PORT}"
  fi

  info "HTTP API:     https://${API_HOSTNAME}${PORT_SUFFIX}"
  info "Web Console:  https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}"
  info "Health:       https://${HEALTH_HOSTNAME}${PORT_SUFFIX}"

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    info "Keycloak:     https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX} (admin/admin)"
  else
    info "Keycloak:     ${KIND_KEYCLOAK_URL}"
  fi

  if [[ "${KIND_JAEGER:-}" == "true" ]]; then
    info "Jaeger UI:    https://jaeger.hypershell.localhost${PORT_SUFFIX}"
  fi

  info "Login:        https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}/auth/login"
  info "Test users:   admin/admin (admins + users), developer/developer (users only)"
else
  info "HTTP API:     http://localhost:8000"
  info "Web Console:  http://localhost:3000"
  info "Health:       http://localhost:8000/healthz"

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    info "Keycloak:     http://localhost:8080 (admin/admin)"
  else
    info "Keycloak:     ${KIND_KEYCLOAK_URL}"
  fi

  if [[ "${KIND_JAEGER:-}" == "true" ]]; then
    info "Jaeger UI:    http://localhost:16686"
  fi

  info "Login:        http://localhost:3000/auth/login"
  info "Test users:   admin/admin (admins + users), developer/developer (users only)"

  echo ""
  warn "Running without cloud-provider-kind - no TLS or hostname-based routing."
  warn "Services are available via kubectl port-forward on the ports above."
  if [[ "${HAVE_SUDO}" == "false" ]]; then
    info "To use full Gateway routing, run: cloud-provider-kind --enable-lb-port-mapping"
    info "Then: make kind-fix-ports"
  fi
fi

echo ""
info "API Server Logs:    kubectl logs -f -l app=hypershell-api-server -n ${KIND_NAMESPACE}"
info "Control Plane Logs: kubectl logs -f -l app=hypershell-controller -n ${KIND_NAMESPACE}"
info "Web Console Logs:   kubectl logs -f -l app=hypershell-web-console -n ${KIND_NAMESPACE}"
