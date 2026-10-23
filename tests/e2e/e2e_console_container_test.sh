#!/usr/bin/env bash
# Check the container command built by e2e-console-container.sh without a
# cluster or a container engine (E2E_CONSOLE_CONTAINER_DRY_RUN prints it).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAUNCHER="${SCRIPT_DIR}/e2e-console-container.sh"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

assert_contains() {
  local needle="$1" haystack="$2" label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (missing %q)\n' "$label" "$needle"
  fi
}

assert_absent() {
  local needle="$1" haystack="$2" label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (unexpected %q)\n' "$label" "$needle"
  fi
}

# kubectl stub: the Gateway address published by cloud-provider-kind.
mkdir -p "$TMP/bin"
cat >"$TMP/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
echo "10.89.0.3"
EOF
chmod +x "$TMP/bin/kubectl"
echo '{"name": "@playwright/test", "version": "1.61.1"}' >"$TMP/package.json"

run_dry() {
  env -i PATH="$TMP/bin:$PATH" HOME="$HOME" \
    E2E_CONSOLE_CONTAINER_DRY_RUN=true \
    E2E_CONSOLE_PLAYWRIGHT_PACKAGE="$TMP/package.json" \
    "$@" bash "$LAUNCHER" | tr '\n' ' '
}

out="$(run_dry CONTAINER_ENGINE=podman E2E_MODE=short E2E_OIDC_USERNAME=admin CI=true)"
assert_contains "podman run --rm --network kind" "$out" "runs on the Kind network"
assert_contains "mcr.microsoft.com/playwright:v1.61.1-noble" "$out" "image tag matches the workspace Playwright"
assert_contains "--add-host console.hypershell.localhost:10.89.0.3" "$out" "Node side resolves the console host to the gateway"
assert_contains "E2E_BROWSER_HOST_RESOLVER_RULES=MAP *.localhost 10.89.0.3" "$out" "Chromium maps *.localhost to the gateway"
assert_contains "--userns=keep-id" "$out" "rootless podman keeps the caller's uid"
assert_contains "-v $REPO_ROOT:$REPO_ROOT:z" "$out" "repository is mounted at its host path"
assert_contains "-w $REPO_ROOT/components/web-console" "$out" "runs from the web console package"
assert_contains "-e E2E_MODE" "$out" "E2E_* variables are passed through"
assert_contains "-e E2E_OIDC_USERNAME" "$out" "login identity is passed through"
assert_contains "-e CI" "$out" "CI is passed through"
assert_absent "-e E2E_CONSOLE_CONTAINER_DRY_RUN" "$out" "launcher-only variables stay outside"
assert_contains "./node_modules/.bin/playwright test --config playwright.live.config.ts console.live.spec.ts" \
  "$out" "runs the workspace Playwright on the live console spec"

out="$(run_dry CONTAINER_ENGINE=docker E2E_CONSOLE_URL=https://console.example.localhost)"
assert_contains "docker run" "$out" "docker works as the engine"
assert_absent "--userns=keep-id" "$out" "keep-id is podman only"
assert_contains "--add-host console.example.localhost:10.89.0.3" "$out" "an explicit E2E_CONSOLE_URL host is mapped"

cat >"$TMP/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
if run_dry CONTAINER_ENGINE=podman >/dev/null 2>&1; then
  FAIL=$((FAIL + 1))
  echo "FAIL: a missing Gateway address must fail"
else
  PASS=$((PASS + 1))
fi

echo "e2e-console-container tests: ${PASS} passed, ${FAIL} failed"
((FAIL == 0))
