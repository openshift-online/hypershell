# Platform topology

The three-tier hub-and-spoke topology separates global identity, cloud-level operations, and regional OpenShell Gateway workloads.

Authoritative source: specs/platform/global-architecture.spec.md

### Global Hub → Cloud Hubs → ManagedClusters. The control plane reconciles tenant workloads downward while metrics aggregate upward.

```mermaid
graph TB
  subgraph GH[Global Hub]
    GKI[Keycloak identity root]
    GHG[Grafana cross-cloud view]
    GKI --> GHG
  end
  subgraph AWS[Cloud Hub: AWS]
    AK[Keycloak]
    AAPI[API Server]
    AWS_CONTROL[Control Plane]
    ADB[(CNPG PostgreSQL)]
    AP[Prometheus]
    AAPI --> ADB
    AWS_CONTROL --> AAPI
    AP --> GHG
  end
  subgraph IBM[Cloud Hub: IBM]
    IK[Keycloak]
    IAPI[API Server]
    ICP[Control Plane]
    IDB[(CNPG PostgreSQL)]
    IP[Prometheus]
    IAPI --> IDB
    ICP --> IAPI
    IP --> GHG
  end
  subgraph MC[ManagedClusters]
    M1[Gateway workloads + Sandboxes]
    M2[Gateway workloads + Sandboxes]
    M3[Gateway workloads + Sandboxes]
    MP[Local Prometheus]
  end
  GKI -->|federates| AK
  GKI -->|federates| IK
  AK -->|identity federation| M1
  AK -->|identity federation| M2
  IK -->|identity federation| M3
  AWS_CONTROL -->|reconcile| M1
  AWS_CONTROL -->|reconcile| M2
  ICP -->|reconcile| M3
  M1 --> MP
  M2 --> MP
  M3 --> MP
  MP -->|metrics upstream| AP
  MP -->|metrics upstream| IP
  style GH fill:#e1f5ff
  style AWS fill:#fff3cd
  style IBM fill:#fff3cd
  style MC fill:#d4edda
```
