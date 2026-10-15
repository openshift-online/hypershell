#!/usr/bin/env bash
# vault.sh - stand up a stand-in HashiCorp Vault for the dev/e2e environment (Kind or
# OpenShift), configured to the HYPERSHELL-369 GitOps Contract
# (openshell-gateway-credentials.spec.md) so vault-backed gateways - now the default -
# work end-to-end in CI/dev.
#
# It mirrors what hypershell-gitops deploys per cluster (same HashiCorp Vault image, KV v2
# at `secret`, kubernetes auth, the `openshell-gateway` role and the namespace-templated
# policy, a serving CA in Secret openshell-gateway-vault-tls), MINUS the production
# unseal/storage plumbing (Secrets Manager / ESO / KMS / managed Postgres): a throwaway
# Vault uses file storage + a scripted init+unseal. Not for production.
#
# Driver-agnostic and idempotent (mirrors the production init Job): the caller sets
# VAULT_KUBECTL (the kube CLI incl. context) and CLUSTER_DRIVER (kind|openshift). Re-running
# against an already-initialized Vault is a no-op, so it is safe on a shared cluster.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Self-contained logging (no dependency on a driver-specific lib).
if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  BOLD='\033[1m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
else
  BOLD=''; BLUE=''; CYAN=''; GREEN=''; RED=''; NC=''
fi
header()  { printf "${BOLD}${BLUE}==> %s${NC}\n" "$*"; }
info()    { printf "${CYAN}    %s${NC}\n" "$*"; }
success() { printf "${GREEN}    %s${NC}\n" "$*"; }
error()   { printf "${RED}ERROR: %s${NC}\n" "$*" >&2; }

# Kube CLI: the caller supplies the binary + context (kind passes
# "kubectl --context kind-<name>"; openshift passes "oc"). Word-split on purpose.
VAULT_KUBECTL="${VAULT_KUBECTL:-kubectl}"
# shellcheck disable=SC2086
vkube() { ${VAULT_KUBECTL} "$@"; }

VDRIVER="${CLUSTER_DRIVER:-kind}"

VAULT_NS="openshell-vault"
VAULT_SVC="openshell-gateway-vault"
VAULT_HOST="${VAULT_SVC}.${VAULT_NS}.svc"
VAULT_TLS_SECRET="openshell-gateway-vault-tls"
VAULT_UNSEAL_SECRET="openshell-gateway-vault-unseal"
# HashiCorp Vault - SAME software/version the per-cluster production Vault runs
# (hypershell-gitops helm chart vault 0.30.0 -> hashicorp/vault:1.19.0).
VAULT_IMAGE="${VAULT_IMAGE:-docker.io/hashicorp/vault:1.19.0}"
VAULT_ROLE="openshell-gateway"

# Pod security: Kind needs an explicit non-root UID + fsGroup so the file-storage emptyDir
# is writable; OpenShift's restricted SCC assigns both from the namespace range and rejects
# a hardcoded UID, so omit them there.
if [ "${VDRIVER}" = "openshift" ]; then
  EXTRA_POD_SEC=""
else
  EXTRA_POD_SEC=$'\n        runAsUser: 100\n        fsGroup: 1000'
fi

header "Stand-in HashiCorp Vault (gateway provider secrets)"
vkube create namespace "${VAULT_NS}" --dry-run=client -o yaml | vkube apply -f -

# Serving TLS: a throwaway CA + server cert whose SAN matches the convention host.
# The control plane reads this CA from the openshell-gateway-vault-tls Secret and
# propagates it into each gateway namespace (gen-postgres-tls.sh mints generic TLS).
if vkube get secret "${VAULT_TLS_SECRET}" -n "${VAULT_NS}" >/dev/null 2>&1; then
  success "TLS Secret '${VAULT_TLS_SECRET}' already exists - reusing its CA"
else
  info "Generating throwaway CA and server certificate for ${VAULT_HOST}..."
  _tls_dir="$(mktemp -d)"
  bash "${REPO_ROOT}/scripts/gen-postgres-tls.sh" "${_tls_dir}" \
    "${VAULT_HOST}" "${VAULT_HOST}.cluster.local" "vault"
  vkube create secret generic "${VAULT_TLS_SECRET}" -n "${VAULT_NS}" \
    --from-file=tls.crt="${_tls_dir}/tls.crt" \
    --from-file=tls.key="${_tls_dir}/tls.key" \
    --from-file=ca.crt="${_tls_dir}/ca.crt" \
    --dry-run=client -o yaml | vkube apply -f -
  rm -rf "${_tls_dir}"
  success "TLS Secret '${VAULT_TLS_SECRET}' created"
fi

info "Deploying Vault (file storage + TLS) in namespace '${VAULT_NS}'..."
vkube apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault
  namespace: ${VAULT_NS}
---
# Vault's kubernetes auth method reviews gateway SA tokens via the TokenReview API.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: openshell-vault-tokenreview
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:auth-delegator
subjects:
  - kind: ServiceAccount
    name: vault
    namespace: ${VAULT_NS}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: vault-config
  namespace: ${VAULT_NS}
