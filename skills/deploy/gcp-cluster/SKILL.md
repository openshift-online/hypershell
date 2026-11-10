---
name: gcp-cluster
description: >
  Deploy HyperShell to an OpenShift Dedicated cluster on GCP with Gateway API
  tenant ingress. Unlike ROKS, GCP OSD workers have full egress to public
  registries so no image mirroring is needed. Use when: "deploy to GCP",
  "new GCP cluster", "provision cloud hub on GCP", "hysh-gcp".
---

# GCP (OpenShift Dedicated) Cluster Deployment

Deploys HyperShell to an OpenShift Dedicated (OSD) cluster running on GCP as a
Cloud Hub with full OIDC security via Keycloak, externally provisioned databases, and
per-gateway console provisioning.

> **Validated end to end (2026-08-21, `hypershell-gcp`, openshell 0.0.109):**
> Full stack deployed with OIDC: Keycloak realm, JWT-authenticated API,
> registered PostgreSQL server, and a tenant gateway with a per-gateway OIDC console.

## Why GCP OSD is simpler than ROKS

| Constraint | ROKS (IBM) | OSD (GCP) |
|------------|-----------|-----------|
| Worker egress | Blocked by default (SG) | **Open** - all public registries reachable |
| Image mirroring | Required for every image | **Not needed** |
| OperatorHub | Broken (catalog pods ImagePullBackOff) | **Works** |
| Gateway API (native) | Works on ROKS >= 4.22 | **Works** (istiod runs, GatewayClass Accepted) |
| Control plane access | HyperShift-hosted (limited) | Standard (full cluster-admin available) |

**Ingress: Gateway API.** Tenant gateways are published through a shared
`Gateway` (`openshift-ingress/openshell-grpc-gateway`). Each gateway gets a
`GRPCRoute`, and its console gets an `HTTPRoute`. The shared Gateway needs a
wildcard domain, a wildcard certificate and a wildcard DNS record. Set them up
with [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md)
(Step 3b).

## Prerequisites

- `oc` CLI logged in to the GCP OSD cluster with cluster-admin (or
  `dedicated-admin`) access
- The cluster is `Ready` in the OpenShift Cluster Manager console

## Step 1: Login and verify cluster

```bash
oc login https://api.<cluster>.<id>.openshiftapps.com:6443 \
  --username=<admin-user> --password='<password>'

# Verify access
oc whoami
oc auth can-i '*' '*' --all-namespaces   # expect: yes

# Check version (must be >= 4.19 for built-in Gateway API)
oc get clusterversion version -o jsonpath='{.status.desired.version}{"\n"}'

# Verify worker egress is open (no image mirroring needed)
oc get nodes -o wide
```

Reference values for `hypershell-gcp` (2026-08-21):

| Parameter | Value |
|-----------|-------|
| Version | `4.22.9` |
| Nodes | 3 masters + 2 workers + 2 infra |
| Ingress domain | `apps.hypershell-gcp.u0zc.p2.openshiftapps.com` |
| Storage class | `standard-csi` (default) |
| GCP project | `hcm-hyperfleet` |

## Step 2: Create the `openshift-default` GatewayClass

Creating the GatewayClass makes the Ingress Operator install `istiod` in
`openshift-ingress`. The shared Gateway uses this class.

```bash
cat <<'EOF' | oc apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: openshift-default
spec:
  controllerName: openshift.io/gateway-controller/v1
EOF

# Verify
oc get gatewayclass openshift-default -o jsonpath='{.status.conditions[?(@.type=="Accepted")].status}{"\n"}'   # True
oc -n openshift-ingress get pods -l app=istiod   # Running
```

## Step 3: Install cert-manager via OperatorHub

cert-manager is required for per-tenant gateway TLS certificates (the internal
CA that mints `openshell-server-tls` and `openshell-client-tls`). On OSD/GCP,
OperatorHub works out of the box, so use the Red Hat-supported operator.

