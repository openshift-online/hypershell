#!/usr/bin/env bash
# Unit tests for scripts/ci/swap-openshift-images-by-digest.sh. No cluster
# required; skopeo and oc are stubbed on PATH.
# Run: bash scripts/ci/swap-openshift-images-by-digest_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENSHIFT_NAMESPACE=hypershell-ci-pr-test
# shellcheck source=swap-openshift-images-by-digest.sh
source "${SCRIPT_DIR}/swap-openshift-images-by-digest.sh"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

assert_fails() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected non-zero)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

STUB_BIN="$(mktemp -d)"
cleanup() { rm -rf "${STUB_BIN}"; }
trap cleanup EXIT
PATH="${STUB_BIN}:${PATH}"

# --- already pinned ---
assert_eq 'quay.io/org/img@sha256:abc' \
  "$(resolve_by_digest 'quay.io/org/img@sha256:abc')" \
  'already-pinned digest is unchanged'

# --- skopeo resolves ---
cat > "${STUB_BIN}/skopeo" <<'EOF'
#!/usr/bin/env bash
echo 'sha256:deadbeef'
EOF
chmod +x "${STUB_BIN}/skopeo"
assert_eq 'quay.io/org/img@sha256:deadbeef' \
  "$(resolve_by_digest 'quay.io/org/img:on-pr-abc123')" \
  'on-pr tag pins via skopeo digest'

# --- built image with no digest fails closed ---
cat > "${STUB_BIN}/skopeo" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${STUB_BIN}/skopeo"
# oc image info also missing / failing
cat > "${STUB_BIN}/oc" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${STUB_BIN}/oc"
KUBECTL=oc
assert_fails 'on-pr tag without digest is a hard error' \
  resolve_by_digest 'quay.io/org/img:on-pr-abc123'

# --- baseline may fall back ---
got="$(resolve_by_digest 'quay.io/org/img:latest' 2>/dev/null || true)"
assert_eq 'quay.io/org/img:latest' "${got}" 'baseline tag may fall back when no digest'

# --- oc image info used when skopeo is absent ---
rm -f "${STUB_BIN}/skopeo"
cat > "${STUB_BIN}/oc" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "image" && "$2" == "info" ]]; then
  echo 'sha256:fromoc'
  exit 0
fi
exit 1
EOF
chmod +x "${STUB_BIN}/oc"
assert_eq 'quay.io/org/img@sha256:fromoc' \
  "$(resolve_by_digest 'quay.io/org/img:on-pr-abc123')" \
  'oc image info pins when skopeo is missing'

echo "swap-by-digest tests: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
