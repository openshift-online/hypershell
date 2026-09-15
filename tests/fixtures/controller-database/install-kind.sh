#!/usr/bin/env bash
# Install a disposable TLS server and synchronize its admin Secret through ESO.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
NS=${HYPERSHELL_NAMESPACE:-hypershell-system}
DIR=$(mktemp -d)
trap 'rm -rf "$DIR"' EXIT
bash "$ROOT/tests/fixtures/controller-database/generate.sh" "$DIR"
kubectl create namespace database-secret-test --dry-run=client -o yaml | kubectl apply -f -
kubectl -n database-secret-test create secret generic database-fixture \
  --from-file="$DIR/server.crt" --from-file="$DIR/server.key" --from-file="$DIR/init.sql" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "$ROOT/tests/fixtures/controller-database/postgres.yaml"
kubectl -n database-secret-test rollout status deployment/gateway-database --timeout=180s
helm upgrade --install database-test-external-secrets external-secrets \
  --repo https://charts.external-secrets.io --version 0.20.4 \
  --namespace external-secrets --create-namespace --wait --timeout 180s >/dev/null
# The test uses ESO's Fake provider. Only disposable fixture values enter it.
jq -n --rawfile ca "$DIR/ca.crt" '{apiVersion:"external-secrets.io/v1",kind:"ClusterSecretStore",
  metadata:{name:"gateway-databases"},spec:{provider:{fake:{data:[{
    key:"hypershell/production/gateway-database",value:({host:"gateway-database.database-secret-test.svc",port:"5432",
      user:"provisioner",password:"fixture-admin",sslrootcert:$ca}|tojson)}]}}}}' | kubectl apply -f -
kubectl wait --for=condition=Ready clustersecretstore/gateway-databases --timeout=60s
sed "s/namespace: hypershell-system/namespace: ${NS}/" \
  "$ROOT/deploy/components/gateway-database-admin-secret/external-secret.yaml" | kubectl apply -f -
kubectl -n "$NS" wait --for=condition=Ready externalsecret/gateway-database-admin --timeout=90s
# Verify that a secret-manager update reaches the same Kubernetes Secret.
kubectl -n database-secret-test exec deployment/gateway-database -- \
  psql -U postgres -v ON_ERROR_STOP=1 -c "ALTER ROLE provisioner PASSWORD 'fixture-rotated'" >/dev/null
kubectl get clustersecretstore gateway-databases -o json | \
  jq '.spec.provider.fake.data[0].value |= (fromjson | .password="fixture-rotated" | tojson)' | kubectl apply -f -
kubectl -n "$NS" annotate externalsecret gateway-database-admin force-sync="$(date +%s)" --overwrite >/dev/null
SYNCED=false
for _ in $(seq 1 45); do
  VALUE=$(kubectl -n "$NS" get secret gateway-database-admin -o jsonpath='{.data.password}' | base64 -d)
  if [[ "$VALUE" == fixture-rotated ]]; then SYNCED=true; break; fi
  sleep 2
done
[[ "$SYNCED" == true ]] || { echo 'ESO did not synchronize the new password' >&2; exit 1; }
kubectl -n "$NS" set env deployment/hypershell-controller GATEWAY_DATABASE_ADMIN_SECRET_NAME=gateway-database-admin
kubectl -n "$NS" rollout status deployment/hypershell-controller --timeout=180s
