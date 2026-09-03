# Web console

The console is a React SPA hosted by a Fastify Backend-for-Frontend. The reusable Gateway Management UI package owns gateway workflows while the host owns routing, session, providers, and adapters.

Authoritative source: specs/web-console/architecture.spec.md; components/web-console/README.md; packages/gateway-management-ui/README.md

### Hexagonal boundary and trust flow: browser code never receives bearer tokens and never imports the generated SDK directly.

```mermaid
graph TB
  subgraph Browser[Browser]
    SPA[React Router SPA<br/>application shell]
    UI[Gateway Management UI<br/>PatternFly 6 + TanStack Query]
    SPA --> UI
  end
  subgraph Server[Same-origin BFF]
    FASTIFY[Fastify static server]
    SESSION[OIDC session + CSRF<br/>HttpOnly cookie]
    PROXY[Authenticated REST proxy<br/>server-held bearer token]
    FASTIFY --> SESSION
    FASTIFY --> PROXY
  end
  subgraph Platform[HyperShell platform]
    API[REST API Server]
    DB[(PostgreSQL)]
    CP[Control Plane]
    MC[ManagedClusters]
    API --> DB
    API --> CP
    CP --> MC
  end
  UI -->|/api, cookie credentials| FASTIFY
  SPA -->|session contract| SESSION
  PROXY -->|server-side Authorization| API
  CP --> MC
  UI -.->|typed GatewayOperations port| ADAPTER[Host API adapter]
  ADAPTER -.-> PROXY
  style Browser fill:#e1f5ff
  style Server fill:#fff3cd
  style Platform fill:#d4edda
  style SESSION fill:#f8d7da
  style ADAPTER fill:#cce5ff
```
