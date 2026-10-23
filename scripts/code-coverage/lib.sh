#!/usr/bin/env bash
# Cluster helpers for make coverage (scripts/code-coverage/run.sh).
# Spec: specs/platform/local-development.spec.md DEV-LOC-18.
#
# Expects scripts/kind/lib.sh to be sourced first (kube, info, CONTAINER_ENGINE,
# KIND_CLUSTER_NAME, KIND_NAMESPACE).
#
# The instrumented pods write coverage to the hypershell-coverage volume
# (deploy/kind/code-coverage). Go and Node write their data only when the process
# exits cleanly, so collection stops the pods first and starts them again after.

COVERAGE_PVC="hypershell-coverage"
COVERAGE_DEPLOYMENTS=(hypershell-api-server hypershell-controller hypershell-web-console)
# Replica counts recorded by stop_instrumented_pods, restored by start_instrumented_pods.
COVERAGE_REPLICAS=()

# Docker's default socket; overridable for tests.
: "${COVERAGE_DOCKER_SOCKET:=/var/run/docker.sock}"

# testcontainers_env: print, space-separated on one line, the environment the
# API server tests need to reach a podman-only container engine. testcontainers
# looks for the Docker socket; when it is missing and podman serves its own
# socket, point DOCKER_HOST there and turn off the Ryuk reaper, which podman
# cannot run. Prints nothing when Docker's socket exists or DOCKER_HOST is set.
testcontainers_env() {
  if [[ -n "${DOCKER_HOST:-}" || -S "${COVERAGE_DOCKER_SOCKET}" ]]; then
    return 0
  fi
  command -v podman >/dev/null 2>&1 || return 0
  local socket
  if [[ "$(uname -s)" == "Darwin" ]]; then
    socket="$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)"
  else
    socket="$(podman info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null)"
  fi
  [[ -n "${socket}" && -S "${socket}" ]] || return 0
  echo "DOCKER_HOST=unix://${socket} TESTCONTAINERS_RYUK_DISABLED=true"
}

# kind_ingress_addr: print 127.0.0.1:<port> for the cloud-provider-kind proxy's
# published 443 port, or nothing when there is no proxy. The Go suite reaches
# *.localhost through E2E_KIND_INGRESS_ADDR (default 127.0.0.1:443), and host
# port 443 only works through a sudo pfctl/iptables redirect that macOS can
# drop mid-run (seen on 2026-10-23: Keycloak "connection refused" on 443 while
# the published port still answered). Same lookup as tests/e2e/drivers/kind.sh.
kind_ingress_addr() {
  local proxy port
  proxy="$(${CONTAINER_ENGINE} ps --format '{{.Names}}' 2>/dev/null | grep '^kindccm-gw' | head -1)"
  [[ -n "${proxy}" ]] || return 0
  port="$(${CONTAINER_ENGINE} port "${proxy}" 443 2>/dev/null | head -1 | grep -oE '[0-9]+$' || true)"
  if [[ -n "${port}" ]]; then
    echo "127.0.0.1:${port}"
  fi
}

# coverage_node: the Kind node container that holds the local-path volume.
coverage_node() {
  echo "${KIND_CLUSTER_NAME}-control-plane"
}

# require_coverage_cluster: fail with a hint unless the cluster was deployed
# with KIND_COVERAGE=true (the coverage volume exists and the API server writes
# Go coverage to it).
require_coverage_cluster() {
  if ! cluster_exists; then
    error "No Kind cluster '${KIND_CLUSTER_NAME}'. Run: KIND_COVERAGE=true make kind-up"
    return 1
  fi
  local env_names
  env_names="$(kube get deployment hypershell-api-server -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="api-server")].env[*].name}' 2>/dev/null)"
  if ! kube get pvc "${COVERAGE_PVC}" -n "${KIND_NAMESPACE}" >/dev/null 2>&1 ||
    [[ " ${env_names} " != *" GOCOVERDIR "* ]]; then
    error "The cluster is not deployed with coverage. Run: KIND_COVERAGE=true make kind-up"
    return 1
  fi
}

# coverage_volume_path: host path of the coverage volume inside the Kind node.
coverage_volume_path() {
  local pv
  pv="$(kube get pvc "${COVERAGE_PVC}" -n "${KIND_NAMESPACE}" -o jsonpath='{.spec.volumeName}')"
  if [[ -z "${pv}" ]]; then
    error "PVC ${COVERAGE_PVC} is not bound yet"
    return 1
  fi
  kube get pv "${pv}" -o jsonpath='{.spec.hostPath.path}{.spec.local.path}'
}

