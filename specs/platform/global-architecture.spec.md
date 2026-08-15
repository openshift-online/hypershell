# Global Architecture

**Date:** 2026-08-14
**Status:** Active

## Overview

HyperShell deploys as a global fleet management platform spanning multiple clouds and regions. The architecture uses a **three-tier hub-and-spoke topology**: a Global Hub provides federated identity root, Cloud Hubs run the operational platform (API, control plane, databases), and ManagedClusters host gateway workloads. Every OpenShift cluster in the topology runs the full operator stack (ArgoCD, Vault, Keycloak, CNPG, Prometheus, Grafana) but serves different purposes at each tier.

## Three-Tier Topology

HyperShell uses a three-tier hub-and-spoke architecture. Each tier runs the full operator stack but serves distinct purposes.

```mermaid
graph TB
    subgraph Global["Global Hub (Identity Root)"]
        GK[Keycloak<br/>Federated to RH SSO]
        GV[Vault<br/>Reserved]
        GG[Grafana<br/>Cross-Cloud Dashboard]
    end

    subgraph AWS["Cloud Hub: AWS"]
        AK[Keycloak<br/>Federates to Global]
        AV[Vault<br/>Service Secrets]
        AArgo[ArgoCD<br/>Fleet GitOps]
        ADB[(PostgreSQL<br/>CNPG)]
        ACP[Control Plane]
        AAPI[API Server]
        AUI[Web UI]
        AP[Prometheus]
        AG[Grafana]
    end

    subgraph IBM["Cloud Hub: IBM Cloud"]
        IK[Keycloak<br/>Federates to Global]
        IV[Vault<br/>Service Secrets]
        IArgo[ArgoCD<br/>Fleet GitOps]
        IDB[(PostgreSQL<br/>CNPG)]
        ICP[Control Plane]
        IAPI[API Server]
        IUI[Web UI]
        IP[Prometheus]
        IG[Grafana]
    end

    subgraph MC1["ManagedCluster: AWS us-east-1"]
        M1K[Keycloak<br/>Gateway Clients]
        M1V[Vault<br/>Gateway Secrets]
        M1DB[(PostgreSQL<br/>CNPG)]
        M1P[Prometheus]
        M1GW[Gateway Namespaces]
    end

    subgraph MC2["ManagedCluster: AWS us-west-2"]
        M2K[Keycloak<br/>Gateway Clients]
        M2V[Vault<br/>Gateway Secrets]
        M2DB[(PostgreSQL<br/>CNPG)]
        M2P[Prometheus]
        M2GW[Gateway Namespaces]
    end

    subgraph MC3["ManagedCluster: IBM us-east"]
        M3K[Keycloak<br/>Gateway Clients]
        M3V[Vault<br/>Gateway Secrets]
        M3DB[(PostgreSQL<br/>CNPG)]
        M3P[Prometheus]
        M3GW[Gateway Namespaces]
    end

    GK -->|Federation| AK
    GK -->|Federation| IK
    AK -->|Federation| M1K
    AK -->|Federation| M2K
    IK -->|Federation| M3K
    
    ACP -->|Reconcile| M1GW
    ACP -->|Reconcile| M2GW
    ICP -->|Reconcile| M3GW
    
    M1P -->|Metrics| AP
    M2P -->|Metrics| AP
    M3P -->|Metrics| IP
    
    AP -->|Aggregate| GG
    IP -->|Aggregate| GG

    style Global fill:#e1f5ff
    style AWS fill:#fff3cd
    style IBM fill:#fff3cd
    style MC1 fill:#d4edda
    style MC2 fill:#d4edda
    style MC3 fill:#d4edda
```

### Tier 1: Global Hub

**Purpose**: Identity federation root and cross-cloud observability.

**Components**:
- Keycloak (federates to Red Hat SSO)
- Vault (reserved for future global secrets)
- Grafana (single-pane-of-glass aggregating metrics from all Cloud Hubs)

**Operational Role**: Provides the root of the Keycloak federation chain. Cloud Hubs federate to Global Keycloak, which federates to Red Hat SSO. Future: global monitoring dashboard.

