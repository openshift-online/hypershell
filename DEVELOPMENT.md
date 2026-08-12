# Local Development Environment

HyperShell provides a single-command local development environment using
[Kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) clusters. The
environment deploys all platform components -- API server, control plane, and
web console -- so developers can test changes end-to-end without external
infrastructure.

## Prerequisites

| Tool | Purpose | Install |
|------|---------|---------|
| [Docker](https://docs.docker.com/get-docker/) or [Podman](https://podman.io/docs/installation) | Container engine | OS package manager |
| [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) | Local Kubernetes clusters | `brew install kind` |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | Kubernetes CLI | `brew install kubectl` |
| [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind) | LoadBalancer + Gateway API for Kind | `brew install cloud-provider-kind` |

The container engine is auto-detected (Podman preferred). Override with
`CONTAINER_ENGINE=docker` or `CONTAINER_ENGINE=podman`.

## Quickstart

```bash
make kind-up
```

This creates a Kind cluster and deploys:

1. Gateway API CRDs (experimental channel, includes BackendTLSPolicy)
2. cloud-provider-kind (LoadBalancer + Gateway API controller)
3. cert-manager (TLS certificate lifecycle)
4. Keycloak (OIDC identity provider)
5. Networking Gateway with wildcard TLS certificates
6. HTTPRoutes for all services
7. API server (with DB migration init container)
8. Control plane (gRPC watcher + reconciler, provisions PostgreSQL)
9. Web console (Node.js BFF + React SPA)

### Expected Output

```
=== HyperShell is running! ===

  HTTP API:      https://api.hypershell.localhost
  Web Console:   https://console.hypershell.localhost
  Health:        https://health.hypershell.localhost
  Keycloak:      https://keycloak.hypershell.localhost (admin/admin)
  Keycloak HTTP: http://keycloak.hypershell.localhost:8080 (admin/admin)
  OIDC Issuer:   http://keycloak.hypershell.localhost:8080/realms/hypershell
```

Services are accessed via `.localhost` hostnames routed through the networking
Gateway. CoreDNS resolves all `*.hypershell.localhost` to loopback, and
OS-level port forwarding (pfctl on macOS, iptables on Linux) redirects
host ports 443 and 8080 to cloud-provider-kind's ephemeral Gateway ports.
The TLS certificate is self-signed -- trust it in your browser or use
`curl --cacert`.

## Per-Component Swap

Baseline images are pulled from the container registry. To build all baseline
images locally instead (e.g. when registry access is unavailable), run:

```bash
LOCAL_IMAGES=true make kind-up
```

To test local changes, swap individual components:

```bash
# Build and deploy API server from working tree
make kind-api-server-up

# Build and deploy control plane from working tree
make kind-control-plane-up

# Start web console with hot reload (default)
make kind-web-console-up

# Start web console with full image rebuild
KIND_HOT_RELOAD=false make kind-web-console-up
```

The web console uses hot reload by default - `kind-web-console-up` starts a
local Vite dev server and proxies through the cluster. Use
`KIND_HOT_RELOAD=false` to build and deploy a full container image instead.
API server and control plane rebuilds replace the running deployment; re-run
after making changes to pick them up.

### Revert to Baseline

```bash
make kind-api-server-down
make kind-control-plane-down
make kind-web-console-down
```

Reverts the component to the registry baseline image. No-op if the component
is already running the baseline.

### Swap Status

```bash
make kind-status
```

Shows which components are running local builds vs. baseline images. Swap state
is tracked in `.kind-swaps` (gitignored).

## Hot Reload

The web console supports hot reload by default. When you run
`make kind-web-console-up`, the host source directory is mounted into the
container and `npm run dev` runs in development mode. File changes on the host
are reflected immediately without rebuilding.

To disable hot reload and use a full image rebuild instead:

```bash
KIND_HOT_RELOAD=false make kind-web-console-up
```

Hot reload is only supported for the web console. The API server and control
plane are Go services that require a full rebuild (`make kind-api-server-up` /
`make kind-control-plane-up`).

## Keycloak

The local Keycloak instance mirrors the downstream Keycloak topology used in
production.

| Setting | Value |
|---------|-------|
| Realm | `hypershell` |
| Frontend client | `hypershell-frontend` (public, standard flow + direct access grants) |
| Provisioner client | `hypershell-provisioner` (confidential, service account) |
| Control plane client | `hypershell-control-plane` (confidential, service account, client_credentials) |
| Admin user | `admin` / `admin` (role: `hypershell-admins`) |
| Developer user | `developer` / `developer` (role: `hypershell-users`) |
| OIDC Issuer URL | `http://keycloak.hypershell.localhost:8080/realms/hypershell` |
| Admin Console | `http://keycloak.hypershell.localhost:8080/admin/` |

### OIDC

Keycloak is configured with `KC_HOSTNAME=http://keycloak.hypershell.localhost:8080`,
which sets the OIDC issuer and all frontend URLs to use plain HTTP on port 8080.
The networking Gateway has a dedicated HTTP listener (`http-keycloak`) on port
8080 scoped to `keycloak.hypershell.localhost`. This avoids TLS/CA-trust
complexity and means the same OIDC issuer URL works from both the host browser
and in-cluster pods (cluster CoreDNS is patched to resolve
`*.hypershell.localhost` to the Gateway LB IP).

The control plane authenticates to the API server's gRPC endpoint using its own
Keycloak service account (`hypershell-control-plane` client, confidential,
`client_credentials` grant). `make kind-up` creates a `hypershell-cp-oidc`
secret and patches the control plane deployment with `OIDC_ISSUER`,
`OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`. When swapped locally, export those
variables in your shell before running the control plane binary.

Port forwarding (pfctl/iptables) maps host port 8080 to the Gateway's ephemeral
HTTP port. If port forwarding is not active (e.g. after a cluster restart),
re-establish it with:

```bash
make kind-fix-ports
```

Verify the OIDC discovery endpoint:

```bash
curl http://keycloak.hypershell.localhost:8080/realms/hypershell/.well-known/openid-configuration
```

### External Keycloak

To test against a shared downstream Keycloak instead of the local instance:

```bash
KIND_KEYCLOAK_URL=https://keycloak.example.com/realms/hypershell make kind-up
```

This skips the local Keycloak deployment and points the gateway OIDC issuer at
the external URL.

## OIDC Authentication (opt-in)

By default, the Kind cluster runs without OIDC authentication: the API server
disables JWT validation and the web console serves pages without requiring login.
Enable OIDC to test the full authentication flow end-to-end:

```bash
KIND_ENABLE_OIDC=true make kind-up
```

### What changes when OIDC is enabled

| Component | Default (no OIDC) | With OIDC |
|-----------|-------------------|-----------|
| API server | `--enable-jwt=false` | `API_ENV=development_oidc`, JWK cert URL configured |
| Web console | No session, no login | `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `SESSION_SECRET` configured |
| Gateway seed | Fleet, cluster, release, DB only | Also creates a Gateway with OIDC config |

Keycloak deploys in both modes. OIDC mode patches the API server and web console
deployments at runtime (the base YAML manifests are unchanged).

### Browser login flow

1. Navigate to `https://console.hypershell.localhost`
2. The BFF redirects to `https://console.hypershell.localhost/auth/login`
3. The login page redirects to Keycloak for authentication
4. Sign in with `admin`/`admin` or `developer`/`developer`
5. Keycloak redirects back to the web console with a valid session

### Hot reload and OIDC

Web console hot reload (`make kind-web-console-up`) runs the Vite dev server
directly on the host for fast iteration. This mode does **not** start the BFF,
so OIDC authentication is unavailable during hot reload. Use the image-based
swap when testing OIDC:

```bash
KIND_HOT_RELOAD=false make kind-web-console-up
```

### CLI token acquisition for curl testing

Obtain an access token via Keycloak's direct access grants:

```bash
TOKEN=$(curl -s -X POST \
  "http://keycloak.hypershell.localhost:8080/realms/hypershell/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=hypershell-frontend" \
  -d "username=admin" \
  -d "password=admin" | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

curl -s -H "Authorization: Bearer ${TOKEN}" \
  https://api.hypershell.localhost/api/hypershell/v1/fleets
```

## Private Registry Pull Secret

If your baseline images live in a private registry, provide a pull secret:

```bash
KIND_PULL_SECRET=/path/to/pull-secret.yaml make kind-up
```

The YAML file is applied into the target namespace with `kubectl apply`. It
should contain a `kubernetes.io/dockerconfigjson` Secret.

## Offline Development

Build all component images from the local working tree instead of pulling from
the container registry:

```bash
LOCAL_IMAGES=true make kind-up
```

This builds api-server, control-plane, and web-console images locally and loads
them into Kind. The Dockerfiles drop the local `rh-trex-ai` replace directive at
build time, so no external dependency checkout is needed.

## Cluster Lifecycle

```bash
make kind-up        # Create cluster + deploy everything
make kind-down      # Remove namespace and its resources
make kind-teardown  # Destroy Kind cluster entirely
make kind-status    # Show cluster info, pods, services, swap state
```

`make kind-up` is idempotent -- running it again on an existing cluster
reapplies manifests and waits for readiness. Swapped components are preserved.

## Environment Variable Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `KIND_CLUSTER_NAME` | `hypershell-dev` | Kind cluster name |
| `KIND_NAMESPACE` | `hypershell-system` | Target namespace for swap/teardown |
| `KIND_HOT_RELOAD` | `true` | Hot reload for web console |
| `KIND_HOST_MOUNT_PATH` | Repository root | Host directory mounted into Kind nodes |
| `KIND_KEYCLOAK_URL` | (unset) | External Keycloak URL; skips local deploy |
| `KEYCLOAK_OIDC_ISSUER` | `http://keycloak.hypershell.localhost:8080/realms/hypershell` | OIDC issuer URL |
| `KIND_ENABLE_OIDC` | (unset) | Set to `true` to enable OIDC authentication across all components |
| `KIND_PULL_SECRET` | (unset) | Path to pull secret YAML for private registries |
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` | Container registry |
| `IMAGE_TAG` | `latest` | Image tag for baseline images |
| `LOCAL_IMAGES` | (unset) | Set to `true` for offline baseline builds |
| `CONTAINER_ENGINE` | Auto-detected | `podman` or `docker` |
| `GATEWAY_API_VERSION` | `v1.5.1` | Gateway API CRD version |
| `CLOUD_PROVIDER_KIND_VERSION` | `v0.11.1` | cloud-provider-kind version |
| `CERT_MANAGER_VERSION` | `v1.21.1` | cert-manager version |
| `KIND_DB_IMAGE` | `registry.access.redhat.com/hi/postgresql:18.4@sha256:9b19...` | Database image for Gateway; override for OSS dev |
| `KIND_NO_SUDO` | (unset) | Set to `true` to skip sudo operations |
| `KIND_DNS_PORT` | `5553` | Host port for CoreDNS container |

## Make Targets

| Target | Description |
|--------|-------------|
| `make kind-up` | Create cluster + prerequisites + deploy + wait |
| `make kind-down` | Remove namespace and its resources |
| `make kind-teardown` | Destroy Kind cluster, stop cloud-provider-kind |
| `make kind-status` | Show cluster info, pods, services, swap state |
| `make kind-api-server-up` | Build + swap API server from working tree |
| `make kind-api-server-down` | Revert API server to baseline image |
| `make kind-control-plane-up` | Build + swap control plane from working tree |
| `make kind-control-plane-down` | Revert control plane to baseline image |
| `make kind-web-console-up` | Hot reload (default) or build + swap web console |
| `make kind-web-console-down` | Revert web console to baseline image |
| `make kind-fix-ports` | Re-establish host port forwarding (443 + 8080) |

## Gateway Access

The control plane provisions openshell-gateway pods that serve gRPC over TLS
using cert-manager-issued certificates. Accessing these gateways from the host
depends on the environment.

### Kind (port-forward)

In Kind, the simplest method is `kubectl port-forward`. This connects directly
to the gateway pod's TLS endpoint, bypassing the networking Gateway entirely:

```bash
kubectl --context kind-hypershell-dev port-forward \
  -n <gateway-namespace> svc/openshell-gateway 7443:8080 &
```

Then register the gateway with the openshell CLI:

```bash
openshell gateway add \
  --name my-gateway \
  --oidc-issuer http://keycloak.hypershell.localhost:8080/realms/hypershell \
  --oidc-client-id hypershell-frontend \
  https://localhost:7443
```

This opens a browser for Keycloak login. Use `admin`/`admin` or
`developer`/`developer`.

The OIDC issuer URL **must** be
`http://keycloak.hypershell.localhost:8080/realms/hypershell` (not the ephemeral
port). Keycloak embeds this URL as the `iss` claim in tokens, so the issuer
passed to `openshell gateway add` must match exactly. This requires host port
8080 to be forwarded -- if it isn't, run `make kind-fix-ports` first.

