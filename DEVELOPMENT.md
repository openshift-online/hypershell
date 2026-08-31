# Local Development Environment

HyperShell provides a single-command local development environment. Kind
([Kind](https://kind.sigs.k8s.io/), Kubernetes in Docker) is the default
single-tenant path. OpenShift uses the same make-target pattern
(`make openshift-up`, `make openshift-<component>-up`) against an existing
cluster and an ephemeral namespace group. The environment deploys all platform
components -- API server, control plane, and web console -- so developers can
test changes end-to-end.

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
  OIDC Issuer:   https://keycloak.hypershell.localhost/realms/hypershell
```

Services are accessed via `.localhost` hostnames routed through the networking
Gateway. CoreDNS resolves all `*.hypershell.localhost` to loopback, and
OS-level port forwarding (pfctl on macOS, iptables on Linux) redirects
host port 443 to cloud-provider-kind's ephemeral Gateway port.
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
| CLI client | `hypershell-cli` (public, standard flow + device authorization grant, used by `hsctl login`) |
| Provisioner client | `hypershell-provisioner` (confidential, service account) |
| Control plane client | `hypershell-control-plane` (confidential, service account, client_credentials) |
| Admin user | `admin` / `admin` (role: `hypershell-admins`) |
| Developer user | `developer` / `developer` (role: `hypershell-users`) |
| OIDC Issuer URL | `https://keycloak.hypershell.localhost/realms/hypershell` |
| Admin Console | `https://keycloak.hypershell.localhost/admin/` |

### OIDC

Keycloak is configured with `KC_HOSTNAME=https://keycloak.hypershell.localhost`,
which pins the OIDC issuer and all frontend URLs to HTTPS on port 443 --
matching production's issuer form. Keycloak itself stays plain HTTP internally
(`KC_HTTP_PORT=8080`, `KC_PROXY_HEADERS=xforwarded`); the networking Gateway
terminates TLS on its `*.hypershell.localhost` :443 listener with the
cert-manager cert and forwards to `keycloak-service:8080`. The same OIDC issuer
URL works from both the host browser and in-cluster pods: cluster CoreDNS is
patched to resolve `*.hypershell.localhost` (including `keycloak`) to the
Gateway LB IP, so pods reach the issuer through the same HTTPS listener. Gateway
pods trust the self-signed CA via the `gateway-trusted-ca` ConfigMap
(`SSL_CERT_FILE`), which `make kind-up` publishes from the `hypershell-https-tls`
secret.

The control plane authenticates to the API server's gRPC endpoint using its own
Keycloak service account (`hypershell-control-plane` client, confidential,
`client_credentials` grant). `make kind-up` creates a `hypershell-cp-oidc`
secret and patches the control plane deployment with `OIDC_ISSUER`,
`OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`. When swapped locally, export those
variables in your shell before running the control plane binary.

Port forwarding (pfctl/iptables) maps host port 443 to the Gateway's ephemeral
HTTPS port. If port forwarding is not active (e.g. after a cluster restart),
re-establish it with:

```bash
make kind-fix-ports
```

Verify the OIDC discovery endpoint (the cert is self-signed, hence `--cacert`
or `-k`):

```bash
curl --cacert <(kubectl --context kind-hypershell-dev get secret hypershell-https-tls \
  -n hypershell-system -o go-template='{{index .data "ca.crt" | base64decode}}') \
  https://keycloak.hypershell.localhost/realms/hypershell/.well-known/openid-configuration
```

### External Keycloak

To test against a shared downstream Keycloak instead of the local instance:

```bash
KIND_KEYCLOAK_URL=https://keycloak.example.com/realms/hypershell make kind-up
```

This skips the local Keycloak deployment and points the gateway OIDC issuer at
the external URL.

## OIDC Authentication

The Kind cluster runs with OIDC authentication enabled. Keycloak is deployed as
the identity provider and all components are configured for JWT validation and
session management during `make kind-up`.

### Browser login flow

1. Navigate to `https://console.hypershell.localhost`
2. The BFF redirects to `https://console.hypershell.localhost/auth/login`
3. The login page redirects to Keycloak for authentication
4. Sign in with `admin`/`admin` or `developer`/`developer`
5. Keycloak redirects back to the web console with a valid session

### hsctl login (management API)

Build the CLI with `make build-cli`, then authenticate against the Kind cluster:

```bash
./components/cli/hsctl login \
  --url https://api.hypershell.localhost \
  --issuer-url https://keycloak.hypershell.localhost/realms/hypershell \
  --insecure
```

For headless environments, add `--no-browser` to use the device authorization flow.
The CLI stores tokens in `~/.config/hypershell/config.json` (or `~/.hypershell.json`
if that legacy path already exists) and uses the `hypershell-cli` Keycloak client.

Check identity with `hsctl whoami` and log out with `hsctl logout`.

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
TOKEN=$(curl -sk -X POST \
  "https://keycloak.hypershell.localhost/realms/hypershell/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=hypershell-frontend" \
  -d "username=admin" \
  -d "password=admin" | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

curl -s -H "Authorization: Bearer ${TOKEN}" \
  https://api.hypershell.localhost/api/hypershell/v1/gateways
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

## OpenShift Development

`make openshift-up` deploys the same stack into an isolated namespace group on
an OpenShift cluster selected by the current kubeconfig context. It does not
create the cluster. An administrator must already have provisioned the shared
Gateway, GatewayClass, certificate issuer, and wildcard certificate (see
`deploy/openshift/infrastructure/GATEWAY-SETUP.md`).

The platform namespace is the current oc project. Select it first, then bring
the environment up:

```bash
oc project alice
make openshift-up
```

`OPENSHIFT_NAMESPACE` overrides that project when you need to target a
namespace other than the one `oc project -q` reports:

```bash
OPENSHIFT_NAMESPACE=alice make openshift-up
```

The name must be a valid RFC 1123 DNS label of at most 54 characters so the
companion Keycloak namespace `${name}-keycloak` stays within the 63-character
limit. If no project is selected and `OPENSHIFT_NAMESPACE` is unset, the
command stops with an error.

`make openshift-up` deploys into the project you selected. It does not ask
for confirmation, and it does not require permission to label the namespace.
When the account can patch namespaces, the scripts stamp HyperShell ownership
labels. When it cannot, the scripts warn and continue. Namespaces that already
belong to a different HyperShell environment, and reserved names (`default`,
`kube-*`, `openshift-*`), are still refused.

The companion Keycloak project `${name}-keycloak` is created with
`oc new-project` when it does not exist (developers can ProjectRequest; they
typically cannot `oc create namespace`). The scripts switch to that project to
apply Keycloak, then switch back to the platform project for the rest of the
stack. OpenShift's default project NetworkPolicies only allow ingress from the
same namespace and from `openshift-ingress`, so the overlay also applies
`keycloak-allow-platform` in the Keycloak project. That policy lets the API
server load JWKS and the control plane call the Admin API over the in-cluster
Service.

This renders `kustomize build deploy/openshift/`, maps `hypershell-system` to
that platform namespace and `keycloak` to `${platform}-keycloak`, applies the
required cluster-scoped RBAC (shared ClusterRole `hypershell-e2e`, per-namespace
ClusterRoleBindings, and the privileged SCC RoleBinding), applies the
manifests (with prune scoped to this environment), registers the web-console
Route as the Keycloak `hypershell-frontend` redirect URI, seeds a
ManagedCluster, GatewayRelease, ManagedDatabase, and Gateway from this machine
against the API and Keycloak Routes (the API server image has no `curl`), and
prints the API, web-console, and Keycloak Routes. The gateway base domain is
read from the shared Gateway's listener hostname, not from
`GATEWAY_API_BASE_DOMAIN`. ClusterRole `hypershell-e2e` is shared by every e2e
environment on the cluster (`oc apply` creates or patches it). ClusterRoleBindings
are prefixed per project so they do not touch stage's `hypershell-controller`.
Cluster-scoped RBAC is not optional: without it the controller cannot create
gateway namespaces or sandboxes, so `make openshift-up` fails if `hypershell-e2e`
is missing and cannot be created. `make openshift-down` deletes this
environment's prefixed ClusterRoleBindings and leaves `hypershell-e2e` in place.

`make openshift-down` and `make openshift-teardown` are the same command.
There is no OpenShift cluster to destroy. Both delete the platform project
and the companion `${name}-keycloak` project. If project deletion is
forbidden, they delete HyperShell resources inside both projects (including
unlabeled Keycloak) and leave the projects. Labels are not required.

### Per-component swap

```bash
make openshift-api-server-up
make openshift-control-plane-up
make openshift-web-console-up
```

Each swap builds from the working tree, pushes an immutable image (commit +
namespace) to the OpenShift internal registry, and rolls out that exact
identity. Matching `-down` targets revert to the baseline registry image.
`make openshift-status` reports which components run a working-tree build
and the exact image each one uses. Swap state is tracked per namespace in
`.openshift-swaps/` (gitignored). A subsequent `make openshift-up` preserves
active swaps.

## Environment Variable Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `KIND_CLUSTER_NAME` | `hypershell-dev` | Kind cluster name |
| `KIND_NAMESPACE` | `hypershell-system` | Target namespace for swap/teardown |
| `KIND_HOT_RELOAD` | `true` | Hot reload for web console |
| `KIND_HOST_MOUNT_PATH` | Repository root | Host directory mounted into Kind nodes |
| `KIND_KEYCLOAK_URL` | (unset) | External Keycloak URL; skips local deploy |
| `KEYCLOAK_OIDC_ISSUER` | `https://keycloak.hypershell.localhost/realms/hypershell` | OIDC issuer URL |
| `KIND_PULL_SECRET` | (unset) | Path to pull secret YAML for private registries |
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` | Container registry |
| `IMAGE_TAG` | `latest` | Image tag for baseline images |
| `LOCAL_IMAGES` | (unset) | Set to `true` for offline baseline builds |
| `CONTAINER_ENGINE` | Auto-detected | `podman` or `docker` |
| `GATEWAY_API_VERSION` | `v1.5.1` | Gateway API CRD version |
| `CLOUD_PROVIDER_KIND_REPO` | `https://github.com/squizzi/cloud-provider-kind.git` | cloud-provider-kind git repo |
| `CLOUD_PROVIDER_KIND_BRANCH` | `hypershell` | cloud-provider-kind branch to build |
| `CERT_MANAGER_VERSION` | `v1.21.1` | cert-manager version |
| `KIND_DB_IMAGE` | `registry.access.redhat.com/hi/postgresql:18.4@sha256:9b19...` | Database image for Gateway; override for OSS dev |
| `KIND_NO_SUDO` | (unset) | Set to `true` to skip sudo operations |
| `KIND_DNS_PORT` | `5553` | Host port for CoreDNS container |
| `OPENSHIFT_NAMESPACE` | `oc project -q` | Override for the platform namespace. Unset, the current oc project is used. Max 54 chars; Keycloak lands in `${name}-keycloak`. |
| `GATEWAY_API_GATEWAY_NAME` | `openshell-grpc-gateway` | Pre-existing shared Gateway name |
| `GATEWAY_API_GATEWAY_NAMESPACE` | `openshift-ingress` | Namespace of the shared Gateway |
| `OPENSHIFT_IMAGE_REGISTRY` | `oc registry info` | Registry used to push swapped images |

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
| `make kind-fix-ports` | Re-establish host port forwarding (443) |
| `make openshift-up` | Deploy into an ephemeral OpenShift namespace group |
| `make openshift-down` | Remove the namespace group (platform project and `${name}-keycloak`) |
| `make openshift-teardown` | Same as `openshift-down` (OpenShift has no cluster to destroy) |
| `make openshift-status` | Show namespaces, pods, Routes, Gateway, swap state |
| `make openshift-api-server-up` | Build, push, and swap API server from working tree |
| `make openshift-api-server-down` | Revert API server to baseline image |
| `make openshift-control-plane-up` | Build, push, and swap control plane from working tree |
| `make openshift-control-plane-down` | Revert control plane to baseline image |
| `make openshift-web-console-up` | Build, push, and swap web console from working tree |
| `make openshift-web-console-down` | Revert web console to baseline image |

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
  --oidc-issuer https://keycloak.hypershell.localhost/realms/hypershell \
  --oidc-client-id hypershell-frontend \
  https://localhost:7443
```

This opens a browser for Keycloak login. Use `admin`/`admin` or
`developer`/`developer`.

The OIDC issuer URL **must** be
`https://keycloak.hypershell.localhost/realms/hypershell` (not the ephemeral
port). Keycloak embeds this URL as the `iss` claim in tokens, so the issuer
passed to `openshell gateway add` must match exactly. This requires host port
443 to be forwarded -- if it isn't, run `make kind-fix-ports` first.

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

### Gateway TLS in Kind

The networking Gateway's `*.gw.localhost` listener uses TLS Terminate mode,
which strips the external TLS and forwards plaintext to the backend.
openshell-gateway pods expect TLS connections (they serve gRPC with their own
cert-manager certificates). BackendTLSPolicy instructs the gateway
implementation to re-encrypt traffic to the backend. The `kind-prereqs` target
builds cloud-provider-kind from a fork that adds BackendTLSPolicy support,
so per-tenant gateways work without port-forward workarounds.

### Creating a gateway with OIDC

`make kind-up` seeds ManagedCluster, GatewayRelease, and ManagedDatabase
but does not create a Gateway. Create one via the API:

```bash
# Get the seeded resource IDs
CLUSTER_ID=$(curl -s http://localhost:8000/api/hypershell/v1/managed_clusters | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")
RELEASE_ID=$(curl -s http://localhost:8000/api/hypershell/v1/gateway_releases | python3 -c "import json,sys; print(json.load(sys.stdin)['items'][0]['id'])")

# Create a gateway with OIDC
curl -s -X POST http://localhost:8000/api/hypershell/v1/gateways \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"dev-gateway\",
    \"cluster_id\": \"${CLUSTER_ID}\",
    \"release_id\": \"${RELEASE_ID}\",
    \"database_id\": \"\",
    \"oidc\": \"{\\\"issuer\\\":\\\"https://keycloak.hypershell.localhost/realms/hypershell\\\",\\\"audience\\\":\\\"hypershell-frontend\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}\"
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
make kind-prereqs
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
Authentication failed: error sending request for url (https://keycloak.hypershell.localhost/...)
```

Host port 443 is not forwarded to the Gateway's ephemeral HTTPS port, or the
gateway pod does not trust the self-signed CA. Re-establish port forwarding:

```bash
make kind-fix-ports
```

If the gateway logs a TLS/certificate error, confirm the `gateway-trusted-ca`
ConfigMap exists in `hypershell-system` (published from `hypershell-https-tls`
by `make kind-up`); the reconciler mounts it into each gateway as
`SSL_CERT_FILE`.

If you see an issuer mismatch error, you are likely using the ephemeral port
directly instead of port 443. The OIDC issuer URL must always be
`https://keycloak.hypershell.localhost/realms/hypershell` because Keycloak
embeds that URL in the token's `iss` claim.

### Keycloak admin console redirect loop

If the Keycloak admin console (`/admin/`) redirects in a loop, verify that the
`KC_HOSTNAME` env var patched by `deploy/kind/kustomization.yaml` is set to
`https://keycloak.hypershell.localhost`. Restart the Keycloak deployment
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
