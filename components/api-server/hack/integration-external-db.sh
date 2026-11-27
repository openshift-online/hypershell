#!/usr/bin/env bash
# Container-free integration tests for the api-server (agent-runtime-images.spec.md,
# ARI-06). Starts a throwaway PostgreSQL with initdb/pg_ctl in a temporary
# directory, runs the integration suite with DB_FACTORY_MODE=external against the
# committed secrets/db.* settings, and stops and removes the database on every exit
# (success, failure, or interrupt). Intended for the agent-runtime sandbox, which
# has the PostgreSQL server binaries but no container engine for testcontainers.
set -euo pipefail

cd "$(dirname "$0")/.."
COMPONENT_DIR="$(pwd)"
SECRETS_DIR="${COMPONENT_DIR}/secrets"

read_secret() {
  local file="${SECRETS_DIR}/$1" fallback="$2"
  if [ -f "${file}" ]; then
    tr -d '\n' <"${file}"
  else
    printf '%s' "${fallback}"
  fi
}

DB_HOST="$(read_secret db.host 127.0.0.1)"
DB_PORT="$(read_secret db.port 5432)"
DB_USER="$(read_secret db.user postgres)"
DB_PASS="$(read_secret db.password postgres)"
DB_NAME="$(read_secret db.name hypershell)"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/hypershell-external-db.XXXXXX")"
DATADIR="${WORKDIR}/data"
SOCKETDIR="${WORKDIR}/sock"
PWFILE="${WORKDIR}/pwfile"
mkdir -p "${DATADIR}" "${SOCKETDIR}"

cleanup() {
  local code=$?
  trap - EXIT INT TERM
  if [ -f "${DATADIR}/postmaster.pid" ]; then
    pg_ctl -D "${DATADIR}" -m immediate stop >/dev/null 2>&1 || true
  fi
  rm -rf "${WORKDIR}"
  exit "${code}"
}
trap cleanup EXIT INT TERM

printf '%s' "${DB_PASS}" >"${PWFILE}"
chmod 600 "${PWFILE}"

echo ">> initializing throwaway PostgreSQL cluster in ${DATADIR}"
initdb -D "${DATADIR}" -U "${DB_USER}" --auth=scram-sha-256 \
  --encoding=UTF8 --pwfile="${PWFILE}" >/dev/null

echo ">> starting PostgreSQL on ${DB_HOST}:${DB_PORT}"
# PostgreSQL resolves hostnames in listen_addresses at startup, so binding the
# configured DB_HOST (e.g. localhost) covers every loopback address the client
# dials (both 127.0.0.1 and ::1).
pg_ctl -D "${DATADIR}" -w -l "${WORKDIR}/postgres.log" \
  -o "-c listen_addresses='${DB_HOST}' -p ${DB_PORT} -k ${SOCKETDIR}" start

export PGPASSWORD="${DB_PASS}"
echo ">> creating database ${DB_NAME}"
createdb -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" "${DB_NAME}"

echo ">> running integration tests with DB_FACTORY_MODE=external"
API_ENV=integration_testing DB_FACTORY_MODE=external \
  go test -p 1 -v ./plugins/... -count=1