```bash
cat <<'EOF' | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: cert-manager-operator
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: cert-manager-operator
  namespace: cert-manager-operator
spec: {}
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-cert-manager-operator
  namespace: cert-manager-operator
spec:
  channel: stable-v1
  installPlanApproval: Automatic
  name: openshift-cert-manager-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

# Wait for pods
oc -n cert-manager get pods -w   # cert-manager, cainjector, webhook all Running
```

## Step 3b: Bootstrap the shared Gateway

Run [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md) once
on this cluster. It creates the wildcard certificate, the shared
`openshift-ingress/openshell-grpc-gateway` and the wildcard DNS record for the
GCP load balancer. Note the base domain; the controller needs it in Step 6.

## Step 4: Install the Agent Sandbox CRD + controller

Required for `openshell sandbox` functionality. On GCP, nodes can pull the
controller image directly from `registry.k8s.io` (no mirroring).

```bash
V=v0.5.5
curl -sL "https://github.com/kubernetes-sigs/agent-sandbox/releases/download/$V/sandbox.yaml" \
  | oc apply -f -

oc -n agent-sandbox-system rollout status deploy/agent-sandbox-controller
oc get crd sandboxes.agents.x-k8s.io -o jsonpath='{.status.conditions[?(@.type=="Established")].status}{"\n"}'   # True
```

## Step 5: Deploy Keycloak

Deploy Keycloak with the `hypershell` realm. The realm import pre-configures
OIDC clients (`hypershell-frontend`, `hypershell-control-plane`) and users.

```bash
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"

cd deploy/keycloak
# Render with the correct domain
oc kustomize . | sed "s/apps.example.com/$BASE_DOMAIN/g" | oc apply -f -

# Wait for Keycloak
oc -n keycloak rollout status deploy/keycloak

# Verify OIDC discovery
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
curl -sk "$KC_URL/realms/hypershell/.well-known/openid-configuration" | python3 -c "
import json,sys; d=json.load(sys.stdin); print('Issuer:', d['issuer'])"
```

### 5.1: Grant control plane Keycloak admin access

The controller manages per-gateway OIDC clients via the Keycloak admin API.
Assign `realm-management` roles to the `hypershell-control-plane` service
account:

```bash
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
ADMIN_TOKEN=$(curl -sk -X POST "$KC_URL/realms/master/protocol/openid-connect/token" \
  -d "grant_type=password" -d "client_id=admin-cli" \
  -d "username=admin" -d "password=admin" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

# Find the control-plane client's service account
CP_ID=$(curl -sk -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KC_URL/admin/realms/hypershell/clients?clientId=hypershell-control-plane" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)[0]['id'])")
SA_ID=$(curl -sk -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KC_URL/admin/realms/hypershell/clients/$CP_ID/service-account-user" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")
RM_ID=$(curl -sk -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KC_URL/admin/realms/hypershell/clients?clientId=realm-management" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)[0]['id'])")

# Assign needed roles
ROLES=$(curl -sk -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KC_URL/admin/realms/hypershell/users/$SA_ID/role-mappings/clients/$RM_ID/available" \
  | python3 -c "
import json,sys
needed={'manage-clients','view-clients','manage-users','view-users','manage-realm','view-realm','query-clients','query-users'}
print(json.dumps([r for r in json.load(sys.stdin) if r['name'] in needed]))")
curl -sk -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  "$KC_URL/admin/realms/hypershell/users/$SA_ID/role-mappings/clients/$RM_ID" \
  -d "$ROLES" -w '%{http_code}\n'   # expect: 204
```

## Step 6: Deploy HyperShell platform

### 6.1: Apply the base overlay then upgrade to `:latest` images

```bash
oc kustomize deploy/openshift | oc apply -f -
```

The `deploy/openshift` overlay (base for all cloud deployments) sets:

- `GATEWAY_API_GATEWAY_NAME=openshell-grpc-gateway` and
  `GATEWAY_API_GATEWAY_NAMESPACE=openshift-ingress` - the shared Gateway
- `GATEWAY_API_BASE_DOMAIN` - a placeholder; set it to the wildcard domain from
  Step 3b (`oc -n hypershell set env deploy/hypershell-controller GATEWAY_API_BASE_DOMAIN=<domain>`)
