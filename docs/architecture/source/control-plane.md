# Control plane

The control plane follows an informer-reconciler pattern: one watcher maintains gRPC streams and dispatches events to resource-specific reconcilers.

Authoritative source: specs/platform/control-plane.spec.md; components/control-plane/

### Event-driven reconciliation from a persisted Gateway to Kubernetes resources on the target ManagedCluster.

```mermaid
sequenceDiagram
  participant User as API client
  participant API as API Server
  participant DB as PostgreSQL
  participant W as Watcher
  participant R as Gateway Reconciler
  participant K as ManagedCluster K8s API
  participant PG as Gateway PostgreSQL server
  User->>API: POST /gateways
  API->>DB: Persist desired state
  DB-->>API: Gateway record
  API-->>W: gRPC Watch event
  W->>R: Dispatch ADDED/MODIFIED
  R->>K: Ensure namespace + PKI
  R->>PG: CREATE ROLE + CREATE DATABASE gw_id (verify-full)
  R->>K: Ensure DB credentials Secret + CA
  R->>K: Ensure Deployment + Service + RBAC
  R->>K: Ensure exposure resources
  K-->>R: Observe readiness
  R->>API: PATCH status + route address
  API-->>User: Gateway is Running
```

### Control-plane internal components and the multi-cluster client pool.

```mermaid
graph TB
  WS[ gRPC Watch streams ] --> W[Watcher]
  W --> F[Event fan-out]
  F --> GR[Gateway Reconciler]
  F --> RR[Release Reconciler]
  F --> NR[GatewayNetwork Reconciler]
  GR --> POOL[Multi-cluster KubeClient pool]
  RR --> POOL
  NR --> POOL
  POOL --> C1[ManagedCluster A]
  POOL --> C2[ManagedCluster B]
  POOL --> HUB[Hub cluster]
  GR --> EXP[Gateway Exposure port]
  EXP --> API[Gateway API / Route adapter]
  GR --> KC[Keycloak + service-account provisioner]
  ADM[Mounted Secret<br/>hypershell-gateway-database-admin] -.-> GR
  GR --> PG[(Gateway PostgreSQL server<br/>sslmode=verify-full)]
  style W fill:#cce5ff
  style F fill:#fff3cd
  style POOL fill:#d4edda
  style EXP fill:#f8d7da
```
