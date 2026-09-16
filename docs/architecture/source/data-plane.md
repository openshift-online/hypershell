# Database architecture

ManagedDatabase registers an externally provisioned PostgreSQL server. HyperShell never creates, resizes or deletes that server; the control plane provisions one database and one login role per gateway inside it.

Authoritative source: specs/platform/openshell-gateway-database.spec.md

### One registered server can back many gateways, each with an isolated database and role.

```mermaid
graph LR
  MD[ManagedDatabase<br/>connection_secret] --> NS[hypershell-managed-db-name<br/>admin credentials Secret]
  NS --> C[(PostgreSQL server<br/>cloud-managed)]
  C --> R1[Database + role<br/>gw_gatewayA]
  C --> R2[Database + role<br/>gw_gatewayB]
  R1 --> S1[Credentials Secret]
  R2 --> S2[Credentials Secret]
  S1 --> GA[Gateway A<br/>tenant namespace]
  S2 --> GB[Gateway B<br/>tenant namespace]
  CP[Control plane<br/>in-process DDL] -.-> C
  style MD fill:#fff3cd
  style C fill:#d4edda
  style CP fill:#cce5ff
```

### Placement and cleanup

```mermaid
graph LR
  G[Gateway create] -->|database_id server-owned| P[Placement<br/>first-created ManagedDatabase]
  P --> DDL[CREATE ROLE + CREATE DATABASE<br/>gw_gatewayID]
  DDL --> TSEC[Gateway namespace<br/>openshell-gateway-db-credentials]
  TSEC --> W[Gateway workload]
  D[Gateway delete] --> DROP[DROP DATABASE + DROP ROLE<br/>single-shot, best effort]
  style P fill:#fff3cd
  style DDL fill:#d4edda
  style TSEC fill:#f8d7da
```

No PostgreSQL workload runs in the cluster: the gateway namespace holds only the credentials Secret.