- `GATEWAY_API_HTTP_LISTENER_NAME` - the shared Gateway listener that console
  `HTTPRoute`s attach to
- `deploy/base/platform-resources/controller-rbac.yaml` - cluster-wide RBAC for tenant reconciliation

**Upgrade to `:latest` images** (the pinned digest images lack OIDC support):

```bash
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"
REGISTRY="quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main"

# API server: latest image + OIDC environment + migration init container
oc -n hypershell set image deploy/hypershell-api-server \
  api-server=$REGISTRY/hypershell-api-server-main:latest
oc -n hypershell patch deploy hypershell-api-server --type=json -p "[
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/initContainers/0/image\",
   \"value\": \"$REGISTRY/hypershell-api-server-main:latest\"},
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/0/env\", \"value\": [
    {\"name\": \"API_ENV\", \"value\": \"development_oidc\"},
    {\"name\": \"RBAC_SERVICE_ACCOUNTS\", \"value\": \"service-account-hypershell-control-plane\"}
  ]},
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/0/command\", \"value\": [
    \"/usr/local/bin/hypershell\", \"serve\",
    \"--api-server-bindaddress=0.0.0.0:8000\",
    \"--grpc-server-bindaddress=0.0.0.0:9000\",
    \"--health-check-server-bindaddress=0.0.0.0:4434\",
    \"--enable-authz=true\", \"--enable-jwt=true\",
    \"--enable-https=false\", \"--enable-metrics-https=false\",
    \"--db-host-file=/secrets/db.host\", \"--db-port-file=/secrets/db.port\",
    \"--db-name-file=/secrets/db.name\", \"--db-user-file=/secrets/db.user\",
    \"--db-password-file=/secrets/db.password\", \"--db-sslmode=disable\",
    \"--jwk-cert-url=https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell/protocol/openid-connect/certs\",
    \"--auth-bypass-paths=/healthcheck,/metrics,/api/hypershell/v1/openapi,/openapi\",
    \"--auth-bypass-methods=/grpc.health.v1.Health/,/grpc.reflection.v1alpha.ServerReflection/\"
  ]}
]"

# Controller: latest image + OIDC + Keycloak admin
oc -n hypershell set image deploy/hypershell-controller \
  controller=$REGISTRY/hypershell-control-plane-main:latest

# Create Keycloak admin secret for the controller
oc -n hypershell create secret generic hypershell-keycloak-admin \
  --from-literal=client-id=hypershell-control-plane \
  --from-literal=client-secret=control-plane-secret \
  --from-literal=realm=hypershell \
  --from-literal=server-url=http://keycloak-service.keycloak.svc.cluster.local:8080

oc -n hypershell set env deploy/hypershell-controller \
  HYPERSHELL_MANAGED_CLUSTER_NAME="gcp-local" \
  OIDC_ISSUER="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell" \
  OIDC_CLIENT_ID="hypershell-control-plane" \
  OIDC_CLIENT_SECRET="control-plane-secret" \
  OIDC_TOKEN_ENDPOINT="http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell/protocol/openid-connect/token" \
  GATEWAY_OIDC_ISSUER_URL="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell"
```

### 6.2: Configure the gateway database server

The controller provisions each gateway's database and login role on an externally
provisioned PostgreSQL server. It reads the admin connection from the
`hypershell-gateway-database-admin` Secret mounted at `/etc/hypershell/gateway-database`
and refuses to start until that Secret exists with valid files. Create it **before**
the controller rollout, with the server's CA bundle; `sslmode` is always
`verify-full`:

```bash
oc -n hypershell create secret generic hypershell-gateway-database-admin \
  --from-literal=host=<server-host> \
  --from-literal=port=5432 \
  --from-literal=user=<admin-user> \
  --from-literal=password=<admin-password> \
  --from-literal=dbname=postgres \
  --from-literal=sslmode=verify-full \
  --from-file=sslrootcert=<server-ca-bundle.pem>
oc -n hypershell rollout restart deploy/hypershell-controller
```

