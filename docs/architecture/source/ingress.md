# Gateway ingress

External gRPC exposure is configuration-driven. AWS uses the shared Gateway API path; environments without a functional Gateway API use OpenShift Route passthrough.

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
  CA[BackendTLSPolicy +<br/>openshell-backend-ca] -.-> POD
  CERT[Wildcard cert-manager<br/>Certificate] -.-> SG
  style DNS fill:#fff3cd
  style LB fill:#f8d7da
  style SG fill:#cce5ff
  style ROUTE fill:#d4edda
```

### Environment-adaptive routing: both modes converge on the same tenant workload and route address contract.

```mermaid
graph TB
  CP[Control Plane]
  MODE{GATEWAY_INGRESS_MODE}
  CP --> MODE
  MODE -->|gateway-api| GA[GRPCRoute + BackendTLSPolicy<br/>shared Gateway prerequisite]
  MODE -->|route| OR[OpenShift Route<br/>TLS passthrough]
  MODE -->|none| INTERNAL[Cluster-internal Service only]
  GA --> HOST[grpcs://gw-<tenant>.<base-domain>:443]
  OR --> HOST
  INTERNAL --> LOCAL[Service DNS / port-forward]
  HOST --> GW[Same OpenShell Gateway pod]
  style CP fill:#f8d7da
  style MODE fill:#fff3cd
  style GA fill:#cce5ff
  style OR fill:#d4edda
  style GW fill:#cce5ff
```
