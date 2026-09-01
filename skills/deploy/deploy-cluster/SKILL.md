---
name: deploy-cluster
description: >
  Deploy HyperShell platform services (API server, controller, PostgreSQL) to an
  OpenShift cluster with full OIDC/JWT security, CNPG-managed databases, and
  per-tenant ingress via Routes. Covers Keycloak deployment, credential bootstrapping,
  image builds, kustomize deployment, and troubleshooting. Use when: "deploy to openshift",
  "deploy platform", "install hypershell", "bootstrap keycloak", or "openshift deploy".
---

# HyperShell OpenShift Deployment

> **Scope:** this skill deploys the **complete platform** (Keycloak, API server,
> controller, CNPG-managed PostgreSQL) with OIDC/JWT security, and is cloud-agnostic
> across OpenShift distributions (ROSA, ROKS, OSD, self-managed). It does **not**
> provision tenant-gateway ingress (Routes or Gateway API); that is handled by the
> control plane at runtime once a gateway resource is created. This is the **canonical
> OpenShift baseline**; cloud-specific overlays (`deploy/ibm`, `deploy/openshift`) layer
> additional requirements on top (e.g. image mirroring for ROKS, where worker egress is
> restricted).
>
> **Prerequisites:** cert-manager must be pre-installed on the cluster (required for
> per-tenant gateway TLS). The control plane reconciliation fails closed without it.
> On OSD/GCP, install via OperatorHub; on ROKS, mirror and apply manually (see
> [`ibm-cluster`](../ibm-cluster/SKILL.md) §5.1).

## Step 1: Create the HyperShell namespace

```bash
oc new-project hypershell-system
```

This namespace houses the API server, controller, web console, and a shared PostgreSQL
cluster. All tenant gateways land in their own namespaces (created by the controller).

## Step 2: Deploy Keycloak with the hypershell realm

Keycloak is the OIDC identity provider for the entire platform. The realm import includes
fixed, deterministic client credentials that the API server and controller use.

**Determine your cluster's base domain:**

```bash
BASE_DOMAIN="apps.<cluster-id>.openshiftapps.com"    # OSD (GCP/AWS)
# or
BASE_DOMAIN=$(oc get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.status.domain}')   # ROKS/self-managed
```

**Deploy Keycloak with domain substitution:**

```bash
cd deploy/keycloak
oc kustomize . | sed "s/apps.example.com/$BASE_DOMAIN/g" | oc apply -f -

# Wait for Keycloak to be ready
oc -n keycloak rollout status deploy/keycloak
```

**Verify OIDC discovery:**

```bash
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
curl -sk "$KC_URL/realms/hypershell/.well-known/openid-configuration" | \
  python3 -c "import json,sys; d=json.load(sys.stdin); print('Issuer:', d['issuer'])"
# expect: Issuer: https://keycloak-keycloak.<BASE_DOMAIN>/realms/hypershell
```

## Step 3: Create the `hypershell-api-config` Secret

The API server and controller require a Secret (`hypershell-api-config`) containing
Keycloak client credentials and OIDC URLs. The realm import pre-defines fixed client IDs
and secrets for both.

```bash
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
ISSUER_URL="$KC_URL/realms/hypershell"

# The realm import defines these clients with fixed credentials:
# - hypershell-control-plane / control-plane-secret
# - hypershell-provisioner / provisioner-secret
# Extract the JWK Certificate URL from the realm's OIDC config:
JWK_CERT_URL="$ISSUER_URL/protocol/openid-connect/certs"

# Create the Secret in hypershell-system namespace
oc -n hypershell-system create secret generic hypershell-api-config \
  --from-literal=api-service.issuerUrl="$ISSUER_URL" \
  --from-literal=api-service.clientId=hypershell-control-plane \
  --from-literal=api-service.clientSecret=control-plane-secret \
  --from-literal=api-service.jwkCertUrl="$JWK_CERT_URL"

# Verify
oc -n hypershell-system get secret hypershell-api-config
```

**Without this Secret, pods will fail with `CreateContainerConfigError`** (the API server
and controller both reference its keys in their `env` blocks).

## Step 4: Build platform images (optional - use `:latest` for production)

For development, build and tag images locally:

```bash
cd components/api-server
make build-api-server       # or: make build-all (builds both api-server and controller)
make build-controller

# Local image tags are:
# localhost/hypershell:dev
# localhost/hypershell-controller:dev
```

For production, the `deploy/openshift/` overlay references published `:latest` images from
the Red Hat registry (`quay.io/redhat-services-prod/...`).

## Step 5: (Optional) Push local images to cluster registry

If using locally-built `:dev` images, push them to the internal registry (not needed for
production, which uses published images):

```bash
REGISTRY=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')
podman login --tls-verify=false -u $(oc whoami) -p $(oc whoami -t) $REGISTRY

podman tag localhost/hypershell:dev $REGISTRY/hypershell-system/hypershell:dev
podman push --tls-verify=false $REGISTRY/hypershell-system/hypershell:dev

podman tag localhost/hypershell-controller:dev $REGISTRY/hypershell-system/hypershell-controller:dev
podman push --tls-verify=false $REGISTRY/hypershell-system/hypershell-controller:dev
```