The admin role needs `CREATEDB`, `CREATEROLE` and `pg_signal_backend`; it does not
need superuser. There is no database resource to create through the API.

### 6.3: Deploy the web console with OIDC

```bash
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"
SESSION_SECRET=$(openssl rand -hex 32)

cd deploy
oc kustomize base/web-console.yaml | oc apply -f -   # or apply web-console.yaml directly

oc -n hypershell set env deploy/hypershell-web-console \
  OIDC_ISSUER="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell" \
  OIDC_CLIENT_ID="hypershell-frontend" \
  OIDC_REDIRECT_URI="https://hypershell-console-hypershell.$BASE_DOMAIN/auth/callback" \
  OIDC_POST_LOGOUT_REDIRECT_URI="https://hypershell-console-hypershell.$BASE_DOMAIN" \
  SESSION_SECRET="$SESSION_SECRET"
```

### 6.4: Wait for all pods and verify

```bash
oc -n hypershell get pods -w
# All Running: api-server, controller, postgres, web-console

# Verify controller detected all capabilities
oc -n hypershell logs deploy/hypershell-controller | grep 'gateway reconciler initialized'
# expect: openshift=true certmanager=true gatewayapi=true keycloak=true

# Verify JWT auth
API="https://$(oc -n hypershell get route hypershell-api -o jsonpath='{.spec.host}')/api/hypershell/v1"
curl -sk -w '%{http_code}' "$API/gateways"   # 401

# Verify console OIDC redirect
curl -sk -o /dev/null -w '%{http_code}' \
  "https://hypershell-console-hypershell.$BASE_DOMAIN/auth/login"   # 302 -> keycloak
```

## Step 7: Create a managed cluster identity

The control plane needs a kubeconfig to reconcile into the target cluster (even
when it is the same cluster). Create a ServiceAccount with cluster-admin and a
long-lived token:

```bash
oc -n hypershell create sa cluster-agent
oc adm policy add-cluster-role-to-user cluster-admin -z cluster-agent -n hypershell

cat <<'EOF' | oc apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: cluster-agent-token
  namespace: hypershell
  annotations:
    kubernetes.io/service-account.name: cluster-agent
type: kubernetes.io/service-account-token
EOF
sleep 5

SERVER=$(oc whoami --show-server)
TOKEN=$(oc -n hypershell get secret cluster-agent-token -o jsonpath='{.data.token}' | base64 -d)
CA=$(oc -n hypershell get secret cluster-agent-token -o jsonpath='{.data.ca\.crt}')

oc -n hypershell create secret generic gcp-local-kubeconfig --from-literal=kubeconfig="$(cat <<KUBEEOF
apiVersion: v1
kind: Config
clusters:
  - name: gcp-cluster
    cluster:
      server: ${SERVER}
      certificate-authority-data: ${CA}
contexts:
  - name: gcp-cluster
    context:
      cluster: gcp-cluster
      user: cluster-agent
current-context: gcp-cluster
users:
  - name: cluster-agent
    user:
      token: ${TOKEN}
KUBEEOF
)"
```

## Step 8: Provision a tenant gateway

With OIDC enabled, API requests require a bearer token. Obtain one from
Keycloak first:

```bash
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
API="https://$(oc -n hypershell get route hypershell-api -o jsonpath='{.spec.host}')/api/hypershell/v1"

TOKEN=$(curl -sk -X POST "$KC_URL/realms/hypershell/protocol/openid-connect/token" \
  -d "grant_type=password" -d "client_id=hypershell-frontend" \
  -d "username=admin" -d "password=admin" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")
AUTH="-H 'Authorization: Bearer $TOKEN'"
```

### 8.1: Create API resources

```bash
# ManagedCluster: do NOT POST it. The controller registers itself as
# HYPERSHELL_MANAGED_CLUSTER_NAME (gcp-local) at startup; a manually created
# record with that name would make its registration a 409 conflict.
CLUSTER_ID=$(curl -sk "$API/managed_clusters" -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import json,sys; print(next(c['id'] for c in json.load(sys.stdin)['items'] if c['name']=='gcp-local' and c.get('oidc_subject')))")

echo "Cluster=$CLUSTER_ID"
```

