# Global Architecture

**Date:** 2026-08-10
**Status:** Active

## Overview

HyperShell deploys as a global fleet management platform spanning multiple clouds and regions. A single OpenShift hub per cloud manages gateway deployments across regions. Managed clusters can run standard Kubernetes (not restricted to OpenShift). The architecture supports three deployment patterns: single-node, global multi-region, and multi-cloud.

## Deployment Patterns

### Single-Node

One OpenShift cluster runs the full HyperShell stack: API server, controller, PostgreSQL (via CNPG), gateways, and supporting services. Suitable for development, testing, and small-scale production.

```
┌──────────────────────────────────────────────────┐
│  OpenShift Cluster                               │
│                                                  │
│  ┌────────────┐ ┌────────────┐ ┌──────────────┐ │
│  │ API Server │ │ Controller │ │ PostgreSQL   │ │
│  │            │ │            │ │ (CNPG)       │ │
│  └────────────┘ └────────────┘ └──────────────┘ │
│                                                  │
│  ┌────────────────────────────────────────────┐  │
│  │ Gateway Namespace                          │  │
│  │ ┌──────────┐ ┌────────────┐ ┌──────────┐  │  │
│  │ │ Gateway  │ │ Supervisor │ │ Sandboxes│  │  │
│  │ └──────────┘ └────────────┘ └──────────┘  │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│  ArgoCD │ Vault │ Keycloak │ Grafana │ Prometheus │
└──────────────────────────────────────────────────┘
```

### Global Multi-Region

One OpenShift hub manages gateways across multiple regions within a single cloud provider. Regional managed clusters run gateways close to users. The hub controller provisions gateways remotely via kubeconfig secrets.

```
                    ┌─────────────────────────────┐
                    │  Hub (us-east-1)             │
                    │  OpenShift                   │
                    │  API + Controller + CNPG     │
                    │  ArgoCD + Vault + Keycloak   │
                    │  Prometheus + Grafana         │
                    └──────┬──────────┬────────────┘
                           │          │
              ┌────────────┘          └────────────┐
              ▼                                    ▼
┌─────────────────────────┐          ┌─────────────────────────┐
│  Managed Cluster        │          │  Managed Cluster        │
│  us-west-2              │          │  eu-west-1              │
│  K8s or OpenShift       │          │  K8s or OpenShift       │
│  ┌────────┐ ┌────────┐  │          │  ┌────────┐ ┌────────┐  │
│  │Gateway │ │Sandbox │  │          │  │Gateway │ │Sandbox │  │
│  └────────┘ └────────┘  │          │  └────────┘ └────────┘  │
│  Prometheus              │          │  Prometheus              │
└─────────────────────────┘          └─────────────────────────┘
```

### Multi-Cloud

Separate hubs per cloud provider, each managing their own regional clusters. A global coordination layer provides cross-cloud visibility.

```
┌─────────────────────────────┐   ┌─────────────────────────────┐
│  IBM Cloud Hub               │   │  AWS Hub                     │
│  OpenShift (ROKS)            │   │  OpenShift (ROSA)            │
│  API + Controller + CNPG     │   │  API + Controller + CNPG     │
│  ArgoCD + Vault              │   │  ArgoCD + Vault              │
│         │                    │   │         │                    │
│    ┌────┴────┐               │   │    ┌────┴────┐               │
│    ▼         ▼               │   │    ▼         ▼               │
│ ┌──────┐ ┌──────┐           │   │ ┌──────┐ ┌──────┐           │
│ │MC    │ │MC    │           │   │ │MC    │ │MC    │           │
│ │east  │ │west  │           │   │ │east  │ │west  │           │
│ └──────┘ └──────┘           │   │ └──────┘ └──────┘           │
└─────────────────────────────┘   └─────────────────────────────┘
```

## Tooling Stack

| Component | Tool | Purpose |
|-----------|------|---------|
| Database operator | CNPG (CloudNativePG) | PostgreSQL lifecycle (replaces per-gateway cloud databases) |
| GitOps | ArgoCD | Reconciles cluster state from Git |
| Secret management | Vault | Stores and rotates secrets with cloud-native drivers |
| Identity | Keycloak | OIDC authentication for gateways and console |
| Cluster provisioning | Terraform | VPC, subnet, and cluster provisioning |
| Installer | Tekton pipelines | Reproducible, deterministic deployment pipelines |
| Monitoring | Prometheus | Metrics collection on all clusters |
| Dashboards | Grafana | Centralized visualization on hub |

## Database Strategy: CNPG

CloudNativePG replaces per-gateway cloud-managed databases (RDS, Cloud SQL). CNPG runs PostgreSQL clusters as Kubernetes-native resources with automated failover, backup, and recovery.

### Requirements

#### Requirement: CNPG Operator Deployment

The CNPG operator SHALL be deployed on the hub cluster. Gateway databases SHALL be provisioned as CNPG Cluster resources in the gateway namespace.

##### Scenario: Gateway Database Provisioning via CNPG

- GIVEN a Gateway resource with a `database_id` referencing a ManagedDatabase
- WHEN the ManagedDatabase specifies `provider: cnpg`
- THEN the controller SHALL create a CNPG Cluster resource in the gateway namespace
- AND the CNPG operator SHALL provision a PostgreSQL instance with automated replication

