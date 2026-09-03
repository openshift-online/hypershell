# Database architecture

ManagedDatabase is the indirection between a Gateway and PostgreSQL. The provider selects either a standalone Deployment or CNPG-managed PostgreSQL.

Authoritative source: docs/cnpg-architecture.md; specs/platform/openshell-gateway-database.spec.md

### CNPG mode: one ManagedDatabase cluster can serve multiple gateways, each with an isolated logical database and role.

```mermaid
graph LR
  MD[ManagedDatabase<br/>provider: cnpg] --> NS[openshell-db-<id>]
  NS --> C[(CNPG Cluster<br/>openshell-db)]
  C --> R1[DatabaseRole + Database<br/>Gateway A]
  C --> R2[DatabaseRole + Database<br/>Gateway B]
  R1 --> S1[Credentials Secret]
  R2 --> S2[Credentials Secret]
  S1 --> GA[Gateway A<br/>tenant namespace]
  S2 --> GB[Gateway B<br/>tenant namespace]
  OP[CNPG Operator] -.-> C
  style MD fill:#fff3cd
  style C fill:#d4edda
  style OP fill:#cce5ff
```

### Deployment mode: the API server auto-creates a dedicated ManagedDatabase for each Gateway.

```mermaid
graph LR
  G[Gateway] -->|database_id| MD[ManagedDatabase<br/>provider: deployment]
  MD --> NS[openshell-db-<id>]
  NS --> SEC[Credentials Secret]
  NS --> PVC[1Gi PVC]
  NS --> PG[PostgreSQL Deployment]
  PG --> SVC[Service :5432]
  SEC -->|copy after readiness| TSEC[Gateway namespace<br/>credentials Secret]
  TSEC --> G
  style MD fill:#fff3cd
  style PG fill:#d4edda
  style PVC fill:#cce5ff
  style TSEC fill:#f8d7da
```
