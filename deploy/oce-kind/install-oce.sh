#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Set OCE_REPO to the matching OCE source checkout}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE to the private installation directory}"
: "${GATEWAY_NAMESPACE:?Set GATEWAY_NAMESPACE to the HyperShell-created gateway namespace}"
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
[[ $(helm version --short) == v3.* ]] || { echo "OCE install requires Helm 3" >&2; exit 1; }
k=(kubectl --kubeconfig "$KUBECONFIG" --context kind-hypershell-oce)
[[ $("${k[@]}" config current-context) == kind-hypershell-oce ]]
[[ $GATEWAY_NAMESPACE =~ ^openshell-[a-f0-9]{16}$ ]]
if [[ -e $OCE_KIND_STATE/install-receipt ]]; then
  echo 'Installation already attempted. Inspect the receipt with occ installation doctor before changing it.' >&2
  exit 1
fi
controller=$(jq -er .controller "$OCE_KIND_STATE/images.json")
assets="$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes"
export CLUSTER_DOMAIN=cluster.local
mkdir -p "$OCE_KIND_STATE/secrets"

# Use separate database logins; the application never receives administrator or
# migrator authority. Keep generated passwords out of arguments and terminal output.
if [[ ! -e $OCE_KIND_STATE/database-ready ]]; then
  python3 - <<'PY'
import os, secrets
from pathlib import Path
p = Path(os.environ['OCE_KIND_STATE'])
passwords = {role: secrets.token_hex(32) for role in ['occ_app', 'occ_migrator']}
sql = ''.join(f"CREATE ROLE {role} LOGIN PASSWORD '{password}' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;\n" for role, password in passwords.items())
sql += '''CREATE DATABASE openclaw_oce_kind;
REVOKE CONNECT ON DATABASE openclaw_oce_kind FROM PUBLIC;
GRANT CONNECT ON DATABASE openclaw_oce_kind TO occ_app, occ_migrator;
GRANT CREATE ON DATABASE openclaw_oce_kind TO occ_migrator;
\\connect openclaw_oce_kind
CREATE SCHEMA occ AUTHORIZATION occ_migrator;
CREATE SCHEMA drizzle AUTHORIZATION occ_migrator;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
'''
(p/'secrets/database.sql').write_text(sql)
for name, role in [('application-url','occ_app'),('migration-url','occ_migrator')]:
    (p/'secrets'/name).write_text(f"postgresql://{role}:{passwords[role]}@postgres.external-cloud-db.svc.cluster.local:5432/openclaw_oce_kind?sslmode=verify-full&sslrootcert=/etc/openclaw/database-ca/ca.pem")
for name in ['auth-secret', 'routing-key']:
    (p/'secrets'/name).write_text(secrets.token_hex(32))
PY
  "${k[@]}" -n external-cloud-db exec -i deployment/postgres -- \
    psql -U postgres -v ON_ERROR_STOP=1 -q < "$OCE_KIND_STATE/secrets/database.sql" \
    > "$OCE_KIND_STATE/secrets/database-setup.log" 2>&1
  rm "$OCE_KIND_STATE/secrets/database.sql"
  touch "$OCE_KIND_STATE/database-ready"
fi
"${k[@]}" -n external-cloud-db get secret postgres-tls -o json |
  jq -r '.data["ca.crt"]' | base64 -d > "$OCE_KIND_STATE/secrets/database-ca.pem"

# Issue a separate client leaf using the gateway's existing CA. OCE receives no CA private key.
"${k[@]}" -n "$GATEWAY_NAMESPACE" apply -f - <<'YAML'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: oce-controller-client
spec:
  secretName: oce-controller-client
  commonName: oce-controller
  subject:
    organizationalUnits: [openshell-admin]
  duration: 720h
  renewBefore: 168h
  privateKey:
    algorithm: ECDSA
    size: 256
  usages: [client auth, digital signature]
  issuerRef:
    name: openshell-gateway-ca-issuer
    kind: Issuer
YAML
"${k[@]}" -n "$GATEWAY_NAMESPACE" wait certificate/oce-controller-client --for=condition=Ready --timeout=2m
"${k[@]}" -n "$GATEWAY_NAMESPACE" get secret oce-controller-client -o json |
  jq '{apiVersion:"v1",kind:"Secret",metadata:{name:"openshell-client-tls",namespace:"oce-system"},type:"Opaque",data:.data}' |
  "${k[@]}" apply -f -
