# Database architecture

Each HyperShell installation uses one externally provisioned PostgreSQL server for gateway databases. Its admin credentials and CA bundle reach the control plane as a mounted Secret; HyperShell never creates, resizes or deletes the server, and the control plane provisions one database and one login role per gateway inside it over a `verify-full` admin TLS connection. The gateway's own connection to its database uses `sslmode=require` instead: the upstream OpenShell Helm chart the gateway is deployed through has no mechanism to mount a CA bundle for this connection.

Authoritative source: specs/platform/openshell-gateway-database.spec.md

### One server backs every gateway, each with an isolated database and role.

```mermaid
graph LR
  MD[Secret hypershell-gateway-database-admin<br/>host/port/user/password/sslrootcert] -->|mounted at /etc/hypershell/gateway-database| CP
  CP -->|verify-full| C[(PostgreSQL server<br/>cloud-managed)]
  C --> R1[Database + role<br/>gw_gatewayA]
  C --> R2[Database + role<br/>gw_gatewayB]
  R1 --> S1[Credentials Secret<br/>uri, sslmode=require]
  R2 --> S2[Credentials Secret<br/>uri, sslmode=require]
  S1 --> GA[Gateway A<br/>tenant namespace]
  S2 --> GB[Gateway B<br/>tenant namespace]
  CP[Control plane<br/>in-process DDL]
  style MD fill:#fff3cd
  style C fill:#d4edda
  style CP fill:#cce5ff
```

### Provisioning and cleanup

```mermaid
graph LR
  G[Gateway create] --> P[Re-read mounted admin files]
  P --> DDL[CREATE ROLE + GRANT to admin<br/>CREATE DATABASE gw_gatewayID]
  DDL --> TSEC[Gateway namespace<br/>openshell-gateway-db-credentials]
  TSEC --> W[Gateway workload<br/>--db-url, sslmode=require]
  D[Gateway delete] --> DROP[terminate backends<br/>DROP DATABASE WITH FORCE + DROP ROLE]
  DROP -->|failure| EV[retry + IncompleteFinalization Event<br/>PostgreSQLDatabase gw_gatewayID]
  style P fill:#fff3cd
  style EV fill:#f8d7da
  style DDL fill:#d4edda
  style TSEC fill:#f8d7da
```

No PostgreSQL workload runs in the cluster and no database resource exists in the API: the gateway namespace holds only the credentials Secret, and the admin credentials never leave the control-plane namespace.
