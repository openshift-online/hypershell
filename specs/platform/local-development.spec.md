# Local Development Environment

**Date:** 2026-08-04
**Status:** Draft
**Jira:** ENGPROD-10281

## Purpose

HyperShell provides a single-command local development environment using Kind (Kubernetes in Docker) clusters. The environment deploys all platform components — API server, control plane, and web console — so developers can test changes end-to-end without external infrastructure. The database is provisioned by the control plane reconciler, not by `kind-up` directly. The tooling is idempotent: running it repeatedly converges to the desired `main` state without errors.

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
| cloud-provider-kind | LoadBalancer and Gateway API support for Kind clusters; required for GRPCRoute and Gateway resources to receive external IPs |
| cert-manager | TLS certificate lifecycle for gateway certificates (issuance, renewal, rotation) |
| Keycloak | OIDC identity provider for local gateway authentication testing (skipped when `KIND_KEYCLOAK_URL` is set) |

[cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind) SHALL be started as a background process after the Kind cluster is created. It provides LoadBalancer service support and Gateway API conformance (HTTPRoute, GRPCRoute) that Kind does not natively offer. On macOS and Windows, where container IPs are not directly reachable from the host, cloud-provider-kind SHALL use port mapping to expose LoadBalancer services on `localhost`. The version SHALL be pinned via a `CLOUD_PROVIDER_KIND_VERSION` variable. `make kind-up` SHALL verify that the `cloud-provider-kind` binary is available in `PATH` and print an install hint (e.g. `brew install cloud-provider-kind` or `go install sigs.k8s.io/cloud-provider-kind@latest`) if it is missing. The process SHALL be stopped by `make kind-down`.

cert-manager SHALL be installed by applying the release manifest from `https://github.com/cert-manager/cert-manager/releases/download/<version>/cert-manager.yaml`, skipping if the `cert-manager` namespace already exists (idempotent), and waiting for both the `cert-manager` and `cert-manager-webhook` deployments to reach ready state before proceeding. The version SHALL be pinned via a `CERT_MANAGER_VERSION` variable (default: `v1.20.0`).

