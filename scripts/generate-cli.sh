#!/usr/bin/env bash
# Regenerate the hsctl CLI from the API server OpenAPI spec with the rh-trex-ai
# cli-generator, pinned in scripts/rh-trex-ai.ref.
#
# The generator writes a complete CLI project. Only the files listed in
# HAND_MAINTAINED below are ours; every other generated file is copied over
# components/cli. HyperShell never edits generated files; it configures the
# generator instead (--config-name, --oidc-client-id, see DEVELOPMENT.md).
#
# Usage: scripts/generate-cli.sh [--check]
#   --check  fail if any generated file differs from the committed one
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/trex-checkout.sh
source "$root/scripts/lib/trex-checkout.sh"
trex_checkout "$root"

# Generated files HyperShell replaces with its own implementation.
HAND_MAINTAINED=(
  cmd/hsctl/main.go
  go.mod
  go.sum
)

# Resource commands that live beside the generated ones but are written by hand:
# the scoped service account commands (they sit under a gateway) and the
# extension kinds, which are served under /api/hypershell/ext/ and so fall
# outside the single API prefix the generator handles.
HAND_MAINTAINED_COMMANDS=(
  cmd/hsctl/create/serviceAccount/cmd.go
  cmd/hsctl/get/serviceAccount/cmd.go
  cmd/hsctl/list/serviceAccounts/cmd.go
  cmd/hsctl/delete/serviceAccount/cmd.go
)
for kind in agentRuntime sandboxTemplate providerSpec providerBinding inferenceRoute secretSource; do
  for verb in create get delete; do
    HAND_MAINTAINED_COMMANDS+=("cmd/hsctl/$verb/$kind/cmd.go")
  done
  HAND_MAINTAINED_COMMANDS+=("cmd/hsctl/list/${kind}s/cmd.go")
done

mode="${1:-}"
cli="$root/components/cli"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

(cd "$TREX_DIR/scripts/cli-generator" && go run . \
  --spec "$root/components/api-server/openapi/openapi.yaml" \
  --out "$tmp" \
  --binary hsctl \
  --project hypershell \
  --oidc-client-id hypershell-cli \
  --config-name hypershell \
  --api-prefix /api/hypershell/v1 \
  --module github.com/openshift-online/hypershell/components/cli) >/dev/null
gofmt -w "$tmp"

drift=()
while IFS= read -r file; do
  skip=0
  for kept in "${HAND_MAINTAINED[@]}"; do
    [[ "$file" == "${kept%% *}" ]] && skip=1
  done
  [[ $skip == 1 ]] && continue

  if [[ "$mode" == "--check" ]]; then
    cmp -s "$tmp/$file" "$cli/$file" || drift+=("$file")
  else
    mkdir -p "$cli/$(dirname "$file")"
    cp "$tmp/$file" "$cli/$file"
  fi
done < <(cd "$tmp" && find . -type f | sed 's#^\./##' | sort)

# A resource command the generator no longer emits (the resource or operation
# left the OpenAPI spec) stays on disk and compiles, but nothing registers it.
# Report it so it is removed instead of lingering, as a stale "delete role" did.
orphans=()
while IFS= read -r file; do
  [[ -f "$tmp/$file" ]] && continue
  kept=0
  for known in "${HAND_MAINTAINED_COMMANDS[@]}"; do
    [[ "$file" == "$known" ]] && kept=1
  done
  ((kept)) || orphans+=("$file")
done < <(cd "$cli" && git ls-files --cached --others --exclude-standard -- cmd/hsctl |
  grep -E '^cmd/hsctl/(create|get|list|update|delete)/[^/]+/cmd\.go$' || true)

if ((${#orphans[@]})); then
  echo "Resource commands the generator no longer emits (delete them, or list them in HAND_MAINTAINED_COMMANDS in $0 if they are hand-written):" >&2
  printf '    - %s\n' "${orphans[@]}" >&2
fi

if [[ "$mode" == "--check" ]]; then
  if ((${#orphans[@]})); then
    exit 1
  fi
  if ((${#drift[@]})); then
    echo "Generated CLI files differ from the committed ones:" >&2
    printf '    - %s\n' "${drift[@]}" >&2
    echo "Run 'make generate-cli' and commit the result." >&2
    exit 1
  fi
  echo "No drift: generated CLI matches committed code"
fi
