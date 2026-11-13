#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Set OCE_REPO to the installation source directory}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# Resolve latest once per installation; retries keep the saved immutable references.
for role in controller runtime; do
  repository="quay.io/aipcc/base-images/agentic/oce-$role"
  if [[ -s $OCE_KIND_STATE/images.json ]]; then
    image=$(jq -er --arg role "$role" '.[$role]' "$OCE_KIND_STATE/images.json")
  else
    image="$repository:latest"
  fi
  docker pull "$image"
  reference=$(docker image inspect --format '{{json .RepoDigests}}' "$image" |
    jq -er --arg repository "$repository" \
      '[.[] | select(startswith($repository + "@sha256:") and test("@sha256:[a-f0-9]{64}$"))][0] // empty') ||
    { echo "No $repository digest found for $image." >&2; exit 1; }
  printf '%s\n' "$reference" > "$OCE_KIND_STATE/$role-image"
done

jq -n --rawfile controller "$OCE_KIND_STATE/controller-image" --rawfile runtime "$OCE_KIND_STATE/runtime-image" \
  '{controller:($controller|rtrimstr("\n")),runtime:($runtime|rtrimstr("\n"))}' > "$OCE_KIND_STATE/images.json.tmp"
mv "$OCE_KIND_STATE/images.json.tmp" "$OCE_KIND_STATE/images.json"
for role in controller runtime; do
  docker exec hypershell-oce-control-plane ctr -n k8s.io images pull "$(cat "$OCE_KIND_STATE/$role-image")"
done

# The image does not ship OCC, the Helm chart, or the operator helpers.
controller=$(cat "$OCE_KIND_STATE/controller-image")
revision=$(docker image inspect --format '{{index .Config.Labels "io.openshell.harness.version"}}' "$controller")
[[ $revision =~ ^[a-f0-9]{40}$ ]] || { echo 'OCE controller image is missing its source revision label.' >&2; exit 1; }
if [[ $OCE_REPO == "$OCE_KIND_STATE/oce-source" ]]; then
  git init "$OCE_REPO"
  git -C "$OCE_REPO" fetch --depth=1 https://github.com/openclaw/openclaw-enterprise.git "$revision"
  # Discard only managed checkout changes left by an earlier patch attempt.
  git -C "$OCE_REPO" checkout --force --detach FETCH_HEAD
  # shortcut: upstream quickstart assumes a co-located gateway, remove when OCE PR #4954 lands.
  if ! python3 "$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes/service-policies.py" --help |
    grep -q -- --openshell-namespace; then
    git -C "$OCE_REPO" apply "$here/openshell-namespace.patch"
  fi
else
  head=$(git -C "$OCE_REPO" rev-parse HEAD 2>/dev/null) || head=unknown
  if [[ $head != "$revision" ]]; then
    echo "warning: OCE_REPO is at $head; controller image was built from $revision." >&2
  fi
fi
GOTOOLCHAIN=auto go -C "$OCE_REPO" build -trimpath -o "$OCE_KIND_STATE/occ" ./cmd/occ
