# Control Plane

**Date:** 2026-09-15
**Status:** Active

## Overview

The HyperShell control plane is a Go service that watches the API server via gRPC streaming RPCs and reconciles the desired state (Gateway and related resources in the database) into actual Kubernetes resources. It follows the informer-reconciler pattern without depending on controller-runtime.

Every control plane instance follows the same lifecycle regardless of where it runs: it self-registers via `POST /managed_clusters/registration` at startup to obtain its `cluster_id`, then watches the API server via gRPC and reconciles the gateways assigned to that cluster. Registration is unconditional - it happens on every startup whether or not OIDC is configured, so the same startup path runs in local development (no auth) and in multi-cloud production. The same binary and reconciler code runs whether the control plane is co-located with the API server on a Cloud Hub or running on a remote ManagedCluster. See [`global-architecture.spec.md` - Control Plane Self-Registration](./global-architecture.spec.md#control-plane-self-registration).

## Architecture

```
Keycloak (OIDC client_credentials)
  │  access_token (managed-cluster-registrar role)
  ▼
Control Plane ──► POST /managed_clusters/registration → cluster_id
  │  gRPC watch streams per Kind (filtered by cluster_id)
  ▼
API Server (PostgreSQL via CNPG)
  │  Gateway events
  ▼
Control Plane (Watcher + Reconciler)
  │  reconciles into K8s resources on managed cluster(s)
  ▼
ManagedCluster(s) (Gateway pods, Services, Configs)
```

## Components

### Watcher

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Gateways, GatewayReleases, ManagedClusters, GatewayNetworks). On each event (create, update, delete), it dispatches to the Reconciler. Gateway events are dispatched through a per-gateway reconcile queue drained by a bounded worker pool: work for a single gateway is serialized while distinct gateways reconcile concurrently up to a configurable cap. The pool size and its configuration are defined in [`gateway-reconcile-concurrency.spec.md`](./gateway-reconcile-concurrency.spec.md).

### Reconciler

The Reconciler receives resource events from the Watcher and converges the Kubernetes state on managed clusters to match. Key responsibilities:

- Deploy/update Gateway workloads on target clusters
- Provision per-gateway PostgreSQL databases and roles on the platform's gateway database server, using the admin credential Secret mounted into the controller
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
| [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) | Per-gateway PostgreSQL provisioning from the mounted admin credential Secret (admin TLS `verify-full`, gateway `require`), credential security, cleanup |
| [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) | TLS certificate management via cert-manager, SAN management, cert rotation |
| [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) | External connectivity: Gateway API (GRPCRoute + BackendTLSPolicy), NetworkPolicy |
| [`openshell-gateway-oidc.spec.md`](./openshell-gateway-oidc.spec.md) | OIDC authentication, role validation, gateway.toml injection |
| [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) | Phase lifecycle, workload-readiness gating, continuous health reconciliation |
| [`gateway-version-selection.spec.md`](./gateway-version-selection.spec.md) | Database-backed version selection: resolving `release_id` to a GatewayRelease image and its precedence over a direct image |
| [`gateway-release-reconciliation.spec.md`](./gateway-release-reconciliation.spec.md) | GatewayRelease reconciliation: image validation, deterministic release status, change propagation to referencing gateways |
| [`gateway-release-rollout.spec.md`](./gateway-release-rollout.spec.md) | Safe release rollout: last-good workload preserved until the new revision passes health gates, revision-aware readiness, observed-release and rollout-state reporting, timeout/failure handling |
| [`gateway-network-reconciliation.spec.md`](./gateway-network-reconciliation.spec.md) | GatewayNetwork reconciliation: topology vocabulary, topology/hub coherence and hub-reference validation, deterministic network status write-back |

### Config

Holds connection configuration for the API server gRPC endpoint, Kubernetes client initialization, and the internal service-account provisioner. The provisioner listens on an in-cluster gRPC port that a NetworkPolicy restricts to the API server pod. The API server does not receive Keycloak administrator credentials.

## Requirements

### Requirement: Self-Registration at Startup

Before opening any gRPC watch stream, every control plane SHALL call
`POST /api/hypershell/v1/managed_clusters/registration` to obtain its `cluster_id`.
Registration is unconditional: it runs on every startup regardless of where the
control plane runs and regardless of whether OIDC is configured. The registration
endpoint is idempotent; the returned `cluster_id` is stable across restarts. The
control plane SHALL use this `cluster_id` as the cluster filter for `WatchGateways`
for the lifetime of the process.

The control plane's registration identity resolves as follows:

- When OIDC `client_credentials` are configured, the control plane authenticates
  with its OIDC token and the API server derives the durable identity from the
  token subject (`oidc_subject`). This is the production path.
- When OIDC is not configured (local development, no auth), the control plane
  registers by `name` alone and the API server treats that name as the identity.

