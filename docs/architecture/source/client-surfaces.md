# Clients and SDKs

HyperShell exposes the same management API to the Go CLI, Go SDK, TypeScript SDK, and browser-facing web console while keeping transport and authentication boundaries explicit.

Authoritative source: components/cli/; components/sdk-go/; components/sdk-typescript/; specs/web-console/architecture.spec.md

### Client surfaces converge on the versioned REST API. The web console uses the generated TypeScript SDK only through its host adapter; the BFF owns browser authentication.

```mermaid
graph TB
  USER[Operator or automation]
  CLI[hsctl CLI<br/>Go HTTP connection]
  GOSDK[Generated Go SDK<br/>client + types]
  TSSDK[Generated TypeScript SDK<br/>Fetch-compatible ESM]
  WEB[React web console<br/>Gateway Management UI]
  BFF[Fastify BFF<br/>relative /api + session cookie]
  API[HyperShell REST API<br/>/api/hypershell/v1]
  USER --> CLI
  USER --> WEB
  CLI --> API
  GOSDK --> API
  WEB --> TSSDK
  WEB --> BFF
  BFF --> API
  API --> DATA[Typed resource contracts<br/>CRUD + pagination + errors]
  style USER fill:#e1f5ff
  style API fill:#cce5ff
  style BFF fill:#fff3cd
  style DATA fill:#d4edda
```
