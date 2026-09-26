#!/usr/bin/env bash
# gRPC TLS smoke test for the Kind development cluster.
#
# Enables TLS on the api-server's gRPC listener with a certificate from the
# cluster CA, points the control plane at it through an external-looking name,
# and verifies the whole path: the listener presents a certificate that chains
# to the CA and negotiates h2, plaintext dials are refused, and the control
# plane registers and opens its gateway watch over TLS. Restores plaintext
# afterwards unless KEEP=true. See specs/platform/hub-grpc-tls.spec.md.
#
#   make kind-grpc-tls-smoke              # run, then revert
#   KEEP=true make kind-grpc-tls-smoke    # run and leave TLS enabled
#   make kind-grpc-tls-smoke ARGS=revert  # restore plaintext
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
require_cluster

MODE="${1:-run}"
: "${KEEP:=false}"
: "${SMOKE_GRPC_PORT:=19000}"
SMOKE_DIR="${REPO_ROOT}/deploy/kind/grpc-tls"
API_DEPLOYMENT="hypershell-api-server"
CP_DEPLOYMENT="hypershell-controller"
CERT_NAME="hypershell-api-grpc-tls"
DIAL_HOST="hypershell-api-server.${KIND_NAMESPACE}"
TMP_DIR="$(mktemp -d)"
PF_PID=""

cleanup() {
  if [[ -n "${PF_PID}" ]]; then
    kill "${PF_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

api_server_command() {
  kube get deployment "${API_DEPLOYMENT}" -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[0].command}'
}

api_has_tls_flag() {
  api_server_command | grep -q -- '--grpc-enable-tls=true'
}

controller_logs() {
  kube logs "deployment/${CP_DEPLOYMENT}" -n "${KIND_NAMESPACE}" 2>/dev/null || true
}

enable_tls() {
  header "Enabling gRPC TLS on the Kind api-server"
  if ! kube get configmap gateway-trusted-ca -n "${KIND_NAMESPACE}" >/dev/null 2>&1; then
    error "ConfigMap gateway-trusted-ca not found in ${KIND_NAMESPACE}; run 'make kind-up' first"
    exit 1
  fi
  kube apply -f "${SMOKE_DIR}/certificate.yaml"
  info "Waiting for certificate ${CERT_NAME} to be issued..."
  kube wait --for=condition=Ready "certificate/${CERT_NAME}" -n "${KIND_NAMESPACE}" --timeout=120s
  if api_has_tls_flag; then
    info "api-server already serves gRPC over TLS"
  else
    kube patch deployment "${API_DEPLOYMENT}" -n "${KIND_NAMESPACE}" \
      --type=json --patch-file "${SMOKE_DIR}/api-server.patch.json"
  fi
  kube patch deployment "${CP_DEPLOYMENT}" -n "${KIND_NAMESPACE}" \
    --type=strategic --patch-file "${SMOKE_DIR}/controller.patch.yaml"
  kube rollout status "deployment/${API_DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=180s
  kube rollout status "deployment/${CP_DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=180s
  success "api-server and controller rolled out with TLS settings"
}

verify_listener() {
  header "Verifying the TLS listener (port-forward :${SMOKE_GRPC_PORT} -> api-server:9000)"
  kube get secret "${CERT_NAME}" -n "${KIND_NAMESPACE}" \
    -o go-template='{{index .data "ca.crt" | base64decode}}' > "${TMP_DIR}/ca.crt"
  kube port-forward -n "${KIND_NAMESPACE}" "svc/${API_DEPLOYMENT}" \
    "${SMOKE_GRPC_PORT}:9000" >"${TMP_DIR}/port-forward.log" 2>&1 &
  PF_PID=$!
  for _ in $(seq 1 30); do
    if grep -q 'Forwarding from' "${TMP_DIR}/port-forward.log" 2>/dev/null; then
      break
    fi
    sleep 1
  done

  local s_client
  s_client=$(openssl s_client -connect "127.0.0.1:${SMOKE_GRPC_PORT}" \
    -servername "${DIAL_HOST}" -verify_hostname "${DIAL_HOST}" -alpn h2 \
    -CAfile "${TMP_DIR}/ca.crt" </dev/null 2>&1 || true)
  if grep -q 'Verify return code: 0 (ok)' <<<"${s_client}"; then
    success "TLS handshake verified against the cluster CA for ${DIAL_HOST}"
  else
    error "TLS handshake did not verify for ${DIAL_HOST}"
    printf '%s\n' "${s_client}" | tail -25 >&2
    return 1
  fi
  if grep -q 'ALPN protocol: h2' <<<"${s_client}"; then
    success "ALPN negotiated h2"
  else
    error "ALPN did not negotiate h2"
    return 1
  fi

  if curl -s --http2-prior-knowledge -m 5 -o /dev/null "http://127.0.0.1:${SMOKE_GRPC_PORT}/" 2>/dev/null; then
    error "a plaintext HTTP/2 connection was accepted on the TLS-only port"
    return 1
  fi
  success "plaintext dial refused"

  if command -v grpcurl >/dev/null 2>&1; then
    # The health and reflection services sit behind the JWT interceptor like
    # every other RPC (the framework ignores --auth-bypass-methods for JWT), so
    # an anonymous call that reaches the server answers Unauthenticated. Either
    # that or SERVING proves the RPC crossed the TLS listener; a handshake or
    # dial error does not.
    local health
    health=$(grpcurl -cacert "${TMP_DIR}/ca.crt" -servername "${DIAL_HOST}" \
      "127.0.0.1:${SMOKE_GRPC_PORT}" grpc.health.v1.Health/Check 2>&1 || true)
    if grep -q 'SERVING' <<<"${health}"; then
      success "gRPC health check over TLS: SERVING"
    elif grep -q 'code = Unauthenticated' <<<"${health}"; then
      success "gRPC call over TLS reached the server (answered Unauthenticated without a token)"
    else
      error "gRPC call over TLS did not reach the server"
      printf '%s\n' "${health}" | tail -5 >&2
      return 1
    fi
    if grpcurl -plaintext "127.0.0.1:${SMOKE_GRPC_PORT}" grpc.health.v1.Health/Check >/dev/null 2>&1; then
      error "gRPC plaintext health check succeeded on the TLS-only port"
      return 1
    fi
    success "gRPC plaintext health check refused"
  else
    warn "grpcurl not installed; skipping the gRPC health checks (openssl and curl checks passed)"
  fi
}

