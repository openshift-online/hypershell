# Local Development Environment

**Date:** 2026-08-04
**Status:** Draft
**Jira:** ENGPROD-10281

## Purpose

HyperShell provides a single-command local development environment using Kind (Kubernetes in Docker) clusters. The environment deploys all platform components — API server, control plane, and web console — so developers can test changes end-to-end without external infrastructure. The database is provisioned by the control plane reconciler, not by `kind-up` directly. The tooling is idempotent: running it repeatedly converges to the desired `main` state without errors. For offline or air-gapped environments, `LOCAL_IMAGES=true` builds all baseline images from the local `main` branch instead of pulling from the registry.

Developers selectively swap individual components with local builds using per-component targets. The baseline cluster runs pre-built images pulled from the container registry; individual components are "swapped in" from local source as needed. Selective swapping converges to the current working tree state.

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
| Keycloak | OIDC identity provider for local gateway authentication testing (skipped when `KIND_KEYCLOAK_URL` is set) |

`make kind-up` SHALL install the Gateway API CRDs before starting cloud-provider-kind. The CRDs SHALL be applied from the upstream release bundle (`https://github.com/kubernetes-sigs/gateway-api/releases/download/<version>/experimental-install.yaml`) using the `experimental` channel, which includes BackendTLSPolicy. The version SHALL be pinned via a `GATEWAY_API_VERSION` variable. On OpenShift 4.19+ these CRDs ship by default; on Kind they must be installed explicitly.

[cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind) SHALL be started as a background process after the Gateway API CRDs are installed and the Kind cluster is created. It provides LoadBalancer service support and acts as a Gateway API controller, implementing the GatewayClass and proxying traffic for GRPCRoute resources. On macOS and Windows, where container IPs are not directly reachable from the host, cloud-provider-kind SHALL use port mapping to expose LoadBalancer services on `localhost`. The version SHALL be pinned via a `CLOUD_PROVIDER_KIND_VERSION` variable. `make kind-up` SHALL verify that the `cloud-provider-kind` binary is available in `PATH` and print an install hint (e.g. `brew install cloud-provider-kind` or `go install sigs.k8s.io/cloud-provider-kind@latest`) if it is missing. The process SHALL be stopped by `make kind-down`.

cert-manager SHALL be installed by applying the release manifest from `https://github.com/cert-manager/cert-manager/releases/download/<version>/cert-manager.yaml`, skipping if the `cert-manager` namespace already exists (idempotent), and waiting for both the `cert-manager` and `cert-manager-webhook` deployments to reach ready state before proceeding. The version SHALL be pinned via a `CERT_MANAGER_VERSION` variable (default: `v1.21.1`).

Keycloak SHALL be deployed into the Kind cluster by default. When the `KIND_KEYCLOAK_URL` environment variable is set, the local Keycloak deployment SHALL be skipped and the Gateway OIDC issuer SHALL point at the external URL instead. This allows developers to test against a shared downstream Keycloak instance (e.g. the production broker — a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients).

### Gateway Resource

`make kind-up` SHALL create a Gateway resource that the control plane reconciler uses to provision the full gateway stack. The Gateway resource SHALL include:

```yaml
apiVersion: hypershell.redhat.com/v1alpha1
kind: Gateway
name: openshell-gateway
database:
  image: registry.access.redhat.com/hi/postgresql:18
serverDnsNames:
  - openshell-gateway.hypershell-system.svc.cluster.local
oidc:
  issuer: http://keycloak-service:8080/realms/hypershell
  audience: hypershell-frontend
  roles_claim: groups
  admin_role: hypershell-admins
  user_role: hypershell-users
```

