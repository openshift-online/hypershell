#!/usr/bin/env bash
# Print the directory of the pinned rh-trex-ai checkout, creating it if needed.
# For callers that need the checkout path (the dependency age check).
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/trex-checkout.sh
source "$root/scripts/lib/trex-checkout.sh"
trex_checkout "$root" >&2
printf '%s\n' "$TREX_DIR"
