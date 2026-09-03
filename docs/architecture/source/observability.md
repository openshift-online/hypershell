# Observability

Metrics, traces, and domain probes make a user workflow observable from the browser through the BFF and API server to PostgreSQL and the control plane.

Authoritative source: specs/platform/api-server-observability.spec.md; specs/web-console/tracing.spec.md; specs/standards/ui/domain-observability.spec.md

### One request trace crosses the web console, BFF, API server, and database. W3C Trace Context is propagated; sensitive values stay out of telemetry.

```mermaid
sequenceDiagram
  participant B as Browser workflow
  participant F as Fastify BFF
  participant A as API Server
  participant D as PostgreSQL
  participant O as OTLP Collector
  B->>F: GET /api/gateways<br/>traceparent
  F->>A: Proxy request + trace context
  A->>D: Query under request span
  D-->>A: Rows
  A-->>F: Response + X-Operation-ID
  F-->>B: Gateway page data
  F-->>O: BFF span
  A-->>O: HTTP/gRPC + DB spans
  Note over F,O: Tokens, cookies, bodies, credentials, and raw IDs are redacted
```

### Metrics hierarchy: local workload signals roll up to cloud and global dashboards.

```mermaid
graph TB
  subgraph MC[ManagedCluster]
    GW[Gateway + node metrics]
    MP[Prometheus]
    GW --> MP
  end
  subgraph CH[Cloud Hub]
    CPROM[Prometheus aggregator]
    GRAF[Grafana cloud dashboards]
    ALERT[Alertmanager]
    CPROM --> GRAF
    CPROM --> ALERT
  end
  subgraph GH[Global Hub]
    GPROM[Global aggregator]
    GGRAF[Cross-cloud Grafana]
    GALERT[Global Alertmanager]
    GPROM --> GGRAF
    GPROM --> GALERT
  end
  MP -->|remote write| CPROM
  CPROM -->|federation| GPROM
  style MC fill:#d4edda
  style CH fill:#fff3cd
  style GH fill:#e1f5ff
```
