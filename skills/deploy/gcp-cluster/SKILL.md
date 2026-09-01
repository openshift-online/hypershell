---
name: gcp-cluster
description: >
  Deploy HyperShell to an OpenShift Dedicated cluster on GCP using the Route
  ingress mode. Unlike ROKS, GCP OSD workers have full egress to public
  registries so no image mirroring is needed. Use when: "deploy to GCP",
  "new GCP cluster", "provision cloud hub on GCP", "hysh-gcp".
---

# GCP (OpenShift Dedicated) Cluster Deployment

Deploys HyperShell to an OpenShift Dedicated (OSD) cluster running on GCP as a
Cloud Hub with full OIDC security via Keycloak, CNPG-managed databases, and
per-gateway console provisioning.

> **Validated end to end (2026-08-21, `hypershell-gcp`, openshell 0.0.109):**
> Full stack deployed with OIDC: Keycloak realm, JWT-authenticated API,
> CNPG database cluster, tenant gateway with per-gateway OIDC console, TLS
> verified via passthrough Route at
> `gw-<tenant>.apps.hypershell-gcp.u0zc.p2.openshiftapps.com`.

## Why GCP OSD is simpler than ROKS

| Constraint | ROKS (IBM) | OSD (GCP) |
|------------|-----------|-----------|
| Worker egress | Blocked by default (SG) | **Open** - all public registries reachable |
| Image mirroring | Required for every image | **Not needed** |
| OperatorHub | Broken (catalog pods ImagePullBackOff) | **Works** |
| Gateway API (native) | Blocked (OSSM images unpullable, IDMS denied) | **Works** (istiod runs, GatewayClass Accepted) |
| Control plane access | HyperShift-hosted (limited) | Standard (full cluster-admin available) |

**Ingress mode: Route.** Although Gateway API works natively on OSD/GCP, Route
mode is chosen to avoid the DNS/TLS/wildcard-cert setup that Gateway API
requires. The OSD-managed `*.apps.<cluster>.<id>.openshiftapps.com` wildcard
provides immediate connectivity. Switch to Gateway API later by configuring a
custom domain, running `cloud-hub-ingress-bootstrap`, and unsetting
`GATEWAY_INGRESS_MODE`.

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

## Step 2: Create the `openshift-default` GatewayClass (optional but recommended)

Even though we use Route mode for ingress, creating the GatewayClass validates
that Gateway API works and prepares for a future switch. On OSD/GCP, unlike
ROKS, istiod pulls and runs without issues.

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

- `GATEWAY_INGRESS_MODE=route` - use OpenShift Routes, not Gateway API
- `GATEWAY_API_BASE_DOMAIN=apps.<cluster>.<id>.openshiftapps.com`
- `deploy/base/controller-rbac.yaml` - cluster-wide RBAC for tenant reconciliation

**Upgrade to `:latest` images** (the pinned digest images lack OIDC and CNPG
support):

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
    {\"name\": \"RBAC_ENFORCE\", \"value\": \"true\"},
    {\"name\": \"RBAC_SERVICE_ACCOUNTS\", \"value\": \"service-account-hypershell-control-plane\"}
  ]},
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/0/command\", \"value\": [
    \"/usr/local/bin/hypershell\", \"serve\",
    \"--api-server-bindaddress=0.0.0.0:8000\",
    \"--grpc-server-bindaddress=0.0.0.0:9000\",
    \"--health-check-server-bindaddress=0.0.0.0:4434\",
    \"--enable-authz=true\", \"--enable-jwt=true\",
    \"--enable-mock=true\", \"--enable-https=false\", \"--enable-metrics-https=false\",
    \"--db-host-file=/secrets/db.host\", \"--db-port-file=/secrets/db.port\",
    \"--db-name-file=/secrets/db.name\", \"--db-user-file=/secrets/db.user\",
    \"--db-password-file=/secrets/db.password\", \"--db-sslmode=disable\",
    \"--jwk-cert-url=https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell/protocol/openid-connect/certs\",
    \"--auth-bypass-paths=/healthcheck,/metrics,/api/hypershell/v1/openapi,/openapi\",
    \"--auth-bypass-methods=/grpc.health.v1.Health/,/grpc.reflection.v1alpha.ServerReflection/,/hypershell.v1.GatewayService/WatchGateways,/hypershell.v1.GatewayReleaseService/WatchGatewayReleases,/hypershell.v1.ManagedClusterService/WatchManagedClusters,/hypershell.v1.ManagedDatabaseService/WatchManagedDatabases,/hypershell.v1.GatewayNetworkService/WatchGatewayNetworks\"
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
  OIDC_ISSUER="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell" \
  OIDC_CLIENT_ID="hypershell-control-plane" \
  OIDC_CLIENT_SECRET="control-plane-secret" \
  OIDC_TOKEN_ENDPOINT="http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell/protocol/openid-connect/token" \
  GATEWAY_OIDC_ISSUER_URL="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell"
