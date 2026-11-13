#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Run the prerequisite exports first}"
: "${OCE_KIND_STATE:?Run the prerequisite exports first}"
cd "$OCE_REPO"

# Build the installation CLI from the same OCE checkout as the images.
GOTOOLCHAIN=auto go build -trimpath -o "$OCE_KIND_STATE/occ" ./cmd/occ

# Read OCE's tested Node base image instead of maintaining a separate pin here.
# Its controller build helper records the source revision automatically.
node_base_image=$(jq -er '.prepare.defaultEnv.NODE_BASE_IMAGE' \
  scripts/ci/test-suites/images-packaging.json)
node scripts/build-controller.mjs docker --target runtime \
  --build-arg NODE_BASE_IMAGE="$node_base_image" \
  -t localhost/oce-hypershell/controller:local

# Resolve OpenClaw source inputs with OCE's helper. Its runtime Dockerfile
# supplies the Node base images and the rest of the runtime build defaults.
source_args=$(node scripts/lib/openclaw-source.mjs build-args)
openclaw_build_args=()
while IFS= read -r argument; do openclaw_build_args+=(--build-arg "$argument"); done <<< "$source_args"
docker build -f deploy/runtime/Dockerfile "${openclaw_build_args[@]}" \
  -t localhost/oce-hypershell/runtime:local .

# Import both images into Kind and record their immutable manifest references.
for role in controller runtime; do
  tag="localhost/oce-hypershell/$role:local"
  docker save "$tag" | docker exec -i hypershell-oce-control-plane ctr -n k8s.io images import -
  digest=$(docker exec hypershell-oce-control-plane ctr -n k8s.io images ls |
    awk -v image="$tag" '$1==image {print $3}')
  [[ $digest =~ ^sha256:[a-f0-9]{64}$ ]]
  reference="localhost/oce-hypershell/$role@$digest"
  docker exec hypershell-oce-control-plane ctr -n k8s.io images tag --force "$tag" "$reference"
  printf '%s\n' "$reference" > "$OCE_KIND_STATE/$role-image"
done

# Save the references for the OCE installer. The parent shell's directory is unchanged.
jq -n --rawfile controller "$OCE_KIND_STATE/controller-image" --rawfile runtime "$OCE_KIND_STATE/runtime-image" \
  '{controller:($controller|rtrimstr("\n")),runtime:($runtime|rtrimstr("\n"))}' > "$OCE_KIND_STATE/images.json"
