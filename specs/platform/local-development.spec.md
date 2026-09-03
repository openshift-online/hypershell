# Local Development Environment

**Date:** 2026-08-04
**Status:** Draft
**Jira:** ENGPROD-10281
**Related:** `openshift-development.spec.md` -- OpenShift lifecycle (`make openshift-up`)

## Purpose

HyperShell provides a single-command local development environment using Kind (Kubernetes in Docker) clusters. The environment deploys all platform components - API server, control plane, and web console - so developers can test changes end-to-end without external infrastructure. Database provisioning follows two paths: `kind-up` applies the static `hypershell-db` CNPG Cluster for the API server, while the ManagedDatabaseReconciler in the control plane provisions gateway databases dynamically. The tooling is idempotent: running it repeatedly converges to the desired `main` state without errors. For offline or air-gapped environments, `LOCAL_IMAGES=true` builds all images from the working tree instead of pulling from the registry. To build from `origin/main` instead (e.g. for baseline comparison), set `BUILD_SOURCE=baseline`.

Developers selectively swap individual components with local builds using per-component targets. The baseline cluster runs pre-built images pulled from the container registry; individual components are "swapped in" from local source as needed. Selective swapping converges to the current working tree state.

The same lifecycle model extends to OpenShift. The make target name selects the
infrastructure: `make kind-up` deploys to Kind (this spec) and `make openshift-up`
deploys the same components into an ephemeral OpenShift namespace group. Kind
targets dispatch through `scripts/cluster/` with `CLUSTER_DRIVER=kind`, wrapping
`scripts/kind/` so Kind behavior does not change. The per-component swap targets
follow the same pattern. `openshift-development.spec.md` specifies the OpenShift
lifecycle commands, the ephemeral-namespace model, cluster-scoped RBAC, and the
OpenShift e2e driver.

## Components Deployed

| Component | Kind | Image | Purpose |
|-----------|------|-------|---------|
| API Server | Deployment | Registry: `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main:latest` | REST + gRPC API, with init container for DB migrations |
| Control Plane | Deployment | Registry: `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-controller-main:latest` | gRPC watcher + reconciler for gateway lifecycle; provisions the database |
| Web Console | Deployment | Registry: `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-web-console-main:latest` | Browser-based management UI (Node.js); supports hot reload |

### Cluster Prerequisites

`make kind-up` SHALL install the following cluster-level prerequisites before deploying HyperShell components:

| Prerequisite | Purpose |
|--------------|---------|
| Gateway API CRDs | Gateway, GRPCRoute, BackendTLSPolicy, and related CRDs; required before cloud-provider-kind can serve as a gateway controller |
| cloud-provider-kind | LoadBalancer and Gateway API controller for Kind clusters; implements GatewayClass and serves as the data-plane proxy for GRPCRoute traffic |
| cert-manager | TLS certificate lifecycle for gateway certificates (issuance, renewal, rotation) |
| CloudNativePG operator | PostgreSQL database lifecycle; provides Cluster, Database, and DatabaseRole CRDs |
| CloudNativePG Cluster (hypershell-system) | PostgreSQL cluster for the API server in the platform namespace |
| Keycloak | OIDC identity provider for local gateway authentication testing (skipped when `KIND_KEYCLOAK_URL` is set) |

`make kind-up` SHALL install the Gateway API CRDs before starting cloud-provider-kind. The CRDs SHALL be applied from the upstream release bundle (`https://github.com/kubernetes-sigs/gateway-api/releases/download/<version>/experimental-install.yaml`) using the `experimental` channel, which includes BackendTLSPolicy. The version SHALL be pinned via a `GATEWAY_API_VERSION` variable. On OpenShift 4.19+ these CRDs ship by default; on Kind they must be installed explicitly.

[cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind) SHALL be started as a background process after the Gateway API CRDs are installed and the Kind cluster is created. It provides LoadBalancer service support and acts as a Gateway API controller, implementing the GatewayClass and proxying traffic for GRPCRoute resources. On macOS and Windows, where container IPs are not directly reachable from the host, cloud-provider-kind SHALL be started with `--enable-lb-port-mapping` to expose Gateway listeners on the host via ephemeral ports. The assigned port is not configurable - cloud-provider-kind allocates an ephemeral host port per listener ([kubernetes-sigs/cloud-provider-kind#417](https://github.com/kubernetes-sigs/cloud-provider-kind/issues/417)). `make kind-up` SHALL discover the mapped HTTPS port from the Docker proxy container (`kindccm-gw-*`) and set up OS-native port forwarding to redirect host port 443 to the ephemeral port (see Port Forwarding). Kind's `extraPortMappings` SHALL NOT be used for Gateway ports - they conflict with cloud-provider-kind's own `--publish` flags on the envoy container. cloud-provider-kind SHALL be built from a fork repository pinned via `CLOUD_PROVIDER_KIND_REPO` and `CLOUD_PROVIDER_KIND_REF` variables. `CLOUD_PROVIDER_KIND_REF` SHALL be an exact commit SHA (the tip of the fork's `hypershell` branch at pin time), mirroring the pinned `KIND_VERSION` pseudo-version so every developer builds the same deterministic binary rather than tracking a moving branch tip. `make kind-prereqs` SHALL fetch that commit by SHA, build the binary into `./bin/cloud-provider-kind`, record the built commit in `bin/.cloud-provider-kind.sha`, and `make kind-up` SHALL prepend `./bin/` to `PATH`. The build SHALL be idempotent by commit: the Go binary auto-stamps its source revision (`vcs.revision`), so `make kind-prereqs` compares the on-disk revision against `CLOUD_PROVIDER_KIND_REF` and skips the rebuild when it is already current. `CLOUD_PROVIDER_KIND_BRANCH` is an optional testing override (empty by default) that builds from a branch tip or arbitrary git ref instead of the pinned SHA; when set, `make kind-prereqs` always rebuilds and records the commit it resolved to.

The fork adds BackendTLSPolicy support (TLS re-encryption to backends) and per-cluster HTTP/2 protocol options for GRPCRoute backends, and advertises ALPN `h2` on the downstream (client-facing) TLS listener so HTTP/2 is negotiated during the TLS handshake; the fork also walks GRPCRoute-attached BackendTLSPolicy resources so gRPC backends are reached over re-encrypted HTTP/2. `make kind-up` SHALL run `make kind-prereqs` to ensure the binary is available before starting it. The process SHALL be stopped by `make kind-down`.

Because cloud-provider-kind publishes the gateway LoadBalancer on ephemeral host ports with no fixed mapping, restarting it republishes those ports and invalidates the pfctl/iptables rules, in-cluster DNAT, and CoreDNS entries pinned to the old ports. `make kind-up` SHALL therefore reuse an already-running cloud-provider-kind and keep its ports stable, restarting it only when the running commit differs from the pinned build (`bin/.cloud-provider-kind.sha`), the daemon is wedged (cannot list clusters), or the developer forces a restart with `KIND_RESTART_CPK=true`. A missing or unknown running-commit marker biases toward a restart so the pinned build is guaranteed. After any restart the port forwarding is re-established (see Port Forwarding).

cert-manager SHALL be installed by applying the release manifest from `https://github.com/cert-manager/cert-manager/releases/download/<version>/cert-manager.yaml`, skipping if the `cert-manager` namespace already exists (idempotent), and waiting for both the `cert-manager` and `cert-manager-webhook` deployments to reach ready state before proceeding. The version SHALL be pinned via a `CERT_MANAGER_VERSION` variable (default: `v1.21.1`).

The CloudNativePG (CNPG) operator SHALL be installed by applying the release manifest from `https://github.com/cloudnative-pg/cloudnative-pg/releases/download/<version>/cnpg-<version>.yaml` on every `kind-up` run (idempotent via server-side apply), and waiting for the `cnpg-controller-manager` deployment in `cnpg-system` to reach ready state before proceeding. The version SHALL be pinned via a `CNPG_VERSION` variable (default: `v1.30.0`).

After the CNPG operator is ready, `make kind-up` SHALL apply the `hypershell-db` CNPG `Cluster` resource for the API server on every run so that changes to `HYPERSHELL_DATABASE_IMAGE` are propagated without requiring a cluster teardown. The gateway database CNPG Cluster is NOT static infrastructure -- it is created dynamically by the ManagedDatabaseReconciler when the seeded `openshell-db` ManagedDatabase is processed by the control plane.

#### API server CNPG Cluster

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: hypershell-db
  namespace: hypershell-system
spec:
  imageName: HYPERSHELL_DATABASE_IMAGE
  instances: 1
  bootstrap:
    initdb:
      database: hypershell
      owner: hypershell
  storage:
    size: 1Gi
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi
```

The API server Deployment SHALL mount the CNPG-generated `hypershell-db-app` Secret (created automatically by the CNPG operator) and read connection parameters from its keys (`host`, `port`, `dbname`, `user`, `password`). The existing `deploy/base/postgres.yaml` (standalone Deployment + Secret + Service) SHALL be removed and replaced by the CNPG Cluster CR.

The API server SHALL retry database connections on startup until the database becomes available, using exponential backoff. This eliminates hard ordering dependencies between the API server Deployment and the CNPG Cluster  -- both can be applied in the same kustomize pass and the API server will wait for PostgreSQL to become ready. This replaces the previous standalone PostgreSQL Deployment that was applied directly by `kind-up`.

`make kind-up` SHALL wait for the API server CNPG Cluster to reach `Ready` status (all instances running) before proceeding to deploy HyperShell components. If the Cluster already exists (from a previous `kind-up`), that step SHALL be skipped (idempotent).

`HYPERSHELL_DATABASE_IMAGE` is a Makefile/kustomize variable substituted into the API server CNPG Cluster CR at deploy time. When unset, the `imageName` field is omitted and CNPG uses its built-in default image. To use a specific image (e.g., a Red Hat Hardened Image), set the variable before running `make kind-up`.

#### Gateway Database (ManagedDatabase Seeding)

The gateway database CNPG Cluster is NOT created by `kind-up` as static infrastructure. Instead, `kind-up` seeds a ManagedDatabase resource named `openshell-db` (provider=cnpg) via the REST API during the resource seeding step. The ManagedDatabaseReconciler in the control plane then creates the namespace and CNPG Cluster dynamically. See [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) for the full lifecycle.

`OPENSHELL_DATABASE_IMAGE` configures the PostgreSQL image for ManagedDatabase CNPG Clusters. When unset, CNPG uses its built-in default image.

Keycloak SHALL be deployed into the Kind cluster by default. When the `KIND_KEYCLOAK_URL` environment variable is set, the local Keycloak deployment SHALL be skipped and the Gateway OIDC issuer SHALL point at the external URL instead. This allows developers to test against a shared downstream Keycloak instance (e.g. the production broker - a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients).

### Gateway Resource

`make kind-up` SHALL seed all resources needed for a functional local environment: ManagedCluster, GatewayRelease, ManagedDatabase (`openshell-db` when `DATABASE_PROVIDER=cnpg`), and a Gateway with OIDC configuration pointing at the local Keycloak instance. The ManagedDatabase seed triggers the ManagedDatabaseReconciler to create the gateway database CNPG Cluster infrastructure. When a single ManagedDatabase exists, gateways created without an explicit `database_id` are auto-assigned to it. The seeding step obtains a Bearer token from Keycloak using the admin user (not the control-plane service account), then creates each resource via the REST API. If any seed step fails (e.g. the resource already exists from a previous run), it SHALL warn and continue rather than abort. This makes `kind-up` fully self-contained -- a developer gets a working gateway without any manual API calls after the initial setup.

The local environment SHALL NOT deploy the gateway's PostgreSQL directly - the control plane reconciler provisions a dedicated database and role for each gateway in the shared CNPG Cluster using CNPG `Database` and `DatabaseRole` CRDs (see `specs/platform/openshell-gateway-database.spec.md`). This ensures the local environment exercises the same database provisioning path used in production. The API server's database is also managed by CNPG via a separate Cluster CR in `hypershell-system` (see Cluster-Level Prerequisites above).

TLS SHALL NOT be disabled. The gateway serves TLS using certificates issued by cert-manager (self-signed CA). The OIDC issuer uses HTTP because the local Keycloak instance runs in dev mode without TLS; the TLS requirement applies to the gateway's own serving certificate, not to the OIDC issuer endpoint. Authentication SHALL use OIDC only - mTLS client authentication is not supported.

### Keycloak Configuration

The Kind cluster Keycloak instance serves as the local equivalent of the downstream Keycloak - a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients. In production, the downstream Keycloak brokers authentication to Red Hat SSO (upstream) and manages per-gateway OIDC clients. The local instance mirrors this topology without the upstream broker, providing the same realm structure and client model.

| Setting | Value |
|---------|-------|
| Realm | `hypershell` |
| Client | `hypershell-frontend` (public, standard flow + direct access grants, used by web console BFF) |
| CLI client | `hypershell-cli` (public, standard flow + device authorization grant, used by `hsctl login`) |
| Provisioner client | `hypershell-provisioner` (confidential, service account with `manage-clients` and `manage-users` roles) |
| Admin role | `hypershell-admins` |
| User role | `hypershell-users` |
| Users | `admin` / `admin` (admin role), `developer` / `developer` (user role) - password matches username (local dev only) |

The OIDC issuer URL SHALL be reachable from both inside the cluster (gateway pod) and outside (developer workstation).

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_KEYCLOAK_URL` | (unset - deploy local) | External Keycloak issuer URL; skips local deployment when set |

### Gateway API Routing

The local cluster uses the Kubernetes Gateway API to route all external traffic - both component services and gateway gRPC - through a single networking Gateway. This mirrors the production topology where a networking Gateway terminates external TLS. Component services (API, console, health) are exposed via HTTPRoute resources at `*.hypershell.localhost` hostnames; gateway gRPC traffic uses GRPCRoute at `*.gw.localhost`. `make kind-up` starts a CoreDNS container for wildcard `*.localhost` resolution (see DNS Resolution) and sets up OS-native port forwarding from host port 443 to the ephemeral port assigned by cloud-provider-kind (see Port Forwarding).

#### Networking Gateway (Cluster-Level)

`make kind-up` SHALL create a networking `Gateway` resource that acts as the shared ingress point for all component HTTPRoutes and gateway GRPCRoutes. This is cluster-level infrastructure - not managed by the control plane reconciler.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: hypershell-gw
  namespace: hypershell-system
spec:
  gatewayClassName: cloud-provider-kind
  listeners:
  - name: https
    hostname: "*.hypershell.localhost"
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - name: hypershell-https-tls
        kind: Secret
    allowedRoutes:
      namespaces:
        from: All
  - name: grpc
    hostname: "*.gw.localhost"
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - name: hypershell-gw-tls
        kind: Secret
    allowedRoutes:
      namespaces:
        from: All
```

cert-manager SHALL issue two wildcard certificates (self-signed CA): `hypershell-https-tls` for `*.hypershell.localhost` (component services) and `hypershell-gw-tls` for `*.gw.localhost` (gateway gRPC). The `allowedRoutes.namespaces.from: All` permits routes from any namespace, supporting multi-namespace deployments.

#### DNS Resolution

`make kind-up` SHALL start a CoreDNS container (`${KIND_CLUSTER_NAME}-dns`) bound to `127.0.0.1:${KIND_DNS_PORT}` that resolves all `*.localhost` queries to `127.0.0.1` (A) and `::1` (AAAA) using the `template` plugin. The Corefile is stored at `deploy/kind/coredns/Corefile` and mounted read-only. The container uses `--restart unless-stopped` so it survives Docker restarts.

OS-specific resolver configuration routes `.localhost` DNS queries to CoreDNS:

| Platform | Mechanism | Persistence |
|----------|-----------|-------------|
| macOS | `/etc/resolver/localhost` file with `nameserver 127.0.0.1` + `port ${KIND_DNS_PORT}` | Survives reboots (file on disk) |
| Fedora (Linux) | `resolvectl dns lo 127.0.0.1:${KIND_DNS_PORT}` + `resolvectl domain lo '~localhost'` | Ephemeral (cleared on reboot); re-applied by `kind-up` |

Wildcard resolution means multi-namespace hostnames (e.g. `api.feature-add-auth.hypershell.localhost`) work automatically - no per-hostname configuration needed. `make kind-down` SHALL stop the CoreDNS container and revert the Linux resolver (macOS resolver file is harmless without CoreDNS). If `sudo` fails during resolver setup, `kind-up` SHALL warn with manual instructions but continue.

#### Port Forwarding

cloud-provider-kind assigns ephemeral host ports for Gateway listeners. `make kind-up` SHALL set up OS-native port forwarding to redirect host port 443 to the discovered ephemeral port:

| Platform | Mechanism | Details |
|----------|-----------|---------|
| macOS | `pfctl` with named anchor `com.hypershell` | `rdr pass on lo0` rule; requires `rdr-anchor` registration in `/etc/pf.conf` rule ordering |
| Fedora (Linux) | `iptables -t nat` OUTPUT chain REDIRECT | Rule tagged with `--comment "hypershell-dev"` for identification |

Both require `sudo`, which cloud-provider-kind already requires. When it sets up port forwarding, `make kind-up` SHALL flush any stale rules from a previous run before installing the current mapping (a stop-then-start sequence), so the forwarding always reflects the live proxy container's ephemeral port rather than a port pinned by an earlier run. If port forwarding setup fails, `kind-up` SHALL fall back to printing URLs with the ephemeral port suffix (e.g. `https://console.hypershell.localhost:51300`). `make kind-down` SHALL flush the forwarding rules. `make kind-status` SHALL report whether forwarding is active and show the target port.

`make kind-up` acquires `sudo` once at the start and SHALL keep the `sudo` timestamp fresh for the whole run (refreshing it periodically in the background). Building images and waiting on rollouts can take several minutes before the pfctl/iptables and DNS resolver steps run; without a keepalive the default `sudo` timeout would expire first and those steps would fail silently. `KIND_NO_SUDO=true` skips all `sudo` operations and falls back to `kubectl port-forward`.

The system SHALL also provide a standalone `make kind-fix-ports` target that re-establishes host port forwarding without a full `make kind-up`. This is the recovery path after cloud-provider-kind restarts (which republishes the gateway LoadBalancer on new ephemeral ports): `make kind-fix-ports` SHALL re-discover the current 443 and 8080 host ports from the proxy container (`kindccm-gw-*`) and re-run the same stop-then-start flush so the mappings track the live ports.

This is a workaround until cloud-provider-kind supports publishing on specific host ports - once fixed upstream, the port forwarding functions can be removed.

#### Component Service Routes (kind-up Managed)

`make kind-up` SHALL create HTTPRoute resources for each component service:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api-server
  namespace: hypershell-system
spec:
  parentRefs:
  - name: hypershell-gw
    namespace: hypershell-system
  hostnames:
  - api.hypershell.localhost
  rules:
  - backendRefs:
    - name: hypershell-api-server
      port: 8000
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: web-console
  namespace: hypershell-system
spec:
  parentRefs:
  - name: hypershell-gw
    namespace: hypershell-system
  hostnames:
  - console.hypershell.localhost
  rules:
  - backendRefs:
    - name: hypershell-web-console
      port: 3000
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: health
  namespace: hypershell-system
spec:
  parentRefs:
  - name: hypershell-gw
    namespace: hypershell-system
  hostnames:
  - health.hypershell.localhost
  rules:
  - backendRefs:
    - name: hypershell-api-server
      port: 8434
```

| Service | Hostname | Purpose |
|---------|----------|---------|
| HTTP API | `api.hypershell.localhost` | REST API access |
| Web Console | `console.hypershell.localhost` | Browser UI |
| Health | `health.hypershell.localhost` | Health check endpoint |
| gRPC | `<gateway-name>.gw.localhost` | gRPC streaming (control plane, CLI) |

For multi-namespace deployments, component hostnames include the namespace: `api.<namespace>.hypershell.localhost` (e.g. `api.feature-add-auth.hypershell.localhost`). `make kind-up` with a custom `KIND_NAMESPACE` creates namespace-scoped HTTPRoutes; wildcard DNS resolves all subdomains automatically.

#### Per-Gateway Route Resources (Control Plane Reconciled)

For each HyperShell Gateway resource, the control plane reconciler creates three Kubernetes resources:

1. **GRPCRoute** - Routes gRPC traffic from the networking Gateway to the gateway pod's Service:
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: GRPCRoute
   metadata:
     name: openshell-gateway
     namespace: hypershell-system
   spec:
     parentRefs:
     - name: hypershell-gw
       namespace: hypershell-system
     hostnames:
     - openshell-gateway.gw.localhost
     rules:
     - backendRefs:
       - name: openshell-gateway
         port: 8080
   ```

2. **BackendTLSPolicy** - Instructs the networking Gateway to establish a TLS connection to the backend pod and verify its certificate against the CA ConfigMap (TLS re-encrypt). The `v1alpha3` API version ships in the Gateway API experimental channel and tracks `GATEWAY_API_VERSION`; the API shape may change before GA:
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1alpha3
   kind: BackendTLSPolicy
   metadata:
     name: openshell-gateway
     namespace: hypershell-system
   spec:
     targetRefs:
     - group: ""
       kind: Service
       name: openshell-gateway
     validation:
       caCertificateRefs:
       - group: ""
         kind: ConfigMap
         name: openshell-backend-ca
       hostname: openshell-gateway.hypershell-system.svc.cluster.local
   ```

3. **CA ConfigMap** - Contains the gateway pod's CA certificate so the networking Gateway can verify the pod's TLS cert:
   ```yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: openshell-backend-ca
     namespace: hypershell-system
   data:
     ca.crt: |
       <contents of openshell-server-tls Secret ca.crt>
   ```

#### TLS Re-Encrypt Flow

```
Client                   Networking Gateway              Gateway Pod
  |                            |                              |
  |--- HTTPS (external) ------>|                              |
  |    wildcard cert           |--- TLS (internal) ---------->|
  |    *.gw.localhost          |    BackendTLSPolicy verifies  |
  |                            |    pod cert via CA ConfigMap  |
  |                            |                              |
```

1. **Client to networking Gateway:** The client connects via HTTPS. The networking Gateway terminates external TLS using the wildcard certificate (`*.gw.localhost`) issued by cert-manager. HTTP/2 is negotiated through ALPN during the TLS handshake.
2. **Networking Gateway to pod:** BackendTLSPolicy instructs the networking Gateway to re-encrypt traffic to the backend pod. The Gateway verifies the pod's certificate against the CA in the `openshell-backend-ca` ConfigMap. The pod's cert is issued by cert-manager from the same self-signed CA.

#### Kind vs Production Differences

| Aspect | Kind (local) | Production (OpenShift) |
|--------|-------------|----------------------|
| GatewayClass | `cloud-provider-kind` | `openshift-default` (controller: `openshift.io/gateway-controller/v1`) |
| Gateway controller | cloud-provider-kind process | OpenShift Service Mesh Operator (Istio/Envoy) |
| External TLS cert | cert-manager self-signed CA | Public CA (e.g. Let's Encrypt, corporate PKI) |
| Internal TLS cert | cert-manager self-signed CA | cert-manager with internal CA |
| LoadBalancer | cloud-provider-kind port mapping | Cloud provider LB or MetalLB |
| Service base domain | `hypershell.localhost` | `<cluster-domain>` (e.g. `apps.cluster.example.com`) |
| Service hostname pattern | `<service>.hypershell.localhost` | Route-based (OpenShift Routes or Gateway API HTTPRoute) |
| gRPC base domain | `gw.localhost` | `gw.<cluster-domain>` |
| gRPC hostname pattern | `<gateway-name>.gw.localhost` | `<gateway-name>-<namespace>.gw.<base-domain>` |
| DNS resolution | CoreDNS container + OS resolver + pfctl/iptables port forwarding | Cluster DNS / external DNS |

### Development Tracing (Jaeger)

The local environment SHALL deploy a Jaeger all-in-one instance so a developer can view distributed traces produced by the web console and the API server (see `web-console/tracing.spec.md` and HYPERSHELL-26). Jaeger all-in-one exposes an OTLP receiver and a query UI in one workload; a separate OpenTelemetry Collector is not required for local development. Trace storage uses in-memory storage, which is cleared on restart and is appropriate for development only.

`make kind-up` SHALL deploy Jaeger into the target namespace with its OTLP receiver enabled, and SHALL create an HTTPRoute that exposes the Jaeger UI at `jaeger.hypershell.localhost`. The Jaeger workload SHALL expose both the OTLP/gRPC receiver (`4317`) used by the API server and the OTLP/HTTP receiver (`4318`) used by the web console, because browsers cannot speak OTLP gRPC. The web console BFF SHALL be configured to export traces to the Jaeger OTLP/HTTP endpoint through the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable on the web-console Deployment. The browser exports through the same-origin BFF telemetry endpoint (see `web-console/tracing.spec.md`), so the browser does not reach Jaeger directly.

| Setting | Value |
|---------|-------|
| Workload | `jaeger` Deployment (all-in-one, in-memory storage) |
| OTLP receiver (API server) | `jaeger.<namespace>.svc.cluster.local:4317` (OTLP/gRPC) |
| OTLP receiver (web console BFF) | `http://jaeger.<namespace>.svc.cluster.local:4318` (OTLP/HTTP) |
| Query UI | `https://jaeger.hypershell.localhost` |

Deployment of Jaeger SHALL be gated by the `KIND_JAEGER` environment variable (opt-in; unset or any value other than `true` skips it). When Jaeger is not deployed, `make kind-up` SHALL skip the Jaeger workload and its route and SHALL leave `OTEL_EXPORTER_OTLP_ENDPOINT` unset on the web-console Deployment, so the BFF starts with tracing disabled without failing readiness.

## Requirements

### Requirement: Single-Command Environment Setup

The system SHALL provide a `make kind-up` target at the repository root that creates a fully functional local HyperShell environment. Baseline images SHALL be pulled from the container registry.

#### Scenario: First Run - Clean State
- GIVEN no Kind cluster exists
- WHEN a developer runs `make kind-up`
- THEN a Kind cluster SHALL be created
- AND all component images SHALL be pulled from the container registry
- AND images SHALL be loaded into the Kind cluster
- AND all Kubernetes resources SHALL be applied (namespace, API server, control plane)
- AND the system SHALL wait for all components to become ready
- AND connection information SHALL be printed to stdout

#### Scenario: Subsequent Run - Idempotent
- GIVEN a Kind cluster is already running from a previous `make kind-up`
- WHEN a developer runs `make kind-up` again
- THEN the cluster creation step SHALL be skipped (idempotent)
- AND Kubernetes manifests SHALL be reapplied
- AND the system SHALL wait for all components to become ready

### Requirement: Per-Component Local Swap

The system SHALL provide per-component Make targets that build a single component from the current working tree, load it into the running cluster, and replace that component's deployment. Each invocation SHALL rebuild the image and replace the running state, even if the component is already swapped. This ensures developers can iterate by running the same target repeatedly after making changes.

#### Scenario: Swap API Server
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-api-server-up`
- THEN the API server image SHALL be built from the current working tree
- AND the image SHALL be loaded into the Kind cluster
- AND the API server deployment SHALL be replaced with the newly built image
- AND the system SHALL wait for the API server to become ready

#### Scenario: Swap Control Plane
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-control-plane-up`
- THEN the control plane image SHALL be built from the current working tree
- AND the image SHALL be loaded into the Kind cluster
- AND the control plane deployment SHALL be replaced with the newly built image
- AND the system SHALL wait for the control plane to become ready

#### Scenario: Swap Web Console
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-web-console-up`
- THEN by default (hot reload enabled), the web console source SHALL be mounted from the host and `npm run dev` SHALL be started in an interactive TTY
- IF `KIND_HOT_RELOAD=false`, the web console image SHALL be built from the current working tree, loaded, and the deployment replaced
- AND the system SHALL wait for the web console to become ready

#### Scenario: Re-Swap Already Swapped Component
- GIVEN the API server is already running a local build
- WHEN a developer runs `make kind-api-server-up` again
- THEN the API server image SHALL be rebuilt from the current working tree
- AND the new image SHALL replace the previously swapped image
- AND the system SHALL wait for the API server to become ready

#### Scenario: No Cluster Running
- GIVEN no Kind cluster exists
- WHEN a developer runs any per-component swap target
- THEN the command SHALL exit with an error
- AND print a message directing the developer to run `make kind-up` first

#### Scenario: Revert API Server Swap
- GIVEN the API server is running a local build
- WHEN a developer runs `make kind-api-server-down`
- THEN the API server image SHALL be reverted to the baseline image
- AND the API server deployment SHALL be restarted
- AND the system SHALL wait for the API server to become ready
- AND swap tracking SHALL be cleared for the API server

#### Scenario: Revert Control Plane Swap
- GIVEN the control plane is running a local build
- WHEN a developer runs `make kind-control-plane-down`
- THEN the control plane image SHALL be reverted to the baseline image
- AND the control plane deployment SHALL be restarted
- AND the system SHALL wait for the control plane to become ready
- AND swap tracking SHALL be cleared for the control plane

#### Scenario: Revert Web Console Swap
- GIVEN the web console is running a local build or hot reload
- WHEN a developer runs `make kind-web-console-down`
- THEN the web console image SHALL be reverted to the baseline image
- AND the web console deployment SHALL be restarted
- AND the system SHALL wait for the web console to become ready
- AND swap tracking SHALL be cleared for the web console

#### Scenario: Revert When Not Swapped
- GIVEN a component is already running the baseline image
- WHEN a developer runs the corresponding `-down` target
- THEN the command SHALL print an info message indicating the component is already running the baseline image
- AND exit without error (no-op)

### Requirement: Cluster Teardown

The system SHALL provide a `make kind-down` target that completely removes the local development cluster.

#### Scenario: Teardown Running Cluster
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-down`
- THEN the Kind cluster and all associated resources SHALL be deleted

#### Scenario: Teardown When No Cluster Exists
- GIVEN no Kind cluster exists
- WHEN a developer runs `make kind-down`
- THEN the command SHALL exit without error

### Requirement: Cluster Status

The system SHALL provide a `make kind-status` target that reports the current state of the local development environment.

#### Scenario: Status of Running Cluster
- GIVEN a Kind cluster is running with deployed components
- WHEN a developer runs `make kind-status`
- THEN cluster connectivity information SHALL be displayed
- AND pod status for all components SHALL be displayed
- AND service hostnames SHALL be displayed
- AND the output SHALL indicate which components have active local swaps versus baseline images
- AND CoreDNS container status SHALL be displayed
- AND port forwarding status (active/inactive, target port) SHALL be displayed

#### Scenario: Status When No Cluster Exists
- GIVEN no Kind cluster is running
- WHEN a developer runs `make kind-status`
- THEN the output SHALL indicate the cluster is not running

### Requirement: Configurable Cluster Name

The system SHALL accept a `KIND_CLUSTER_NAME` environment variable to control the Kind cluster name. If unset, the default SHALL be `hypershell-dev`.

#### Scenario: Custom Cluster Name
- GIVEN `KIND_CLUSTER_NAME` is set to `my-cluster`
- WHEN a developer runs `make kind-up`
- THEN a Kind cluster named `my-cluster` SHALL be created
- AND `make kind-down`, `make kind-status`, and per-component targets SHALL operate on `my-cluster`

#### Scenario: Default Cluster Name
- GIVEN `KIND_CLUSTER_NAME` is not set
- WHEN a developer runs `make kind-up`
- THEN a Kind cluster named `hypershell-dev` SHALL be created

### Requirement: Hostname-Based Service Access

All services SHALL be accessible via `.localhost` hostnames routed through the networking Gateway (see Gateway API Routing). The system SHALL NOT use `kubectl port-forward` for service access. DNS resolution is provided by a CoreDNS container with OS-specific resolver configuration (see DNS Resolution). Port forwarding redirects host port 443 to the ephemeral port assigned by cloud-provider-kind (see Port Forwarding), enabling clean `https://` URLs without a port suffix.

| Service | Hostname | Purpose |
|---------|----------|---------|
| HTTP API | `https://api.hypershell.localhost` | REST API access |
| Web Console | `https://console.hypershell.localhost` | Browser UI |
| Health | `https://health.hypershell.localhost` | Health check endpoint |
| Jaeger UI | `https://jaeger.hypershell.localhost` | Distributed tracing UI (when `KIND_JAEGER=true`) |
| gRPC | `https://<gateway-name>.gw.localhost` | gRPC streaming (control plane, CLI) |

The self-signed TLS certificate issued by cert-manager must be trusted by the developer's browser or CLI tool (e.g. `curl --cacert`).

For multi-namespace deployments, hostnames include the namespace: `api.<namespace>.hypershell.localhost`. Wildcard DNS resolves all subdomains automatically - no per-hostname configuration is needed. All namespaces share the same Gateway - each namespace is differentiated by hostname, not port number.

#### Scenario: Default Access (Port Forwarding Active)
- GIVEN no special configuration
- WHEN `make kind-up` completes and port forwarding succeeds
- THEN the HTTP API SHALL be accessible at `https://api.hypershell.localhost`
- AND the web console SHALL be accessible at `https://console.hypershell.localhost`
- AND the health endpoint SHALL be accessible at `https://health.hypershell.localhost`
- AND the banner SHALL print URLs without a port suffix

#### Scenario: Graceful Fallback (Port Forwarding Unavailable)
- GIVEN port forwarding setup fails (e.g. sudo denied)
- WHEN `make kind-up` completes
- THEN the banner SHALL print URLs with the ephemeral port suffix (e.g. `https://console.hypershell.localhost:<PORT>`)
- AND services SHALL remain accessible at the ephemeral port

#### Scenario: Multi-Namespace Hostname Differentiation
- GIVEN the default deployment is running in `hypershell-system`
- AND a second deployment is running in `hypershell-feature-add-auth`
- WHEN a developer accesses `https://api.feature-add-auth.hypershell.localhost`
- THEN the request SHALL route to the API server in the `hypershell-feature-add-auth` namespace

### Requirement: Container Engine Support

The system SHALL support both Podman and Docker as container engines. The engine SHALL be auto-detected, preferring Podman when available, and MAY be overridden via the `CONTAINER_ENGINE` environment variable.

#### Scenario: Podman Available
- GIVEN Podman is installed and available in PATH
- WHEN a developer runs `make kind-up`
- THEN Podman SHALL be used to build and manage container images

#### Scenario: Docker Fallback
- GIVEN Podman is not installed
- AND Docker is installed
- WHEN a developer runs `make kind-up`
- THEN Docker SHALL be used to build and manage container images

### Requirement: Image Reference Consistency

All image names and tags used across Makefile targets, Kind load commands, and Kubernetes manifests SHALL resolve to the same artifacts. This reinforces the cross-cutting convention in `specs/standards/platform/cross-cutting.spec.md`.

When `LOCAL_IMAGES=true`, the deployed image refs are the `localhost/` names (e.g. `localhost/hypershell-controller:dev`), not the registry refs. The substitution happens at kustomize apply time via an `images` transformer, so manifests reach the cluster with the correct refs  -- no post-apply patching, no wasted rollout.

### Requirement: Security Context Compliance

All containers in the Kind deployment manifests SHALL set restricted security contexts per `specs/standards/security/security.spec.md`: `runAsNonRoot: true`, `capabilities.drop: ["ALL"]`, and `allowPrivilegeEscalation: false`.

### Requirement: Swap Tracking

The system SHALL track which components have been swapped to local builds using a `.kind-swaps` file at the repository root. This file SHALL be listed in `.gitignore`. The file records the set of currently swapped components so that `make kind-status` can report this information. Running `make kind-up` SHALL preserve existing swap state: for non-swapped components, it pulls the latest baseline images and reapplies manifests normally; for swapped components, it skips manifest reapplication to avoid overwriting the locally-built image. Swap tracking is not cleared by `kind-up`.

#### Scenario: Swap Reported in Status
- GIVEN a developer has run `make kind-api-server-up`
- WHEN they run `make kind-status`
- THEN the output SHALL indicate the API server is running a local build
- AND the control plane is running the baseline image

#### Scenario: Kind-Up Preserves Swaps
- GIVEN a developer has swapped the API server to a local build
- WHEN they run `make kind-up`
- THEN non-swapped components SHALL be redeployed from registry images
- AND the API server SHALL remain running the locally-built image
- AND swap tracking SHALL be preserved

### Requirement: Developer Documentation

The repository SHALL include a `DEVELOPMENT.md` guide that documents the local development environment. The guide SHALL cover:

- Prerequisites (Docker or Podman, Kind, kubectl; `oc` for the OpenShift path)
- `make kind-up` quickstart with expected output
- Per-component swap workflow (`make kind-<component>-up` / `make kind-<component>-down`)
- Hot reload setup for the web console (`KIND_HOT_RELOAD=true`)
- Environment variable reference (all `KIND_*`, `IMAGE_*`, and `CONTAINER_ENGINE` variables)
- Keycloak configuration and `KIND_KEYCLOAK_URL` for external OIDC
- Troubleshooting common issues (port conflicts, container engine not running, image pull failures)
- The OpenShift counterpart (`make openshift-up`, `make openshift-<component>-up`) in the same guide; that workflow is specified by `openshift-development.spec.md`

The documentation SHALL be kept in sync with this spec. When a new Make target, environment variable, or component is added, the guide SHALL be updated in the same PR.

#### Scenario: Documentation Exists
- GIVEN a developer clones the repository
- WHEN they look for local development instructions
- THEN `DEVELOPMENT.md` SHALL exist and describe how to set up and use the Kind environment
- AND it SHALL document `make openshift-up` against an existing OpenShift cluster

#### Scenario: Documentation Stays Current
- GIVEN a PR adds or changes a `kind-*` or `openshift-*` Make target or environment variable
- WHEN the PR is reviewed
- THEN the reviewer SHALL verify that `DEVELOPMENT.md` is updated to reflect the change

### Requirement: Hot Reload Support

The Kind cluster configuration SHALL include `extraMounts` that map a host directory into the cluster nodes, enabling `hostPath` volumes for live source mounting. The web console is the first component to support hot reload.

| Setting | Value |
|---------|-------|
| Host path | `KIND_HOST_MOUNT_PATH` env var (default: repository root via `git rev-parse --show-toplevel`) |
| Container path | `/mnt/host` on each Kind node |
| Read-only | `false` (writable, required for npm file watchers) |

When hot reload is enabled (the default), `kind-<component>-up` for a supported component SHALL mount the host source directory into the container and run a dev server (e.g. `npm run dev`) in an interactive TTY attached to the developer's terminal, instead of performing a full image rebuild. File changes on the host are reflected inside the container immediately. When hot reload is disabled (`KIND_HOT_RELOAD=false`), `kind-<component>-up` SHALL rebuild the image from the working tree and replace the deployment as normal.

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_HOT_RELOAD` | `true` | Hot reload mode for components that support it; set to `false` to disable |

| Component | Hot Reload Support |
|-----------|-------------------|
| Web Console | Yes - mounts `components/web-console/` and runs `npm run dev` |
| API Server | No - Go service, rebuild-and-replace only |
| Control Plane | No - Go service, rebuild-and-replace only |

#### Scenario: Web Console Hot Reload (Default)
- GIVEN a Kind cluster is running
- AND `KIND_HOT_RELOAD` is not set or is `true`
- WHEN a developer runs `make kind-web-console-up`
- THEN the `components/web-console/` source directory SHALL be mounted from the host into the container via a `hostPath` volume
- AND `npm run dev` SHALL be started in an interactive TTY attached to the developer's terminal
- AND file changes on the host SHALL be reflected inside the container without rebuilding

#### Scenario: Web Console Without Hot Reload
- GIVEN a Kind cluster is running
- AND `KIND_HOT_RELOAD=false` is set
- WHEN a developer runs `make kind-web-console-up`
- THEN the web console image SHALL be rebuilt from the working tree
- AND the deployment SHALL be replaced with the newly built image

#### Scenario: Hot Reload on Unsupported Component
- GIVEN hot reload is enabled (default)
- WHEN a developer runs `make kind-api-server-up` or `make kind-control-plane-up`
- THEN the component SHALL fall back to the normal rebuild-and-replace flow
- AND an info message SHALL indicate that hot reload is not supported for that component

### Requirement: Container Registry

The system SHALL pull baseline images from the container registry at `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/`. The registry path and image tag SHALL be configurable via environment variables.

| Env Var | Default |
|---------|---------|
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` |
| `IMAGE_TAG` | `latest` |

### Requirement: Offline Development (LOCAL_IMAGES)

The system SHALL support offline development by building all images from the local repository instead of pulling from the container registry. When `LOCAL_IMAGES=true` is set, `make kind-up` SHALL build every component image from the working tree (current branch), load them into the Kind cluster, and apply manifests with `localhost/` image refs so that pods start with local images on the first rollout. This ensures the deployed images match the scripts, manifests, and seed data on the current branch  -- and `kubectl describe pod` shows the actual local image name (e.g. `localhost/hypershell-controller:dev`) rather than a registry ref that could be mistaken for a remote pull. To build from `origin/main` instead (e.g. for baseline comparison), set `BUILD_SOURCE=baseline`.

Image ref substitution SHALL happen at manifest-apply time via a kustomize `images` transformer, not as a post-apply `kubectl set image` patch. This avoids a wasted rollout where pods first start with registry refs (potentially failing to pull) and then get killed and replaced with local refs. The kustomize transformer maps each registry image name to its `localhost/` equivalent before the manifests reach the API server.

| Env Var | Default | Description |
|---------|---------|-------------|
| `LOCAL_IMAGES` | (unset - pull from registry) | Set to `true` to build images locally instead of pulling from the container registry |
| `BUILD_SOURCE` | `worktree` | Image build source: `worktree` (current branch) or `baseline` (`origin/main`) |

Local image names:

| Component | Registry Ref | Local Image Ref |
|-----------|-------------|----------------|
| API server | `${IMAGE_REGISTRY}/hypershell-api-server-main` | `localhost/hypershell:dev` |
| Control plane | `${IMAGE_REGISTRY}/hypershell-control-plane-main` | `localhost/hypershell-controller:dev` |
| Web console | `${IMAGE_REGISTRY}/hypershell-web-console-main` | `localhost/hypershell-web-console:dev` |

#### Scenario: First Run - Offline
- GIVEN no Kind cluster exists
- AND the developer has no access to the container registry
- AND `LOCAL_IMAGES=true` is set
- WHEN the developer runs `make kind-up`
- THEN all component images SHALL be built from the working tree with `localhost/` image names
- AND images SHALL be loaded into the Kind cluster via tarball archive
- AND kustomize SHALL apply manifests with a `localhost/` images transformer so pods start with local refs
- AND the cluster SHALL reach a ready state without any registry pulls for platform components
- AND `kubectl get pods -o jsonpath='{.items[*].spec.containers[*].image}'` SHALL show `localhost/` refs for locally-built components

#### Scenario: Subsequent Run - Rebuild from Working Tree
- GIVEN a Kind cluster is running with locally-built images
- AND `LOCAL_IMAGES=true` is set
- WHEN the developer runs `make kind-up` again
- THEN all non-swapped component images SHALL be rebuilt from the working tree
- AND updated images SHALL be loaded into the Kind cluster
- AND kustomize SHALL apply manifests with the `localhost/` images transformer
- AND swapped components SHALL be preserved

#### Scenario: Baseline Build from origin/main
- GIVEN `LOCAL_IMAGES=true` and `BUILD_SOURCE=baseline` are set
- WHEN the developer runs `make kind-up`
- THEN all component images SHALL be built from `origin/main` with `localhost/` image names
- AND images SHALL be loaded into the Kind cluster
- AND kustomize SHALL apply manifests with the `localhost/` images transformer

### Requirement: Red Hat Hardened Images

All container images deployed into the Kind cluster SHALL use [Red Hat Hardened Images](https://images.redhat.com/) (HI). HI images are distroless, CIS-hardened, and signed at build time.

Developers SHALL provide a pull secret file via the `PULL_SECRET` environment
variable to authenticate against private registries such as
`registry.access.redhat.com` and Quay. `KIND_PULL_SECRET` SHALL remain accepted
as an alias when `PULL_SECRET` is unset. When set, `make kind-up` SHALL apply
the secret to the target namespace and patch the default ServiceAccount with
`imagePullSecrets` so that pods can pull HI images without per-pod secret
references. OpenShift component swaps SHALL use the same file to log the
container engine into `SWAP_REGISTRY` and SHALL NOT require an interactive
`podman login` when the secret contains credentials for that registry host.

> **Database images:** Two environment variables control PostgreSQL images for the two CNPG provisioning paths:
> - `HYPERSHELL_DATABASE_IMAGE` - configures the API server's static `hypershell-db` CNPG Cluster. `make kind-up` patches the Cluster with this image after applying manifests. When unset, CNPG uses its built-in default image.
> - `OPENSHELL_DATABASE_IMAGE` - configures gateway database CNPG Clusters created dynamically by the ManagedDatabaseReconciler. Set this on the control plane process (or in the Kind deployment environment) before provisioning gateways. When unset, CNPG uses its built-in default image.
>
> To satisfy the all-images HI requirement, set **both** variables to the HI PostgreSQL image before running `make kind-up`.

### Requirement: Multiple Namespace Deployments

The system SHALL support deploying the platform into multiple namespaces within a single Kind cluster. Each namespace gets its own set of HyperShell components with isolated hostnames, enabling developers to work on multiple features in parallel (e.g. when handing separate branches to agents).

All `kind-*` targets operate on the namespace specified by `KIND_NAMESPACE` (default: `hypershell-system`). Running `make kind-up` with a custom `KIND_NAMESPACE` deploys the platform into that namespace, creating it if it does not exist. `make kind-down` removes the target namespace and its resources; `make kind-teardown` destroys the entire Kind cluster.

#### Scenario: Deploy to Additional Namespace
- GIVEN a Kind cluster is running with the default deployment in `hypershell-system`
- WHEN a developer runs `KIND_NAMESPACE=hypershell-feature-add-auth make kind-up`
- THEN the namespace `hypershell-feature-add-auth` SHALL be created (if it does not exist)
- AND a full set of HyperShell components SHALL be deployed into that namespace
- AND namespace-scoped HTTPRoutes SHALL be created (e.g. `api.feature-add-auth.hypershell.localhost`)
- AND wildcard DNS SHALL resolve the new hostnames automatically
- AND both deployments SHALL run independently without interference

#### Scenario: Status Reports All Deployments
- GIVEN the platform is deployed into multiple namespaces
- WHEN a developer runs `make kind-status`
- THEN the output SHALL list all namespaces with their hostnames and swap state

#### Scenario: Teardown Namespace Deployment
- GIVEN the platform is deployed in `hypershell-system` and `hypershell-feature-add-auth`
- WHEN a developer runs `KIND_NAMESPACE=hypershell-feature-add-auth make kind-down`
- THEN only the `hypershell-feature-add-auth` namespace and its resources SHALL be deleted
- AND the default deployment in `hypershell-system` SHALL continue running
- AND the developer SHALL be prompted to run `make kind-teardown` if no HyperShell namespaces remain

#### Scenario: Per-Component Swap Scoped to Namespace
- GIVEN the platform is deployed in multiple namespaces
- WHEN a developer runs `KIND_NAMESPACE=hypershell-feature-add-auth make kind-api-server-up`
- THEN the API server SHALL be swapped only in the `hypershell-feature-add-auth` namespace
- AND the default deployment SHALL remain unchanged

### Requirement: Development Tracing

The system SHALL deploy a Jaeger all-in-one instance in the local environment and configure the web console BFF to export traces to it, so a developer can verify one end-to-end trace. Deployment SHALL be gated by `KIND_JAEGER` (opt-in; deployed only when set to `true`).

#### Scenario: Jaeger Available After Setup
- GIVEN `KIND_JAEGER` is `true`
- WHEN a developer runs `make kind-up`
- THEN a Jaeger all-in-one workload SHALL be deployed into the target namespace
- AND the workload SHALL expose the OTLP/gRPC receiver on `4317` and the OTLP/HTTP receiver on `4318`
- AND an HTTPRoute SHALL expose the Jaeger UI at `jaeger.hypershell.localhost`
- AND the web console BFF SHALL be configured with `OTEL_EXPORTER_OTLP_ENDPOINT` pointing at the Jaeger OTLP/HTTP receiver

#### Scenario: End-to-End Trace Visible
- GIVEN the local environment is running with tracing enabled
- WHEN a developer completes a gateway workflow in the web console
- THEN one trace SHALL be visible in the Jaeger UI
- AND it SHALL contain the browser workflow span and the BFF server span joined by the same trace identifier

#### Scenario: Tracing Disabled
- GIVEN `KIND_JAEGER` is unset (or any value other than `true`)
- WHEN a developer runs `make kind-up`
- THEN the Jaeger workload and its route SHALL NOT be deployed
- AND `OTEL_EXPORTER_OTLP_ENDPOINT` SHALL be left unset on the web-console Deployment
- AND the web console BFF SHALL start with tracing disabled without failing readiness

## Environment Variable Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_CLUSTER_NAME` | `hypershell-dev` | Kind cluster name |
| `KIND_NO_SUDO` | (unset) | Set to `true` to skip all sudo operations (cloud-provider-kind, DNS resolver, port forwarding); services use ephemeral ports |
| `KIND_DNS_PORT` | `5553` | Host port for the CoreDNS container (UDP+TCP) |
| `KIND_HOT_RELOAD` | `true` | Hot reload for supported components; set to `false` to disable |
| `KIND_HOST_MOUNT_PATH` | Repository root (`git rev-parse --show-toplevel`) | Host directory mounted into Kind nodes for hot reload |
| `KIND_KEYCLOAK_URL` | (unset - deploy local) | External Keycloak issuer URL; skips local deployment when set |
| `PULL_SECRET` | (unset) | Path to a Kubernetes pull secret YAML file (`kubernetes.io/dockerconfigjson`); applied to the Kind target namespace and used to log the container engine in for OpenShift swaps |
| `KIND_PULL_SECRET` | (unset) | Alias for `PULL_SECRET` |
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` | Container registry path for baseline images |
| `IMAGE_TAG` | `latest` | Image tag for baseline images |
| `LOCAL_IMAGES` | (unset - pull from registry) | Set to `true` to build images locally instead of pulling from registry |
| `BUILD_SOURCE` | `worktree` | Image build source when `LOCAL_IMAGES=true`: `worktree` (current branch) or `baseline` (`origin/main`) |
| `CONTAINER_ENGINE` | Auto-detected (Podman preferred) | Container engine (`podman` or `docker`) |
| `GATEWAY_API_VERSION` | (pinned in Makefile) | Gateway API CRD release version |
| `CLOUD_PROVIDER_KIND_REPO` | (pinned in Makefile) | Git repository URL for cloud-provider-kind fork (BackendTLSPolicy + ALPN h2 support) |
| `CLOUD_PROVIDER_KIND_REF` | (pinned in Makefile) | Exact commit SHA of the cloud-provider-kind fork to build (deterministic; idempotent-by-SHA rebuild) |
| `CLOUD_PROVIDER_KIND_BRANCH` | (unset) | Optional testing override: build from a branch tip or arbitrary git ref instead of the pinned SHA; always rebuilds when set |
| `KIND_RESTART_CPK` | (unset) | Set to `true` to force `make kind-up` to restart cloud-provider-kind (republishes ephemeral LB ports; otherwise the running instance is reused to keep ports stable) |
| `CERT_MANAGER_VERSION` | `v1.21.1` | cert-manager release version |
| `CNPG_VERSION` | `v1.30.0` | CloudNativePG operator release version |
| `HYPERSHELL_DATABASE_IMAGE` | (unset - CNPG default) | PostgreSQL image for the API server CNPG Cluster; set `spec.imageName` on the Cluster CR |
| `KIND_NAMESPACE` | `hypershell-system` | Target namespace for all `kind-*` targets |
| `KIND_JAEGER` | (unset) | Set to `true` to deploy Jaeger all-in-one for distributed tracing (OTLP gRPC `4317` + HTTP `4318`) and export web console traces to it |

## Make Targets Summary

All Kind targets operate on `KIND_NAMESPACE` (default: `hypershell-system`). OpenShift uses the same target names with an `openshift-` prefix (`make openshift-up`, `make openshift-api-server-up`, and siblings); those commands are specified in `openshift-development.spec.md`. `make kind-teardown` destroys the Kind cluster. `make openshift-teardown` is the same as `make openshift-down` -- there is no OpenShift cluster to destroy.

| Target | Behavior |
|--------|----------|
| `make kind-up` | Create cluster (if needed) + deploy into `KIND_NAMESPACE` + start CoreDNS + configure resolver + set up port forwarding + wait for readiness |
| `make kind-down` | Remove `KIND_NAMESPACE` and its resources; prompt for `kind-teardown` when last namespace is removed |
| `make kind-teardown` | Destroy the Kind cluster + stop cloud-provider-kind + stop CoreDNS + flush port forwarding rules + revert resolver |
| `make kind-status` | Show cluster info, pods, services, hostnames, DNS status, port forwarding status, and active component swaps |
| `make kind-fix-ports` | Re-establish host port forwarding (443 + 8080) after a cloud-provider-kind restart; re-discovers ephemeral ports and re-runs the stop-then-start flush |
| `make kind-api-server-up` | Build api-server from working tree + load + replace deployment + wait (cluster must exist; idempotent - rebuilds and replaces on every call) |
| `make kind-api-server-down` | Revert api-server to baseline image + restart + wait |
| `make kind-control-plane-up` | Build control-plane from working tree + load + replace deployment + wait (cluster must exist; idempotent - rebuilds and replaces on every call) |
| `make kind-control-plane-down` | Revert control-plane to baseline image + restart + wait |
| `make kind-web-console-up` | Default (hot reload): mount source + run `npm run dev` in interactive TTY; with `KIND_HOT_RELOAD=false`: build + load + replace deployment + wait |
| `make kind-web-console-down` | Revert web-console to baseline image + restart + wait |

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Registry pull for baseline images | Faster setup; no local build required for baseline; per-component swap handles local development |
| Per-component swap for iterative development | More ergonomic than blanket rebuild; discoverable via tab-completion; `LOCAL_IMAGES=true` builds from the working tree by default so images match the branch's scripts and manifests; `BUILD_SOURCE=baseline` optionally builds from `origin/main` for comparison |
| Hostname routing via networking Gateway as default | All component services route through the networking Gateway using HTTPRoute resources at `*.hypershell.localhost`. Developers access services by name (`api.hypershell.localhost`) instead of memorizing port numbers. Multi-namespace deployments get distinct hostnames without any per-hostname configuration thanks to wildcard DNS |
| CoreDNS for wildcard DNS | A CoreDNS container resolves all `*.localhost` to loopback, eliminating per-hostname `/etc/hosts` management. OS resolver config routes `.localhost` queries to CoreDNS (macOS: `/etc/resolver/localhost`; Linux: `resolvectl`). Multi-namespace hostnames work automatically |
| OS-native port forwarding (pfctl/iptables) | Redirects host:443 to cloud-provider-kind's ephemeral port, enabling clean `https://` URLs. Workaround until cloud-provider-kind supports publishing on specific host ports. Graceful fallback: if sudo fails, URLs show the ephemeral port suffix |
| Pre-check cluster existence for idempotency | Check `kind get clusters` for the target name before attempting creation; skip if already present. Avoids `\|\| true` which swallows real failures (Docker not running, resource exhaustion) |
| Images loaded via tarball archive | Compatible with both Podman and Docker; avoids registry dependency. `kind load docker-image` is Podman-incompatible; `kind load image-archive` works with any container engine |
| Localhost image refs for LOCAL_IMAGES via kustomize transformer | Local builds use `localhost/` image names (e.g. `localhost/hypershell-controller:dev`). A kustomize `images` transformer maps registry refs to localhost refs at manifest-apply time, so pods start with the correct image on the first rollout  -- no wasted deploy-then-patch cycle. `kubectl describe pod` shows the actual local image, not a registry ref that could be mistaken for a remote pull |
| Rebuild-and-replace on every swap call | Each `kind-<component>-up` rebuilds from the working tree and replaces the deployment, even if already swapped; developers iterate by re-running the same target |
| Web console as first-class component | Node.js frontend (`components/web-console/`) deployed alongside API server and control plane; supports hot reload via `KIND_HOT_RELOAD` for rapid UI iteration |
| Jaeger all-in-one for local tracing | One workload provides both OTLP receivers (gRPC `4317` for the API server, HTTP `4318` for the web console) and the query UI with in-memory storage; avoids a separate OpenTelemetry Collector in development. The BFF exports over OTLP/HTTP; the browser exports through the same-origin BFF endpoint. `KIND_JAEGER=true` opts in |
| Hot reload on by default | Swap targets for supported components (web console) mount host source and run a dev server in an interactive TTY by default; `KIND_HOT_RELOAD=false` opts out to rebuild-and-replace. Keeps the same `kind-<component>-up` entrypoint for both workflows |
| Per-component targets require existing cluster | Avoids implicit full-stack deployment; keeps intent explicit |
| All databases provisioned by CNPG operator | The API server database (in `hypershell-system`) uses a static CNPG Cluster. Gateway databases use CNPG Clusters created dynamically by the ManagedDatabaseReconciler  -- each ManagedDatabase (provider=cnpg) gets its own namespace and CNPG Cluster. No standalone PostgreSQL Deployments |
| Red Hat Hardened Images | HI images (`registry.access.redhat.com/hi/...`) are distroless, CIS-hardened, and signed at build time. No fallback to standard RHEL images - HI is the only supported path |
| Independent database image configuration | `HYPERSHELL_DATABASE_IMAGE` configures the API server CNPG Cluster image. `OPENSHELL_DATABASE_IMAGE` configures the image for ManagedDatabase CNPG Clusters (applied by the ManagedDatabaseReconciler). When unset, CNPG uses its built-in default |
| CNPG as cluster prerequisite | CloudNativePG operator installed via release manifest (like cert-manager). The API server CNPG Cluster is static infrastructure in `hypershell-system`. Gateway CNPG Clusters are created dynamically by the ManagedDatabaseReconciler per ManagedDatabase resource |
| Multiple deployments via namespace isolation | Additional platform deployments go into separate namespaces within the same Kind cluster with distinct hostnames via `KIND_NAMESPACE=<name> make kind-up`. More performant than separate Kind clusters; shares cluster-level resources (Gateway API CRDs, cert-manager, cloud-provider-kind). `kind-down` removes the target namespace; `kind-teardown` destroys the cluster |
| Gateway API CRDs from experimental channel | Experimental channel includes BackendTLSPolicy (required for TLS re-encrypt); standard channel does not. CRDs must be installed before cloud-provider-kind starts |
| cloud-provider-kind as Gateway API controller | Kind has no built-in LoadBalancer or Gateway API support; cloud-provider-kind provides both, implementing the GatewayClass and serving as the data-plane proxy for GRPCRoute traffic |
| cloud-provider-kind pinned by exact SHA, built idempotently | `CLOUD_PROVIDER_KIND_REF` pins an exact fork commit (mirrors the `KIND_VERSION` pseudo-version pin) so every developer builds the same deterministic binary. `make kind-prereqs` compares the on-disk `vcs.revision` against the pin and skips the rebuild when current; `CLOUD_PROVIDER_KIND_BRANCH` is an optional override for testing a moving branch tip |
| cloud-provider-kind fork advertises ALPN h2 downstream | The gateway speaks gRPC (HTTP/2). The fork advertises ALPN `h2` on the client-facing TLS listener so HTTP/2 is negotiated in the TLS handshake, and walks GRPCRoute-attached BackendTLSPolicy so backends are reached over re-encrypted HTTP/2 - matching the production Envoy/Istio behavior |
| Reuse cloud-provider-kind; restart only on change | Restarting cloud-provider-kind republishes the gateway LoadBalancer on new ephemeral host ports, invalidating pfctl/iptables rules, in-cluster DNAT, and CoreDNS. `make kind-up` reuses a running instance and restarts only on a pinned-build change, a wedged daemon, or `KIND_RESTART_CPK=true`. `make kind-fix-ports` recovers forwarding after an intentional restart |
| Background sudo keepalive during kind-up | `make kind-up` caches sudo once and refreshes the timestamp every ~50s so the pfctl/iptables and DNS resolver steps - which run minutes later, after image builds and rollouts - do not fail silently when the default sudo timeout expires |
| GRPCRoute, not HTTPRoute | OpenShell gateway speaks gRPC; GRPCRoute is the correct Gateway API resource type for gRPC backends |
| BackendTLSPolicy for TLS re-encrypt | Matches production topology - the networking Gateway verifies the pod's cert via CA ConfigMap rather than terminating TLS and sending plaintext to the pod. Exercises the same code path the control plane uses on OpenShift |
| Networking Gateway installed by kind-up | Cluster-level infrastructure (GatewayClass + Gateway), not per-tenant; the control plane only manages per-gateway route resources (GRPCRoute, BackendTLSPolicy, CA ConfigMap) |
| cert-manager as prerequisite | Automates TLS certificate lifecycle (issuance, renewal, rotation) for gateway certificates; eliminates manual re-runs of the certgen job |
| Keycloak for local OIDC | Local instance mirrors the downstream Keycloak topology (realm `hypershell`, per-gateway clients, provisioner service account); `KIND_KEYCLOAK_URL` override allows testing against an external instance |
| OIDC only, no mTLS | Team agreed to drop mTLS client auth; OIDC is the recommended auth mode for Kubernetes deployments per upstream docs |
| TLS always enabled | BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod (see Gateway API Routing section); the gateway must serve TLS even in local environments. cert-manager issues a self-signed CA for both the wildcard listener cert and the pod's server cert |
| Configurable `IMAGE_REGISTRY` and `IMAGE_TAG` | Allows teams to test against different builds or staging registries |
| Single root Makefile | All targets live in the root Makefile - build, test, codegen, and cluster lifecycle. Component-level Makefiles (`components/api-server/Makefile`, etc.) are deprecated; a single entrypoint eliminates indirection and makes `make <tab>` discoverable. Kind and OpenShift lifecycle dispatch through `scripts/cluster/` (`CLUSTER_DRIVER=kind` or `openshift`). The Kind driver wraps `scripts/kind/` (`lib.sh`, `up.sh`, `down.sh`, `teardown.sh`, `status.sh`, `build-images.sh`, `swap-component.sh`) without behavior change. The Makefile exports configuration and selects the driver by target name. Output uses colored headers (`NO_COLOR` respected) |