#### Requirement: Database Lifecycle Independence

ManagedDatabase resources SHALL have an independent lifecycle from Gateways. A single CNPG cluster MAY serve multiple gateways within the same namespace.

## Namespace Strategy

Gateway and its sandboxes coexist in the same namespace as a scalable unit. Each gateway deployment gets its own namespace containing:

- Gateway pod (StatefulSet)
- Supervisor sidecar
- Sandbox pods
- PostgreSQL (CNPG Cluster or in-namespace Deployment)
- TLS certificates (cert-manager or certgen)
- NetworkPolicies
- RBAC resources

```
namespace: openshell-<gateway-name>
├── Gateway StatefulSet
├── Supervisor
├── Sandbox pods (dynamic)
├── PostgreSQL (CNPG Cluster)
├── TLS Secrets
├── ConfigMaps
├── NetworkPolicies
└── RBAC (Roles, RoleBindings, ServiceAccounts)
```

## Installer Pipeline

Tekton pipelines provide a reproducible installer for HyperShell deployments. The pipeline replaces manual bash scripts with deterministic, auditable steps.

### Requirements

#### Requirement: Deterministic Installation

HyperShell installation SHALL be performed via Tekton pipelines that execute idempotent steps. Manual `oc apply` or `kubectl` scripts SHALL NOT be the primary installation method in production.

##### Scenario: Fresh Cluster Installation

- GIVEN a bare OpenShift cluster with Tekton installed
- WHEN the HyperShell installer pipeline runs
- THEN it SHALL provision: CNPG operator, cert-manager, API server, controller, PostgreSQL, ArgoCD, Vault, Keycloak
- AND the installation SHALL be idempotent (safe to re-run)

#### Requirement: Cattle Not Pets

Infrastructure SHALL be treated as disposable. Any cluster can be torn down and rebuilt from the pipeline without manual intervention or state recovery.

## Monitoring Architecture

Prometheus runs on every managed cluster. Regional aggregation feeds into the global hub.

```
Managed Cluster (region)          Hub Cluster
┌─────────────────────┐          ┌─────────────────────┐
│ Prometheus (local)  │ ──────▶  │ Prometheus (global)  │
│ Gateway metrics     │          │ Grafana dashboards   │
│ Node metrics        │          │ Alertmanager         │
└─────────────────────┘          └─────────────────────┘
```

### Requirements

#### Requirement: Managed Cluster Metrics

Every managed cluster SHALL run a Prometheus instance that scrapes gateway and node metrics. Metrics SHALL be forwarded to the regional or global hub.

#### Requirement: Hub Dashboards

The hub cluster SHALL run Grafana with dashboards for fleet-wide gateway health, resource utilization, and provisioning status across all managed clusters.

## Managed Cluster Flexibility

Managed clusters are not restricted to OpenShift. Standard Kubernetes distributions (EKS, GKE, vanilla K8s) are valid targets. The controller auto-detects cluster capabilities:

| Capability | Detection | Behavior when absent |
|------------|-----------|---------------------|
| OpenShift Routes | `route.openshift.io` API group | Use Gateway API or NodePort |
| cert-manager | `cert-manager.io` API group | Block deployment (required) |
| Gateway API | `gateway.networking.k8s.io/v1` GRPCRoute | Skip GRPCRoute/BackendTLSPolicy |
| Agent Sandbox CRD | `agents.x-k8s.io` | Sandbox creation blocked |

## GitOps Repository Structure

ArgoCD manages cluster state from a Git repository. Each cloud and region has its own overlay.

```
gitops-repo/
├── base/
│   ├── hypershell/
│   │   ├── api-server.yaml
│   │   ├── controller.yaml
│   │   └── postgres.yaml
│   ├── cnpg/
│   ├── cert-manager/
│   ├── vault/
│   └── keycloak/
├── overlays/
│   ├── ibm-us-east/
│   │   ├── kustomization.yaml
│   │   └── patches/
│   ├── aws-us-east/
│   │   ├── kustomization.yaml
│   │   └── patches/
│   └── aws-eu-west/
│       ├── kustomization.yaml
│       └── patches/
└── clusters/
    ├── ibm-hub.yaml        (ArgoCD Application)
    ├── aws-hub.yaml
    └── managed/
        ├── rosa-vteam.yaml
        └── eks-staging.yaml
```

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| One OpenShift hub per cloud | Reduces cross-cloud latency; hub needs OpenShift for Routes and SCC |
| Managed clusters can be standard K8s | Maximizes deployment flexibility; only the hub needs OpenShift |
| CNPG over cloud-managed databases | Kubernetes-native lifecycle, portable across clouds, no vendor lock-in |
| Tekton over bash scripts | Deterministic, auditable, cattle-not-pets infrastructure |
| ArgoCD for GitOps | Declarative cluster state, drift detection, multi-cluster support |
| Vault for secrets | Centralized rotation, cloud-native drivers, audit trail |
| Prometheus on all clusters | Uniform metrics pipeline, regional aggregation to global hub |
| Namespace-per-gateway | Isolation boundary for RBAC, NetworkPolicy, and resource quotas |
| Terraform for provisioning | IaC for VPC, subnet, and cluster lifecycle; cloud-agnostic |
