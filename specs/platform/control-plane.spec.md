# Control Plane

**Date:** 2026-08-03
**Status:** Active

## Overview

The HyperShell control plane is a Go service that watches the API server via gRPC streaming RPCs and reconciles the desired state (Gateway and related resources in the database) into Kubernetes resources in its local execution cluster. Each controller processes only its assigned gateways. It follows the informer-reconciler pattern without depending on controller-runtime.

## Architecture

```
Parent API Server (PostgreSQL)
  │  gRPC watch streams per Kind
  ▼
Control Plane (Watcher + Reconciler)
  │  reconciles into K8s resources
  ▼
Local Execution Cluster (Gateway pods, Services, Configs)
```

## Components

### Watcher

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Gateways, GatewayReleases, ManagedClusters, GatewayNetworks). On each event (create, update, delete), it dispatches to the Reconciler. Gateway events are dispatched through a per-gateway reconcile queue drained by a bounded worker pool: work for a single gateway is serialized while distinct gateways reconcile concurrently up to a configurable cap. The pool size and its configuration are defined in [`gateway-reconcile-concurrency.spec.md`](./gateway-reconcile-concurrency.spec.md).

### Reconciler

The Reconciler receives resource events from the Watcher and converges its local Kubernetes state for its assigned gateways. Key responsibilities:

- Deploy/update assigned Gateway workloads in the local execution cluster
- Create per-gateway SQL databases and roles on the controller-configured PostgreSQL server
- Configure TLS certificates via cert-manager
- Create GRPCRoute and BackendTLSPolicy for external gateway exposure
- Inject OIDC authentication configuration into gateway deployments
- Configure network meshes between gateways
- Manage release rollouts (including canary strategies)
- Provision and reconcile OpenShellGatewayServiceAccount identities in Keycloak through an internal in-cluster gRPC service
- Update resource status back to the API server

#### Gateway Provisioning Specifications

Gateway reconciliation is defined in detail across dedicated sub-specs:

| Sub-Spec | Scope |
|---|---|
| [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) | Core provisioning: GatewayReconciler, manifest templating, deployment resources, RBAC, OpenShift adjustments |
| [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) | PostgreSQL provisioning, credential security, manual rotation, deletion protection |
| [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) | TLS certificate management via cert-manager, SAN management, cert rotation |
| [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) | External connectivity: Gateway API (GRPCRoute + BackendTLSPolicy), NetworkPolicy |
| [`openshell-gateway-oidc.spec.md`](./openshell-gateway-oidc.spec.md) | OIDC authentication, role validation, gateway.toml injection |
| [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) | Phase lifecycle, workload-readiness gating, continuous health reconciliation |
| [`gateway-version-selection.spec.md`](./gateway-version-selection.spec.md) | Database-backed version selection: resolving `release_id` to a GatewayRelease image and its precedence over a direct image |
| [`gateway-release-reconciliation.spec.md`](./gateway-release-reconciliation.spec.md) | GatewayRelease reconciliation: image validation, deterministic release status, change propagation to referencing gateways |
| [`gateway-network-reconciliation.spec.md`](./gateway-network-reconciliation.spec.md) | GatewayNetwork reconciliation: topology vocabulary, topology/hub coherence and hub-reference validation, deterministic network status write-back |

### Config

Holds the local PostgreSQL credential reference, API server gRPC connection configuration, Kubernetes client initialization, and internal service-account provisioner configuration. The provisioner listens on an in-cluster gRPC port that a NetworkPolicy restricts to the API server pod. The API server does not receive Keycloak administrator credentials.

## Requirements

### Requirement: Spoke Self-Registration at Startup

Before opening any gRPC watch stream, the control plane SHALL call
`POST /api/hypershell/v1/managed_clusters/registration` using its OIDC
`client_credentials` token. The registration endpoint is idempotent; the returned
`cluster_id` is stable across restarts. The control plane SHALL use this `cluster_id`
as the cluster filter for `WatchGateways` for the lifetime of the process.

`HYPERSHELL_MANAGED_CLUSTER_NAME` (unique per spoke, set in gitops) is the only
cluster-identity configuration required. `HYPERSHELL_CLUSTER_ID` SHALL NOT appear in
gitops -- it is resolved at runtime via registration.

After startup, the control plane SHALL call `/registration` on a regular interval
(default: 60 seconds) to update `last_seen_at` on the hub. These subsequent calls are
no-ops for registration data and return the same `cluster_id`. See
`platform/managed-cluster-registration.spec.md` for full registration semantics.

#### Scenario: Successful startup registration

- GIVEN the spoke service account has `managed-cluster-registrar` in Keycloak
- AND `HYPERSHELL_MANAGED_CLUSTER_NAME` is set
- WHEN the control plane starts
- THEN it calls `POST /managed_clusters/registration` before opening `WatchGateways`
- AND uses the returned `cluster_id` to filter the watch stream to this cluster's gateways

#### Scenario: Transient failure retried with backoff

