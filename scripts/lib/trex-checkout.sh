#!/usr/bin/env bash
# Shared by the generate-* scripts: check out rh-trex-ai at the pinned commit.
#
# The rh-trex-ai generators are not published as Go modules (their go.mod files
# use local replace directives for openapi-ir), so they run from a checkout.
#
# Sets TREX_DIR to the checkout. Environment overrides:
#   TREX_REF    full commit SHA (default: scripts/rh-trex-ai.ref)
#   TREX_REPO   repository URL (default: upstream)
#   TREX_CACHE  checkout directory (default: ~/.cache/hypershell/rh-trex-ai)

# Git exports these to hooks; left set they redirect the git -C calls below to
# this repository instead of the generator cache.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

trex_checkout() {
  local root="$1"
  TREX_REF="${TREX_REF:-$(tr -d '[:space:]' <"$root/scripts/rh-trex-ai.ref")}"
  TREX_REPO="${TREX_REPO:-https://github.com/openshift-online/rh-trex-ai}"
  TREX_DIR="${TREX_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/hypershell/rh-trex-ai}"

  if [[ ! "$TREX_REF" =~ ^[0-9a-f]{40}$ ]]; then
    echo "TREX_REF must be a full 40-character commit SHA, got '$TREX_REF'" >&2
    return 1
  fi

  if [[ ! -d "$TREX_DIR/.git" ]]; then
    mkdir -p "$TREX_DIR"
    git -C "$TREX_DIR" init -q
    git -C "$TREX_DIR" remote add origin "$TREX_REPO"
  else
    local current_url
    current_url="$(git -C "$TREX_DIR" remote get-url origin)"
    if [[ "$current_url" != "$TREX_REPO" ]]; then
      git -C "$TREX_DIR" remote set-url origin "$TREX_REPO"
    fi
  fi
  if ! git -C "$TREX_DIR" cat-file -e "$TREX_REF^{commit}" 2>/dev/null; then
    if ! git -C "$TREX_DIR" fetch -q --depth 1 origin "$TREX_REF"; then
      echo "cannot fetch rh-trex-ai $TREX_REF from $TREX_REPO" >&2
      echo "connect to the network, or set TREX_CACHE to a checkout that already has it" >&2
      return 1
    fi
  fi

  # Generators are code that runs in hooks and CI, so refuse a checkout whose
  # files differ from the pinned commit instead of generating with edited
  # templates. Nothing is discarded: TREX_CACHE may be a developer's own clone.
  if [[ -n "$(git -C "$TREX_DIR" status --porcelain --untracked-files=no)" ]]; then
    echo "$TREX_DIR has local changes; the generators must run from the pinned commit" >&2
    echo "remove the directory (it is only a cache) or point TREX_CACHE elsewhere" >&2
    return 1
  fi
  git -C "$TREX_DIR" checkout -q --detach "$TREX_REF"

  local head
  head="$(git -C "$TREX_DIR" rev-parse HEAD)"
  if [[ "$head" != "$TREX_REF" ]]; then
    echo "rh-trex-ai checkout is at $head, expected $TREX_REF" >&2
    return 1
  fi
}