`HYPERSHELL_MANAGED_CLUSTER_NAME` (unique per control plane instance, set in
configuration or gitops) is the only cluster-identity configuration required, and
is always sent as the registration `name`. `HYPERSHELL_CLUSTER_ID` SHALL NOT appear
in gitops - it is resolved at runtime via registration.

After startup, the control plane SHALL call `/registration` on a regular interval
(default: 60 seconds) to update `last_seen_at`. These subsequent calls are
no-ops for registration data and return the same `cluster_id`. See
`platform/managed-cluster-registration.spec.md` for full registration semantics.

#### Scenario: Successful startup registration

- GIVEN the control plane service account has `managed-cluster-registrar` in Keycloak
- AND `HYPERSHELL_MANAGED_CLUSTER_NAME` is set
- WHEN the control plane starts
- THEN it calls `POST /managed_clusters/registration` before opening `WatchGateways`
- AND uses the returned `cluster_id` to filter the watch stream to this cluster's gateways

#### Scenario: Local development on a fresh database

- GIVEN a local development environment with an empty database
- AND OIDC authentication is disabled (no `managed-cluster-registrar` token available)
- AND `HYPERSHELL_MANAGED_CLUSTER_NAME=local`
- WHEN the control plane starts and calls `/registration`
- THEN the API server creates a ManagedCluster record named `local`, keyed on the name
- AND returns a `cluster_id` that the control plane uses to reconcile gateways
- AND the startup path is identical to production apart from the absence of a token

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

- GIVEN a control plane that previously registered and received `cluster_id: X`
- WHEN the control plane restarts and calls `/registration` again
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
  - A per-gateway PostgreSQL database and role on the gateway database server (admin credentials read from the mounted Secret) and the tenant credentials Secret - see [database spec](./openshell-gateway-database.spec.md)
  - cert-manager Issuer and Certificate resources for TLS - see [TLS spec](./openshell-gateway-tls.spec.md)
  - JWT key generation Job (`openshell-gateway-certgen`)
  - Gateway Deployment, Service, ServiceAccounts, Roles, RoleBindings, ConfigMap, NetworkPolicies
  - GRPCRoute and BackendTLSPolicy (when `route` field is set) - see [routing spec](./openshell-gateway-routing.spec.md)
  - OIDC configuration in gateway.toml (when `oidc.issuer` is set) - see [OIDC spec](./openshell-gateway-oidc.spec.md)
- AND set the Gateway's `phase` to `Provisioning` while applying manifests, and to `Running` only after the `openshell-gateway` Deployment is observed Ready - see [health spec](./openshell-gateway-health.spec.md)

### Requirement: Gateway Database Admin Credentials

The control plane SHALL read the gateway database server's administrative connection from the Secret mounted at `GATEWAY_DATABASE_ADMIN_DIR` (default `/etc/hypershell/gateway-database`), SHALL validate the mounted files at startup and refuse to start when they are invalid, and SHALL NOT model the database server as an API resource. There is no database reconciler: per-gateway databases are provisioned and cleaned up by the GatewayReconciler.

#### Scenario: Controller starts with valid admin credential files
- GIVEN the admin Secret is mounted with `host`, `port`, `user`, `password` and a PEM `sslrootcert`
- WHEN the control plane starts
- THEN it SHALL validate the files without connecting to the server
- AND it SHALL serve watch streams for Gateways, GatewayReleases, ManagedClusters and GatewayNetworks

#### Scenario: Controller refuses to start on invalid admin credential files
- GIVEN a required admin credential file is missing, or `sslmode` is present with a value other than `verify-full`
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming the file and exit non-zero

See [database spec](./openshell-gateway-database.spec.md) for full provisioning details.

### Requirement: Resource Cleanup

When a Gateway is deleted, the control plane SHALL clean up all associated Kubernetes resources on the target cluster.

#### Scenario: Gateway Deletion
- GIVEN a Gateway deletion event from the watch stream
- WHEN the reconciler processes it
- THEN it SHALL delete all K8s resources associated with that Gateway
- AND confirm cleanup completion

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
| Same binary everywhere | The control plane runs the same binary whether co-located with the API server or on a remote ManagedCluster. The only difference is operational configuration (OIDC creds, gRPC address, `HYPERSHELL_MANAGED_CLUSTER_NAME`). No code fork or separate build. |
| Self-registration before watch | Every control plane calls `/managed_clusters/registration` before opening `WatchGateways`, so it always has a valid `cluster_id` to filter on. Registration is unconditional and works with or without OIDC: production uses the OIDC token identity, local dev (no auth) registers by name alone. When auth is enabled, a 403 (missing `managed-cluster-registrar` role) exits immediately to surface misconfiguration. This works identically from local dev (fresh database) to production. |
| `cluster_id` filter on watch events | The control plane processes only the gateways matching its own `cluster_id`, so it never reconciles another cluster's gateways. The filter is applied server-side: the control plane passes its `cluster_id` in the `WatchGateways` request and in its seed listing, and the API server returns only matching gateways. |
