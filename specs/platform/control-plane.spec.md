# Control Plane

**Date:** 2026-08-03
**Status:** Active

## Overview

The HyperShell control plane is a Go service that watches the API server via gRPC streaming RPCs and reconciles the desired state (Gateway and related resources in the database) into actual Kubernetes resources across managed clusters. It follows the informer-reconciler pattern without depending on controller-runtime.

## Architecture

```
API Server (PostgreSQL via CNPG)
  │  gRPC watch streams per Kind
  ▼
Control Plane (Watcher + Reconciler)
  │  reconciles into K8s resources
  ▼
Managed Clusters (Gateway pods, Services, Configs)
```

## Components

### Watcher

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Gateways, GatewayReleases, ManagedClusters, ManagedDatabases, GatewayNetworks). On each event (create, update, delete), it dispatches to the Reconciler.

### Reconciler

The Reconciler receives resource events from the Watcher and converges the Kubernetes state on managed clusters to match. Key responsibilities:

- Deploy/update Gateway workloads on target clusters
- Provision per-gateway PostgreSQL databases and roles via CNPG `Database` and `DatabaseRole` CRDs in the ManagedDatabase's CNPG Cluster (resolved via the gateway's `database_id`)
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

### Config

Holds connection configuration for the API server gRPC endpoint, Kubernetes client initialization, and the internal service-account provisioner. The provisioner listens on an in-cluster gRPC port that a NetworkPolicy restricts to the API server pod. The API server does not receive Keycloak administrator credentials.

## Requirements

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
  - CNPG database resources (DatabaseRole, Database CRDs, credential Secrets) in the ManagedDatabase's CNPG Cluster (resolved via `database_id`) - see [database spec](./openshell-gateway-database.spec.md)
  - cert-manager Issuer and Certificate resources for TLS - see [TLS spec](./openshell-gateway-tls.spec.md)
  - JWT key generation Job (`openshell-gateway-certgen`)
  - Gateway Deployment, Service, ServiceAccounts, Roles, RoleBindings, ConfigMap, NetworkPolicies
  - GRPCRoute and BackendTLSPolicy (when `route` field is set) - see [routing spec](./openshell-gateway-routing.spec.md)
  - OIDC configuration in gateway.toml (when `oidc.issuer` is set) - see [OIDC spec](./openshell-gateway-oidc.spec.md)
- AND set the Gateway's `phase` to `Provisioning` while applying manifests, and to `Running` only after the `openshell-gateway` Deployment is observed Ready - see [health spec](./openshell-gateway-health.spec.md)

### Requirement: ManagedDatabase Reconciliation

The control plane SHALL reconcile ManagedDatabase resources with `provider: "cnpg"` into CNPG Cluster infrastructure. Each CNPG-backed ManagedDatabase gets its own namespace and CNPG Cluster.

#### Scenario: New ManagedDatabase Created (provider=cnpg)
- GIVEN a new ManagedDatabase resource with `provider: "cnpg"` appears via the watch stream
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL create the namespace `openshell-db-<hex16>` (derived from the ManagedDatabase KSUID)
- AND it SHALL create a CNPG `Cluster` CR in that namespace
- AND it SHALL wait for the Cluster to reach Ready status
- AND it SHALL update the ManagedDatabase status in the API server

#### Scenario: ManagedDatabase Deletion
- GIVEN a ManagedDatabase deletion event from the watch stream
- AND no Gateways reference this ManagedDatabase via `database_id`
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL delete the CNPG Cluster CR
- AND it SHALL delete the namespace

#### Scenario: ManagedDatabase Deletion Blocked
- GIVEN a ManagedDatabase that is referenced by one or more Gateways
- WHEN a user attempts to delete the ManagedDatabase
- THEN the API server SHALL reject the deletion with HTTP 409

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
