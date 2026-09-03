# Provisioning and delivery

Terraform provisions cloud infrastructure; Tekton provides deterministic installation; ArgoCD continuously reconciles Git-managed cluster state; the control plane handles tenant resources.

Authoritative source: specs/platform/global-architecture.spec.md; specs/platform/local-development.spec.md; .tekton/

### Infrastructure and application delivery responsibilities are separated by lifecycle and scope.

```mermaid
graph LR
  GIT[Git repository<br/>base + overlays] --> ARGO[ArgoCD<br/>continuous GitOps]
  TF[Terraform<br/>VPC · subnet · cluster] --> CLUSTERS[Cloud Hubs +<br/>ManagedClusters]
  TEKTON[Tekton installer<br/>idempotent pipeline] --> PREREQ[CNPG · cert-manager<br/>Vault · Keycloak · operators]
  ARGO --> PLATFORM[API Server · Control Plane<br/>PostgreSQL · Web Console]
  PLATFORM -->|gRPC watch + reconcile| TENANT[Tenant namespaces<br/>Gateway · Sandboxes · routes]
  PREREQ --> PLATFORM
  CLUSTERS --> PLATFORM
  DEV[Kind development<br/>make kind-up] --> PREREQ
  style GIT fill:#e1f5ff
  style TF fill:#fff3cd
  style TEKTON fill:#fff3cd
  style ARGO fill:#cce5ff
  style PLATFORM fill:#d4edda
  style TENANT fill:#f8d7da
```
