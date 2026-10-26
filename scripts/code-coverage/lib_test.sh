#!/usr/bin/env bash
# Unit tests for scripts/code-coverage/lib.sh helpers. No cluster required.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../kind/lib.sh
source "${SCRIPT_DIR}/../kind/lib.sh"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

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

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

# A real Unix socket standing in for podman's API socket.
PODMAN_SOCKET="${WORKDIR}/podman.sock"
python3 -c 'import socket, sys; socket.socket(socket.AF_UNIX).bind(sys.argv[1])' "${PODMAN_SOCKET}"

# A fake podman that reports that socket for both the Linux and macOS queries.
mkdir -p "${WORKDIR}/bin"
cat >"${WORKDIR}/bin/podman" <<EOF
#!/usr/bin/env bash
echo "${PODMAN_SOCKET}"
EOF
chmod +x "${WORKDIR}/bin/podman"
PATH="${WORKDIR}/bin:${PATH}"

# Shadow uname so the test controls the Linux / macOS branch.
FAKE_OS=Linux
uname() { echo "${FAKE_OS}"; }

# Testcontainers reaches a podman-only engine through podman's socket.
# Verifies: DEV-LOC-18.07
unset DOCKER_HOST
COVERAGE_DOCKER_SOCKET="${WORKDIR}/missing-docker.sock"
assert_eq "DOCKER_HOST=unix://${PODMAN_SOCKET} TESTCONTAINERS_RYUK_DISABLED=true" \
  "$(testcontainers_env)" "Linux podman-only host uses podman's socket"

FAKE_OS=Darwin
assert_eq "DOCKER_HOST=unix://${PODMAN_SOCKET} TESTCONTAINERS_RYUK_DISABLED=true" \
  "$(testcontainers_env)" "macOS podman machine uses its host socket"
FAKE_OS=Linux

COVERAGE_DOCKER_SOCKET="${PODMAN_SOCKET}"
assert_eq "" "$(testcontainers_env)" "an existing Docker socket needs nothing"
COVERAGE_DOCKER_SOCKET="${WORKDIR}/missing-docker.sock"

DOCKER_HOST="unix:///somewhere/else.sock"
assert_eq "" "$(testcontainers_env)" "an explicit DOCKER_HOST is left alone"
unset DOCKER_HOST

rm -f "${PODMAN_SOCKET}"
assert_eq "" "$(testcontainers_env)" "no podman socket means no override"

# The Go suite reaches the Kind ingress on the proxy's published port.
# Verifies: DEV-LOC-18.09
cat >"${WORKDIR}/bin/fake-engine" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  ps) printf '%s\n' hypershell-dev-control-plane "${FAKE_PROXY:-}" ;;
  port) [[ "$2" == kindccm-gw-* && "$3" == 443 ]] && echo "0.0.0.0:35955" ;;
esac
EOF
chmod +x "${WORKDIR}/bin/fake-engine"
CONTAINER_ENGINE="${WORKDIR}/bin/fake-engine"
export FAKE_PROXY=kindccm-gw-4087acfa9d43
assert_eq "127.0.0.1:35955" "$(kind_ingress_addr)" "published 443 port of the kindccm proxy"
export FAKE_PROXY=""
assert_eq "" "$(kind_ingress_addr)" "no proxy container means no address"

echo "Coverage lib tests: ${PASS} passed, ${FAIL} failed"
if ((FAIL > 0)); then
  exit 1
fi