# stop_instrumented_pods: record replica counts, scale to zero, and wait until
# the pods are gone so every process has flushed its coverage data.
stop_instrumented_pods() {
  local deployment replicas
  COVERAGE_REPLICAS=()
  for deployment in "${COVERAGE_DEPLOYMENTS[@]}"; do
    replicas="$(kube get deployment "${deployment}" -n "${KIND_NAMESPACE}" -o jsonpath='{.spec.replicas}')"
    COVERAGE_REPLICAS+=("${replicas:-0}")
  done
  kube scale deployment "${COVERAGE_DEPLOYMENTS[@]}" -n "${KIND_NAMESPACE}" --replicas=0 >/dev/null
  for deployment in "${COVERAGE_DEPLOYMENTS[@]}"; do
    # The grace period is 90s (deploy/kind/code-coverage); allow for it plus margin.
    kube wait pod -l "app=${deployment}" -n "${KIND_NAMESPACE}" --for=delete --timeout=150s >/dev/null 2>&1 || true
    if [[ -n "$(kube get pods -l "app=${deployment}" -n "${KIND_NAMESPACE}" -o name 2>/dev/null)" ]]; then
      error "Pods of ${deployment} did not stop; their coverage data may be missing"
      return 1
    fi
  done
}

# start_instrumented_pods: restore the replica counts and wait for the rollouts.
start_instrumented_pods() {
  local i deployment
  for i in "${!COVERAGE_DEPLOYMENTS[@]}"; do
    deployment="${COVERAGE_DEPLOYMENTS[$i]}"
    kube scale deployment "${deployment}" -n "${KIND_NAMESPACE}" --replicas="${COVERAGE_REPLICAS[$i]}" >/dev/null
  done
  for i in "${!COVERAGE_DEPLOYMENTS[@]}"; do
    [[ "${COVERAGE_REPLICAS[$i]}" -gt 0 ]] || continue
    kube rollout status "deployment/${COVERAGE_DEPLOYMENTS[$i]}" -n "${KIND_NAMESPACE}" --timeout=300s || return 1
  done
}

# reset_cluster_coverage: restart the pods with an empty volume, so collected
# data covers only what runs after this call (plus pod startup).
reset_cluster_coverage() {
  local volume
  volume="$(coverage_volume_path)" || return 1
  info "Stopping instrumented pods..."
  stop_instrumented_pods || return 1
  info "Clearing ${volume} on $(coverage_node)..."
  ${CONTAINER_ENGINE} exec "$(coverage_node)" find "${volume}" -mindepth 1 -delete || return 1
  info "Starting instrumented pods..."
  start_instrumented_pods
}

# collect_cluster_coverage DEST: stop the pods so they write their data, copy
# the volume to DEST (Go files in DEST/go, Node files in DEST/node), and start
# the pods again.
collect_cluster_coverage() {
  local dest="$1" volume
  volume="$(coverage_volume_path)" || return 1
  info "Stopping instrumented pods to flush coverage data..."
  stop_instrumented_pods || return 1
  mkdir -p "${dest}/go"
  ${CONTAINER_ENGINE} cp "$(coverage_node):${volume}/." "${dest}/" || return 1
  local file
  for file in "${dest}"/covmeta.* "${dest}"/covcounters.*; do
    [[ -e "${file}" ]] && mv "${file}" "${dest}/go/"
  done
  info "Copied $(find "${dest}" -type f | wc -l | tr -d ' ') coverage files to ${dest}"
  info "Starting instrumented pods..."
  start_instrumented_pods
}

# extract_bff_dist DEST: copy the compiled BFF (with its inline source maps)
# out of the web console image the cluster runs.
extract_bff_dist() {
  local dest="$1" image container
  image="$(kube get deployment hypershell-web-console -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="web-console")].image}')"
  container="$(${CONTAINER_ENGINE} create "${image}")" || {
    error "Cannot open web console image ${image}; it must be a local build"
    return 1
  }
  mkdir -p "${dest}"
  ${CONTAINER_ENGINE} cp "${container}:/app/dist/." "${dest}/"
  local status=$?
  ${CONTAINER_ENGINE} rm "${container}" >/dev/null
  return "${status}"
}
