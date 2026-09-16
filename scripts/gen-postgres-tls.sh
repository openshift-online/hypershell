#!/usr/bin/env bash
# gen-postgres-tls.sh - mint a throwaway CA and a PostgreSQL server certificate
# for the dev/e2e stand-in servers (Kind's external-cloud-db postgres and the
# OpenShift bundled hypershell-postgres Deployment).
#
# The control plane connects to every gateway database server with
# sslmode=verify-full, so a stand-in server must present a certificate whose
# SAN matches the host the controller dials, signed by a CA the controller can
# be handed as `sslrootcert`. This script produces exactly that:
#
#   <out-dir>/ca.crt   CA certificate (PEM)     -> admin Secret key sslrootcert
#   <out-dir>/ca.key   CA private key            -> discard after signing
#   <out-dir>/tls.crt  server certificate (PEM)  -> ssl_cert_file
#   <out-dir>/tls.key  server private key (PEM)  -> ssl_key_file
#
# Usage:
#   scripts/gen-postgres-tls.sh <out-dir> <primary-host> [extra-dns-san ...]
#
# The primary host is the certificate CN and first DNS SAN; extra SANs are
# appended verbatim (short Service names, for example). Only openssl is
# required; the flags used work on both OpenSSL 1.1.1+/3.x (CI, Linux) and the
# LibreSSL shipped with macOS.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <out-dir> <primary-host> [extra-dns-san ...]" >&2
  exit 2
fi

out_dir="$1"
primary_host="$2"
shift 2

if [[ -z "${out_dir}" || -z "${primary_host}" ]]; then
  echo "gen-postgres-tls: out-dir and primary-host must be non-empty" >&2
  exit 2
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "gen-postgres-tls: openssl is required" >&2
  exit 1
fi

mkdir -p "${out_dir}"

san="DNS:${primary_host}"
for extra in "$@"; do
  [[ -n "${extra}" ]] || continue
  san="${san},DNS:${extra}"
done

# CA: 10 years is plenty for a throwaway dev environment and avoids surprise
# expiry on long-lived developer clusters.
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -sha256 \
  -subj "/CN=hypershell-dev-postgres-ca" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -keyout "${out_dir}/ca.key" -out "${out_dir}/ca.crt" >/dev/null 2>&1

openssl req -new -newkey rsa:2048 -nodes -sha256 \
  -subj "/CN=${primary_host}" \
  -keyout "${out_dir}/tls.key" -out "${out_dir}/server.csr" >/dev/null 2>&1

printf 'subjectAltName=%s\nextendedKeyUsage=serverAuth\nbasicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\n' \
  "${san}" > "${out_dir}/server.ext"

openssl x509 -req -days 3650 -sha256 \
  -in "${out_dir}/server.csr" \
  -CA "${out_dir}/ca.crt" -CAkey "${out_dir}/ca.key" -CAcreateserial \
  -extfile "${out_dir}/server.ext" \
  -out "${out_dir}/tls.crt" >/dev/null 2>&1

rm -f "${out_dir}/server.csr" "${out_dir}/server.ext" "${out_dir}/ca.srl"

# Fail loudly if the chain does not verify; a silent mismatch would surface
# much later as an opaque "certificate verify failed" from the controller.
openssl verify -CAfile "${out_dir}/ca.crt" "${out_dir}/tls.crt" >/dev/null
