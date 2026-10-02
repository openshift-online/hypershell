#!/usr/bin/env bash
#
# Fleet-identity firewall enforcement (data-architecture.spec §3.5).
#
# The fleet-dashboard image and its source are PUBLIC. They must reveal nothing
# about the fleet's structure: no instance names, cluster names, hostnames, the
# GitOps repo slug, or a hard-coded environment->hub map. Every such value
# arrives at runtime (FD_* env / mounted config) or is discovered dynamically.
#
# This scan greps the fleet-dashboard component sources -- including tests and
# fixtures -- for that deny-list and fails CI on any hit. It is intentionally
# conservative (high signal): product-level names that are already public
# (the "hypershell" product name, metric names, the delivery.hypershell.app/*
# label schema, promoter CRD kinds) do NOT match.
#
# Run locally: bash scripts/check_fleet_dashboard_firewall.sh

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

scan_dirs=(
  "components/fleet-dashboard"
  "packages/fleet-dashboard-ui"
)

# Deny-list, extended-regex. Each pattern targets a concrete fleet identifier.
#  - hyp<N> / hyp<N>mc<N> : instance names (hyp0, hyp6, hyp0mc0). Word boundary
#    keeps the product name "hypershell" (hyp + 'e') and hashes from matching.
#  - hypc-* / hshells*    : cluster names.
#  - *.infra.hypershell.app / glass.infra : fleet hostnames.
#  - hypershell-gitops    : the private GitOps repo slug.
#
# NOTE: the firewall bans fleet *identifiers* (concrete values), not variable
# names. A comment naming the prototype's `ENV_HUB` anti-pattern is legitimate
# firewall documentation as long as it carries no literal instance/env values --
# those are caught by the hyp<N> pattern above.
patterns=(
  '\bhyp[0-9]+(mc[0-9]+)?\b'
  '\bhypc-[a-z0-9-]+'
  '\bhshells[0-9a-z]+'
  '[a-z0-9.-]*\.infra\.hypershell\.app'
  '\bglass\.infra\b'
  '\bhypershell-gitops\b'
)

# Build a single alternation for one grep pass.
alternation="$(IFS='|'; echo "${patterns[*]}")"

hits=0
for dir in "${scan_dirs[@]}"; do
  target="${root}/${dir}"
  [[ -d "${target}" ]] || continue
  # Exclude dependency/build artifacts and lockfiles (opaque hashes / vendored
  # code we do not author); everything we author -- src, tests, fixtures,
  # configs, Dockerfile -- is in scope.
  while IFS= read -r -d '' file; do
    if matches="$(grep -nEi "${alternation}" "${file}")"; then
      echo "FIREWALL VIOLATION in ${file#"${root}/"}:"
      echo "${matches}" | sed 's/^/  /'
      hits=1
    fi
  done < <(
    find "${target}" \
      \( -name node_modules -o -name dist -o -name .git -o -name vendor \) -prune -o \
      -type f \
      ! -name '*.sum' ! -name '*.lock' ! -name 'pnpm-lock.yaml' \
      ! -name 'check_fleet_dashboard_firewall.sh' \
      -print0
  )
done

if [[ "${hits}" -ne 0 ]]; then
  echo
  echo "Fleet-identity firewall check FAILED (data-architecture.spec §3.5)."
  echo "The fleet-dashboard image is public: remove fleet-identifying values from"
  echo "source/tests/fixtures and supply them at runtime via FD_* env or mounted config."
  exit 1
fi

echo "Fleet-identity firewall check passed: no fleet-identifying values found."