verify_controller() {
  header "Verifying the control plane dials over TLS"
  local logs=""
  for _ in $(seq 1 60); do
    logs=$(controller_logs)
    if grep -q 'transport=tls' <<<"${logs}" \
      && grep -q 'registered as cluster_id' <<<"${logs}" \
      && grep -q 'into reconcile queue on watch (re)connect' <<<"${logs}"; then
      break
    fi
    sleep 2
  done
  if ! grep -q 'transport=tls' <<<"${logs}"; then
    error "controller did not log transport=tls for ${DIAL_HOST}:9000"
    printf '%s\n' "${logs}" | tail -20 >&2
    return 1
  fi
  success "controller selected TLS for ${DIAL_HOST}:9000"
  if ! grep -q 'registered as cluster_id' <<<"${logs}"; then
    error "controller did not register"
    printf '%s\n' "${logs}" | tail -20 >&2
    return 1
  fi
  success "controller registered its cluster identity"
  if ! grep -q 'into reconcile queue on watch (re)connect' <<<"${logs}"; then
    error "controller did not establish WatchGateways over TLS"
    printf '%s\n' "${logs}" | tail -20 >&2
    return 1
  fi
  success "WatchGateways stream established and seeded over TLS"
  if grep -q 'watch stream disconnected' <<<"${logs}"; then
    warn "controller logged watch reconnects; inspect 'kubectl logs deployment/${CP_DEPLOYMENT} -n ${KIND_NAMESPACE}'"
  fi
}

disable_tls() {
  header "Restoring plaintext gRPC"
  if api_has_tls_flag; then
    kube rollout undo "deployment/${API_DEPLOYMENT}" -n "${KIND_NAMESPACE}"
  fi
  kube rollout undo "deployment/${CP_DEPLOYMENT}" -n "${KIND_NAMESPACE}"
  kube delete -f "${SMOKE_DIR}/certificate.yaml" --ignore-not-found
  kube delete secret "${CERT_NAME}" -n "${KIND_NAMESPACE}" --ignore-not-found
  kube rollout status "deployment/${API_DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=180s
  kube rollout status "deployment/${CP_DEPLOYMENT}" -n "${KIND_NAMESPACE}" --timeout=180s
  if api_has_tls_flag; then
    warn "api-server still carries --grpc-enable-tls after the rollback; re-run 'make kind-up' to reapply the overlay"
    return 1
  fi
  local logs=""
  for _ in $(seq 1 30); do
    logs=$(controller_logs)
    if grep -q 'transport=plaintext' <<<"${logs}"; then
      break
    fi
    sleep 2
  done
  if ! grep -q 'transport=plaintext' <<<"${logs}"; then
    warn "controller did not log transport=plaintext yet; check its logs"
    return 1
  fi
  success "plaintext gRPC restored"
}

case "${MODE}" in
  run)
    enable_tls
    verify_listener
    verify_controller
    success "gRPC TLS smoke test passed"
    if [[ "${KEEP}" == "true" ]]; then
      info "KEEP=true: leaving TLS enabled; run 'make kind-grpc-tls-smoke ARGS=revert' to restore plaintext"
    else
      disable_tls
    fi
    ;;
  revert)
    disable_tls
    ;;
  *)
    error "Usage: grpc-tls-smoke.sh [run|revert]   (KEEP=true keeps TLS enabled after a run)"
    exit 1
    ;;
esac