The e2e test (`components/pr-test/e2e-openshell.sh`) uses the same port-forward
fallback when no passthrough route is available.

### OpenShift (automatic)

On OpenShift, the control plane automatically creates networking resources when
a gateway has `Route.Enabled` set:

1. A per-namespace **Gateway** with `openshift-default` gateway class
2. A **GRPCRoute** routing to `openshell-gateway:8080`
3. A **BackendTLSPolicy** for TLS re-encryption to the backend (the OpenShift
   router terminates external TLS, then re-encrypts to the pod using the
   gateway's cert-manager CA)
4. A **NetworkPolicy** allowing ingress from `openshift-ingress` router pods

The gateway becomes reachable at
`grpcs://<openshell-gateway-NAMESPACE>.<GATEWAY_API_BASE_DOMAIN>:443` with no
port-forward needed. The control plane writes this address back to the API
server's `route_address` field.

### Why Kind requires port-forward

The networking Gateway's `*.gw.localhost` listener uses TLS Terminate mode,
which strips the external TLS and forwards plaintext to the backend. But
openshell-gateway pods expect TLS connections (they serve gRPC with their own
cert-manager certificates). On OpenShift this is solved with BackendTLSPolicy
(re-encryption), which cloud-provider-kind does not support.

### Creating a gateway with OIDC

`make kind-up` seeds Fleet, ManagedCluster, GatewayRelease, and ManagedDatabase
but does not create a Gateway. Create one via the API:

```bash
# Get the seeded resource IDs
FLEET_ID=$(curl -s http://localhost:8000/api/hypershell/v1/fleets | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")
CLUSTER_ID=$(curl -s http://localhost:8000/api/hypershell/v1/managed_clusters | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")
RELEASE_ID=$(curl -s http://localhost:8000/api/hypershell/v1/gateway_releases | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")
DATABASE_ID=$(curl -s http://localhost:8000/api/hypershell/v1/managed_databases | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")

# Create a gateway with OIDC
curl -s -X POST http://localhost:8000/api/hypershell/v1/gateways \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"dev-gateway\",
    \"fleet_id\": \"${FLEET_ID}\",
    \"cluster_id\": \"${CLUSTER_ID}\",
    \"release_id\": \"${RELEASE_ID}\",
    \"database_id\": \"${DATABASE_ID}\",
    \"namespace\": \"openshell-dev\",
    \"oidc\": \"{\\\"issuer\\\":\\\"http://keycloak.hypershell.localhost:8080/realms/hypershell\\\",\\\"audience\\\":\\\"hypershell-frontend\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}\"
  }"
```

Wait ~30s for the control plane to reconcile, then port-forward and register.

## Troubleshooting

### Container engine not running

```
Cannot connect to the Docker daemon
```

Start Docker Desktop or Podman:
```bash
# Docker
open -a Docker
# Podman
podman machine start
```

### Image pull failures

If the container registry is unreachable, use offline mode:
```bash
LOCAL_IMAGES=true make kind-up
```

### cloud-provider-kind not found

```bash
brew install cloud-provider-kind
# or
go install sigs.k8s.io/cloud-provider-kind@latest
```

### DNS resolution not working

`make kind-up` runs a CoreDNS container (`hypershell-dns`) for wildcard
`*.hypershell.localhost` resolution. If hostnames don't resolve:
```bash
# Check if the DNS container is running
docker ps --filter name=hypershell-dns

# Restart it
docker restart hypershell-dns

# Verify resolution
dig @127.0.0.1 -p 5553 api.hypershell.localhost
```

### OIDC discovery fails

```
Authentication failed: error sending request for url (http://keycloak.hypershell.localhost:8080/...)
```

Port 8080 is not forwarded to the Gateway's ephemeral HTTP port. Re-establish
port forwarding:

```bash
make kind-fix-ports
```

If you see an issuer mismatch error, you are likely using the ephemeral port
directly instead of port 8080. The OIDC issuer URL must always be
`http://keycloak.hypershell.localhost:8080/realms/hypershell` because Keycloak
embeds that URL in the token's `iss` claim.

### Keycloak admin console redirect loop

If the Keycloak admin console (`/admin/`) redirects in a loop, verify that the
`KC_HOSTNAME` env var in `deploy/kind/prerequisites/keycloak.yaml` is set to
`http://keycloak.hypershell.localhost:8080`. Restart the Keycloak deployment
after changes:

```bash
kubectl --context kind-hypershell-dev rollout restart deployment/keycloak -n keycloak
```

### Pods stuck in ImagePullBackOff

The baseline images require access to `quay.io`. If behind a firewall, use
offline mode or configure a pull secret:
```bash
# Offline: build from local working tree
LOCAL_IMAGES=true make kind-up

# Or: provide registry credentials
KIND_PULL_SECRET=/path/to/pull-secret.yaml make kind-up
```
