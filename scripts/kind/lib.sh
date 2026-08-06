#!/usr/bin/env bash
set -euo pipefail

# --- Color output (respects NO_COLOR and non-TTY) ---

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  BOLD='\033[1m'
  BLUE='\033[0;34m'
  CYAN='\033[0;36m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RED='\033[0;31m'
  NC='\033[0m'
else
  BOLD='' BLUE='' CYAN='' GREEN='' YELLOW='' RED='' NC=''
fi

header()  { printf "${BOLD}${BLUE}==> %s${NC}\n" "$*"; }
info()    { printf "${CYAN}    %s${NC}\n" "$*"; }
success() { printf "${GREEN}    %s${NC}\n" "$*"; }
warn()    { printf "${YELLOW}    %s${NC}\n" "$*"; }
error()   { printf "${RED}ERROR: %s${NC}\n" "$*" >&2; }

# --- Defaults (defensive — Make exports these, but scripts can run standalone) ---

: "${KIND_CLUSTER_NAME:=hypershell-dev}"
: "${KIND_NAMESPACE:=hypershell-system}"
: "${CONTAINER_ENGINE:=$(command -v podman 2>/dev/null || echo docker)}"

# --- Cluster helpers ---

cluster_exists() {
  kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"
}

require_cluster() {
  if ! cluster_exists; then
    error "No Kind cluster '${KIND_CLUSTER_NAME}' running. Run 'make kind-up' first."
    exit 1
  fi
}

kctx() {
  echo "kind-${KIND_CLUSTER_NAME}"
}

# --- Swap tracking (.kind-swaps) ---

SWAP_FILE=".kind-swaps"

track_swap() {
  local component="$1"
  grep -q "^${component}$" "${SWAP_FILE}" 2>/dev/null || echo "${component}" >> "${SWAP_FILE}"
}

clear_swap() {
  local component="$1"
  if [[ -f "${SWAP_FILE}" ]]; then
    sed -i.bak "/^${component}$/d" "${SWAP_FILE}" 2>/dev/null
    rm -f "${SWAP_FILE}.bak"
  fi
}

is_swapped() {
  local component="$1"
  grep -q "^${component}$" "${SWAP_FILE}" 2>/dev/null
}
