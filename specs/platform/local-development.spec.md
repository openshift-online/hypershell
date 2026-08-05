# Local Development Environment

**Date:** 2026-08-04
**Status:** Draft
**Jira:** ENGPROD-10281

## Purpose

HyperShell provides a single-command local development environment using Kind (Kubernetes in Docker) clusters. The environment deploys all platform components — API server and control plane — so developers can test changes end-to-end without external infrastructure. The database is provisioned by the control plane reconciler, not by `kind-up` directly. The tooling is idempotent: running it repeatedly converges to the desired `main` state without errors.

Developers selectively swap individual components with local builds using per-component targets. The baseline cluster runs pre-built images pulled from the container registry; individual components are "swapped in" from local source as needed. Selective swapping converges to the current working tree state.

## Components Deployed

| Component | Kind | Image | Purpose |
|-----------|------|-------|---------|
| API Server | Deployment | Registry: `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main:latest` | REST + gRPC API, with init container for DB migrations |
| Control Plane | Deployment | Registry: `quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-controller-main:latest` | gRPC watcher + reconciler for gateway lifecycle; provisions the database |

### Cluster Prerequisites

`make kind-up` SHALL install the following cluster-level prerequisites before deploying HyperShell components:

| Prerequisite | Purpose |
|--------------|---------|
| cert-manager | TLS certificate lifecycle for gateway certificates (issuance, renewal, rotation) |
| Keycloak | OIDC identity provider for local gateway authentication testing (skipped when `KIND_KEYCLOAK_URL` is set) |

cert-manager SHALL be installed via `kubectl apply -f` from the cert-manager release manifests (with CRDs enabled) and the setup SHALL wait for the cert-manager controller deployment to reach ready state before proceeding.

