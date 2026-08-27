#!/usr/bin/env bash
# Kind lifecycle driver. Wraps the existing scripts/kind/ entrypoints so
# make kind-* keeps today's behavior and a new infrastructure target only
# has to add a driver file.
set -euo pipefail

KIND_SCRIPTS="${REPO_ROOT}/scripts/kind"

cluster_up() {
  exec "${KIND_SCRIPTS}/up.sh"
}

cluster_down() {
  exec "${KIND_SCRIPTS}/down.sh"
}

cluster_teardown() {
  exec "${KIND_SCRIPTS}/teardown.sh"
}

cluster_status() {
  exec "${KIND_SCRIPTS}/status.sh"
}

component_swap() {
  exec "${KIND_SCRIPTS}/swap-component.sh" up "$1"
}

component_revert() {
  exec "${KIND_SCRIPTS}/swap-component.sh" down "$1"
}
