# Domain model

The API models fleets, placements, releases, databases, gateways, networks, and gateway-scoped automation identities as first-class resources.

Authoritative source: specs/platform/data-model.spec.md

### Current API relationships. Fleet is the present organizational scope; the specification notes a future simplification to top-level resources.

```mermaid
erDiagram
  Fleet ||--o{ ManagedCluster : owns
  Fleet ||--o{ ManagedDatabase : owns
  Fleet ||--o{ GatewayRelease : owns
  Fleet ||--o{ Gateway : owns
  Fleet ||--o{ GatewayNetwork : owns
  ManagedCluster ||--o{ Gateway : hosts
  ManagedDatabase ||--o{ Gateway : backs
  GatewayRelease ||--o{ Gateway : deploys
  Gateway ||--o{ OpenShellGatewayServiceAccount : authorizes
  GatewayNetwork }o--|| Gateway : hub
  Fleet {
    string id PK
    string name
    string status
  }
  ManagedCluster {
    string id PK
    string provider
    string region
    string kubeconfig_secret
  }
  ManagedDatabase {
    string id PK
    string provider
    string namespace
    string status
  }
  GatewayRelease {
    string id PK
    string image
    string rollout_strategy
  }
  Gateway {
    string id PK
    string namespace
    string cluster_id FK
    string database_id FK
    string release_id FK
    string phase
    string route_address
  }
  GatewayNetwork {
    string id PK
    string topology
    string tunnel_mode
  }
  OpenShellGatewayServiceAccount {
    string id PK
    string gateway_id FK
    string role
    string status
  }
```