Keycloak SHALL be deployed into the Kind cluster by default. When the `KIND_KEYCLOAK_URL` environment variable is set, the local Keycloak deployment SHALL be skipped and the Gateway OIDC issuer SHALL point at the external URL instead. This allows developers to test against a shared downstream Keycloak instance (e.g. the production broker described in the [downstream Keycloak design](https://gist.github.com/jhjaggars/5042c84888fb0c24020377a21d98f9a1)).

### Gateway Resource

`make kind-up` SHALL create a Gateway resource that the control plane reconciler uses to provision the full gateway stack. The Gateway resource SHALL include:

```yaml
kind: Gateway
name: openshell-gateway
database:
  type: postgres
  image: registry.redhat.io/rhel9/postgresql-18:latest
serverDnsNames:
  - openshell-gateway.hypershell-system.svc.cluster.local
oidc:
  issuer: http://keycloak-service:8080/realms/openshell
  audience: openshell-cli
  roles_claim: realm_access.roles
  admin_role: openshell-admin
  user_role: openshell-user
```

The local environment SHALL NOT deploy PostgreSQL directly. The control plane reconciler provisions a production-style PostgreSQL database via the GatewayReconciler (see `openshell-gateway-database.spec.md`). This ensures the local environment exercises the same database provisioning path used in production.

### Keycloak Configuration

The Kind cluster Keycloak instance serves as the local equivalent of the downstream Keycloak described in the [downstream Keycloak design](https://gist.github.com/jhjaggars/5042c84888fb0c24020377a21d98f9a1). In production, the downstream Keycloak brokers authentication to Red Hat SSO (upstream) and manages per-gateway OIDC clients. The local instance mirrors this topology without the upstream broker, providing the same realm structure and client model.

| Setting | Value |
|---------|-------|
| Realm | `openshell` |
| Client | `openshell-cli` (public, standard flow + direct access grants) |
| Provisioner client | `openshell-provisioner` (confidential, service account with `manage-clients` and `manage-users` roles) |
| Admin role | `openshell-admin` |
| User role | `openshell-user` |
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

The system SHALL provide per-component Make targets that build a single component from the current working tree, load it into the running cluster, and restart only that component's deployment.

#### Scenario: Swap API Server
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-api-server-up`
- THEN the API server image SHALL be built from the current working tree
- AND the image SHALL be loaded into the Kind cluster
- AND the API server deployment SHALL be restarted
- AND the system SHALL wait for the API server to become ready

#### Scenario: Swap Control Plane
- GIVEN a Kind cluster is running
- WHEN a developer runs `make kind-control-plane-up`
- THEN the control plane image SHALL be built from the current working tree
- AND the image SHALL be loaded into the Kind cluster
- AND the control plane deployment SHALL be restarted
- AND the system SHALL wait for the control plane to become ready

#### Scenario: No Cluster Running
- GIVEN no Kind cluster exists
- WHEN a developer runs `make kind-api-server-up` or `make kind-control-plane-up`
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

#### Scenario: Revert When Not Swapped
- GIVEN a component is already running the baseline image
- WHEN a developer runs `make kind-api-server-down` or `make kind-control-plane-down`
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

#### Scenario: Default Ports
- GIVEN no port environment variables are set
- WHEN `make kind-up` completes
- THEN the HTTP API SHALL be accessible at `localhost:23080`
- AND the gRPC endpoint SHALL be accessible at `localhost:29000`
- AND the health endpoint SHALL be accessible at `localhost:24434`

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

The system SHALL track which components have been swapped to local builds using a `.kind-swaps` file at the repository root. This file SHALL be listed in `.gitignore`. The file records the set of currently swapped components so that `make kind-status` can report this information. Running `make kind-up` SHALL reset all swaps by clearing the file (since it rebuilds everything from scratch).

#### Scenario: Swap Reported in Status
- GIVEN a developer has run `make kind-api-server-up`
- WHEN they run `make kind-status`
- THEN the output SHALL indicate the API server is running a local build
- AND the control plane is running the baseline image

#### Scenario: Kind-Up Resets Swaps
- GIVEN a developer has swapped the API server to a local build
- WHEN they run `make kind-up`
- THEN all components SHALL be redeployed from registry images
- AND swap tracking SHALL be reset

### Requirement: No Separate Rebuild Target

The system SHALL NOT provide a separate `make kind-rebuild` target. The `make kind-up` target SHALL absorb full rebuild-and-redeploy behavior. Per-component targets (`make kind-api-server-up`, `make kind-control-plane-up`) handle selective rebuilds from local source.

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
| `make kind-api-server-up` | Build api-server from working tree + load + restart + wait (cluster must exist) |
| `make kind-api-server-down` | Revert api-server to baseline image + restart + wait |
| `make kind-control-plane-up` | Build control-plane from working tree + load + restart + wait (cluster must exist) |
| `make kind-control-plane-down` | Revert control-plane to baseline image + restart + wait |

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Registry pull for baseline images | Faster setup; no local build required for baseline; per-component swap handles local development |
| Per-component swap instead of `LOCAL_IMAGES` env var | More ergonomic; discoverable via tab-completion; avoids env var memorization |
| No separate `kind-rebuild` | `kind-up` always converges; per-component targets handle selective rebuilds |
| NodePort + `extraPortMappings` for all services | Deterministic host ports; no background `kubectl port-forward` processes required |
| Per-service configurable ports via env vars | Avoids conflicts when running multiple clusters or services on the same host |
| `kind create cluster \|\| true` for idempotency | Re-running after cluster exists is a no-op, not an error |
| Images loaded via tarball archive | Compatible with both Podman and Docker; avoids registry dependency |
| Deployment restart on swap | Forces pods to pick up newly loaded images even when the tag hasn't changed |
| Per-component targets require existing cluster | Avoids implicit full-stack deployment; keeps intent explicit |
| Database provisioned by control plane | Gateway configured with `database.type: postgres` and RHEL postgresql-18 image; control plane reconciler provisions the database via GatewayReconciler (`openshell-gateway-database.spec.md`), exercising the same path as production |
| PostgreSQL 18 with RHEL hardened image | Red Hat hardened image (`registry.redhat.io/rhel9/postgresql-18`) avoids Docker Hub rate limits and matches production image policy |
| cert-manager as prerequisite | Automates TLS certificate lifecycle (issuance, renewal, rotation) for gateway certificates; eliminates manual re-runs of the certgen job |
| Keycloak for local OIDC | Local instance mirrors the downstream Keycloak topology (realm `openshell`, per-gateway clients, provisioner service account); `KIND_KEYCLOAK_URL` override allows testing against an external instance |
| Configurable `IMAGE_REGISTRY` and `IMAGE_TAG` | Allows teams to test against different builds or staging registries |
