#!/usr/bin/env bash
# Contract tests for delivering the hsctl browser module in the web console image
# (specs/platform/hsctl-browser-build.spec.md, CLI-WASM-05): a CLI change
# rebuilds the console image, the hermetic build prefetches the CLI Go modules,
# the image compiles the module from components/cli before the Vite build, and
# the module stays out of the native release targets (CLI-REL).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PASS=0
FAIL=0

assert_ok() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s\n' "${label}"
  fi
}

DOCKERFILE="${REPO_ROOT}/components/web-console/Dockerfile"

# The module is copied into the runtime directory before the Vite build reads it.
runtime_copied_before_build() {
  awk '
    /COPY --from=hsctl-wasm .*components\/web-console\/\.hsctl-runtime\// { copied = NR }
    /pnpm run build:web/ { built = NR }
    END { exit (copied && built && copied < built) ? 0 : 1 }
  ' "${DOCKERFILE}"
}

# Verifies: CLI-WASM-05.01, CLI-WASM-05
for suffix in pull-request push merge-queue; do
  f="${REPO_ROOT}/.tekton/hypershell-web-console-main-${suffix}.yaml"
  assert_ok "web-console ${suffix} pipeline runs when components/cli changes" \
    grep -q '"components/cli/\*\*\*".pathChanged()' "${f}"
  assert_ok "web-console ${suffix} pipeline runs when the module build script changes" \
    grep -q '"scripts/build-hsctl-wasm.sh".pathChanged()' "${f}"
  assert_ok "web-console ${suffix} pipeline prefetches the CLI Go modules" \
    grep -q '{"type": "gomod", "path": "components/cli"}' "${f}"
done

assert_ok "Konflux component detection mirrors the CLI trigger" \
  grep -q '\^components/cli/|\^scripts/build-hsctl-wasm' "${REPO_ROOT}/.github/actions/detect-konflux-components/action.yml"
assert_ok "web console image downloads the CLI modules from components/cli/go.mod" \
  grep -q '^COPY components/cli/go.mod components/cli/go.sum components/cli/$' "${DOCKERFILE}"
assert_ok "web console image builds the module with the shared build script" \
  grep -q 'sh scripts/build-hsctl-wasm.sh /workspace/hsctl-runtime' "${DOCKERFILE}"
assert_ok "web console image copies the module in before the Vite build" runtime_copied_before_build
assert_ok "the build context includes the module build script" \
  grep -qx '!scripts/build-hsctl-wasm.sh' "${REPO_ROOT}/.dockerignore"
assert_ok "the build script copies wasm_exec.js from the compiling toolchain" \
  grep -q 'lib/wasm/wasm_exec.js' "${REPO_ROOT}/scripts/build-hsctl-wasm.sh"

# The browser module is not a release asset (CLI-REL-02, CLI-REL-03).
if grep -q 'hsctl-wasm\|GOOS=js\|wasm' "${REPO_ROOT}/components/cli/build-release.sh"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: components/cli/build-release.sh builds the browser module'
else
  PASS=$((PASS + 1))
fi

echo "PASS=${PASS} FAIL=${FAIL}"
if [[ "${FAIL}" -ne 0 ]]; then
  exit 1
fi
