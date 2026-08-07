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

  HTTP API:     https://api.hypershell.localhost
  Web Console:  https://console.hypershell.localhost
  Health:       https://health.hypershell.localhost
  Keycloak:     https://keycloak.hypershell.localhost (admin/admin)
```

Services are accessed via `.localhost` hostnames routed through the networking
Gateway. CoreDNS resolves all `*.hypershell.localhost` to loopback, and
OS-level port forwarding (pfctl on macOS, iptables on Linux) redirects
host port 443 to cloud-provider-kind's ephemeral Gateway port. The TLS
certificate is self-signed — trust it in your browser or use `curl --cacert`.

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

The web console uses hot reload by default — `kind-web-console-up` starts a
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
| Admin user | `admin` / `admin` (role: `hypershell-admins`) |
| Developer user | `developer` / `developer` (role: `hypershell-users`) |

### External Keycloak

To test against a shared downstream Keycloak instead of the local instance:

```bash
KIND_KEYCLOAK_URL=https://keycloak.example.com/realms/hypershell make kind-up
```

This skips the local Keycloak deployment and points the gateway OIDC issuer at
the external URL.

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
| `KIND_PULL_SECRET` | (unset) | Path to pull secret YAML for private registries |
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` | Container registry |
| `IMAGE_TAG` | `latest` | Image tag for baseline images |
| `LOCAL_IMAGES` | (unset) | Set to `true` for offline baseline builds |
| `CONTAINER_ENGINE` | Auto-detected | `podman` or `docker` |
| `GATEWAY_API_VERSION` | `v1.5.1` | Gateway API CRD version |
| `CLOUD_PROVIDER_KIND_VERSION` | `v0.11.1` | cloud-provider-kind version |
| `CERT_MANAGER_VERSION` | `v1.21.1` | cert-manager version |

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

### Pods stuck in ImagePullBackOff

The baseline images require access to `quay.io`. If behind a firewall, use
offline mode or configure a pull secret:
```bash
# Offline: build from local working tree
LOCAL_IMAGES=true make kind-up

# Or: provide registry credentials
KIND_PULL_SECRET=/path/to/pull-secret.yaml make kind-up
```
