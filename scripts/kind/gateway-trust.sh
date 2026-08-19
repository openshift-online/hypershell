#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

# The Kind cluster serves every *.hypershell.localhost host (Keycloak, API,
# gateways) with a leaf certificate signed by cert-manager's self-signed
# "hypershell-ca". The openshell CLI validates the OIDC issuer over HTTPS, so it
# must trust that CA. Being rustls-based, it honors SSL_CERT_FILE -- the same
# mechanism the e2e suite uses (see tests/e2e/e2e-openshell.sh). Extract the CA
# and print the export that points the CLI at it.
#
# Note: SSL_CERT_FILE *replaces* the system trust store rather than adding to it,
# so while it is set this CLI trusts only hypershell-ca. That is exactly right
# for local dev against *.hypershell.localhost; unset it before pointing the CLI
# at a public endpoint.
#
# All diagnostics go to stderr so the sole stdout line is the export command,
# making `eval "$(make kind-gateway-trust)"` set SSL_CERT_FILE in your shell.

CA_SECRET="hypershell-ca-secret"
CA_FILE="${REPO_ROOT}/bin/hypershell-ca.crt"

header "Gateway Trust CA" >&2

CA_PEM="$(kube get secret "${CA_SECRET}" -n "${KIND_NAMESPACE}" \
  -o go-template='{{index .data "ca.crt" | base64decode}}' 2>/dev/null || true)"
if [[ -z "${CA_PEM}" ]]; then
  error "Secret ${CA_SECRET} has no ca.crt in namespace ${KIND_NAMESPACE}."
  error "Is the cluster fully up? Run 'make kind-up' and wait for cert-manager."
  exit 1
fi

mkdir -p "${REPO_ROOT}/bin"
printf '%s\n' "${CA_PEM}" > "${CA_FILE}"

info "Extracted self-signed CA (CN=hypershell-ca) -> ${CA_FILE}" >&2
info "Point the openshell CLI at it so its OIDC/gateway TLS validates:" >&2
echo "" >&2
info "  eval \"\$(make kind-gateway-trust)\"     # set it in the current shell" >&2
info "  # ...or copy the line printed below into your shell." >&2
echo "" >&2

# Sole stdout line: the export the caller can eval or copy.
echo "export SSL_CERT_FILE=${CA_FILE}"