### 8.2: Create the Gateway

```bash
# Create takes a required placement intent, not a cluster_id. The selector
# matches HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES on the controller.
GATEWAY=$(curl -sk -X POST "$API/gateways" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" -d "{
  \"name\": \"gcp-test-gw\",
  \"placement\": {\"selector\": [\"network/public\", \"provider/aws\"]},
  \"namespace\": \"openshell-gcptest\",
  \"image\": \"quay.io/opendatahub/odh-openshell-gateway:v0.1.6-rhaiv.4\"
}")
echo "$GATEWAY" | python3 -m json.tool
```

The `:latest` controller fully reconciles the gateway with no manual patches
needed: the per-gateway database on the registered server, the Keycloak OIDC
client, correct DB
credentials secret (`uri` key), init containers, volumes, and the `GRPCRoute`
and console `HTTPRoute` on the shared Gateway are all handled automatically.

## Step 9: Verify end to end

```bash
NS=openshell-gcptest
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"

# All tenant pods Running
oc -n "$NS" get pods
# openshell-gateway 1/1 Running
# openshell-console 2/2 Running (per-gateway OIDC console)

HOST=$(oc -n "$NS" get grpcroute openshell-gateway -o jsonpath='{.spec.hostnames[0]}')

# The shared Gateway serves the publicly trusted wildcard certificate
echo | openssl s_client -connect "$HOST:443" -servername "$HOST" 2>&1 \
  | grep 'Verify return code'   # 0 (ok)

# gRPC transport through the shared Gateway
openshell status --gateway-endpoint "https://$HOST:443"
# Status: Connected

# Verify per-gateway OIDC console
CONSOLE_URL="https://$(oc -n "$NS" get httproute openshell-console -o jsonpath='{.spec.hostnames[0]}')"
curl -sk -o /dev/null -w '%{http_code}' "$CONSOLE_URL/auth/login"   # 302 -> keycloak

# Verify the gateway database credentials were written into the tenant namespace
oc -n "$NS" get secret openshell-gateway-db-credentials \
  -o jsonpath='{.data.dbname}' | base64 -d   # gw_<gateway-id>
```

## Step 10: Run the e2e test suite

Run the Go e2e suite with the OpenShift driver. It discovers the API, console and
Keycloak Routes in the platform namespace, and each gateway endpoint from its
`GRPCRoute`:

