#!/usr/bin/env bash
# Point Kind deployments at specific container images and wait for rollouts.
#
# Used by CI after kind-up brings the cluster online with baseline images:
# Konflux PR images are swapped in once their builds finish.
#
# Environment (all optional; only non-empty values are applied):
#   API_SERVER_IMAGE
#   CONTROL_PLANE_IMAGE
#   WEB_CONSOLE_IMAGE
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

_api_img="${API_SERVER_IMAGE:-}"
_cp_img="${CONTROL_PLANE_IMAGE:-}"
_wc_img="${WEB_CONSOLE_IMAGE:-}"

if [[ -z "${_api_img}" && -z "${_cp_img}" && -z "${_wc_img}" ]]; then
  info "No component image overrides set; nothing to swap."
  exit 0
fi

deployment_image() {
  local deployment="$1" container="$2"
  kube get "deployment/${deployment}" -n "${KIND_NAMESPACE}" \
    -o "jsonpath={.spec.template.spec.containers[?(@.name==\"${container}\")].image}" 2>/dev/null || true
}

swap_deployment() {
  local deployment="$1" component="$2" image="$3"
  shift 3
  local containers=("$@")

  [[ -n "${image}" ]] || return 0
  if is_swapped "${component}"; then
    return 0
  fi

  local current="${image}"
  for container in "${containers[@]}"; do
    local running
    running="$(deployment_image "${deployment}" "${container}")"
    if [[ "${running}" != "${image}" ]]; then
      current=""
      break
    fi
  done

  if [[ -n "${current}" ]]; then
    info "  ${component} already on ${image}"
    return 0
  fi

  info "  ${component} -> ${image}"
  local set_image_args=()
  for container in "${containers[@]}"; do
    set_image_args+=("${container}=${image}")
  done
  # shellcheck disable=SC2068
  kube set image "deployment/${deployment}" ${set_image_args[@]} -n "${KIND_NAMESPACE}"
  kube rollout restart "deployment/${deployment}" -n "${KIND_NAMESPACE}"
  kube rollout status "deployment/${deployment}" -n "${KIND_NAMESPACE}" --timeout=300s
}

header "Component Image Swap"

swap_deployment "hypershell-api-server" api-server "${_api_img}" api-server migrate
swap_deployment "hypershell-controller" control-plane "${_cp_img}" controller
swap_deployment "hypershell-web-console" web-console "${_wc_img}" web-console

success "Component images ready"
