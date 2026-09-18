#!/usr/bin/env bash
# Unit test for scripts/gen-postgres-tls.sh: the generated server certificate
# must chain to the generated CA and carry every requested DNS SAN, because the
# control plane verifies stand-in servers with sslmode=verify-full.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS=0
FAIL=0

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

if bash "${SCRIPT_DIR}/gen-postgres-tls.sh" "${tmp}/out" \
  postgres.external-cloud-db.svc.cluster.local postgres.external-cloud-db.svc postgres; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: gen-postgres-tls.sh exited non-zero'
fi

for f in ca.crt ca.key tls.crt tls.key; do
  if [[ -s "${tmp}/out/${f}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo "FAIL: ${f} missing or empty"
  fi
done
for f in server.csr server.ext ca.srl; do
  if [[ -e "${tmp}/out/${f}" ]]; then
    FAIL=$((FAIL + 1))
    echo "FAIL: scratch file ${f} left behind"
  else
    PASS=$((PASS + 1))
  fi
done

if openssl verify -CAfile "${tmp}/out/ca.crt" "${tmp}/out/tls.crt" >/dev/null 2>&1; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: server certificate does not verify against the generated CA'
fi

san_line="$(openssl x509 -in "${tmp}/out/tls.crt" -noout -text | grep -A1 'Subject Alternative Name' | tail -1)"
for want in 'DNS:postgres.external-cloud-db.svc.cluster.local' 'DNS:postgres.external-cloud-db.svc' 'DNS:postgres'; do
  if [[ "${san_line}" == *"${want}"* ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo "FAIL: SAN ${want} missing from ${san_line}"
  fi
done

if openssl x509 -in "${tmp}/out/ca.crt" -noout -text | grep -A1 'Basic Constraints' | grep -q 'CA:TRUE'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: CA certificate is not marked CA:TRUE'
fi

# The verify-full client also checks the hostname against the SAN; make sure a
# verification against the primary host succeeds and a foreign host fails.
# -verify_hostname is an OpenSSL 1.1.1+/3.x option (Linux, CI); the LibreSSL
# shipped with macOS lacks it, so skip the two hostname checks there.
if openssl verify -help 2>&1 | grep -q -- '-verify_hostname'; then
  if openssl verify -CAfile "${tmp}/out/ca.crt" -verify_hostname postgres.external-cloud-db.svc.cluster.local "${tmp}/out/tls.crt" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo 'FAIL: hostname verification against the primary SAN failed'
  fi
  if openssl verify -CAfile "${tmp}/out/ca.crt" -verify_hostname evil.example.invalid "${tmp}/out/tls.crt" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    echo 'FAIL: hostname verification accepted a host that is not in the SAN'
  else
    PASS=$((PASS + 1))
  fi
else
  echo 'skip: openssl verify has no -verify_hostname (LibreSSL); hostname checks not run'
fi

if bash "${SCRIPT_DIR}/gen-postgres-tls.sh" "${tmp}/missing" >/dev/null 2>&1; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: missing primary host was accepted'
else
  PASS=$((PASS + 1))
fi

echo "gen-postgres-tls_test: ${PASS} passed, ${FAIL} failed"
[[ "${FAIL}" -eq 0 ]]
