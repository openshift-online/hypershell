# Gateway workload

Each Gateway receives an API-assigned namespace containing the OpenShell Gateway, its Supervisor, dynamic Sandboxes, security policy, certificates, and access configuration.

Authoritative source: specs/platform/openshell-gateway.spec.md; specs/platform/openshell-gateway-sandbox-count.spec.md

### Namespace-per-Gateway is the isolation and lifecycle boundary. The control plane owns the Kubernetes resources; the Gateway owns sandbox runtime state.

```mermaid
graph TB
  subgraph NS[openshell-<gateway-id> tenant namespace]
    GW[OpenShell Gateway<br/>Deployment :8080 gRPC<br/>:8081 health · :9090 metrics]
    SUP[Supervisor sidecar]
    S1[Sandbox 1]
    S2[Sandbox 2]
    SN[Sandbox N · dynamic]
    SVC[openshell-gateway<br/>ClusterIP Service]
    CFG[ConfigMap<br/>gateway.toml]
    TLS[TLS Secrets<br/>server + client CA]
    JWT[JWT key Secret]
    RBAC[ServiceAccounts · Roles<br/>RoleBindings · SCC binding]
    NP[NetworkPolicies]
    GW --- SUP
    GW --> SVC
    GW --> CFG
    GW --> TLS
    GW --> JWT
    GW --> RBAC
    GW --> NP
    GW --> S1
    GW --> S2
    GW --> SN
    S1 -->|SSH :2222| GW
    S2 -->|gRPC| GW
    SN -->|gRPC| GW
  end
  DB[(ManagedDatabase<br/>logical gateway DB)]
  GW -->|OPENSHELL_DB_URL| DB
  CERT[cert-manager] -.-> TLS
  AGENT[Agent Sandbox controller] -.-> S1
  style NS fill:#f5f8fa
  style GW fill:#cce5ff
  style SUP fill:#fff3cd
  style DB fill:#d4edda
  style RBAC fill:#f8d7da
  style NP fill:#f8d7da
```