### Tier 2: Cloud Hub

**Purpose**: Primary operational unit. One highly-available instance per cloud provider.

**Components**:
- API Server, Control Plane, Web UI (HA deployment)
- PostgreSQL (via CNPG) - source of truth for Fleet, Gateway, ManagedCluster resources
- ArgoCD - reconciles this cloud's infrastructure from Git
- Keycloak - federates to Global Keycloak, serves cloud services
- Vault - secrets for cloud hub services (API server, control plane)
- Prometheus - aggregates metrics from this cloud's ManagedClusters
- Grafana - cloud-level dashboards

**Operational Role**: The control plane watches the API server via gRPC and reconciles gateway resources into ManagedClusters. ArgoCD defines and provisions ManagedClusters.

### Tier 3: ManagedCluster

**Purpose**: Hosts gateway workloads. Multiple per cloud, deployed close to users (regional).

**Components**:
- Keycloak - federates to Cloud Hub Keycloak, holds OIDC clients for gateways on this cluster
- Vault - keystore for gateway secrets
- PostgreSQL (via CNPG) - gateway-specific databases
- Prometheus - local metrics (forwarded to Cloud Hub)
- Gateway namespaces (each contains: Gateway pod, Supervisor, Sandboxes, CNPG Cluster, TLS secrets, RBAC)

**Operational Role**: Runs gateway workloads. Users authenticate openshell CLI against Keycloak on the ManagedCluster where their gateway lives.

## Data Flows

### Keycloak Federation Chain

Identity flows from Red Hat SSO down through the tier hierarchy. Each Keycloak federates to the one above it.

```mermaid
graph LR
    RHSSO[Red Hat SSO]
    Global[Global Keycloak]
    Cloud[Cloud Hub Keycloak]
    MC[ManagedCluster Keycloak]
    
    RHSSO -->|Federates| Global
    Global -->|Federates| Cloud
    Cloud -->|Federates| MC
    
    MC -->|Holds| Clients[Gateway OIDC Clients]
    
    style RHSSO fill:#e74c3c
    style Global fill:#3498db
    style Cloud fill:#f39c12
    style MC fill:#2ecc71
    style Clients fill:#95a5a6
```

**Federation Path**: Red Hat SSO → Global Keycloak → Cloud Keycloak → ManagedCluster Keycloak

**Client Registration**: Gateway OIDC clients are registered in the ManagedCluster Keycloak where the gateway runs.

### Gateway Authentication Flow

When a user authenticates their openshell CLI to a gateway:

```mermaid
sequenceDiagram
    participant User as openshell CLI
    participant Gateway as Gateway Pod
    participant MCK as ManagedCluster<br/>Keycloak
    participant CloudK as Cloud Hub<br/>Keycloak
    participant GlobalK as Global<br/>Keycloak
    participant RHSSO as Red Hat SSO

    User->>Gateway: Connect to gateway
    Gateway->>MCK: Redirect to OIDC login
    MCK->>User: Show login page
    User->>MCK: Provide credentials
    MCK->>CloudK: Federate authentication
    CloudK->>GlobalK: Federate authentication
    GlobalK->>RHSSO: Federate authentication
    RHSSO-->>GlobalK: Auth response
    GlobalK-->>CloudK: Auth response
    CloudK-->>MCK: Auth response
    MCK->>User: Issue token
    User->>Gateway: Request with token
    Gateway->>MCK: Validate token
    MCK-->>Gateway: Token valid
    Gateway-->>User: Connected
```

### Control Plane Reconciliation Flow

The control plane on the Cloud Hub watches the API server and reconciles gateway resources into ManagedClusters.

```mermaid
sequenceDiagram
    participant User as API Client
    participant API as API Server<br/>(Cloud Hub)
    participant DB as PostgreSQL<br/>(CNPG)
    participant CP as Control Plane<br/>(Cloud Hub)
    participant MC as ManagedCluster<br/>K8s API

    User->>API: POST /gateways
    API->>DB: INSERT gateway record
    DB-->>API: Row created
    API->>CP: gRPC Watch event<br/>(Gateway created)
    CP->>MC: kubectl apply<br/>Gateway namespace
    MC-->>CP: Namespace created
    CP->>MC: kubectl apply<br/>Gateway StatefulSet
    MC-->>CP: StatefulSet created
    CP->>MC: kubectl apply<br/>CNPG Cluster
    MC-->>CP: PostgreSQL provisioning
    CP->>DB: UPDATE gateway status
    DB-->>CP: Status updated
    CP->>API: gRPC status update
    API-->>User: Gateway deployed
```