- GIVEN the API server is temporarily unreachable at startup
- WHEN the control plane attempts to register
- THEN it SHALL retry with exponential backoff
- AND it SHALL NOT open `WatchGateways` until registration succeeds

#### Scenario: 403 exits immediately

- GIVEN the API server returns 403 (managed-cluster-registrar not assigned in Keycloak)
- WHEN the control plane attempts to start
- THEN it SHALL NOT retry and SHALL NOT open `WatchGateways`
- AND it SHALL log a clear error identifying the missing role and exit

#### Scenario: Re-registration after restart returns same cluster_id

- GIVEN a spoke that previously registered and received `cluster_id: X`
- WHEN the spoke restarts and calls `/registration` again
- THEN the response is 200 with the same `cluster_id: X`
- AND `last_seen_at` is updated on the `ManagedCluster` record

### Requirement: gRPC Watch Streams

The control plane SHALL connect to the API server via gRPC watch streams for each resource Kind. On connection failure, it SHALL reconnect with exponential backoff.

#### Scenario: Watch Reconnection
- GIVEN the API server becomes unreachable
- WHEN the gRPC stream disconnects
- THEN the watcher SHALL retry with exponential backoff
- AND resume processing from the last known state

### Requirement: Gateway Reconciliation

The control plane SHALL reconcile Gateway resources into Kubernetes Deployments, Services, ConfigMaps, and supporting resources on the target managed cluster. Full provisioning details are defined in the [gateway sub-specs](./openshell-gateway.spec.md).

#### Scenario: New Gateway Created
- GIVEN a new Gateway resource appears via the watch stream
- WHEN the reconciler processes it
- THEN it SHALL create the corresponding K8s resources on the cluster identified by `cluster_id`:
  - SQL database and role on the configured PostgreSQL server, with a credential Secret in the gateway namespace - see [database spec](./openshell-gateway-database.spec.md)
  - cert-manager Issuer and Certificate resources for TLS - see [TLS spec](./openshell-gateway-tls.spec.md)
  - JWT key generation Job (`openshell-gateway-certgen`)
  - Gateway Deployment, Service, ServiceAccounts, Roles, RoleBindings, ConfigMap, NetworkPolicies
  - GRPCRoute and BackendTLSPolicy (when `route` field is set) - see [routing spec](./openshell-gateway-routing.spec.md)
  - OIDC configuration in gateway.toml (when `oidc.issuer` is set) - see [OIDC spec](./openshell-gateway-oidc.spec.md)
- AND set the Gateway's `phase` to `Provisioning` while applying manifests, and to `Running` only after the `openshell-gateway` Deployment is observed Ready - see [health spec](./openshell-gateway-health.spec.md)

### Requirement: Controller-Local Database Configuration

The controller SHALL use its installation-supplied PostgreSQL configuration for
all database operations on its assigned gateways. It SHALL NOT watch a database
API resource or create PostgreSQL server infrastructure. Connection readiness,
credential isolation, and durable cleanup SHALL follow the
[database specification](./openshell-gateway-database.spec.md).

Before reconciliation starts, the controller SHALL verify that the API uses the
matching controller-local generation. An old or unknown API SHALL produce an
unsupported-generation error without Kubernetes or SQL mutations. There is no
fallback to the old database-provider runtime.

#### Scenario: Local server configuration is sufficient

- GIVEN the installation has supplied a PostgreSQL server and credential Secret
- WHEN a gateway is assigned to this controller
- THEN it SHALL create the gateway SQL database and role on that server
- AND it SHALL NOT require a database registration or API lookup

### Requirement: Resource Cleanup

When a Gateway is deleted, the control plane SHALL clean up all associated Kubernetes resources on the target cluster.

#### Scenario: Gateway Deletion
- GIVEN a Gateway deletion event from the watch stream
- WHEN the reconciler processes it
- THEN it SHALL delete all K8s resources associated with that Gateway
- AND it SHALL complete or durably retain pending SQL cleanup before removing required credentials
- AND it SHALL leave the PostgreSQL server and other gateways intact

### Requirement: Status Synchronization

The control plane SHALL continuously reconcile the `phase` and `status` fields of Gateway resources in the API server to reflect actual cluster state, even after a Gateway has reached `Running`. The phase gate that prevents redundant re-provisioning SHALL NOT suppress these health updates. Full lifecycle semantics are defined in the [health spec](./openshell-gateway-health.spec.md).

#### Scenario: Gateway Health Check
- GIVEN a Gateway with `phase` `Running` on a managed cluster
- WHEN the control plane observes its `openshell-gateway` Deployment health
- THEN it SHALL update the Gateway's `status` in the API server
- AND set `phase` to `Degraded` when ready replicas fall below desired
- AND set `phase` back to `Running` when the workload recovers

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| gRPC watch streams (not polling) | Real-time event delivery, efficient resource usage |
| Separate module from API server | Independent lifecycle, separate deployment |
| No controller-runtime dependency | Lightweight, custom reconciliation without CRD overhead |
| Multi-cluster client pool | Each managed cluster gets its own KubeClient for isolation |
