#!/usr/bin/env bash
# vault.sh - stand up a stand-in HashiCorp Vault for the dev/e2e environment (Kind or
# OpenShift), configured to the HYPERSHELL-369 GitOps Contract
# (openshell-gateway-credentials.spec.md) so vault-backed gateways - now the default -
# work end-to-end in CI/dev.
#
# It mirrors what hypershell-gitops deploys per cluster (same HashiCorp Vault image, KV v2
# at `secret`, kubernetes auth, the `openshell-gateway` role and the capability-scoped
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
VPOD=""; VPOD_READY=
for _ in $(seq 1 60); do
  VPOD="$(vkube -n "${VAULT_NS}" get pod -l app=${VAULT_SVC} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -n "${VPOD}" ] && [ "$(vkube -n "${VAULT_NS}" get pod "${VPOD}" -o jsonpath='{.status.phase}' 2>/dev/null)" = "Running" ]; then
    VPOD_READY=1; break
  fi
  sleep 5
done
# Check the pod actually reached Running, not just that its name exists: the name is set
# as soon as the pod object is created (Pending/ContainerCreating), so a plain name check
# would let bring-up continue with a pod that never became ready.
[ -n "${VPOD_READY}" ] || { error "Vault pod did not reach Running within 5 minutes"; exit 1; }