The local environment SHALL NOT deploy PostgreSQL directly — the control plane reconciler provisions a production-style PostgreSQL database via the GatewayReconciler (see `specs/platform/openshell-gateway-database.spec.md` — planned, see [#2](https://github.com/openshift-online/hypershell/pull/2)). This ensures the local environment exercises the same database provisioning path used in production.

TLS SHALL NOT be disabled. The gateway serves TLS using certificates issued by cert-manager (self-signed CA). The OIDC issuer uses HTTP because the local Keycloak instance runs in dev mode without TLS; the TLS requirement applies to the gateway's own serving certificate, not to the OIDC issuer endpoint. Authentication SHALL use OIDC only — mTLS client authentication is not supported.

### Keycloak Configuration

The Kind cluster Keycloak instance serves as the local equivalent of the downstream Keycloak — a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients. In production, the downstream Keycloak brokers authentication to Red Hat SSO (upstream) and manages per-gateway OIDC clients. The local instance mirrors this topology without the upstream broker, providing the same realm structure and client model.

| Setting | Value |
|---------|-------|
| Realm | `hypershell` |
| Client | `hypershell-frontend` (public, standard flow + direct access grants) |
| Provisioner client | `hypershell-provisioner` (confidential, service account with `manage-clients` and `manage-users` roles) |
| Admin role | `hypershell-admins` |
| User role | `hypershell-users` |
| Users | `admin` / `admin` (admin role), `developer` / `developer` (user role) — password matches username (local dev only) |

The OIDC issuer URL SHALL be reachable from both inside the cluster (gateway pod) and outside (developer workstation).

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_KEYCLOAK_URL` | (unset — deploy local) | External Keycloak issuer URL; skips local deployment when set |

### Gateway API Routing

The local cluster uses the Kubernetes Gateway API to route all external traffic — both component services and gateway gRPC — through a single networking Gateway. This mirrors the production topology where a networking Gateway terminates external TLS. Component services (API, console, health) are exposed via HTTPRoute resources at `*.hypershell.localhost` hostnames; gateway gRPC traffic uses GRPCRoute at `*.gw.localhost`. `make kind-up` configures `/etc/hosts` entries for hostname resolution (prompts for `sudo` on first run). For environments where `/etc/hosts` cannot be modified (e.g. CI), `KIND_USE_NODEPORT=true` falls back to NodePort + `extraPortMappings` with port-based access.

#### Networking Gateway (Cluster-Level)

`make kind-up` SHALL create a networking `Gateway` resource that acts as the shared ingress point for all component HTTPRoutes and gateway GRPCRoutes. This is cluster-level infrastructure — not managed by the control plane reconciler.

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

`make kind-up` SHALL add `/etc/hosts` entries mapping component hostnames to `127.0.0.1`. On first run, the developer is prompted for `sudo` access. Subsequent runs update entries idempotently.

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

For multi-namespace deployments, component hostnames include the namespace: `api.<namespace>.hypershell.localhost` (e.g. `api.feature-add-auth.hypershell.localhost`). `make kind-deploy` creates namespace-scoped HTTPRoutes and adds the corresponding `/etc/hosts` entries.

#### Per-Gateway Route Resources (Control Plane Reconciled)

For each HyperShell Gateway resource, the control plane reconciler creates three Kubernetes resources:

1. **GRPCRoute** — Routes gRPC traffic from the networking Gateway to the gateway pod's Service:
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

2. **BackendTLSPolicy** — Instructs the networking Gateway to establish a TLS connection to the backend pod and verify its certificate against the CA ConfigMap (TLS re-encrypt):
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

3. **CA ConfigMap** — Contains the gateway pod's CA certificate so the networking Gateway can verify the pod's TLS cert:
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
| DNS resolution | `/etc/hosts` managed by `kind-up` | Cluster DNS / external DNS |

## Requirements

### Requirement: Single-Command Environment Setup

The system SHALL provide a `make kind-up` target at the repository root that creates a fully functional local HyperShell environment. Baseline images SHALL be pulled from the container registry.

#### Scenario: First Run — Clean State
- GIVEN no Kind cluster exists
- WHEN a developer runs `make kind-up`
- THEN a Kind cluster SHALL be created
- AND all component images SHALL be pulled from the container registry
- AND images SHALL be loaded into the Kind cluster
- AND all Kubernetes resources SHALL be applied (namespace, API server, control plane)
- AND the system SHALL wait for all components to become ready
- AND connection information SHALL be printed to stdout

#### Scenario: Subsequent Run — Idempotent
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
- AND service hostnames (or host ports in NodePort mode) SHALL be displayed
- AND the output SHALL indicate which components have active local swaps versus baseline images

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

All services SHALL be accessible via `.localhost` hostnames routed through the networking Gateway (see Gateway API Routing). The system SHALL NOT use `kubectl port-forward` for service access.

| Service | Hostname | Purpose |
|---------|----------|---------|
| HTTP API | `https://api.hypershell.localhost` | REST API access |
| Web Console | `https://console.hypershell.localhost` | Browser UI |
| Health | `https://health.hypershell.localhost` | Health check endpoint |
| gRPC | `https://<gateway-name>.gw.localhost` | gRPC streaming (control plane, CLI) |

`make kind-up` SHALL configure `/etc/hosts` entries for all service hostnames, mapping them to `127.0.0.1`. The developer is prompted for `sudo` on first run. Subsequent runs update entries idempotently. The self-signed TLS certificate issued by cert-manager must be trusted by the developer's browser or CLI tool (e.g. `curl --cacert`).

For multi-namespace deployments, hostnames include the namespace: `api.<namespace>.hypershell.localhost`. `make kind-deploy` adds the corresponding `/etc/hosts` entries automatically. No port management is required — each namespace is differentiated by hostname, not port number.

#### Scenario: Default Access
- GIVEN no special configuration
- WHEN `make kind-up` completes
- THEN the HTTP API SHALL be accessible at `https://api.hypershell.localhost`
- AND the web console SHALL be accessible at `https://console.hypershell.localhost`
- AND the health endpoint SHALL be accessible at `https://health.hypershell.localhost`
- AND `/etc/hosts` SHALL contain entries for all service hostnames

#### Scenario: Multi-Namespace Hostname Differentiation
- GIVEN the default deployment is running in `hypershell-system`
- AND a second deployment is running in `hypershell-feature-add-auth`
- WHEN a developer accesses `https://api.feature-add-auth.hypershell.localhost`
- THEN the request SHALL route to the API server in the `hypershell-feature-add-auth` namespace

#### Scenario: NodePort Fallback
- GIVEN `KIND_USE_NODEPORT=true` is set
- WHEN `make kind-up` completes
- THEN services SHALL be exposed via NodePort + `extraPortMappings` instead of hostname routing
- AND the HTTP API SHALL be accessible at `localhost:23080`
- AND the gRPC endpoint SHALL be accessible at `localhost:29000`
- AND the health endpoint SHALL be accessible at `localhost:24434`
- AND the web console SHALL be accessible at `localhost:23000`
- AND `/etc/hosts` SHALL NOT be modified

When `KIND_USE_NODEPORT=true` is set, per-service ports are configurable via environment variables:

| Service | Env Var | Default Host Port | NodePort |
|---------|---------|-------------------|----------|
| HTTP API | `KIND_API_PORT` | `23080` | `30080` |
| gRPC | `KIND_GRPC_PORT` | `29000` | `30090` |
| Health | `KIND_HEALTH_PORT` | `24434` | `30434` |
| Web Console | `KIND_CONSOLE_PORT` | `23000` | `30300` |

For multi-namespace deployments in NodePort mode, the system SHALL auto-allocate ports using `KIND_PORT_OFFSET` (adds N to each base port). When the allocated port would conflict, `make kind-deploy` SHALL warn the user.

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

- Prerequisites (Docker or Podman, Kind, kubectl)
- `make kind-up` quickstart with expected output
- Per-component swap workflow (`make kind-<component>-up` / `make kind-<component>-down`)
- Hot reload setup for the web console (`KIND_HOT_RELOAD=true`)
- Environment variable reference (all `KIND_*`, `IMAGE_*`, and `CONTAINER_ENGINE` variables)
- Keycloak configuration and `KIND_KEYCLOAK_URL` for external OIDC
- Troubleshooting common issues (port conflicts, container engine not running, image pull failures)

The documentation SHALL be kept in sync with this spec. When a new Make target, environment variable, or component is added, the guide SHALL be updated in the same PR.

#### Scenario: Documentation Exists
- GIVEN a developer clones the repository
- WHEN they look for local development instructions
- THEN `DEVELOPMENT.md` SHALL exist and describe how to set up and use the Kind environment

#### Scenario: Documentation Stays Current
- GIVEN a PR adds or changes a `kind-*` Make target or environment variable
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
| Web Console | Yes — mounts `components/web-console/` and runs `npm run dev` |
| API Server | No — Go service, rebuild-and-replace only |
| Control Plane | No — Go service, rebuild-and-replace only |

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

The system SHALL support offline development by building all baseline images from the local repository instead of pulling from the container registry. When `LOCAL_IMAGES=true` is set, `make kind-up` SHALL build every component image from `origin/main` and load them into the Kind cluster. Repeated `kind-up` invocations with `LOCAL_IMAGES=true` SHALL rebuild from `origin/main`, picking up any new commits — analogous to a `git fetch` for images.

| Env Var | Default | Description |
|---------|---------|-------------|
| `LOCAL_IMAGES` | (unset — pull from registry) | Set to `true` to build baseline images from the local repository instead of pulling from the container registry |

#### Scenario: First Run — Offline
- GIVEN no Kind cluster exists
- AND the developer has no access to the container registry
- AND `LOCAL_IMAGES=true` is set
- WHEN the developer runs `make kind-up`
- THEN all component images SHALL be built from the local repository
- AND images SHALL be loaded into the Kind cluster
- AND the cluster SHALL reach a ready state without any registry pulls for platform components

#### Scenario: Subsequent Run — Rebuild from Main
- GIVEN a Kind cluster is running with locally-built images
- AND `LOCAL_IMAGES=true` is set
- WHEN the developer runs `make kind-up` again
- THEN all non-swapped component images SHALL be rebuilt from `origin/main`
- AND updated images SHALL be loaded into the Kind cluster
- AND swapped components SHALL be preserved

### Requirement: Red Hat Hardened Images

All container images deployed into the Kind cluster SHALL use [Red Hat Hardened Images](https://images.redhat.com/) (HI). HI images are distroless, CIS-hardened, and signed at build time.

The database image specified in the Gateway resource SHALL default to `registry.access.redhat.com/hi/postgresql:18`. Developers SHALL configure a pull secret in the Kind cluster to authenticate against `registry.access.redhat.com`. The pull secret is expected to be present before running `make kind-up`; the Makefile SHALL NOT manage registry credentials.

The database image SHALL be overridable via the `KIND_DB_IMAGE` environment variable (e.g. `KIND_DB_IMAGE=docker.io/library/postgres:18`). This enables OSS contributors who do not have access to the Red Hat registry to use an alternative PostgreSQL image. Overriding the image is not officially supported — compatibility issues with non-HI images are the contributor's responsibility.

#### Scenario: HI Image Used for Database
- GIVEN a Kind cluster is running
- WHEN the control plane reconciler provisions a database for a Gateway
- THEN the database pod SHALL use the HI PostgreSQL image (`registry.access.redhat.com/hi/postgresql:18`)
- AND the image SHALL be pulled using a pre-configured pull secret set by the contributor

### Requirement: Multiple Namespace Deployments

The system SHALL support deploying the platform into multiple namespaces within a single Kind cluster. Each namespace gets its own set of HyperShell components with isolated hostnames, enabling developers to work on multiple features in parallel (e.g. when handing separate branches to agents).

The system SHALL provide a `make kind-deploy` target that deploys the platform into a new namespace. The namespace is derived from the current git branch name, sanitized to a valid Kubernetes namespace (lowercase, alphanumeric and hyphens, max 63 characters). Each namespace gets its own HTTPRoutes with namespace-prefixed hostnames (e.g. `api.<namespace>.hypershell.localhost`) and corresponding `/etc/hosts` entries. The default deployment (`make kind-up`) uses the `hypershell-system` namespace and base hostnames; `kind-deploy` creates additional deployments alongside it.

Per-component swap and teardown targets operate on the specified namespace when `KIND_NAMESPACE` is set.

#### Scenario: Deploy to Additional Namespace from Branch
- GIVEN a Kind cluster is running with the default deployment in `hypershell-system`
- AND the developer is on branch `feature/add-auth`
- WHEN they run `make kind-deploy`
- THEN a namespace `hypershell-feature-add-auth` SHALL be created (derived from the branch name)
- AND a full set of HyperShell components SHALL be deployed into that namespace
- AND namespace-scoped HTTPRoutes SHALL be created (e.g. `api.feature-add-auth.hypershell.localhost`)
- AND `/etc/hosts` entries SHALL be added for the new hostnames
- AND both deployments SHALL run independently without interference

#### Scenario: Status Reports All Deployments
- GIVEN the platform is deployed into multiple namespaces
- WHEN a developer runs `make kind-status`
- THEN the output SHALL list all namespaces with their hostnames and swap state

#### Scenario: Teardown Namespace Deployment
- GIVEN the platform is deployed in `hypershell-system` and `hypershell-feature-add-auth`
- WHEN a developer runs `make kind-undeploy KIND_NAMESPACE=hypershell-feature-add-auth`
- THEN only the `hypershell-feature-add-auth` namespace and its resources SHALL be deleted
- AND the default deployment in `hypershell-system` SHALL continue running

#### Scenario: Per-Component Swap Scoped to Namespace
- GIVEN the platform is deployed in multiple namespaces
- WHEN a developer runs `KIND_NAMESPACE=hypershell-feature-add-auth make kind-api-server-up`
- THEN the API server SHALL be swapped only in the `hypershell-feature-add-auth` namespace
- AND the default deployment SHALL remain unchanged

## Environment Variable Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_CLUSTER_NAME` | `hypershell-dev` | Kind cluster name |
| `KIND_USE_NODEPORT` | (unset — hostname routing) | Set to `true` to use NodePort + `extraPortMappings` instead of hostname-based routing |
| `KIND_API_PORT` | `23080` | Host port for HTTP API (NodePort mode only) |
| `KIND_GRPC_PORT` | `29000` | Host port for gRPC (NodePort mode only) |
| `KIND_HEALTH_PORT` | `24434` | Host port for health endpoint (NodePort mode only) |
| `KIND_CONSOLE_PORT` | `23000` | Host port for web console (NodePort mode only) |
| `KIND_PORT_OFFSET` | (auto-assigned) | Port offset for multi-namespace deployments (NodePort mode only) |
| `KIND_HOT_RELOAD` | `true` | Hot reload for supported components; set to `false` to disable |
| `KIND_HOST_MOUNT_PATH` | Repository root (`git rev-parse --show-toplevel`) | Host directory mounted into Kind nodes for hot reload |
| `KIND_KEYCLOAK_URL` | (unset — deploy local) | External Keycloak issuer URL; skips local deployment when set |
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` | Container registry path for baseline images |
| `IMAGE_TAG` | `latest` | Image tag for baseline images |
| `LOCAL_IMAGES` | (unset — pull from registry) | Set to `true` to build baseline images from `origin/main` instead of pulling from registry |
| `CONTAINER_ENGINE` | Auto-detected (Podman preferred) | Container engine (`podman` or `docker`) |
| `GATEWAY_API_VERSION` | (pinned in Makefile) | Gateway API CRD release version |
| `CLOUD_PROVIDER_KIND_VERSION` | (pinned in Makefile) | cloud-provider-kind binary version |
| `CERT_MANAGER_VERSION` | `v1.21.1` | cert-manager release version |
| `KIND_DB_IMAGE` | `registry.access.redhat.com/hi/postgresql:18` | Database image for Gateway resource; override for OSS dev (unsupported) |
| `KIND_NAMESPACE` | `hypershell-system` | Target namespace for deployment; scopes swap/teardown targets to the specified namespace |

## Make Targets Summary

| Target | Behavior |
|--------|----------|
| `make kind-up` | Create cluster (if needed) + pull baseline images from registry + deploy + wait for readiness |
| `make kind-down` | Delete the Kind cluster |
| `make kind-status` | Show cluster info, pods, services, hostnames, and active component swaps |
| `make kind-api-server-up` | Build api-server from working tree + load + replace deployment + wait (cluster must exist; idempotent — rebuilds and replaces on every call) |
| `make kind-api-server-down` | Revert api-server to baseline image + restart + wait |
| `make kind-control-plane-up` | Build control-plane from working tree + load + replace deployment + wait (cluster must exist; idempotent — rebuilds and replaces on every call) |
| `make kind-control-plane-down` | Revert control-plane to baseline image + restart + wait |
| `make kind-web-console-up` | Default (hot reload): mount source + run `npm run dev` in interactive TTY; with `KIND_HOT_RELOAD=false`: build + load + replace deployment + wait |
| `make kind-web-console-down` | Revert web-console to baseline image + restart + wait |
| `make kind-deploy` | Deploy platform into a new namespace (derived from current branch) with namespace-scoped hostnames + configure `/etc/hosts` + wait |
| `make kind-undeploy` | Delete a namespace deployment and its resources (`KIND_NAMESPACE` required) |

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Registry pull for baseline images | Faster setup; no local build required for baseline; per-component swap handles local development |
| Per-component swap for iterative development | More ergonomic than blanket rebuild; discoverable via tab-completion; `LOCAL_IMAGES=true` serves a separate purpose — offline baseline builds from `main` when registry access is unavailable |
| Hostname routing via networking Gateway as default | All component services route through the networking Gateway using HTTPRoute resources at `*.hypershell.localhost`. Developers access services by name (`api.hypershell.localhost`) instead of memorizing port numbers. Multi-namespace deployments get distinct hostnames instead of port offsets. Requires `/etc/hosts` entries (managed by `kind-up`). `KIND_USE_NODEPORT=true` provides a fallback for CI or environments where `/etc/hosts` cannot be modified |
| NodePort fallback via `KIND_USE_NODEPORT` | When hostname routing is unavailable, services fall back to NodePort + `extraPortMappings` with configurable ports and auto-offset for multi-namespace |
| Pre-check cluster existence for idempotency | Check `kind get clusters` for the target name before attempting creation; skip if already present. Avoids `\|\| true` which swallows real failures (Docker not running, resource exhaustion) |
| Images loaded via tarball archive | Compatible with both Podman and Docker; avoids registry dependency |
| Rebuild-and-replace on every swap call | Each `kind-<component>-up` rebuilds from the working tree and replaces the deployment, even if already swapped; developers iterate by re-running the same target |
| Web console as first-class component | Node.js frontend (`components/web-console/`) deployed alongside API server and control plane; supports hot reload via `KIND_HOT_RELOAD` for rapid UI iteration |
| Hot reload on by default | Swap targets for supported components (web console) mount host source and run a dev server in an interactive TTY by default; `KIND_HOT_RELOAD=false` opts out to rebuild-and-replace. Keeps the same `kind-<component>-up` entrypoint for both workflows |
| Per-component targets require existing cluster | Avoids implicit full-stack deployment; keeps intent explicit |
| Database provisioned by control plane | Gateway configured with HI postgresql image; control plane reconciler provisions the database via GatewayReconciler (`specs/platform/openshell-gateway-database.spec.md` — planned, see [#2](https://github.com/openshift-online/hypershell/pull/2)), exercising the same path as production |
| Red Hat Hardened Images | HI images (`registry.access.redhat.com/hi/...`) are distroless, CIS-hardened, and signed at build time. No fallback to standard RHEL images — HI is the only supported path. `KIND_DB_IMAGE` override enables OSS contributors without Red Hat registry access to use an alternative image (unsupported) |
| Multiple deployments via namespace isolation | Additional platform deployments go into separate namespaces within the same Kind cluster with distinct hostnames. More performant than separate Kind clusters; shares cluster-level resources (Gateway API CRDs, cert-manager, cloud-provider-kind). Namespace derived from branch name via explicit `kind-deploy` target; each namespace gets namespace-prefixed hostnames (e.g. `api.<namespace>.hypershell.localhost`) |
| Gateway API CRDs from experimental channel | Experimental channel includes BackendTLSPolicy (required for TLS re-encrypt); standard channel does not. CRDs must be installed before cloud-provider-kind starts |
| cloud-provider-kind as Gateway API controller | Kind has no built-in LoadBalancer or Gateway API support; cloud-provider-kind provides both, implementing the GatewayClass and serving as the data-plane proxy for GRPCRoute traffic |
| GRPCRoute, not HTTPRoute | OpenShell gateway speaks gRPC; GRPCRoute is the correct Gateway API resource type for gRPC backends |
| BackendTLSPolicy for TLS re-encrypt | Matches production topology — the networking Gateway verifies the pod's cert via CA ConfigMap rather than terminating TLS and sending plaintext to the pod. Exercises the same code path the control plane uses on OpenShift |
| Networking Gateway installed by kind-up | Cluster-level infrastructure (GatewayClass + Gateway), not per-tenant; the control plane only manages per-gateway route resources (GRPCRoute, BackendTLSPolicy, CA ConfigMap) |
| cert-manager as prerequisite | Automates TLS certificate lifecycle (issuance, renewal, rotation) for gateway certificates; eliminates manual re-runs of the certgen job |
| Keycloak for local OIDC | Local instance mirrors the downstream Keycloak topology (realm `hypershell`, per-gateway clients, provisioner service account); `KIND_KEYCLOAK_URL` override allows testing against an external instance |
| OIDC only, no mTLS | Team agreed to drop mTLS client auth; OIDC is the recommended auth mode for Kubernetes deployments per upstream docs |
| TLS always enabled | BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod (see Gateway API Routing section); the gateway must serve TLS even in local environments. cert-manager issues a self-signed CA for both the wildcard listener cert and the pod's server cert |
| Configurable `IMAGE_REGISTRY` and `IMAGE_TAG` | Allows teams to test against different builds or staging registries |
| Single root Makefile | All targets live in the root Makefile — build, test, codegen, and cluster lifecycle. Component-level Makefiles (`components/api-server/Makefile`, etc.) are deprecated; a single entrypoint eliminates indirection and makes `make <tab>` discoverable |
