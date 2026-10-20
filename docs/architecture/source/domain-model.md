# Domain model

The API models top-level placement, gateway, and gateway-scoped automation resources. Gateway databases are not API resources; the control plane provisions them from a mounted admin credential Secret. Tenancy is enforced through platform-level and per-gateway RBAC rather than resource grouping.

Authoritative source: specs/platform/data-model.spec.md; specs/security/rbac-enforcement.spec.md

### Current API relationships. Resources are top-level; placement references converge on Gateway, while service accounts are scoped to one Gateway.

```mermaid
erDiagram
  ManagedCluster ||--o{ Gateway : hosts
  Gateway ||--o{ OpenShellGatewayServiceAccount : authorizes
  ManagedCluster {
    string id PK
    string name
    string provider
    string region
    string status
  }
  Gateway {
    string id PK
    string name
    string cluster_id FK
    string namespace
    string phase
    string status
  }
  OpenShellGatewayServiceAccount {
    string id PK
    string gateway_id FK
    string role
    string status
    string created_by_user_id FK
  }
```