```bash
OPENSHIFT_NAMESPACE=hypershell E2E_INFRA_DRIVER=openshift make e2e
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| API returns 200 (no auth) on unauthenticated requests | `API_ENV=development` forcibly disables JWT in `OverrideConfig` | Set `API_ENV=development_oidc` |
| Controller exits with `... is required: every control plane registers with the hub` | `HYPERSHELL_MANAGED_CLUSTER_NAME`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, or `OIDC_CLIENT_SECRET` env var missing | Set the cluster name and all three OIDC env vars on the controller deployment |
| Controller exits with `managed-cluster registration failed: ... registration conflict` | A ManagedCluster with the controller's name already exists and was not registered by it (e.g. created with `POST /managed_clusters`) | Delete that record (re-point its gateways) and restart the controller |
| Controller gets `Unauthenticated: missing authorization token` on gRPC | Controller image lacks OIDC token provider (old pinned digest) | Upgrade controller to `:latest` image |
| API server `relation "users" does not exist` or `column "X" missing` | Init container (migration) uses old image with older schema | Update init container image to `:latest` to match the main container |
| Controller crash-loops with `gateway database admin credentials` in the log | `hypershell-gateway-database-admin` is missing, lacks a required key (`host`/`port`/`user`/`password`/`sslrootcert`), or sets `sslmode` to anything but `verify-full` | Create or fix the Secret (Step 6.2) and restart the controller |
| Gateway stuck with `DatabaseReady: Failed`; controller log shows connection, auth or TLS errors | The cluster cannot reach the server, the admin credentials are wrong, or the CA bundle does not match the server certificate | Check egress to the server and the Secret's `host`/`user`/`password`/`sslrootcert`; the reconcile retries on its own |
| Gateway `DatabaseReady: Failed`; controller log shows a permission error on `CREATE DATABASE`/`CREATE ROLE` | The admin role lacks `CREATEDB` or `CREATEROLE` | Grant both on the server |
| Gateway pod `CreateContainerConfigError` on `uri` key | DB credentials secret has `url` key but gateway expects `uri` | `:latest` controller creates the correct `uri` key; if migrating from old controller, patch the secret |
| Controller `403 Forbidden` managing Keycloak clients | `hypershell-control-plane` SA missing `realm-management` roles | Assign roles via Keycloak admin API (Step 5.1) |
| Web console crashes with `SESSION_SECRET is required` | `SESSION_SECRET` env var not set when `OIDC_ISSUER` is present | Set `SESSION_SECRET` to a random hex string |
| GRPCRoute `Accepted=False` or the Gateway is not `Programmed` | Shared Gateway missing, wrong `GATEWAY_API_*` values, or listener hostname does not cover `GATEWAY_API_BASE_DOMAIN` | See the troubleshooting section of `cloud-hub-ingress-bootstrap` |

## Architecture notes

### Environment system

The API server `API_ENV` environment variable selects the runtime behavior:

| `API_ENV` | JWT Auth | Use Case |
|-----------|----------|----------|
| `development` | **Forcibly disabled** | Local dev without auth |
| `development_oidc` | Enabled (flags) | Dev with Keycloak OIDC |
| `production` | Enabled (flags) | Production with external SSO |

This deployment uses `development_oidc` to enable JWT auth against Keycloak.
`--enable-https=false` and `--enable-metrics-https=false` are required because
the API server pod doesn't have TLS certs; the OpenShift Route handles TLS.

### RBAC configuration

JWT authentication is on in this deployment, so the gateway RBAC model
(`gateway:creator/owner/viewer`) is enforced automatically -- there is no
separate enforcement variable to set. `RBAC_SERVICE_ACCOUNTS` must include the
controller's service account name (`service-account-hypershell-control-plane`)
so its gRPC `UpdateGateway` calls bypass the RBAC interceptor; the
controller's `client_credentials` token doesn't carry gateway roles. This is
the bootstrap fallback: once a control plane has registered its managed
cluster, its JWT `sub` alone makes it a control-plane identity, so other
(spoke) control planes need no entry.

### gRPC auth chain

The rh-trex-ai framework runs two interceptors in series:

1. **Bearer token interceptor** - checks `Authorization: Bearer <token>`,
   configurable `--auth-bypass-methods` (prefix match)
2. **JWT interceptor** - validates JWK signature, **hardcoded bypass** only for
   health + reflection methods

The controller authenticates every RPC, including the watch streams, via its
OIDC `client_credentials` grant (Keycloak `hypershell-control-plane` service
account). `--auth-bypass-methods` lists only health and reflection; the watch
RPCs are never exempt, and `WatchGateways`/`ListGateways` from a registered
control plane must carry its own `cluster_id`
(`specs/platform/managed-cluster-registration.spec.md`).

### Controller OIDC token endpoint

`OIDC_TOKEN_ENDPOINT` is set to the internal Keycloak service URL
(`http://keycloak-service.keycloak.svc.cluster.local:8080/...`) instead of the
external Route URL. This avoids TLS verification issues when the controller
accesses Keycloak in-cluster and removes the dependency on external DNS
resolution.

### Gateway database provisioning

The controller provisions each gateway's database and login role (`gw_<gateway-id>`)
directly on the PostgreSQL server named by the `hypershell-gateway-database-admin`
Secret mounted into it, using a short-lived `verify-full` admin connection whose
files are re-read on every operation. No database workload and no operator run in
the cluster, and nothing in the API refers to databases; the gateway namespace
receives only the `openshell-gateway-db-credentials` Secret (its own role, database,
`uri` and the server CA as `sslrootcert`).