Keycloak SHALL be deployed into the Kind cluster by default. When the `KIND_KEYCLOAK_URL` environment variable is set, the local Keycloak deployment SHALL be skipped and the Gateway OIDC issuer SHALL point at the external URL instead. This allows developers to test against a shared downstream Keycloak instance (e.g. the production broker described in the [downstream Keycloak design](https://gist.github.com/jhjaggars/5042c84888fb0c24020377a21d98f9a1) — a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients).

### Gateway Resource

`make kind-up` SHALL create a Gateway resource that the control plane reconciler uses to provision the full gateway stack. The Gateway resource SHALL include:

```yaml
apiVersion: hypershell.redhat.com/v1alpha1
kind: Gateway
name: openshell-gateway
database:
  type: postgres
  image: registry.redhat.io/rhel9/postgresql-18:latest
serverDnsNames:
  - openshell-gateway.hypershell-system.svc.cluster.local
oidc:
  issuer: http://keycloak-service:8080/realms/hypershell
  audience: hypershell-frontend
  roles_claim: groups
  admin_role: hypershell-admins
  user_role: hypershell-users
```

The gateway SHALL always use PostgreSQL (`database.type: postgres`). SQLite is not supported. The local environment SHALL NOT deploy PostgreSQL directly — the control plane reconciler provisions a production-style PostgreSQL database via the GatewayReconciler (see `specs/platform/openshell-gateway-database.spec.md`). This ensures the local environment exercises the same database provisioning path used in production.

TLS SHALL NOT be disabled. The gateway serves TLS using certificates issued by cert-manager (self-signed CA). Authentication SHALL use OIDC only — mTLS client authentication is not supported.

### Keycloak Configuration

The Kind cluster Keycloak instance serves as the local equivalent of the downstream Keycloak described in the [downstream Keycloak design](https://gist.github.com/jhjaggars/5042c84888fb0c24020377a21d98f9a1) (a downstream Keycloak that brokers authentication to Red Hat SSO and manages per-gateway OIDC clients). In production, the downstream Keycloak brokers authentication to Red Hat SSO (upstream) and manages per-gateway OIDC clients. The local instance mirrors this topology without the upstream broker, providing the same realm structure and client model.

| Setting | Value |
|---------|-------|
| Realm | `hypershell` |
| Client | `hypershell-frontend` (public, standard flow + direct access grants) |
| Provisioner client | `hypershell-provisioner` (confidential, service account with `manage-clients` and `manage-users` roles) |
| Admin role | `hypershell-admins` |
| User role | `hypershell-users` |
| Users | `admin` (admin role), `developer` (user role) |

The OIDC issuer URL SHALL be reachable from both inside the cluster (gateway pod) and outside (developer workstation).

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_KEYCLOAK_URL` | (unset — deploy local) | External Keycloak issuer URL; skips local deployment when set |

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
- THEN if `KIND_HOT_RELOAD=true`, the web console source SHALL be mounted from the host and a dev server (`npm run dev`) SHALL be started
- OTHERWISE the web console image SHALL be built from the current working tree, loaded, and the deployment replaced
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
- AND service endpoints with their host ports SHALL be displayed
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

### Requirement: Deterministic Host Port Exposure

All services that developers need to access during development SHALL be exposed to the host using Kind `extraPortMappings` and Kubernetes `NodePort` services. The system SHALL NOT use `kubectl port-forward` for service access. Each exposed port SHALL have a corresponding environment variable for configuration.

| Service | Env Var | Default Host Port | NodePort | Purpose |
|---------|---------|-------------------|----------|---------|
| HTTP API | `KIND_API_PORT` | `23080` | `30080` | REST API access |
| gRPC | `KIND_GRPC_PORT` | `29000` | `30090` | gRPC streaming (control plane, CLI) |
| Health | `KIND_HEALTH_PORT` | `24434` | `30434` | Health check endpoint |
| Web Console | `KIND_CONSOLE_PORT` | `23000` | `30300` | Browser UI |

#### Scenario: Default Ports
- GIVEN no port environment variables are set
- WHEN `make kind-up` completes
- THEN the HTTP API SHALL be accessible at `localhost:23080`
- AND the gRPC endpoint SHALL be accessible at `localhost:29000`
- AND the health endpoint SHALL be accessible at `localhost:24434`
- AND the web console SHALL be accessible at `localhost:23000`

#### Scenario: Custom Ports
- GIVEN `KIND_API_PORT` is set to `8080`
- WHEN `make kind-up` completes
- THEN the HTTP API SHALL be accessible at `localhost:8080`
- AND all other services SHALL use their default ports

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

The system SHALL track which components have been swapped to local builds using a `.kind-swaps` file at the repository root. This file SHALL be listed in `.gitignore`. The file records the set of currently swapped components so that `make kind-status` can report this information. Running `make kind-up` SHALL preserve existing swap state — it pulls the latest baseline images from `main` and reapplies manifests, but does not rebuild from the working tree or clear swap tracking.

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

### Requirement: No Separate Rebuild Target

The system SHALL NOT provide a separate `make kind-rebuild` target. The `make kind-up` target SHALL absorb full rebuild-and-redeploy behavior. Per-component targets (`make kind-api-server-up`, `make kind-control-plane-up`, `make kind-web-console-up`) handle selective rebuilds from local source.

### Requirement: Hot Reload Support

The Kind cluster configuration SHALL include `extraMounts` that map a host directory into the cluster nodes, enabling `hostPath` volumes for live source mounting. The web console is the first component to support hot reload.

| Setting | Value |
|---------|-------|
| Host path | `KIND_HOST_MOUNT_PATH` env var (default: repository root) |
| Container path | `/mnt/host` on each Kind node |
| Read-only | `false` (writable, required for npm file watchers) |

When hot reload is enabled, `kind-<component>-up` for a supported component SHALL mount the host source directory into the container and run a dev server (e.g. `npm run dev`) instead of performing a full image rebuild. File changes on the host are reflected inside the container immediately. When hot reload is disabled (the default), `kind-<component>-up` SHALL rebuild the image from the working tree and replace the deployment as normal.

| Env Var | Default | Description |
|---------|---------|-------------|
| `KIND_HOT_RELOAD` | (unset — disabled) | Set to `true` to enable hot reload mode for components that support it |

| Component | Hot Reload Support |
|-----------|-------------------|
| Web Console | Yes — mounts `components/web-console/` and runs `npm run dev` |
| API Server | No — Go service, rebuild-and-replace only |
| Control Plane | No — Go service, rebuild-and-replace only |

#### Scenario: Web Console Hot Reload
- GIVEN a Kind cluster is running
- AND `KIND_HOT_RELOAD=true` is set
- WHEN a developer runs `make kind-web-console-up`
- THEN the `components/web-console/` source directory SHALL be mounted from the host into the container via a `hostPath` volume
- AND `npm run dev` SHALL be started inside the container
- AND file changes on the host SHALL be reflected inside the container without rebuilding

#### Scenario: Web Console Without Hot Reload
- GIVEN a Kind cluster is running
- AND `KIND_HOT_RELOAD` is not set
- WHEN a developer runs `make kind-web-console-up`
- THEN the web console image SHALL be rebuilt from the working tree
- AND the deployment SHALL be replaced with the newly built image

#### Scenario: Hot Reload on Unsupported Component
- GIVEN `KIND_HOT_RELOAD=true` is set
- WHEN a developer runs `make kind-api-server-up` or `make kind-control-plane-up`
- THEN the component SHALL fall back to the normal rebuild-and-replace flow
- AND an info message SHALL indicate that hot reload is not supported for that component

### Requirement: Container Registry

The system SHALL pull baseline images from the container registry at `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/`. The registry path and image tag SHALL be configurable via environment variables.

| Env Var | Default |
|---------|---------|
| `IMAGE_REGISTRY` | `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main` |
| `IMAGE_TAG` | `latest` |

## Make Targets Summary

| Target | Behavior |
|--------|----------|
| `make kind-up` | Create cluster (if needed) + pull baseline images from registry + deploy + wait for readiness |
| `make kind-down` | Delete the Kind cluster |
| `make kind-status` | Show cluster info, pods, services, host ports, and active component swaps |
| `make kind-api-server-up` | Build api-server from working tree + load + replace deployment + wait (cluster must exist; idempotent — rebuilds and replaces on every call) |
| `make kind-api-server-down` | Revert api-server to baseline image + restart + wait |
| `make kind-control-plane-up` | Build control-plane from working tree + load + replace deployment + wait (cluster must exist; idempotent — rebuilds and replaces on every call) |
| `make kind-control-plane-down` | Revert control-plane to baseline image + restart + wait |
| `make kind-web-console-up` | With `KIND_HOT_RELOAD`: mount source + run `npm run dev`; without: build + load + replace deployment + wait |
| `make kind-web-console-down` | Revert web-console to baseline image + restart + wait |

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Registry pull for baseline images | Faster setup; no local build required for baseline; per-component swap handles local development |
| Per-component swap instead of `LOCAL_IMAGES` env var | More ergonomic; discoverable via tab-completion; avoids env var memorization |
| No separate `kind-rebuild` | `kind-up` always converges; per-component targets handle selective rebuilds |
| NodePort + `extraPortMappings` for all services | Deterministic host ports; no background `kubectl port-forward` processes required |
| Per-service configurable ports via env vars | Avoids conflicts when running multiple clusters or services on the same host |
| Pre-check cluster existence for idempotency | Check `kind get clusters` for the target name before attempting creation; skip if already present. Avoids `\|\| true` which swallows real failures (Docker not running, resource exhaustion) |
| Images loaded via tarball archive | Compatible with both Podman and Docker; avoids registry dependency |
| Rebuild-and-replace on every swap call | Each `kind-<component>-up` rebuilds from the working tree and replaces the deployment, even if already swapped; developers iterate by re-running the same target |
| Web console as first-class component | Node.js frontend (`components/web-console/`) deployed alongside API server and control plane; supports hot reload via `KIND_HOT_RELOAD` for rapid UI iteration |
| Hot reload via `KIND_HOT_RELOAD` flag | When enabled, swap targets for supported components (web console) mount host source and run a dev server instead of rebuilding; when disabled (default), swap targets rebuild and replace as normal. Keeps the same `kind-<component>-up` entrypoint for both workflows |
| Per-component targets require existing cluster | Avoids implicit full-stack deployment; keeps intent explicit |
| Database provisioned by control plane | Gateway configured with `database.type: postgres` and RHEL postgresql-18 image; control plane reconciler provisions the database via GatewayReconciler (`specs/platform/openshell-gateway-database.spec.md`), exercising the same path as production |
| PostgreSQL 18 from Red Hat registry | Red Hat registry image (`registry.redhat.io/rhel9/postgresql-18`) avoids Docker Hub rate limits and matches production image policy; hardened image (HI) adoption is planned for a future iteration |
| cloud-provider-kind for LoadBalancer and Gateway API | Kind has no built-in LoadBalancer support; cloud-provider-kind provides it plus Gateway API conformance (HTTPRoute, GRPCRoute), required for GRPCRoute-based gateway traffic routing |
| cert-manager as prerequisite | Automates TLS certificate lifecycle (issuance, renewal, rotation) for gateway certificates; eliminates manual re-runs of the certgen job |
| Keycloak for local OIDC | Local instance mirrors the downstream Keycloak topology (realm `hypershell`, per-gateway clients, provisioner service account); `KIND_KEYCLOAK_URL` override allows testing against an external instance |
| OIDC only, no mTLS | Team agreed to drop mTLS client auth; OIDC is the recommended auth mode for Kubernetes deployments per upstream docs |
| TLS always enabled | BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod; the gateway must serve TLS even in local environments. cert-manager issues a self-signed CA and server certificate |
| Postgres only, no SQLite | SQLite option dropped; postgres Deployment is the only supported workload mode, eliminating StatefulSet/PVC coupling |
| Configurable `IMAGE_REGISTRY` and `IMAGE_TAG` | Allows teams to test against different builds or staging registries |
| Single root Makefile | All targets live in the root Makefile — build, test, codegen, and cluster lifecycle. Component-level Makefiles (`components/api-server/Makefile`, etc.) are deprecated; a single entrypoint eliminates indirection and makes `make <tab>` discoverable |