**Key Points**:
- PostgreSQL on the Cloud Hub is the source of truth for all resource state
- Control Plane watches API server via gRPC streams
- Control Plane reconciles resources into ManagedClusters via kubeconfig secrets
- Gateway databases run as CNPG Clusters in the gateway namespace on the ManagedCluster


## Ingress Architecture

HyperShell utilizes a **dual-ingress strategy** on OpenShift clusters, separating platform management traffic from tenant gateway traffic. This ensures isolation, security, and optimal routing for different protocols.

### 1. Platform Services (Default OpenShift Ingress)

Traffic destined for HyperShell management services (API Server, Web Console, Keycloak) uses the default OpenShift routing tier.

- **Domain:** Standard OpenShift wildcard domain (e.g., `*.apps.rosa...`)
- **Load Balancer:** AWS Internal Network Load Balancer (NLB)
- **Routing Object:** OpenShift `Route` (HAProxy)
- **Mechanism:** The wildcard DNS resolves to the internal NLB, which forwards traffic to the OpenShift router pods. HAProxy uses hostname-based routing to direct traffic to the correct service.

### 2. Tenant Gateways (Kubernetes Gateway API)

Traffic destined for the actual managed gRPC gateways bypasses the default OpenShift router and uses a dedicated ingress path managed by the Kubernetes Gateway API (backed by Istio).

- **Domain:** Dedicated base domain (e.g., `*.openshell.stage.devshift.net`)
- **Load Balancer:** Dedicated AWS Classic ELB (provisioned by the Gateway API controller)
- **Routing Object:** `GRPCRoute` attached to a central `Gateway`
- **Mechanism:** A single shared `Gateway` object (`openshell-grpc-gateway`) in the `openshift-ingress` namespace terminates TLS using a wildcard certificate. When the control plane provisions a new gateway in a tenant namespace, it creates a `GRPCRoute` that automatically attaches to this shared `Gateway`. External DNS (e.g., Route53) manages the CNAME mapping the wildcard domain to the AWS ELB address.

This architecture allows the control plane to dynamically route traffic for new gateways without needing to provision individual load balancers or DNS records per tenant.

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

Gateway and its sandboxes coexist in the same namespace as a scalable unit. Each gateway deployment gets its own namespace containing all required resources.

```mermaid
graph TB
    subgraph NS["namespace: openshell-<gateway-name>"]
        subgraph Workloads
            GW[Gateway StatefulSet<br/>+ Supervisor Sidecar]
            SB1[Sandbox Pod 1]
            SB2[Sandbox Pod 2]
            SBN[Sandbox Pod N<br/>dynamic]
        end
        
        subgraph Data
            DB[(PostgreSQL<br/>CNPG Cluster)]
        end
        
        subgraph Config
            CM[ConfigMaps]
            SEC[TLS Secrets]
            VSEC[Vault Secrets]
        end
        
        subgraph Security
            NP[NetworkPolicies]
            SA[ServiceAccounts]
            ROLE[Roles]
            RB[RoleBindings]
        end
    end
    
    GW --> DB
    GW --> CM
    GW --> SEC
    SB1 --> VSEC
    SB2 --> VSEC
    SBN --> VSEC
    
    style NS fill:#f0f0f0
    style Workloads fill:#d4edda
    style Data fill:#cce5ff
    style Config fill:#fff3cd
    style Security fill:#f8d7da
```

**Resources per Gateway Namespace**:
- **Workloads**: Gateway StatefulSet (with Supervisor sidecar), dynamic Sandbox pods
- **Data**: PostgreSQL (CNPG Cluster) provisioned by ManagedDatabase controller
- **Config**: ConfigMaps, TLS secrets (cert-manager or certgen), Vault-backed secrets
- **Security**: NetworkPolicies, ServiceAccounts, Roles, RoleBindings

