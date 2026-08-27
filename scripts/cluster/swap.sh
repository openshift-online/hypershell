#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

ACTION="${1:-}"
COMPONENT="${2:-}"
if [[ -z "${ACTION}" ]] || [[ -z "${COMPONENT}" ]]; then
  error "Usage: swap.sh up|down <api-server|control-plane|web-console>"
  exit 1
fi

load_cluster_driver
case "${ACTION}" in
  up) component_swap "${COMPONENT}" ;;
  down) component_revert "${COMPONENT}" ;;
  *)
    error "Unknown action: ${ACTION}"
    error "Valid actions: up, down"
    exit 1
    ;;
esac
