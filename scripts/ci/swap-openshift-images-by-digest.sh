#!/usr/bin/env bash
# swap-openshift-images-by-digest.sh - inject the pull request's component images
# into the deployed environment by immutable digest
# (ephemeral-pr-environments.spec.md: Image Gating and Swap).
#
# The workflow gates on the Konflux builds for the head commit and passes each
# built image reference here. This script resolves each reference to its manifest
# digest and rolls the deployment to repo@sha256:<digest>, so the environment
# runs exactly the artifact CI verified and a later re-push of a mutable tag
# cannot change it.
#
# Built pull-request images (tag on-pr-<sha>) MUST resolve to a digest; a missing
# skopeo/oc inspect or a registry that hides the digest is a hard error, not a
# silent tag swap. Baseline images (unchanged components) may fall back to the
# tag when no digest is available; that fallback is recorded in the run output
# rather than silent, matching the spec's last-resort clause.
#
# Environment:
#   OPENSHIFT_NAMESPACE   target platform namespace (required)
#   API_SERVER_IMAGE      api-server image ref (optional)
#   CONTROL_PLANE_IMAGE   control-plane image ref (optional)
#   WEB_CONSOLE_IMAGE     web-console image ref (optional)
#   ROLLOUT_TIMEOUT       per-deployment rollout ceiling (default 300s)
#   PR_ENV_KUBECTL        kubectl/oc binary (default: oc)
set -euo pipefail

KUBECTL="${PR_ENV_KUBECTL:-oc}"
: "${OPENSHIFT_NAMESPACE:?OPENSHIFT_NAMESPACE is required}"
: "${ROLLOUT_TIMEOUT:=300s}"

# inspect_digest <image-ref> -> sha256:<digest> or empty when neither skopeo
# nor oc can resolve one.
inspect_digest() {
  local ref="$1"
  local digest=""
  if command -v skopeo >/dev/null 2>&1; then
    digest="$(skopeo inspect --format '{{.Digest}}' "docker://${ref}" 2>/dev/null || true)"
  fi
  if [[ "${digest}" != sha256:* ]] && command -v "${KUBECTL}" >/dev/null 2>&1; then
    digest="$("${KUBECTL}" image info "${ref}" -o jsonpath='{.digest}' 2>/dev/null || true)"
  fi
  printf '%s' "${digest}"
}

# resolve_by_digest <image-ref> -> repo@sha256:<digest>. Built on-pr-<sha> tags
# fail closed when no digest is available. Other refs (baseline) fall back to
# the original tag with a recorded warning. An already-pinned @sha256 ref is
# returned unchanged.
resolve_by_digest() {
  local ref="$1"
  if [[ "${ref}" == *@sha256:* ]]; then
    printf '%s' "${ref}"
    return 0
  fi
  local repo="${ref%:*}"
  local digest
  digest="$(inspect_digest "${ref}")"
  if [[ "${digest}" == sha256:* ]]; then
    printf '%s@%s' "${repo}" "${digest}"
    return 0
  fi
  if [[ "${ref}" == *":on-pr-"* ]]; then
    echo "ERROR: no digest for built image ${ref}; refusing mutable-tag fallback." >&2
    echo "Install skopeo (or use oc image info) and ensure the runner can inspect the registry." >&2
    return 1
  fi
  echo "WARNING: no digest available for ${ref}; falling back to the mutable tag" >&2
  printf '%s' "${ref}"
}

swap_deployment() {
  local deployment="$1" ref="$2"
  shift 2
  local containers=("$@")
  [[ -n "${ref}" ]] || return 0

  local image
  image="$(resolve_by_digest "${ref}")"
  echo "  ${deployment} -> ${image}"

  local set_args=()
  local c
  for c in "${containers[@]}"; do
    set_args+=("${c}=${image}")
  done
  "${KUBECTL}" set image "deployment/${deployment}" "${set_args[@]}" -n "${OPENSHIFT_NAMESPACE}"
  "${KUBECTL}" rollout status "deployment/${deployment}" -n "${OPENSHIFT_NAMESPACE}" --timeout="${ROLLOUT_TIMEOUT}"
}

swap_pr_images() {
  if [[ -z "${API_SERVER_IMAGE:-}" && -z "${CONTROL_PLANE_IMAGE:-}" && -z "${WEB_CONSOLE_IMAGE:-}" ]]; then
    echo "No component image overrides set; environment keeps baseline images."
    return 0
  fi

  echo "Swapping pull-request component images by digest into ${OPENSHIFT_NAMESPACE}"
  swap_deployment hypershell-api-server "${API_SERVER_IMAGE:-}" api-server migrate
  swap_deployment hypershell-controller "${CONTROL_PLANE_IMAGE:-}" controller
  swap_deployment hypershell-web-console "${WEB_CONSOLE_IMAGE:-}" web-console
  echo "Component image swap complete."
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  swap_pr_images
fi
