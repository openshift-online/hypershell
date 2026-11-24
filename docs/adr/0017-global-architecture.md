# ADR-0017: Global Architecture design decisions

Rationale relocated from [`specs/platform/global-architecture.spec.md`](../../specs/platform/global-architecture.spec.md) (PLAT-ARCH). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Three-tier topology | Separates concerns: Global (identity root), Cloud Hub (operations), ManagedCluster (workloads) |
| Cloud Hub as primary unit | One HA instance per cloud runs API/control-plane/database; cloud isolation for latency and compliance |
| Full operator stack on all tiers | Every cluster has ArgoCD, Vault, Keycloak, Prometheus - but serves different purposes per tier. Because each cluster's own ArgoCD installs its stack from Git, the stack is present before any tenant workload lands. |
| Federated Keycloak chain | RH SSO → Global → Cloud Hub → ManagedCluster - identity flows down, authentication bubbles up |
| Vault per tier with distinct purposes | Cloud Hub Vault: service secrets; ManagedCluster Vault: gateway keystores |
| Cloud-managed PostgreSQL only | All databases run on externally provisioned, cloud-managed servers that supply backup, point-in-time recovery, and HA. HyperShell registers servers and manages only per-gateway databases and roles, so no database operator or in-cluster PostgreSQL workload is maintained, and there is a single provisioning model. |
| PostgreSQL on Cloud Hub as source of truth | All Gateway/ManagedCluster resource state lives in Cloud Hub database |
| ManagedClusters can be standard K8s | Maximizes deployment flexibility; only hubs need OpenShift |
| ArgoCD on every cluster (pull model) | Every cluster runs its own ArgoCD and self-reconciles only its own path from the central GitOps repo. No cluster stores another's kubeconfig and no hub pushes manifests outward, so credential blast radius is minimized and a hub outage never stalls a spoke's platform reconciliation. (Distinct from control-plane tenant reconciliation, which remains a runtime push from the Cloud Hub.) |
| Prometheus metrics hierarchy | ManagedCluster → Cloud Hub → Global Hub; supports cloud-level and cross-cloud dashboards |
| Namespace-per-gateway | Isolation boundary for RBAC and resource quotas; the gateway database lives on the registered server outside the cluster |
| Terraform for provisioning | IaC for VPC, subnet, and cluster lifecycle; cloud-agnostic |
| Gateway OIDC clients on ManagedCluster | openshell CLI authenticates against Keycloak where the gateway runs (low latency) |
| Shared Ingress Gateway for Tenant gRPC | A wildcard DNS record (`*.domain`) can only resolve to a single Load Balancer. A per-tenant gateway model (1 LB per tenant) fundamentally breaks wildcard routing, requiring per-tenant DNS automation and cert management. A shared Gateway allows N tenants to securely share 1 LB, 1 wildcard cert, and 1 static DNS record via `GRPCRoute` attachments. |
