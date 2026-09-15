#!/usr/bin/env bash
set -euo pipefail
PHASE=${1:?phase required}
DB_NAME=${2:?database name required}
[[ "$DB_NAME" =~ ^gw_[a-z0-9]+$ ]] || { echo 'Invalid fixture database name' >&2; exit 1; }
if [[ "$PHASE" == provision ]]; then
  GW_NAMESPACE=${3:?gateway namespace required}
  DB_NAMESPACE=${4:-}
  [[ $(kubectl -n "$GW_NAMESPACE" get secret openshell-gateway-db-credentials -o jsonpath='{.data.sslmode}' | base64 -d) == verify-full ]]
  if [[ -n "$DB_NAMESPACE" ]] && kubectl get namespace "$DB_NAMESPACE" >/dev/null 2>&1; then
    echo 'Secret override created a ManagedDatabase namespace' >&2
    exit 1
  fi
  if kubectl -n "$GW_NAMESPACE" get deployment openshell-gateway-db >/dev/null 2>&1; then
    echo 'Secret override created an in-cluster gateway database' >&2
    exit 1
  fi
  COUNT=$(kubectl -n database-secret-test exec deployment/gateway-database -- \
    psql -U postgres -tAc "SELECT count(*) FROM pg_stat_ssl s JOIN pg_stat_activity a USING (pid) WHERE a.datname='$DB_NAME' AND s.ssl")
  [[ "$COUNT" -gt 0 ]] || { echo 'Gateway has no live PostgreSQL TLS connection' >&2; exit 1; }
elif [[ "$PHASE" == delete ]]; then
  for _ in $(seq 1 30); do
    COUNT=$(kubectl -n database-secret-test exec deployment/gateway-database -- \
      psql -U postgres -tAc "SELECT (SELECT count(*) FROM pg_database WHERE datname='$DB_NAME') + (SELECT count(*) FROM pg_roles WHERE rolname='$DB_NAME')")
    [[ "$COUNT" == 0 ]] && exit 0
    sleep 2
  done
  echo 'Gateway database or role remains after deletion' >&2
  exit 1
else
  echo 'Unknown fixture assertion phase' >&2
  exit 1
fi