vx()  { vkube -n "${VAULT_NS}" exec "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true "$@"; }
vxt() { vkube -n "${VAULT_NS}" exec "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true VAULT_TOKEN="$1" "${@:2}"; }
# Stdin variant (exec -i), for commands that read from stdin like `vault policy write - `.
vxti() { vkube -n "${VAULT_NS}" exec -i "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true VAULT_TOKEN="$1" "${@:2}"; }

# vxi: stdin variant WITHOUT a token, for unauthenticated stdin commands (generate-root).
vxi() { vkube -n "${VAULT_NS}" exec -i "${VPOD}" -- env VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true "$@"; }

# gen_root_token re-derives a root token from the unseal key via the generate-root flow, so
# a re-run can reconcile config even though the previous run revoked root (mirrors the
# hypershell-gitops init Job's recover path). $1 = unseal key -> prints a new root token.
gen_root_token() {
  vx vault operator generate-root -cancel >/dev/null 2>&1 || true
  local otp nonce encoded
  otp="$(vx vault operator generate-root -generate-otp | tr -d '\r')"
  nonce="$(vx vault operator generate-root -init -otp="$otp" -format=json \
           | sed -n 's/.*"nonce":[[:space:]]*"\([^"]*\)".*/\1/p')"
  encoded="$(printf '%s' "$1" | vxi vault operator generate-root -nonce="$nonce" -format=json - \
           | sed -n 's/.*"encoded_token":[[:space:]]*"\([^"]*\)".*/\1/p')"
  vx vault operator generate-root -decode="$encoded" -otp="$otp" -format=json \
     | sed -n 's/.*"token":[[:space:]]*"\([^"]*\)".*/\1/p'
}

# apply_config writes the full gateway-provider config as root (every step idempotent): the
# KV v2 engine, kubernetes auth + local-cluster config, the capability-scoped policy, and the
# shared role. Re-applying is how a policy change in this repo takes effect on an
# already-initialized (e.g. shared-cluster) Vault. $1 = root token.
apply_config() {
  local rt="$1"
  vxt "$rt" vault secrets enable -path=secret -version=2 kv >/dev/null 2>&1 || true
  vxt "$rt" vault auth enable kubernetes >/dev/null 2>&1 || true
  vxt "$rt" sh -c 'vault write auth/kubernetes/config \
    kubernetes_host="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}" \
    kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
    token_reviewer_jwt=@/var/run/secrets/kubernetes.io/serviceaccount/token'

  # Capability-model policy: the OpenShell gateway runtime stores every provider credential
  # at the flat, hash-addressed path secret/data/openshell/provider-credentials/<sha> (the
  # <sha> is a SHA-256 seeded by a random provider UUID, so it is unguessable). The gateway
  # reaches a secret only by the opaque handle it already holds in its own DB; NO list is
  # granted, so one gateway can neither enumerate nor guess another's path. That capability
  # boundary, on top of the per-cluster Vault, is the isolation unit. See
  # openshell-gateway-credentials.spec.md (Provider Credential Isolation).
  printf '%s\n' \
    'path "secret/data/openshell/provider-credentials/*" {' \
    '  capabilities = ["create", "read", "update", "delete"]' \
    '}' \
    'path "secret/metadata/openshell/provider-credentials/*" {' \
    '  capabilities = ["read", "delete"]' \
    '}' | vxti "$rt" vault policy write openshell-gateway-provider -

  vxt "$rt" vault write "auth/kubernetes/role/${VAULT_ROLE}" \
    bound_service_account_names="openshell-gateway" \
    bound_service_account_namespaces='*' \
    audience=vault \
    token_policies="openshell-gateway-provider" \
    token_ttl=3600 >/dev/null
}

# run_selftest validates the capability-scoped policy the way a gateway does, so the
# GitOps Contract is checked once here in the fixture rather than by the e2e suite reaching
# into Vault with a synthetic identity. It mints a vault-audience token for a throwaway
# openshell-gateway ServiceAccount (the role binds bound_service_account_namespaces=*),
# logs in under the openshell-gateway role, and asserts: a round-trip under
# provider-credentials/* works, LIST is denied (no enumeration), and any path outside the
# prefix is denied. Fails the script if the policy drifts. See
# openshell-gateway-credentials.spec.md (Provider Credential Isolation / E2E Vault Coverage).
run_selftest() {
  local rt="$1" ns="openshell-vault-selftest" jwt out rev
  vkube create namespace "${ns}" --dry-run=client -o yaml | vkube apply -f - >/dev/null
  vkube create serviceaccount openshell-gateway -n "${ns}" --dry-run=client -o yaml | vkube apply -f - >/dev/null
  jwt="$(vkube create token openshell-gateway -n "${ns}" --audience vault --duration 10m 2>/dev/null || true)"
  [ -n "${jwt}" ] || { error "vault self-test: could not mint a vault-audience token"; exit 1; }
  # The login + checks run inside the pod; the SA JWT is read from stdin (never argv).
  out="$(printf '%s\n' "${jwt}" | vxi sh -c '
    read -r JWT
    CT=$(vault write -field=token auth/kubernetes/login role='"${VAULT_ROLE}"' jwt="$JWT") || { echo LOGIN_FAILED; exit 1; }
    export VAULT_TOKEN="$CT"
    K="secret/openshell/provider-credentials/selftest"
    if vault kv put "$K" value=ok >/dev/null 2>&1 && [ "$(vault kv get -field=value "$K" 2>/dev/null)" = "ok" ]; then echo ROUNDTRIP_OK; else echo ROUNDTRIP_FAIL; fi
    vault kv metadata delete "$K" >/dev/null 2>&1 || true
    if vault kv list secret/openshell/provider-credentials 2>&1 | grep -qiE "permission denied|403"; then echo LIST_DENIED; else echo LIST_ALLOWED; fi
    if vault kv put secret/openshell/not-allowed/x value=nope 2>&1 | grep -qiE "permission denied|403"; then echo OUTSIDE_DENIED; else echo OUTSIDE_ALLOWED; fi
  ')"
  printf '%s\n' "${out}" | grep -q ROUNDTRIP_OK  || { error "vault self-test: provider-credentials round-trip failed (out: ${out})"; exit 1; }
  printf '%s\n' "${out}" | grep -q LIST_DENIED   || { error "vault self-test: LIST not denied - policy too broad (out: ${out})"; exit 1; }
  printf '%s\n' "${out}" | grep -q OUTSIDE_DENIED || { error "vault self-test: out-of-prefix write not denied (out: ${out})"; exit 1; }

  # Revocation (openshell-gateway-vault.spec.md section 10): removing the policy revokes all
  # gateway access. Delete the policy as root, confirm the gateway identity is then denied,
  # and restore it via apply_config (idempotent) so the fixture is left usable. This needs
  # root, so the caller runs the self-test in the setup window, before root is revoked.
  vxt "${rt}" vault policy delete openshell-gateway-provider >/dev/null 2>&1 || { error "vault self-test: could not delete policy for the revocation check"; exit 1; }
  rev="$(printf '%s\n' "${jwt}" | vxi sh -c '
    read -r JWT
    CT=$(vault write -field=token auth/kubernetes/login role='"${VAULT_ROLE}"' jwt="$JWT" 2>/dev/null) || CT=""
    [ -n "$CT" ] && export VAULT_TOKEN="$CT"
    if [ -z "$CT" ] || vault kv get secret/openshell/provider-credentials/selftest 2>&1 | grep -qiE "permission denied|403"; then echo REVOKED_DENIED; else echo REVOKED_ALLOWED; fi
  ' || true)"
  apply_config "${rt}" # restore the policy/role (idempotent); leaves the fixture usable
  vkube delete namespace "${ns}" --wait=false >/dev/null 2>&1 || true
  printf '%s\n' "${rev}" | grep -q REVOKED_DENIED || { error "vault self-test: gateway NOT denied after policy removal - revocation broken (out: ${rev})"; exit 1; }
  success "Capability policy self-test passed (round-trip ok; list denied; out-of-prefix denied; revoked->denied)"
}

info "Waiting for the Vault API..."
# Bounded so a Vault that never answers fails the bring-up instead of hanging it. Up (0) or
# sealed (2) both mean the API is reachable; only unreachable (1) keeps us waiting.
VAULT_API_UP=
for _ in $(seq 1 60); do
  if vx vault status >/dev/null 2>&1 || [ "$?" -eq 2 ]; then VAULT_API_UP=1; break; fi
  sleep 3
done
[ -n "${VAULT_API_UP}" ] || { error "Vault API did not come up within 3 minutes"; exit 1; }

if vx vault status 2>/dev/null | grep -q "Initialized.*true"; then
  # Already initialized (e.g. a shared cluster across PR environments). Reconcile the config
  # non-destructively so a policy change in this repo always takes effect: the previous run
  # revoked root, so re-derive one from the stashed unseal key via generate-root, re-apply
  # the config (idempotent), then revoke root again. No data is touched.
  info "Vault already initialized - reconciling config (idempotent)..."
  UNSEAL_KEY="$(vkube get secret "${VAULT_UNSEAL_SECRET}" -n "${VAULT_NS}" -o jsonpath='{.data.unseal-key}' 2>/dev/null | base64 -d 2>/dev/null)"
  [ -n "${UNSEAL_KEY}" ] || { error "no stashed unseal key in Secret ${VAULT_UNSEAL_SECRET}; cannot reconcile"; exit 1; }
  if vx vault status 2>/dev/null | grep -q "Sealed.*true"; then
    vx vault operator unseal "${UNSEAL_KEY}" >/dev/null
  fi
  RT="$(gen_root_token "${UNSEAL_KEY}")"
  [ -n "${RT}" ] || { error "generate-root reconcile failed"; exit 1; }
  apply_config "${RT}"
  run_selftest "${RT}"
  vxt "${RT}" vault token revoke -self >/dev/null 2>&1 || true
  success "Vault reconciled at https://${VAULT_HOST}:8200 (capability-scoped policy re-applied)"
  exit 0
fi

info "Initializing + unsealing Vault (Shamir 1/1)..."
INIT_JSON="$(vx vault operator init -key-shares=1 -key-threshold=1 -format=json)"
UNSEAL_KEY="$(printf '%s' "${INIT_JSON}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["unseal_keys_b64"][0])')"
ROOT_TOKEN="$(printf '%s' "${INIT_JSON}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["root_token"])')"
# Stash the unseal key so a re-run can re-derive root (generate-root) and reconcile config,
# and so an unattended pod restart during a run can be re-unsealed (dev/e2e convenience).
vkube create secret generic "${VAULT_UNSEAL_SECRET}" -n "${VAULT_NS}" \
  --from-literal=unseal-key="${UNSEAL_KEY}" --dry-run=client -o yaml | vkube apply -f - >/dev/null
vx vault operator unseal "${UNSEAL_KEY}" >/dev/null

info "Configuring KV v2, kubernetes auth, role and capability-scoped policy..."
apply_config "${ROOT_TOKEN}"

run_selftest "${ROOT_TOKEN}"

# Drop root; gateways and the control plane only ever authenticate via kubernetes auth.
vxt "${ROOT_TOKEN}" vault token revoke -self >/dev/null 2>&1 || true

success "Vault ready at https://${VAULT_HOST}:8200 (KV v2 'secret', role '${VAULT_ROLE}')"