data:
  vault.hcl: |
    storage "file" { path = "/vault/data" }
    listener "tcp" {
      address       = "0.0.0.0:8200"
      tls_cert_file = "/vault/tls/tls.crt"
      tls_key_file  = "/vault/tls/tls.key"
    }
    disable_mlock = true
    api_addr      = "https://${VAULT_HOST}:8200"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${VAULT_SVC}
  namespace: ${VAULT_NS}
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: ${VAULT_SVC}
  template:
    metadata:
      labels:
        app: ${VAULT_SVC}
    spec:
      serviceAccountName: vault
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault${EXTRA_POD_SEC}
      containers:
        - name: vault
          image: ${VAULT_IMAGE}
          command: ["vault", "server", "-config=/vault/config/vault.hcl"]
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
          ports:
            - containerPort: 8200
              name: https
          volumeMounts:
            - name: config
              mountPath: /vault/config
            - name: tls
              mountPath: /vault/tls
              readOnly: true
            - name: data
              mountPath: /vault/data
          readinessProbe:
            httpGet:
              path: "/v1/sys/health?standbyok=true"
              port: 8200
              scheme: HTTPS
            periodSeconds: 5
      volumes:
        - name: config
          configMap:
            name: vault-config
        - name: tls
          secret:
            secretName: ${VAULT_TLS_SECRET}
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: ${VAULT_SVC}
  namespace: ${VAULT_NS}
spec:
  selector:
    app: ${VAULT_SVC}
  ports:
    - name: https
      port: 8200
      targetPort: 8200
EOF

# Wait for the pod to be Running (a sealed/uninitialized Vault never reports Ready,
# and this script is what initializes+unseals it - mirror the gitops init Job).
info "Waiting for the Vault pod to be Running..."
for _ in $(seq 1 60); do
  VPOD="$(vkube -n "${VAULT_NS}" get pod -l app=${VAULT_SVC} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  [ -n "${VPOD}" ] && [ "$(vkube -n "${VAULT_NS}" get pod "${VPOD}" -o jsonpath='{.status.phase}' 2>/dev/null)" = "Running" ] && break
  sleep 5
done
[ -n "${VPOD:-}" ] || { error "Vault pod did not start"; exit 1; }

vx()  { vkube -n "${VAULT_NS}" exec "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true "$@"; }
vxt() { vkube -n "${VAULT_NS}" exec "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true VAULT_TOKEN="$1" "${@:2}"; }
# Stdin variant (exec -i), for commands that read from stdin like `vault policy write - `.
vxti() { vkube -n "${VAULT_NS}" exec -i "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true VAULT_TOKEN="$1" "${@:2}"; }

info "Waiting for the Vault API..."
until vx vault status >/dev/null 2>&1 || [ "$?" -eq 2 ]; do sleep 3; done

if vx vault status 2>/dev/null | grep -q "Initialized.*true"; then
  success "Vault already initialized - skipping init/config (idempotent)"
  exit 0
fi

info "Initializing + unsealing Vault (Shamir 1/1)..."
INIT_JSON="$(vx vault operator init -key-shares=1 -key-threshold=1 -format=json)"
UNSEAL_KEY="$(printf '%s' "${INIT_JSON}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["unseal_keys_b64"][0])')"
ROOT_TOKEN="$(printf '%s' "${INIT_JSON}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["root_token"])')"
# Stash the unseal key so an unattended pod restart during a run can be re-unsealed
# by hand if needed (dev/e2e convenience; not a production unseal mechanism).
vkube create secret generic "${VAULT_UNSEAL_SECRET}" -n "${VAULT_NS}" \
  --from-literal=unseal-key="${UNSEAL_KEY}" --dry-run=client -o yaml | vkube apply -f - >/dev/null
vx vault operator unseal "${UNSEAL_KEY}" >/dev/null

info "Configuring KV v2, kubernetes auth, role and templated policy..."
vxt "${ROOT_TOKEN}" vault secrets enable -path=secret -version=2 kv >/dev/null 2>&1 || true
vxt "${ROOT_TOKEN}" vault auth enable kubernetes >/dev/null 2>&1 || true
vxt "${ROOT_TOKEN}" sh -c 'vault write auth/kubernetes/config \
  kubernetes_host="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}" \
  kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
  token_reviewer_jwt=@/var/run/secrets/kubernetes.io/serviceaccount/token'

# Namespace-templated policy: each gateway reaches only its own namespace path. Grant
# list on the namespace parent too, so the control plane can enumerate it for
# purge-on-delete (openshell-gateway-credentials.spec.md GitOps Contract).
ACCESSOR="$(vxt "${ROOT_TOKEN}" vault auth list | awk '$1=="kubernetes/"{print $3}')"
[ -n "${ACCESSOR}" ] || { error "could not resolve kubernetes auth accessor"; exit 1; }
printf '%s\n' \
  "path \"secret/data/openshell/{{identity.entity.aliases.${ACCESSOR}.metadata.service_account_namespace}}/*\" {" \
  "  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]" \
  "}" \
  "path \"secret/metadata/openshell/{{identity.entity.aliases.${ACCESSOR}.metadata.service_account_namespace}}/*\" {" \
  "  capabilities = [\"read\", \"list\", \"delete\"]" \
  "}" \
  "path \"secret/metadata/openshell/{{identity.entity.aliases.${ACCESSOR}.metadata.service_account_namespace}}\" {" \
  "  capabilities = [\"list\"]" \
  "}" | vxti "${ROOT_TOKEN}" vault policy write openshell-gateway-provider -

vxt "${ROOT_TOKEN}" vault write "auth/kubernetes/role/${VAULT_ROLE}" \
  bound_service_account_names="openshell-gateway" \
  bound_service_account_namespaces='*' \
  audience=vault \
  token_policies="openshell-gateway-provider" \
  token_ttl=3600 >/dev/null

# Drop root; gateways and the control plane only ever authenticate via kubernetes auth.
vxt "${ROOT_TOKEN}" vault token revoke -self >/dev/null 2>&1 || true

success "Vault ready at https://${VAULT_HOST}:8200 (KV v2 'secret', role '${VAULT_ROLE}')"
