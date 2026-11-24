#!/usr/bin/env bash
# vault-tls.sh - helpers for the stand-in Vault serving TLS (sourced by vault.sh).
#
# vault_tls_pair_valid checks that a directory holding ca.crt and tls.crt is a usable
# serving-TLS pair: ca.crt is a PEM certificate and tls.crt chains to it. vault.sh uses
# it before reusing an existing openshell-gateway-vault-tls Secret, so a Secret that a
# test or a killed run left corrupted is regenerated instead of reused.
# $1 = directory. Returns 0 when valid, non-zero otherwise. Prints nothing.
vault_tls_pair_valid() {
  local dir="$1"
  [ -s "${dir}/ca.crt" ] && [ -s "${dir}/tls.crt" ] || return 1
  openssl x509 -in "${dir}/ca.crt" -noout >/dev/null 2>&1 || return 1
  openssl verify -CAfile "${dir}/ca.crt" "${dir}/tls.crt" >/dev/null 2>&1
}
