# API server

The Go API server is the source of truth for desired HyperShell resource state. REST serves humans and SDKs; gRPC watch streams drive reconciliation.

Authoritative source: components/api-server/CLAUDE.md; specs/platform/api-server-observability.spec.md

### API server boundaries: generated Kind plugins provide uniform CRUD, persistence, REST and gRPC presentation, while the control plane consumes watch events.

```mermaid
graph LR
  CLI[hsctl CLI]
  GO[Go SDK]
  TS[TypeScript SDK]
  BFF[Web Console BFF]
  HTTP[REST API<br/>/api/hypershell/v1]
  RPC[gRPC API<br/>Watch + CRUD]
  PLUGINS[Kind plugins<br/>Fleet · Gateway · Cluster<br/>Database · Release · Network]
  DB[(PostgreSQL<br/>CNPG-backed)]
  CP[Control Plane<br/>watch clients]
  CLI --> HTTP
  GO --> HTTP
  TS --> HTTP
  BFF --> HTTP
  HTTP --> PLUGINS
  RPC --> PLUGINS
  PLUGINS --> DB
  RPC -->|stream events| CP
  PLUGINS -.->|CRUD events| RPC
  style HTTP fill:#cce5ff
  style RPC fill:#cce5ff
  style PLUGINS fill:#fff3cd
  style DB fill:#d4edda
  style CP fill:#f8d7da
```
