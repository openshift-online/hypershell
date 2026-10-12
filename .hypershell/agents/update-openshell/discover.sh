#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

# update-openshell discovery. Emits one work item when a newer stable midstream
# OpenShell release (opendatahub-io/openshell vX.Y.Z-rhaiv.N tag) exists that is
# not already pinned in this repository. SDLC discover.sh contract: reads
# REPOSITORY/MAX_ITEMS/DRY_RUN, emits newline-delimited JSON on stdout, progress
# on stderr, always exits 0 (discovery failure is not fatal).

required_variables=(REPOSITORY MAX_ITEMS DRY_RUN)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Required variable %s is empty.\n' "$variable" >&2
    exit 1
  fi
done

if [[ ! "$REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  printf 'REPOSITORY is not valid.\n' >&2
  exit 1
fi
if [[ ! "$MAX_ITEMS" =~ ^[1-9][0-9]*$ ]]; then
  printf 'MAX_ITEMS is not valid.\n' >&2
  exit 1
fi

# Repo root, derived from this script's location (coordinator-local checkout).
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)

printf 'update-openshell discovery: resolving latest stable midstream tag.\n' >&2
latest_tag=$(gh api repos/opendatahub-io/openshell/tags \
  --jq '[.[] | select(.name | test("^v[0-9]"))] | .[0].name' 2>/dev/null || true)
if [[ -z "$latest_tag" ]]; then
  printf 'update-openshell discovery: could not resolve latest tag; nothing to do.\n' >&2
  exit 0
fi

# The latest tag IS the image tag. If it already appears anywhere in the repo,
# the pin sweep is current and there is no work.
if grep -rqF "odh-openshell-gateway:$latest_tag" "$repo_root" 2>/dev/null; then
  printf 'update-openshell discovery: already pinned to %s; nothing to do.\n' "$latest_tag" >&2
  exit 0
fi

current_tag=$(grep -rhoE 'odh-openshell-gateway:v[0-9][^"[:space:]]*' "$repo_root" 2>/dev/null \
  | sed 's/.*://' | sort -u | tail -1 || true)

printf 'update-openshell discovery: update available (current=%s latest=%s).\n' \
  "${current_tag:-unknown}" "$latest_tag" >&2

# One aggregate work item. run.sh re-resolves the latest tag itself; these fields
# are informational (the coordinator passes no per-item env for this agent).
printf '{"update_available":true,"latest_tag":"%s","current_tag":"%s"}\n' \
  "$latest_tag" "${current_tag:-unknown}"