```

### 6.2: Install CNPG operator (v1.30+)

The `:latest` controller uses CloudNativePG for per-gateway database
provisioning. CNPG v1.30+ is required for the `DatabaseRole` CRD.

```bash
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.30/releases/cnpg-1.30.0.yaml

# On OSD, the CNPG controller needs the privileged SCC
oc adm policy add-scc-to-user privileged -z cnpg-manager -n cnpg-system
oc -n cnpg-system scale deploy cnpg-controller-manager --replicas=0
sleep 2
oc -n cnpg-system scale deploy cnpg-controller-manager --replicas=1
oc -n cnpg-system rollout status deploy/cnpg-controller-manager
```

Grant the controller RBAC for CNPG resources:

```bash
cat <<'EOF' | oc apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hypershell-cnpg-manager
rules:
  - apiGroups: ["postgresql.cnpg.io"]
    resources: ["clusters", "databases", "databaseroles", "publications", "subscriptions"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hypershell-cnpg-manager
subjects:
  - kind: ServiceAccount
    name: hypershell-controller
    namespace: hypershell
roleRef:
  kind: ClusterRole
  name: hypershell-cnpg-manager
  apiGroup: rbac.authorization.k8s.io
EOF
```

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
# expect: openshift=true certmanager=true gatewayapi=true cnpg=true keycloak=true

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
# ManagedCluster
CLUSTER=$(curl -sk -X POST "$API/managed_clusters" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"name\":\"gcp-local\",\"provider\":\"gcp\",\"region\":\"us-central1\",\"kubeconfig_secret\":\"gcp-local-kubeconfig\"}")
CLUSTER_ID=$(echo "$CLUSTER" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")

# GatewayRelease
RELEASE=$(curl -sk -X POST "$API/gateway_releases" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"name\":\"openshell-0.0.109\",\"image\":\"quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0\"}")
RELEASE_ID=$(echo "$RELEASE" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")

# ManagedDatabase (provider=cnpg for CNPG-managed provisioning)
DB=$(curl -sk -X POST "$API/managed_databases" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"name\":\"gcp-db\",\"provider\":\"cnpg\",\"region\":\"us-central1\",\"engine\":\"postgresql\"}")
DB_ID=$(echo "$DB" | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")

echo "Cluster=$CLUSTER_ID Release=$RELEASE_ID DB=$DB_ID"
```

### 8.2: Create the Gateway

```bash
GATEWAY=$(curl -sk -X POST "$API/gateways" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" -d "{
  \"name\": \"gcp-test-gw\",
  \"cluster_id\": \"$CLUSTER_ID\",
  \"release_id\": \"$RELEASE_ID\",
  \"database_id\": \"$DB_ID\",
  \"namespace\": \"openshell-gcptest\",
  \"image\": \"quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0\",
  \"route\": \"{\\\"enabled\\\": true}\"
}")
echo "$GATEWAY" | python3 -m json.tool
```

The `:latest` controller fully reconciles the gateway with no manual patches
needed: CNPG database cluster, per-gateway Keycloak OIDC client, correct DB
credentials secret (`uri` key), init containers, volumes, and Route hostname
are all handled automatically.

## Step 9: Verify end to end

```bash
NS=openshell-gcptest
BASE_DOMAIN="apps.<cluster>.<id>.openshiftapps.com"

# All tenant pods Running
oc -n "$NS" get pods
# openshell-gateway 1/1 Running
# openshell-console 2/2 Running (per-gateway OIDC console)

HOST=$(oc -n "$NS" get route openshell-gateway-grpc -o jsonpath='{.spec.host}')

# TLS verification (passthrough serves the per-tenant cert)
oc -n "$NS" get secret openshell-server-tls -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/tenant-ca.crt
echo | openssl s_client -connect "$HOST:443" -servername "$HOST" -CAfile /tmp/tenant-ca.crt 2>&1 \
  | grep 'Verify return code'   # 0 (ok)

# gRPC transport through the Route
openshell status --gateway-endpoint "https://$HOST:443" --gateway-insecure
# Status: Connected, Version: 0.0.109

# Verify per-gateway OIDC console
CONSOLE_URL="https://$(oc -n "$NS" get route openshell-console -o jsonpath='{.spec.host}')"
curl -sk -o /dev/null -w '%{http_code}' "$CONSOLE_URL/auth/login"   # 302 -> keycloak

# Verify CNPG database cluster healthy
DB_NS=$(oc get namespaces -l hypershell.redhat.io/managed=true --no-headers \
  -o custom-columns=':metadata.name' | grep openshell-db)
oc -n "$DB_NS" get cluster openshell-db -o jsonpath='{.status.phase}'   # Cluster in healthy state
```

## Step 10: Run the e2e test suite

```bash
GATEWAY_NAME=angel SKIP_CLEANUP=1 bash components/pr-test/e2e-openshell-gcp.sh
```

To create a fresh gateway instead of reusing an existing one:

```bash
bash components/pr-test/e2e-openshell-gcp.sh
```

### Reference run (2026-08-21, `angel` gateway, openshell 0.0.109)

```
HyperShell OpenShell Gateway End-to-End Test (GCP OSD)
────────────────────────────────────────────────

  0. JWT authentication verification
  1. Gateway provisioning via HyperShell API (OIDC)
  1b. OIDC Role Bridge (per-gateway client roles)
  2. Gateway infrastructure verification
  3. OIDC token acquisition (per-gateway client)
  3a. CA certificate setup
  4. Route discovery + openshell CLI registration
  5. Gateway connectivity
  6. Sandbox lifecycle (create → ready)
  7. Sandbox interaction
  8. Developer user RBAC verification

  HyperShell API:    https://hypershell-api-hypershell.apps.hypershell-gcp.u0zc.p2.openshiftapps.com
  Keycloak:          https://keycloak-keycloak.apps.hypershell-gcp.u0zc.p2.openshiftapps.com
  OIDC issuer:       https://keycloak-keycloak.apps.hypershell-gcp.u0zc.p2.openshiftapps.com/realms/hypershell
  Gateway name:      angel
  Cluster:           3IEMEyUoM01Vq5dccnD01TsxzNf
  Release:           3IEMG6W8UhcfH3sruR2Zosc9LJL
  Database:          3IEMG7C1TcAnrbzZHjiJii0UPZ8

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Results: 23 passed, 0 failed

  ✓ Unauthenticated request rejected (HTTP 401)
  ✓ Authenticated request accepted (HTTP 200)
  ✓ Gateway already exists: angel (3IEWSLgFpF3DHsaGxl5IexOrrME, phase=Running)
  ✓ Keycloak admin service account ready (realm: hypershell)
  ✓ Assigned openshell-admin to admin on angel-3IEWSLgFpF3DHsaGxl5IexOrrME
  ✓ Assigned openshell-user to developer on angel-3IEWSLgFpF3DHsaGxl5IexOrrME
  ✓ Gateway pod ready (quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0)
  ✓ Gateway service: 172.30.26.165:8080
  ✓ TLS certificates provisioned
  ✓ CNPG database cluster healthy in openshell-db-913eb1e752d32f24 (phase: Cluster in healthy state)
  ✓ OIDC token acquired (user: admin, aud=angel-3IEWSLgFpF3DHsaGxl5IexOrrME, roles=openshell-user,openshell-admin)
  ✓ CA certificate extracted and SSL_CERT_FILE set
  ✓ Passthrough route: gw-openshell-944b372e89b1cb7e.apps.hypershell-gcp.u0zc.p2.openshiftapps.com
  ✓ openshell CLI registered (OIDC mode)
  ✓ Gateway connected (version: 0.0.109)
  ✓ Sandbox pod created: default--e2e-1645 (Running)
  ✓ Sandbox exec: command executed inside sandbox
  ✓ Developer OIDC token acquired (user: developer, roles=openshell-user)
  ✓ Developer gateway registered (OIDC mode)
  ✓ Developer user: gateway connected
  ✓ Developer granted 'user' membership on 'default' workspace
  ✓ Developer user: sandbox created
  ✓ Developer user: sandbox exec succeeded
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Switching to Gateway API mode later

If a custom domain with wildcard DNS is configured later:

1. Create a ClusterIssuer + wildcard Certificate (see `cloud-hub-ingress-bootstrap`)
2. Create a shared Gateway in `openshift-ingress` (the `openshift-default`
   GatewayClass is already Accepted)
3. Set `GATEWAY_API_GATEWAY_NAME`, `GATEWAY_API_GATEWAY_NAMESPACE`,
   `GATEWAY_API_BASE_DOMAIN` on the controller
4. Unset `GATEWAY_INGRESS_MODE` (removes it, defaults to `gateway-api`)
5. Re-reconcile existing gateways - the controller will emit `GRPCRoute`s and
   remove the Routes

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| API returns 200 (no auth) on unauthenticated requests | `API_ENV=development` forcibly disables JWT in `OverrideConfig` | Set `API_ENV=development_oidc` |
| API server crashes with `ReadFiles` / missing secret files | `API_ENV=production` requires secret files on disk | Use `development_oidc` + `--enable-mock=true` instead |
| Controller logs `OIDC authentication disabled for gRPC` | `OIDC_ISSUER`, `OIDC_CLIENT_ID`, or `OIDC_CLIENT_SECRET` env vars missing | Set all three OIDC env vars on the controller deployment |
| Controller gets `Unauthenticated: missing authorization token` on gRPC | Controller image lacks OIDC token provider (old pinned digest) | Upgrade controller to `:latest` image |
| API server `relation "users" does not exist` or `column "X" missing` | Init container (migration) uses old image with older schema | Update init container image to `:latest` to match the main container |
| CNPG controller pods crash with SCC errors | OSD restricts UIDs; CNPG wants UID 10001 | `oc adm policy add-scc-to-user privileged -z cnpg-manager -n cnpg-system`, then bounce |
| Controller can't create CNPG `DatabaseRole` CRDs | CNPG version < 1.30 lacks the `databaseroles` CRD | Upgrade CNPG to v1.30+ |
| Controller can't create/watch CNPG resources | Missing RBAC for `postgresql.cnpg.io` API group | Create the `hypershell-cnpg-manager` ClusterRole/Binding (Step 6.2) |
| Gateway pod `CreateContainerConfigError` on `uri` key | DB credentials secret has `url` key but gateway expects `uri` | `:latest` controller creates the correct `uri` key; if migrating from old controller, patch the secret |
| Controller `403 Forbidden` managing Keycloak clients | `hypershell-control-plane` SA missing `realm-management` roles | Assign roles via Keycloak admin API (Step 5.1) |
| Web console crashes with `SESSION_SECRET is required` | `SESSION_SECRET` env var not set when `OIDC_ISSUER` is present | Set `SESSION_SECRET` to a random hex string |
| `openshell` CLI gets `BadSignature` TLS error | CLI has cached a stale CA cert from a previous cert-manager issuance | Re-fetch the CA from `openshell-server-tls` secret, or use `--insecure` |

## Architecture notes

### Environment system

The API server `API_ENV` environment variable selects the runtime behavior:

| `API_ENV` | JWT Auth | Mock Secrets | Use Case |
|-----------|----------|-------------|----------|
| `development` | **Forcibly disabled** | Yes | Local dev without auth |
| `development_oidc` | Enabled (flags) | Yes | Dev with Keycloak OIDC |
| `production` | Enabled | No (reads files) | Production with external SSO |

This deployment uses `development_oidc` + `--enable-mock=true` to enable JWT
auth while avoiding the need for production secret files. `--enable-mock=true`
makes `ReadFiles()` return nil (skipping disk reads for OCM/SSO secrets).
`--enable-https=false` and `--enable-metrics-https=false` are required because
the API server pod doesn't have TLS certs; the OpenShift Route handles TLS.

### RBAC configuration

`RBAC_ENFORCE=true` enables the three-role gateway RBAC model
(`gateway:creator/owner/viewer`). `RBAC_SERVICE_ACCOUNTS` must include the
controller's service account name (`service-account-hypershell-control-plane`)
so its gRPC `UpdateGateway` calls bypass the RBAC interceptor; the
controller's `client_credentials` token doesn't carry gateway roles.

### gRPC auth chain

The rh-trex-ai framework runs two interceptors in series:

1. **Bearer token interceptor** - checks `Authorization: Bearer <token>`,
   configurable `--auth-bypass-methods` (prefix match)
2. **JWT interceptor** - validates JWK signature, **hardcoded bypass** only for
   health + reflection methods

The controller authenticates via OIDC `client_credentials` grant (Keycloak
`hypershell-control-plane` service account). Watch methods are bypassed in the
bearer interceptor via `--auth-bypass-methods`.

### Controller OIDC token endpoint

`OIDC_TOKEN_ENDPOINT` is set to the internal Keycloak service URL
(`http://keycloak-service.keycloak.svc.cluster.local:8080/...`) instead of the
external Route URL. This avoids TLS verification issues when the controller
accesses Keycloak in-cluster and removes the dependency on external DNS
resolution.

### CNPG database provisioning

The `:latest` controller uses CloudNativePG instead of a standalone PostgreSQL
deployment for per-gateway databases. The `ManagedDatabase` must have
`provider=cnpg` (not `local`). The controller creates CNPG `Cluster`,
`Database`, and `DatabaseRole` CRDs in an auto-generated namespace
(`openshell-db-<hex>`, where hex is derived from the first 8 bytes of the
ManagedDatabase KSUID).