This namespace-per-gateway strategy provides:
- Isolation boundary for RBAC and network policies
- Resource quotas per gateway
- Self-contained lifecycle (delete namespace = delete gateway)
- CNPG Cluster co-located with gateway workload

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

Prometheus runs on every cluster. Metrics flow from ManagedClusters → Cloud Hub → Global Hub.

```mermaid
graph TB
    subgraph MC1["ManagedCluster: AWS us-east-1"]
        P1[Prometheus]
        M1[Gateway Metrics]
        N1[Node Metrics]
        M1 --> P1
        N1 --> P1
    end

    subgraph MC2["ManagedCluster: AWS us-west-2"]
        P2[Prometheus]
        M2[Gateway Metrics]
        N2[Node Metrics]
        M2 --> P2
        N2 --> P2
    end

    subgraph MC3["ManagedCluster: IBM us-east"]
        P3[Prometheus]
        M3[Gateway Metrics]
        N3[Node Metrics]
        M3 --> P3
        N3 --> P3
    end

    subgraph CloudAWS["Cloud Hub: AWS"]
        PA[Prometheus<br/>Cloud Aggregator]
        GA[Grafana<br/>Cloud Dashboards]
        AA[Alertmanager]
        PA --> GA
        PA --> AA
    end

    subgraph CloudIBM["Cloud Hub: IBM"]
        PI[Prometheus<br/>Cloud Aggregator]
        GI[Grafana<br/>Cloud Dashboards]
        AI[Alertmanager]
        PI --> GI
        PI --> AI
    end

    subgraph Global["Global Hub"]
        PG[Prometheus<br/>Global Aggregator]
        GG[Grafana<br/>Single Pane of Glass]
        AG[Alertmanager]
        PG --> GG
        PG --> AG
    end

    P1 -->|Remote Write| PA
    P2 -->|Remote Write| PA
    P3 -->|Remote Write| PI

    PA -->|Federation| PG
    PI -->|Federation| PG

    style MC1 fill:#d4edda
    style MC2 fill:#d4edda
    style MC3 fill:#d4edda
    style CloudAWS fill:#fff3cd
    style CloudIBM fill:#fff3cd
    style Global fill:#e1f5ff
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
| Three-tier topology | Separates concerns: Global (identity root), Cloud Hub (operations), ManagedCluster (workloads) |
| Cloud Hub as primary unit | One HA instance per cloud runs API/control-plane/database; cloud isolation for latency and compliance |
| Full operator stack on all tiers | Every cluster has ArgoCD, Vault, Keycloak, CNPG, Prometheus - but serves different purposes per tier |
| Federated Keycloak chain | RH SSO → Global → Cloud Hub → ManagedCluster - identity flows down, authentication bubbles up |
| Vault per tier with distinct purposes | Cloud Hub Vault: service secrets; ManagedCluster Vault: gateway keystores |
| CNPG on all clusters | Kubernetes-native lifecycle, portable across clouds, no vendor lock-in |
| PostgreSQL on Cloud Hub as source of truth | All Fleet/Gateway/ManagedCluster resource state lives in Cloud Hub database |
| ManagedClusters can be standard K8s | Maximizes deployment flexibility; only hubs need OpenShift |
| Tekton over bash scripts | Deterministic, auditable, cattle-not-pets infrastructure |
| ArgoCD on Cloud Hubs | Each Cloud Hub ArgoCD reconciles its own ManagedClusters from Git |
| Prometheus metrics hierarchy | ManagedCluster → Cloud Hub → Global Hub; supports cloud-level and cross-cloud dashboards |
| Namespace-per-gateway | Isolation boundary for RBAC, NetworkPolicy, resource quotas, and CNPG Cluster |
| Terraform for provisioning | IaC for VPC, subnet, and cluster lifecycle; cloud-agnostic |
| Gateway OIDC clients on ManagedCluster | openshell CLI authenticates against Keycloak where the gateway runs (low latency) |
