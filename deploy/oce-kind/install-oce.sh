#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Set OCE_REPO to the matching OCE source checkout}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE to the private installation directory}"
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
[[ $(helm version --short) == v3.* ]] || { echo "OCE install requires Helm 3" >&2; exit 1; }
k=(kubectl --kubeconfig "$KUBECONFIG" --context kind-hypershell-oce)
[[ $("${k[@]}" config current-context) == kind-hypershell-oce ]]
# Serialize installation attempts that share credentials and checkpoints.
exec 9>"$OCE_KIND_STATE/install.lock"
flock -n 9 || { echo 'Another OCE installer is using this state directory.' >&2; exit 1; }

print_state_recovery() {
  printf '\nSelected OCE_KIND_STATE: %s\nSelected kubeconfig: %s\n' "$OCE_KIND_STATE" "$KUBECONFIG" >&2
  cat >&2 <<'HELP'

A target mismatch means the selected cluster or connection does not match the
saved installation state. This can happen after recreating kind with the same name.
The installer always reads kubeconfig from OCE_KIND_STATE; changing KUBECONFIG
alone will not switch it to another installation.

If you already prepared a state directory for the current cluster, replace the
path below with that directory and paste these commands into your current terminal:

  set +e +u
  export OCE_KIND_STATE=/absolute/path/to/current-cluster-state
  export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
  export PATH="$OCE_KIND_STATE/bin:$PATH"
  export HELM_PLUGINS="$OCE_KIND_STATE/helm-plugins"
  unset GATEWAY_NAMESPACE

Then rerun the complete guarded "Install OCE" block in deploy/oce-kind/manual-setup.md.
The installer cannot change these exports in your parent terminal for you.

If the cluster was recreated and no matching state directory exists yet, follow
the numbered steps in deploy/oce-kind/manual-setup.md under
"State left over from a deleted cluster" to prepare one first.
Keep the old directory in place: kind may still mount its pod-security.yaml.
Do not delete its receipt or copy its credentials into the replacement state.

If you intended to resume the original cluster, restore its matching kubeconfig
and state directory. For other doctor errors, fix the reported problem before
retrying; a failed check does not mean it is safe to start a new installation.
HELP
}

# A reused kind name does not identify the same cluster after recreation.
cluster_uid=$("${k[@]}" get namespace kube-system -o jsonpath='{.metadata.uid}')
[[ -n $cluster_uid ]]
if [[ -e $OCE_KIND_STATE/cluster-uid ]]; then
  if [[ $(cat "$OCE_KIND_STATE/cluster-uid") != "$cluster_uid" ]]; then
    echo 'This state directory belongs to another cluster. Installation stopped.' >&2
    print_state_recovery
    exit 1
  fi
elif [[ -e $OCE_KIND_STATE/install-receipt ]]; then
  # Older state has no cluster checkpoint; let OCE validate its original receipt.
  if ! "$OCE_KIND_STATE/occ" installation doctor --kubeconfig "$KUBECONFIG" \
    --context kind-hypershell-oce --kube-namespace oce-system --release oce \
    --receipt "$OCE_KIND_STATE/install-receipt"; then
    echo 'The saved installation receipt could not be verified. Installation stopped.' >&2
    print_state_recovery
    exit 1
  fi
elif [[ -e $OCE_KIND_STATE/database-ready || -d $OCE_KIND_STATE/secrets ]]; then
  echo 'Existing database state has no cluster checkpoint. Inspect it before retrying; do not regenerate credentials.' >&2
  exit 1
fi
printf '%s\n' "$cluster_uid" > "$OCE_KIND_STATE/cluster-uid"

controller=$(jq -er .controller "$OCE_KIND_STATE/images.json")
assets="$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes"
if [[ -e $OCE_KIND_STATE/install-receipt ]]; then
  # Activation upgrades replace the initialization Job, so check the current release.
  status=$(helm --kubeconfig "$KUBECONFIG" --kube-context kind-hypershell-oce \
    -n oce-system status oce -o json | jq -er '.info.status')
  if [[ $status != deployed ]]; then
    echo "OCE release is $status. Inspect the receipt and initialization Job; initialization will not be replayed." >&2
    exit 1
  fi
  for file in installation.json values.json gateway-namespace; do
    [[ -s $OCE_KIND_STATE/$file ]] || { echo "Missing saved $file; recover the original installation state." >&2; exit 1; }
  done
  echo 'OCE is already installed. Keeping its configuration and resuming access setup.'
else
  # A lost receipt must not turn an existing release into a fresh installation.
  releases=$(helm --kubeconfig "$KUBECONFIG" --kube-context kind-hypershell-oce \
    -n oce-system list --all --filter '^oce$' -o json)
  if ! jq -e 'type == "array" and length == 0' <<< "$releases" >/dev/null; then
    echo 'OCE has a Helm release but no saved receipt. Recover its original state directory before retrying.' >&2
    exit 1
  fi
  # Resolve the live gateway and validate/render all inputs before provisioning resources.
  python3 "$here/render-oce.py"
  read -r GATEWAY_NAMESPACE < "$OCE_KIND_STATE/gateway-namespace"
  export GATEWAY_NAMESPACE
  export CLUSTER_DOMAIN=cluster.local
  mkdir -p "$OCE_KIND_STATE/secrets"

  # Use separate database logins; the application never receives administrator or
  # migrator authority. Keep generated passwords out of arguments and terminal output.
  if [[ ! -e $OCE_KIND_STATE/database-ready ]]; then
    if [[ -e $OCE_KIND_STATE/secrets/application-url || -e $OCE_KIND_STATE/secrets/database.sql ]]; then
      echo 'Database setup was interrupted. Inspect secrets/database-setup.log before retrying; existing credentials were preserved.' >&2
      exit 1
    fi
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
fi
"${k[@]}" -n oce-system rollout status deployment/openclaw-enterprise-api --timeout=5m

# Recover only missing files, and never truncate a saved credential on a failed read.
if [[ ! -s $OCE_KIND_STATE/initial-admin-password || ! -s $OCE_KIND_STATE/initial-admin-service-key.json ]]; then
  # A reader left by an interrupted attempt is disposable; the PVC is read-only.
  "${k[@]}" -n oce-system delete pod oce-owner-bootstrap-reader --ignore-not-found
  jq -n --arg image "$controller" --arg namespace oce-system --arg claim occ-bootstrap-admin-password \
    --argjson nodeSelector '{"oce-role":"control"}' -f "$assets/bootstrap-reader.jq" | "${k[@]}" create -f -
  "${k[@]}" -n oce-system wait pod/oce-owner-bootstrap-reader --for=condition=Ready --timeout=2m
  for file in initial-admin-password initial-admin-service-key.json; do
    [[ ! -s $OCE_KIND_STATE/$file ]] || continue
    "${k[@]}" -n oce-system exec oce-owner-bootstrap-reader -- \
      node -e 'process.stdout.write(require("node:fs").readFileSync(process.argv[1]))' \
      "/bootstrap/$file" > "$OCE_KIND_STATE/$file.tmp"
    test -s "$OCE_KIND_STATE/$file.tmp"
    mv "$OCE_KIND_STATE/$file.tmp" "$OCE_KIND_STATE/$file"
  done
fi
"${k[@]}" -n oce-system delete pod oce-owner-bootstrap-reader --ignore-not-found
echo 'OCE initialized. Complete routing, namespace RBAC and browser access as described in manual-setup.md.'
