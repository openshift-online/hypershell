#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

REPO_ROOT="$(git rev-parse --show-toplevel)"
BUILD_SOURCE="${BUILD_SOURCE:-worktree}"

WORKTREE_DIR=""
cleanup_worktree() {
  if [[ -n "${WORKTREE_DIR}" ]] && [[ -d "${WORKTREE_DIR}" ]]; then
    git worktree remove --force "${WORKTREE_DIR}" 2>/dev/null || rm -rf "${WORKTREE_DIR}"
  fi
}

if [[ "${BUILD_SOURCE}" == "baseline" ]]; then
  header "Building Baseline Images from origin/main"
  trap cleanup_worktree EXIT

  info "Fetching origin/main..."
  git fetch origin main --quiet

  WORKTREE_DIR=$(mktemp -d /tmp/hypershell-baseline-XXXXXX)
  rm -rf "${WORKTREE_DIR}"
  git worktree add --detach "${WORKTREE_DIR}" origin/main --quiet
  BUILD_DIR="${WORKTREE_DIR}"
  info "Building from origin/main ($(git -C "${WORKTREE_DIR}" rev-parse --short HEAD))"
else
  header "Building Images from Working Tree"
  BUILD_DIR="${REPO_ROOT}"
  info "Building from working tree ($(git rev-parse --short HEAD))"
fi

info "Building API server..."
${CONTAINER_ENGINE} build -t "${api_server_local}" \
  -f "${BUILD_DIR}/components/api-server/Dockerfile" \
  --build-arg GIT_VERSION="${build_version}" \
  --build-arg BUILD_TIME="${build_time}" \
  "${BUILD_DIR}/components/api-server"

info "Building control plane..."
${CONTAINER_ENGINE} build -t "${control_plane_local}" \
  -f "${BUILD_DIR}/components/control-plane/Dockerfile" "${BUILD_DIR}"

info "Building web console..."
${CONTAINER_ENGINE} build -t "${web_console_local}" \
  -f "${BUILD_DIR}/components/web-console/Dockerfile" "${BUILD_DIR}"

success "All images built"

info "Tagging images with registry refs..."
${CONTAINER_ENGINE} tag "${api_server_local}" "${api_server_ref}"
${CONTAINER_ENGINE} tag "${control_plane_local}" "${control_plane_ref}"
${CONTAINER_ENGINE} tag "${web_console_local}" "${web_console_ref}"

if cluster_exists; then
  info "Loading images into Kind cluster..."
  tmpdir=$(mktemp -d /tmp/kind-images-XXXXXX)
  for img in "${api_server_ref}" "${control_plane_ref}" "${web_console_ref}"; do
    archive="${tmpdir}/$(echo "${img}" | tr '/:' '__').tar"
    ${CONTAINER_ENGINE} save "${img}" -o "${archive}"
    kind load image-archive "${archive}" --name "${KIND_CLUSTER_NAME}"
  done
  rm -rf "${tmpdir}"
  success "Images loaded into Kind"
fi
