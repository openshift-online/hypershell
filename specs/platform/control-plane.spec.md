# Control Plane

**Date:** 2026-08-03
**Status:** Active

## Overview

The HyperShell control plane is a Go service that watches the API server via gRPC streaming RPCs and reconciles the desired state (Fleet resources in the database) into actual Kubernetes resources across managed clusters. It follows the informer-reconciler pattern without depending on controller-runtime.

## Architecture

```
API Server (PostgreSQL)
  │  gRPC watch streams per Kind
  ▼
Control Plane (Watcher + Reconciler)
  │  reconciles into K8s resources
  ▼
Managed Clusters (Gateway pods, Services, Configs)
```

## Components

### Watcher

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Fleets, Gateways, GatewayReleases, ManagedClusters, ManagedDatabases, GatewayNetworks). On each event (create, update, delete), it dispatches to the Reconciler.

### Reconciler

The Reconciler receives resource events from the Watcher and converges the Kubernetes state on managed clusters to match. Key responsibilities:

- Deploy/update Gateway workloads on target clusters
- Provision PostgreSQL databases for gateways
- Configure TLS certificates via cert-manager
- Create GRPCRoute and BackendTLSPolicy for external gateway exposure
- Inject OIDC authentication configuration into gateway deployments
- Configure network meshes between gateways
- Manage release rollouts (including canary strategies)
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

### Config

Holds connection configuration for the API server gRPC endpoint and Kubernetes client initialization.

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
  - PostgreSQL database resources (Secret, PVC, Deployment, Service, NetworkPolicy) - see [database spec](./openshell-gateway-database.spec.md)
  - cert-manager Issuer and Certificate resources for TLS - see [TLS spec](./openshell-gateway-tls.spec.md)
  - JWT key generation Job (`openshell-gateway-certgen`)
  - Gateway Deployment, Service, ServiceAccounts, Roles, RoleBindings, ConfigMap, NetworkPolicies
  - GRPCRoute and BackendTLSPolicy (when `route` field is set) - see [routing spec](./openshell-gateway-routing.spec.md)
  - OIDC configuration in gateway.toml (when `oidc.issuer` is set) - see [OIDC spec](./openshell-gateway-oidc.spec.md)
- AND update the Gateway's `phase` to reflect provisioning status

### Requirement: Resource Cleanup

When a Gateway is deleted, the control plane SHALL clean up all associated Kubernetes resources on the target cluster.

#### Scenario: Gateway Deletion
- GIVEN a Gateway deletion event from the watch stream
- WHEN the reconciler processes it
- THEN it SHALL delete all K8s resources associated with that Gateway
- AND confirm cleanup completion

### Requirement: Status Synchronization

The control plane SHALL periodically update the `status` field of resources in the API server to reflect actual cluster state.

#### Scenario: Gateway Health Check
- GIVEN a running Gateway on a managed cluster
- WHEN the control plane checks its health
- THEN it SHALL update the Gateway's `status` in the API server
- AND set `phase` to "Degraded" if the workload is unhealthy

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| gRPC watch streams (not polling) | Real-time event delivery, efficient resource usage |
| Separate module from API server | Independent lifecycle, separate deployment |
| No controller-runtime dependency | Lightweight, custom reconciliation without CRD overhead |
| Multi-cluster client pool | Each managed cluster gets its own KubeClient for isolation |