## Step 6: Deploy platform services via kustomize

```bash
oc kustomize deploy/openshift/ --load-restrictor LoadRestrictionsNone | oc apply -f -
```

This creates:
- Namespace: `hypershell-system`
- ServiceAccounts: `hypershell-api-server`, `hypershell-controller`, `hypershell-web-console`
- CNPG `Cluster` resource: `hypershell-db` (PostgreSQL, managed by CloudNativePG)
- Deployments: `hypershell-api-server`, `hypershell-controller`, `hypershell-web-console`
- Services: internal ClusterIP for gRPC/HTTP
- Route: `hypershell-api` with TLS edge termination
- RBAC: cluster roles and bindings for the controller
- Cert-manager Issuers and Certificates for per-tenant PKI

**Important:** the checked-in `deploy/openshift/kustomization.yaml` contains a placeholder
value for `GATEWAY_API_BASE_DOMAIN` (set to `openshell.stage.example.com`). This is
**only used if Gateway API ingress mode is enabled** (via setting `GATEWAY_INGRESS_MODE=gateway-api`).
For Route mode (the default in this overlay), the controller auto-generates per-tenant
`Route`s without needing this parameter. If you later switch to Gateway API mode via
configuration changes, update this placeholder to match your actual cluster domain (or
override it via a kustomize patch / `oc set env`).

## Step 7: Wait for platform rollout

```bash
oc -n hypershell-system rollout status deploy/hypershell-api-server --timeout=300s
oc -n hypershell-system rollout status deploy/hypershell-controller --timeout=300s
oc -n hypershell-system rollout status deploy/hypershell-web-console --timeout=300s
```

Check that PostgreSQL cluster is healthy:

```bash
oc -n hypershell-system get cluster hypershell-db
# expect: Status.Phase = "Cluster in healthy state"
```

## Step 8: Verify platform is working

```bash
API="https://$(oc -n hypershell-system get route hypershell-api -o jsonpath='{.spec.host}')/api/hypershell/v1"

# Health check (no auth required)
curl -sk "$API/healthcheck"
# expect: HTTP 200, empty response

# API access (requires JWT from Keycloak) this will 401 without a token:
curl -sk -w '%{http_code}\n' "$API/managed_clusters"
# expect: 401
```

Obtain a JWT token from Keycloak to verify authenticated access:

```bash
KC_URL="https://keycloak-keycloak.$BASE_DOMAIN"
TOKEN=$(curl -sk -X POST "$KC_URL/realms/hypershell/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=hypershell-frontend" \
  -d "username=admin" \
  -d "password=admin" | \
  python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

# Authenticated API call
curl -sk -H "Authorization: Bearer $TOKEN" -w '%{http_code}\n' "$API/managed_clusters"
# expect: 200, empty list
```

## Step 9: Deploy the web console

The web console is a separate OIDC-enabled React app that connects to the API server.

```bash
oc -n hypershell-system set env deploy/hypershell-web-console \
  OIDC_ISSUER="https://keycloak-keycloak.$BASE_DOMAIN/realms/hypershell" \
  OIDC_CLIENT_ID="hypershell-frontend" \
  OIDC_REDIRECT_URI="https://$(oc -n hypershell-system get route hypershell-web-console -o jsonpath='{.spec.host}')/auth/callback" \
  OIDC_POST_LOGOUT_REDIRECT_URI="https://$(oc -n hypershell-system get route hypershell-web-console -o jsonpath='{.spec.host}')" \
  SESSION_SECRET="$(openssl rand -hex 32)"

oc -n hypershell-system rollout status deploy/hypershell-web-console --timeout=120s
```

## Step 10: Access the platform

```bash
CONSOLE_URL="https://$(oc -n hypershell-system get route hypershell-console -o jsonpath='{.spec.host}')"
echo "Open in browser: $CONSOLE_URL"
# Login with: admin / admin (default Keycloak credentials)
```

## Gateway Image Environment Variables

The controller uses two **required** environment variables (set in `deploy/base/controller.yaml`)
to provision tenant gateways. These have **no fallback defaults** if unset, gateway
reconciliation fails immediately:

```yaml
env:
  - name: GATEWAY_IMAGE
    value: quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0@sha256:a80b...
  - name: GATEWAY_SUPERVISOR_IMAGE
    value: quay.io/opendatahub/odh-openshell-supervisor:v0.0.109-rhaiv.0@sha256:96e...
```

When a gateway is created, the controller reads these environment variables to determine
which image(s) to deploy. To update to a new upstream OpenShell release, update these
values in `deploy/base/controller.yaml` and reapply the deployment.

## OpenShift-Specific Differences from Kind

