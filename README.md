<h1 align="center"><sup><img src="images/brand/logo.png" alt="HyperShell logo" width="80" align="middle"></sup>&nbsp;HyperShell</h1>

HyperShell provisions and manages [OpenShell](https://github.com/NVIDIA/OpenShell) gateways at scale and across clouds.

## Kubernetes Prerequisites

The control plane requires the following resources to be present on the target cluster before it can fully reconcile gateways.

### Agent Sandbox controller

The [Agent Sandbox controller](https://github.com/kubernetes-sigs/agent-sandbox) (`agents.x-k8s.io`) must be installed on the cluster. Gateway pods manage sandboxes via the `Sandbox` custom resource, and the provisioned RBAC grants permissions on `agents.x-k8s.io/sandboxes`. NetworkPolicies also reference sandbox labels (`agents.x-k8s.io/sandbox-name-hash`) for pod-level traffic control.

### cert-manager

[cert-manager](https://cert-manager.io/) must be installed for automatic TLS certificate provisioning. The control plane auto-detects cert-manager at startup. If absent, TLS certificates must be provisioned manually via the certgen job.

```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

### Gateway API CRDs

The [Gateway API](https://gateway-api.sigs.k8s.io/) CRDs must be installed if external routing via GRPCRoute is desired. The control plane auto-detects Gateway API availability at startup and skips route provisioning if the CRDs are not present.

```shell
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/latest/download/standard-install.yaml
```

### Shared Gateway

A pre-existing Gateway resource must be provisioned by an administrator before the control plane can create tenant GRPCRoutes. The Gateway should use a wildcard hostname and a cert-manager-issued wildcard TLS certificate. All tenant GRPCRoutes attach to this shared Gateway.

See `deploy/openshift/infrastructure/GATEWAY-SETUP.md` for step-by-step setup instructions including cert-manager configuration.

The Gateway name is configured via the `GATEWAY_API_GATEWAY_NAME` environment variable (required).

### Trusted CA bundle (optional)

If the gateway needs to interact with an OIDC issuer (e.g., Keycloak) that uses a self-signed or private CA certificate, create a ConfigMap named `gateway-trusted-ca` in the control plane namespace (default: `hypershell`). The control plane copies this ConfigMap into each tenant namespace and mounts it into gateway pods so they can validate the issuer's TLS certificate when fetching JWKS keys or verifying tokens.

```shell
kubectl -n hypershell create configmap gateway-trusted-ca --from-file=ca-bundle.crt=/path/to/ca.crt
```

### Keycloak OIDC client provisioning (`hypershell-keycloak-admin`)

The control plane provisions an OIDC client in Keycloak for each gateway it reconciles. It authenticates to Keycloak using a confidential client whose credentials are read from a Secret named `hypershell-keycloak-admin` in the control plane namespace (default: `hypershell`). If this Secret is absent at startup, Keycloak integration is silently disabled for the lifetime of that pod.

#### 1. Create a realm

Log in to the Keycloak Admin Console and create a realm for HyperShell (e.g., `hypershell`), or use an existing realm. You can also do this via the REST API:

```shell
KEYCLOAK_URL="https://keycloak.example.com"
ADMIN_TOKEN=$(curl -s -X POST "$KEYCLOAK_URL/realms/master/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=admin-cli&username=admin&password=<admin-password>&grant_type=password" \
  | jq -r '.access_token')

curl -s -X POST "$KEYCLOAK_URL/admin/realms" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"realm": "hypershell", "enabled": true, "displayName": "HyperShell"}'
```

#### 2. Create a confidential client

Create a client named `hypershell-control-plane` in the realm. It must use service-account authentication (no standard or direct-grant flows):

```shell
curl -s -X POST "$KEYCLOAK_URL/admin/realms/hypershell/clients" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "clientId": "hypershell-control-plane",
    "name": "HyperShell Control Plane",
    "enabled": true,
    "clientAuthenticatorType": "client-secret",
    "serviceAccountsEnabled": true,
    "standardFlowEnabled": false,
    "directAccessGrantsEnabled": false,
    "publicClient": false
  }'
```

#### 3. Grant realm-management roles to the service account

The control plane needs `manage-clients`, `manage-users`, and `view-users` from the built-in `realm-management` client so it can create and delete gateway OIDC clients:

```shell
# Retrieve IDs
CLIENT_UUID=$(curl -s "$KEYCLOAK_URL/admin/realms/hypershell/clients?clientId=hypershell-control-plane" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.[0].id')
SA_USER_ID=$(curl -s "$KEYCLOAK_URL/admin/realms/hypershell/clients/$CLIENT_UUID/service-account-user" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.id')
RM_UUID=$(curl -s "$KEYCLOAK_URL/admin/realms/hypershell/clients?clientId=realm-management" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.[0].id')

# Fetch the three role objects and assign them
ROLES=$(curl -s "$KEYCLOAK_URL/admin/realms/hypershell/clients/$RM_UUID/roles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  | jq '[.[] | select(.name | IN("manage-clients","manage-users","view-users"))]')

curl -s -X POST "$KEYCLOAK_URL/admin/realms/hypershell/users/$SA_USER_ID/role-mappings/clients/$RM_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$ROLES"
```

#### 4. Create the Secret

Retrieve the generated client secret and create the Kubernetes Secret in the control plane namespace:

```shell
CLIENT_SECRET=$(curl -s "$KEYCLOAK_URL/admin/realms/hypershell/clients/$CLIENT_UUID/client-secret" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.value')

kubectl -n hypershell create secret generic hypershell-keycloak-admin \
  --from-literal=server-url="$KEYCLOAK_URL/" \
  --from-literal=realm="hypershell" \
  --from-literal=client-id="hypershell-control-plane" \
  --from-literal=client-secret="$CLIENT_SECRET"
```

If you need to rotate the client secret or update any value, delete and recreate the Secret then restart the control plane pod -- the Secret is read once at startup.

```shell
kubectl -n hypershell delete secret hypershell-keycloak-admin
# recreate with updated values, then:
kubectl -n hypershell rollout restart deployment/hypershell-control-plane
```

Confirm the control plane picked up the configuration:

```shell
kubectl -n hypershell logs deployment/hypershell-control-plane | grep -i keycloak
# Expected: INFO keycloak integration enabled: server=... realm=hypershell
```

### Base domain (`GATEWAY_API_BASE_DOMAIN`)

The control plane requires `GATEWAY_API_BASE_DOMAIN` to derive GRPCRoute hostnames for tenant gateways. Without it, GRPCRoute creation is skipped and gateways will not be externally reachable.

On OpenShift, look up the cluster's default base domain:

```shell
oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}'
```

This typically returns a value like `apps.<cluster-name>.<base-domain>`. Set this value as `GATEWAY_API_BASE_DOMAIN` on the controller deployment:

```shell
oc set env deployment/hypershell-controller -n hypershell \
  GATEWAY_API_BASE_DOMAIN="$(oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}')"
```

Or edit `components/api-server/deploy/openshift/controller.yaml` and replace the placeholder value before applying.

### Control plane environment variables

| Variable | Default | Description |
|---|---|---|
| `HYPERSHELL_GRPC_SERVER_ADDR` | `localhost:9000` | gRPC address of the API server |
| `HYPERSHELL_API_SERVER_URL` | `http://localhost:8000` | HTTP address of the API server |
| `HYPERSHELL_NAMESPACE` | `hypershell` | Namespace the control plane runs in (used for trusted CA bundle source) |
| `GATEWAY_API_GATEWAY_NAME` | *(required)* | Name of the pre-existing Gateway resource that tenant GRPCRoutes attach to |
| `GATEWAY_API_GATEWAY_NAMESPACE` | `openshift-ingress` | Namespace where the pre-existing Gateway resource lives |
| `GATEWAY_API_BASE_DOMAIN` | *(none)* | Base domain for tenant hostname generation (e.g., `openshell.example.com` → `gw-<ns>.openshell.example.com`) |
| `GATEWAY_MANIFESTS_DIR` | `/manifests/gateway` | Path to gateway manifest templates |

## Observability

### Prometheus metrics

The API server exposes Prometheus metrics on its metrics port (default `:8080/metrics`).

| Metric | Type | Description |
|---|---|---|
| `hypershell_gateways_running` | Gauge | Current number of running gateways. Queried live from the database on each scrape. |

### Grafana dashboard

A pre-built Grafana dashboard is provided at `dashboards/hypershell-dashboard.yml`. It is packaged as a Kubernetes ConfigMap with the `grafana_dashboard: "true"` label so it is picked up automatically by the Grafana sidecar.

The dashboard includes:

- **Running Gateways** — stat panel showing the current gateway count
- **Running Gateways Over Time** — timeseries graph of gateway count over the selected time range

To deploy the dashboard, apply the ConfigMap to the namespace where Grafana is running:

```shell
kubectl apply -f dashboards/hypershell-dashboard.yml
```
