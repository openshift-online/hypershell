# HyperShell architecture atlas

A navigable set of architectural drawings for the HyperShell platform, from the global topology to the tenant Gateway runtime.

Last mapped from the active architecture specifications in this repository. Read the project README and the linked sources for implementation details.

### End-to-end view: clients enter through the web/API surfaces, desired state is persisted by the API server, and the control plane reconciles OpenShell Gateways into ManagedClusters.

```mermaid
graph TB
  USER[Users + automation] --> EDGE[Web Console BFF / hsctl / SDKs]
  EDGE --> API[API Server]
  API --> DB[(PostgreSQL<br/>desired state)]
  API -->|gRPC watch| CP[Control Plane]
  CP --> MC[ManagedClusters]
  MC --> GW[OpenShell Gateways<br/>Supervisors + Sandboxes]
  CP --> OBS[Metrics + status]
  MC --> OBS
  OBS --> DASH[Cloud + Global dashboards]
  ID[Keycloak federation] --> EDGE
  ID --> GW
  style EDGE fill:#e1f5ff
  style API fill:#cce5ff
  style DB fill:#fff3cd
  style CP fill:#f8d7da
  style MC fill:#d4edda
  style GW fill:#d4edda
  style OBS fill:#cce5ff
```
