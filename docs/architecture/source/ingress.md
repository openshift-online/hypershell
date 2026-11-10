# Gateway ingress

External gRPC exposure uses the Kubernetes Gateway API on every cloud. Each tenant gateway gets a GRPCRoute, and its console gets an HTTPRoute, on the shared `openshift-ingress/openshell-grpc-gateway`. Without a Gateway API, the control plane manages no ingress and the gateway stays cluster-internal.

Authoritative source: specs/platform/global-architecture.spec.md; specs/platform/openshell-gateway-routing.spec.md

### Reference gateway-api mode: one shared Gateway and load balancer serve many tenant GRPCRoutes.

```mermaid
graph LR
  CLI[openshell CLI] --> DNS[Wildcard DNS<br/>Route53 CNAME]
  DNS --> LB[Cloud Load Balancer]
  LB --> SG[Shared Gateway<br/>openshift-ingress :443]
  SG --> ROUTE[GRPCRoute<br/>per tenant namespace]
  ROUTE --> SVC[openshell-gateway<br/>Service :8080]
  SVC --> POD[Gateway Pod<br/>TLS backend]
  CA[BackendTLSPolicy +<br/>openshell-gateway-backend-ca] -.-> POD
  CERT[Wildcard cert-manager<br/>Certificate] -.-> SG
  style DNS fill:#fff3cd
  style LB fill:#f8d7da
  style SG fill:#cce5ff
  style ROUTE fill:#d4edda
```

### Per-gateway opt-in (`route.certificate`): the opted-in gateway gets its own Gateway, wildcard certificate, and wildcard DNS.

A Gateway provisioned with `route: { certificate: {} }` does not attach to the shared Gateway. The control plane creates, in the tenant namespace, a per-tenant Gateway `gw-<ns>` with two port-443 listeners (`grpc` exact hostname, `sandbox` wildcard hostname), a single cert-manager Certificate covering both `gw-<ns>.<base-domain>` and `*.gw-<ns>.<base-domain>`, and a wildcard-hostname HTTPRoute that forwards sandbox-service traffic to the gateway Service on port 8080. Both DNS records are published via the per-tenant Gateway's `external-dns.kubernetes.io/hostname` infrastructure annotation (manual records are the fallback without external-dns). The per-gateway console rides the `sandbox` listener at `console-<ns>.gw-<ns>.<base-domain>`. See specs/platform/openshell-gateway-routing.spec.md (Per-Gateway Dedicated Route and Certificate).

```mermaid
graph LR
  CLI[openshell CLI] --> EXACT[DNS gw-<ns>.<base-domain>]
  SRV[sandbox services<br/>sandbox--service.gw-<ns>] --> WILD[DNS *.gw-<ns>.<base-domain>]
  CONSOLE[console-<ns>.gw-<ns>] --> WILD
  EXACT --> LB[Gateway LB<br/>tenant namespace]
  WILD --> LB
  LB --> PG[Per-tenant Gateway gw-<ns> :443<br/>grpc + sandbox listeners]
  PG --> GRPC[GRPCRoute<br/>exact hostname<br/>grpc listener]
  PG --> HTTP[HTTPRoute openshell-gateway-sandbox<br/>wildcard hostname<br/>sandbox listener]
  GRPC --> SVC[openshell-gateway<br/>Service :8080]
  HTTP --> SVC
  SVC --> POD[Gateway Pod<br/>TLS backend<br/>Host header preserved]
  EXT[Certificate openshell-gateway-external<br/>gw-<ns> + *.gw-<ns> SANs<br/>DNS-01 issuer] -.-> PG
  CA[BackendTLSPolicy +<br/>openshell-gateway-backend-ca] -.-> POD
  style EXACT fill:#fff3cd
  style WILD fill:#fff3cd
  style LB fill:#f8d7da
  style PG fill:#cce5ff
  style GRPC fill:#d4edda
  style HTTP fill:#d4edda
```

### Tenant exposure: the Gateway API publishes every gateway; without it the gateway reconcile fails.

```mermaid
graph TB
  CP[Control Plane]
  CHECK{Gateway API installed?}
  CP --> CHECK
  CHECK -->|yes| GA[GRPCRoute + console HTTPRoute<br/>+ BackendTLSPolicy<br/>shared Gateway prerequisite]
  CHECK -->|no| FAIL[EnvironmentReady Failed<br/>reconcile retries]
  GA --> HOST[grpcs://gw-<tenant>.<base-domain>:443]
  HOST --> GW[OpenShell Gateway pod]
  style CP fill:#f8d7da
  style CHECK fill:#fff3cd
  style GA fill:#cce5ff
  style FAIL fill:#f8d7da
  style GW fill:#cce5ff
```