"${k[@]}" -n oce-system create secret generic occ-database \
  --from-file="application-url=$OCE_KIND_STATE/secrets/application-url" \
  --from-file="migration-url=$OCE_KIND_STATE/secrets/migration-url" --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" -n oce-system create secret generic occ-database-ca \
  --from-file="ca.pem=$OCE_KIND_STATE/secrets/database-ca.pem" --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" -n oce-system create secret generic occ-auth \
  --from-file="secret=$OCE_KIND_STATE/secrets/auth-secret" --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" -n oce-system create secret generic occ-private-gateway-key \
  --from-file="occ=$OCE_KIND_STATE/secrets/routing-key" --dry-run=client -o json | "${k[@]}" apply -f -
python3 "$here/render-oce.py"
source "$here/openshell-images.env"
export GATEWAY_SUPERVISOR_IMAGE GATEWAY_SANDBOX_RUNTIME_IMAGE
python3 - "$here/runtime-admission.yaml" <<'PYTHON'
import json, os, sys
from pathlib import Path
state = Path(os.environ['OCE_KIND_STATE'])
text = Path(sys.argv[1]).read_text()
for key, value in {
    '__GATEWAY_NAMESPACE__': os.environ['GATEWAY_NAMESPACE'],
    '__RUNTIME_IMAGE__': json.loads((state/'images.json').read_text())['runtime'],
    '__SUPERVISOR_IMAGE__': os.environ['GATEWAY_SUPERVISOR_IMAGE'],
    '__SANDBOX_IMAGE__': os.environ['GATEWAY_SANDBOX_RUNTIME_IMAGE'],
}.items():
    text = text.replace(key, value)
(state/'runtime-admission.yaml').write_text(text)
PYTHON
"${k[@]}" apply -f "$OCE_KIND_STATE/runtime-admission.yaml"
"${k[@]}" apply -f "$OCE_KIND_STATE/management-policy.json"
"${k[@]}" apply -f "$OCE_KIND_STATE/gateway-ingress.json"
"${k[@]}" -n "$GATEWAY_NAMESPACE" create role oce-gateway-observer \
  --verb=get,list,watch --resource=pods,services,endpoints --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" -n "$GATEWAY_NAMESPACE" create rolebinding oce-gateway-observer --role=oce-gateway-observer \
  --serviceaccount=oce-system:openclaw-enterprise-api --serviceaccount=oce-system:openclaw-enterprise-worker \
  --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" -n oce-system create secret generic occ-installation-startup \
  --from-file="installation.yaml=$OCE_KIND_STATE/installation.json" --dry-run=client -o json | "${k[@]}" apply -f -
"${k[@]}" apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: occ-bootstrap-admin-password
  namespace: oce-system
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: standard
  resources:
    requests:
      storage: 1Gi
YAML
if [[ ! -e $OCE_KIND_STATE/bootstrap-volume-ready ]]; then
  "$OCE_REPO/scripts/prepare-bootstrap-volume" --kubeconfig "$KUBECONFIG" --context kind-hypershell-oce \
    --namespace oce-system --claim occ-bootstrap-admin-password --image "$controller" --node-selector oce-role=control
  touch "$OCE_KIND_STATE/bootstrap-volume-ready"
fi
"$OCE_KIND_STATE/occ" installation install --release oce --chart "$OCE_REPO/deploy/helm/openclaw-enterprise" \
  --kubeconfig "$KUBECONFIG" --context kind-hypershell-oce --kube-namespace oce-system \
  --values "$OCE_KIND_STATE/values.json" --receipt "$OCE_KIND_STATE/install-receipt" --wait-timeout 10m
"${k[@]}" -n oce-system rollout status deployment/openclaw-enterprise-api --timeout=5m

jq -n --arg image "$controller" --arg namespace oce-system --arg claim occ-bootstrap-admin-password \
  --argjson nodeSelector '{"oce-role":"control"}' -f "$assets/bootstrap-reader.jq" | "${k[@]}" create -f -
"${k[@]}" -n oce-system wait pod/oce-owner-bootstrap-reader --for=condition=Ready --timeout=2m
for file in initial-admin-password initial-admin-service-key.json; do
  "${k[@]}" -n oce-system exec oce-owner-bootstrap-reader -- \
    node -e 'process.stdout.write(require("node:fs").readFileSync(process.argv[1]))' \
    "/bootstrap/$file" > "$OCE_KIND_STATE/$file"
done
"${k[@]}" -n oce-system delete pod oce-owner-bootstrap-reader
echo 'OCE initialized. Complete routing, namespace RBAC and browser access as described in README.md.'