| Concern | Kind | OpenShift |
|---------|------|-----------|
| Image registry | `localhost/` with `imagePullPolicy: Never` | Internal or external registry with `imagePullPolicy: IfNotPresent\|Always` |
| Namespace | `hypershell` | `hypershell-system` |
| External access | NodePort on 30080 | Route with TLS edge termination |
| PostgreSQL | StatefulSet with `postgres:13` (in-cluster) | CNPG `Cluster` resource (`hypershell-db`, auto-managed) |
| PostgreSQL storage | `emptyDir` | PersistentVolume via cluster StorageClass |
| OIDC | mock / disabled | Keycloak-backed, JWT required for all API access (except `/healthcheck` and OpenAPI) |
| Auth | disabled (dev) | OIDC/JWT enabled by default; RBAC enforced on API server |
| Controller ingress mode | hardcoded to `route` | Configurable via `GATEWAY_INGRESS_MODE` (default: `route`) |

## Troubleshooting

### Pods stuck in `CreateContainerConfigError`

**Cause:** The `hypershell-api-config` Secret is missing or has incorrect keys.

**Fix:** verify the Secret exists and has the correct keys:

```bash
oc -n hypershell-system get secret hypershell-api-config -o json | jq '.data | keys'
# expect: ["api-service.clientSecret", "api-service.clientId", "api-service.issuerUrl", "api-service.jwkCertUrl"]
```

If missing, re-create it (see Step 3).

### API server logs show JWT verification failure

```
error: failed to validate JWT: invalid issuer URL
```

**Cause:** The `hypershell-api-config` Secret's `api-service.issuerUrl` does not match the
actual Keycloak issuer, or Keycloak is not reachable from the pod.

**Fix:** verify the issuer URL matches (it should be `https://keycloak-keycloak.<BASE_DOMAIN>/realms/hypershell`):

```bash
oc -n hypershell-system get secret hypershell-api-config -o jsonpath='{.data.api-service\.issuerUrl}' | base64 -d
```

Verify pods can reach Keycloak:

```bash
oc -n hypershell-system exec -it deployment/hypershell-api-server -- \
  curl -sk https://keycloak-keycloak.<BASE_DOMAIN>/realms/hypershell/.well-known/openid-configuration
```

### Controller pod logs show `cert-manager is required but not available`

**Cause:** cert-manager is not installed on the cluster, or not ready.

**Fix:** verify cert-manager is running:

```bash
oc get namespace cert-manager
oc -n cert-manager get pods
```

If cert-manager is not installed, install it before deploying HyperShell (see scope note above).

### Database cluster stuck in `Creating` or `Waiting`

**Cause:** PersistentVolumes are not available, or the CNPG operator is not installed.

**Fix:** verify the CNPG operator is running:

```bash
oc get crd clusters.postgresql.cnpg.io
oc get namespace cnpg-system
```

Check PersistentVolume availability:

```bash
oc get pv
oc get pvc -n hypershell-system
# all PVCs should be Bound
```

If PVCs are `Pending`, the cluster has insufficient storage resources.

### Init container `migrate` fails with database connection error

**Cause:** PostgreSQL is not ready, or credentials are incorrect.

**Fix:** check PostgreSQL cluster health:

```bash
oc -n hypershell-system describe cluster hypershell-db
oc -n hypershell-system logs -l postgresql=hypershell-db
```

Verify the Secret `hypershell-db-app` exists and has correct credentials:

```bash
oc -n hypershell-system get secret hypershell-db-app -o json | jq '.data | keys'
# expect: ["dbname", "host", "password", "port", "user"]
```

### Old Keycloak password not updating

Keycloak uses a backing H2 in-memory database (`start-dev` mode). Pod restarts reset the
database, so any admin password changes are lost. See `deploy/keycloak/kustomization.yaml`
TODO note: switching to persistent PostgreSQL is a follow-up for production use.

## Teardown

```bash
# Delete HyperShell platform
oc delete project hypershell-system

# Delete shared Keycloak (if not serving other platforms on this cluster)
oc delete project keycloak
```

## Cloud-Hub Parameter Overrides

The steps above use generic OpenShift defaults. On specific cloud providers, override:

| Parameter | ROSA / AWS | ROKS / IBM | OSD / GCP |
|-----------|-----------|-----------|----------|
| Cluster login | `oc login https://api.<cluster>.<id>.us-west-2.amaoznamazonaws.com:6443` | `ibmcloud oc cluster config -c <cluster>` | `oc login https://api.<cluster>.<id>.openshiftapps.com:6443` |
| Base domain | `apps.<cluster>.<id>.us-west-2.amazonamazonaws.com` | `<ingress-subdomain>` (IBM-provided) | `apps.<cluster>.<id>.openshiftapps.com` |
| Ingress mode | `gateway-api` (via `cloud-hub-ingress-bootstrap`) | `route` (via `deploy/ibm` overlay + **image mirroring**) | `route` (direct, no mirroring) |
| Cert-manager | Install via OperatorHub | Mirror + apply manually (ROKS OperatorHub broken) | Install via OperatorHub |
| Image sourcing | Build or use `:latest` | **Mirror all images** into internal registry (nodes isolated) | Use published `:latest` (full egress) |
| Tenant database | CNPG (this skill) | CNPG (this skill) + mirror PostgreSQL image | CNPG (this skill) |
| Namespace | `hypershell-system` (same) | `hypershell-system` (same) | `hypershell-system` (same) |
