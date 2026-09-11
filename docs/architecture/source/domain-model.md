# Domain model

The API models top-level placement, release, database, gateway, network, and gateway-scoped automation resources. Tenancy is enforced through platform-level and per-gateway RBAC rather than resource grouping.

Authoritative source: specs/platform/data-model.spec.md; specs/security/rbac-enforcement.spec.md

### Current API relationships. Resources are top-level; placement, release, and database references converge on Gateway, while service accounts are scoped to one Gateway.

```mermaid
erDiagram
  ManagedCluster ||--o{ Gateway : hosts
  GatewayRelease ||--o{ Gateway : deployed_as
  ManagedDatabase ||--o{ Gateway : backed_by
  Gateway ||--o{ OpenShellGatewayServiceAccount : authorizes
  Gateway ||--o| GatewayNetwork : hub_gateway
  ManagedCluster {
    string id PK
    string name
    string provider
    string region
    string status
  }
  ManagedDatabase {
    string id PK
    string name
    string provider
    string region
    string engine
    string status
  }
  GatewayRelease {
    string id PK
    string name
    string image
    string rollout_strategy
    string status
  }
  Gateway {
    string id PK
    string name
    string cluster_id FK
    string release_id FK
    string database_id FK
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
  GatewayNetwork {
    string id PK
    string name
    string topology
    string tunnel_mode
    string hub_gateway_id FK
    string status
  }
```
