# OpenShell Gateway Specification

**Date:** 2026-08-05
**Status:** Draft
**Related:** `control-plane.spec.md` - CP reconciliation patterns; `data-model.spec.md` - Gateway kind definition; `security/gateway-rbac-policy.spec.md` - gateway RBAC
**Skill:** `skills/build/full-stack-pipeline/` - wave-based implementation pipeline
**Upstream:** [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell) - gateway Helm chart, `server.externalDbSecret` pattern; [OpenShell OIDC User Authentication](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-user-authentication)

### Sub-Specifications

This specification covers core provisioning. Domain-specific concerns are defined in dedicated sub-specs:

| Sub-Spec | Scope |
|---|---|
| [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) | TLS certificate management via cert-manager, SAN management, cert rotation |
| [`openshell-gateway-oidc.spec.md`](./openshell-gateway-oidc.spec.md) | OIDC authentication, role validation, gateway.toml injection |
| [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) | External connectivity: Gateway API (GRPCRoute + BackendTLSPolicy), NetworkPolicy, route discovery |
| [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) | PostgreSQL provisioning, credential security, manual rotation, deletion protection |
| [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) | Credential storage driver selection (encrypted DB, Kubernetes Secrets, Vault), RBAC, TOML generation |
| [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md) | Automated per-gateway Keycloak OIDC client provisioning, gateway ownership, visibility scoping |

---

## Purpose

The control plane SHALL provision and reconcile OpenShell gateway deployments in dedicated, API-assigned namespaces through a fully API-driven model. The API server persists Gateway resources in PostgreSQL. The control plane discovers Gateway resources via the same gRPC watch stream used for all other resources and reconciles them into Kubernetes gateway deployments.

This specification covers core gateway provisioning. OIDC, TLS, routing, and database concerns are defined in dedicated sub-specs (see table above).

- **Core Provisioning** - Gateway as API resource, GatewayReconciler, shared kustomize library, manifest templating, config validation, kustomize overlays, gateway deployment resources, failure handling
- **Keycloak Integration** - Automated per-gateway OIDC client provisioning, gateway ownership, visibility scoping (see [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md))
- **OpenShift-Specific** - SCC bindings, security context adjustments

---

## Architecture

### Flow

```
hsctl apply -k overlays/tenant-a/
    │  renders kustomization.yaml (Project + Gateway)
    │  POST/PATCH each resource to API server
    ▼
API Server (PostgreSQL)
    │  validates admin_users exist in Keycloak
    │  persists Gateway resource with admin_users
    │  emits gRPC watch event
    ▼
Control Plane - GatewayReconciler (internal/reconciler/)
    │  receives Gateway ADDED/MODIFIED event
    │  provisions Keycloak OIDC client and populates oidc config (see keycloak spec)
    │  validates image, DNS names, TOML config
    │  creates the API-assigned namespace when absent
    │  applies gateway K8s manifests to that namespace
    ▼
Kubernetes (Deployment, Service, RBAC, certgen Job, NetworkPolicy)
```

### Gateway Namespace Ownership

The API server assigns the Gateway namespace as `openshell-<id-hex-8>` before persistence and event publication, where the suffix is the lowercase hexadecimal encoding of 8 bytes from the Gateway KSUID's random payload. This produces a 26-character namespace (e.g., `openshell-a1b2c3d4e5f67890`) that fits comfortably within Kubernetes DNS label limits even when used as a component in longer derived names. Clients do not select or update it. The GatewayReconciler creates that namespace if it is absent and then uses the persisted value for all resources. Gateway renames do not change namespace.

### Relationship to PlatformReconciler

The PlatformReconciler performs GitOps continuous sync from git repositories. It uses the shared kustomize library to render manifests, which may include `kind: Gateway` documents. The sync engine applies Gateway resources to the API server just like any other kind. The GatewayReconciler then reconciles them into Kubernetes.

### Route Exposure Data Flow

```
External Client (openshell CLI)
    │  TLS/HTTP2 (ALPN-negotiated)
    ▼
Per-Tenant Gateway (OpenShift gateway controller / Envoy)
    │  Terminates TLS, negotiates HTTP/2 via ALPN
    │  GRPCRoute matches on hostname, forwards to backendRef
    │  BackendTLSPolicy: re-encrypts to pod, verifies cert via CA
    ▼
openshell-gateway Service (ClusterIP :8080)
    │  gRPC/TLS (self-signed cert from openshell-server-tls Secret)
    ▼
openshell-gateway Pod
```

### Cluster Prerequisites

The following resources must exist on the cluster before the GatewayReconciler can operate. They are configuration prerequisites - the control plane does not create them.

1. **GatewayClass** - A GatewayClass matching `GATEWAY_API_GATEWAY_CLASS` (default: `openshift-default`). Must be created by an administrator. Referenced by per-tenant Gateway resources. On Kind clusters, use `cloud-provider-kind`.
2. **Wildcard TLS Secret (`grpc-gateway-certs`)** - The cluster's wildcard certificate for `*.<base-domain>`, stored as a Secret named `grpc-gateway-certs` in the HyperShell control plane namespace. The GatewayReconciler copies it into each tenant namespace for per-tenant Gateway TLS termination (see "Gateway Ingress TLS Certificate" below). An administrator must provision this Secret before any gateway can be exposed externally.
3. **cert-manager** - Required for TLS certificate lifecycle management. See [TLS spec](./openshell-gateway-tls.spec.md) and the "TLS Certificate Management via cert-manager" requirement below.

### Per-Tenant Route Resources (Managed by Control Plane)

For each gateway with `route` configuration, the control plane creates Gateway API resources across two namespaces:

1. **Gateway** -- A dedicated Gateway resource per openshell gateway, created in `openshift-ingress` (so the ingress operator auto-creates a DNSRecord pointing the hostname to the Gateway's ELB):
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: Gateway
   metadata:
     name: gw-<tenant-namespace>
     namespace: openshift-ingress
     labels:
       app.kubernetes.io/name: openshell
       app.kubernetes.io/component: gateway
       app.kubernetes.io/managed-by: hypershell-control-plane
       hypershell.redhat.io/tenant: <tenant-namespace>
   spec:
     gatewayClassName: <GATEWAY_API_GATEWAY_CLASS>   # default: openshift-default
     listeners:
     - name: grpc
       hostname: "gw-<tenant-namespace>.<base-domain>"
       port: 443
       protocol: HTTPS
       tls:
         mode: Terminate
         certificateRefs:
         - name: grpc-gateway-certs
           kind: Secret
       allowedRoutes:
         namespaces:
           from: Selector
           selector:
             matchLabels:
               kubernetes.io/metadata.name: <tenant-namespace>
   ```
   The `<base-domain>` is read from `ingresses.config.openshift.io/cluster` `.spec.domain` (e.g., `apps-crc.testing`). Each tenant gets its own Gateway with a hostname scoped to the tenant namespace. The `grpc-gateway-certs` Secret must exist in `openshift-ingress` (see "Gateway Ingress TLS Certificate" below).

2. **GRPCRoute** -- In the tenant namespace, with a cross-namespace parentRef to the Gateway in `openshift-ingress`:
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: GRPCRoute
   metadata:
     name: openshell-gateway
     namespace: <tenant-namespace>
   spec:
     parentRefs:
     - name: gw-<tenant-namespace>
       namespace: openshift-ingress
     hostnames:
     - gw-<tenant-namespace>.<base-domain>
     rules:
     - backendRefs:
       - name: openshell-gateway
         port: 8080
   ```

3. **BackendTLSPolicy** - Enables TLS verification from the Gateway to the pod:
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: BackendTLSPolicy
   metadata:
     name: openshell-gateway
     namespace: <tenant-namespace>
   spec:
     targetRefs:
     - group: ""
       kind: Service
       name: openshell-gateway
     validation:
       caCertificateRefs:
       - group: ""
         kind: ConfigMap
         name: openshell-backend-ca
       hostname: openshell-gateway.<namespace>.svc.cluster.local
   ```

4. **CA ConfigMap** - Contains the gateway pod's CA certificate for BackendTLSPolicy:
   ```yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: openshell-backend-ca
     namespace: <tenant-namespace>
   data:
     ca.crt: |
       <contents of openshell-server-tls Secret ca.crt>
   ```

### Gateway Ingress TLS Certificate

The per-tenant Gateway listener terminates external TLS using the cluster's wildcard certificate for `*.<base-domain>`. This certificate is stored as a Secret named `grpc-gateway-certs` in the `openshift-ingress` namespace, where all Gateway resources are created.

- **Location:** Secret `grpc-gateway-certs` in the `openshift-ingress` namespace
- The Secret is a cluster prerequisite -- it must be created before any gateway can be exposed externally (see README)
- All per-tenant Gateways in `openshift-ingress` reference the same Secret
- The wildcard private key is NOT copied into tenant namespaces -- it stays in `openshift-ingress`

This avoids per-tenant certificate issuance for the ingress listener - the `gw-<tenant-namespace>.<base-domain>` hostname is covered by the wildcard certificate.

> **Security improvement:** Unlike the previous approach that copied the wildcard private key into each tenant namespace, the key now stays in `openshift-ingress` (an admin-only namespace). Tenant namespace principals cannot extract the wildcard key.

### Route TLS Strategy

The Gateway API approach uses HTTPS on the listener and BackendTLSPolicy for re-encryption:

1. **Client to Gateway.** The per-tenant Gateway listener uses HTTPS (port 443) with a TLS certificate for `gw-<tenant-namespace>.<base-domain>`. Clients connect via `https://` and HTTP/2 is negotiated through ALPN during the TLS handshake.
2. **Gateway to Pod.** BackendTLSPolicy instructs the Gateway to establish a TLS connection to the backend pod, verifying the pod's certificate against the CA in the `openshell-backend-ca` ConfigMap. The pod's TLS remains enabled (no `disableTls` needed). BackendTLSPolicy requires OpenShift 4.22+.
3. **Fallback.** If BackendTLSPolicy is not supported by the cluster's gateway controller, the control plane SHALL skip BackendTLSPolicy creation and log a warning. The gateway pod's TLS configuration would need to be disabled manually in this case.

### Route Hostname Convention

Gateway and GRPCRoute hostnames follow the pattern: `gw-<tenant-namespace>.<base-domain>`

Examples:
- `gw-openshell-a1b2c3d4e5f67890.apps-crc.testing`
- `gw-openshell-b9c8d7e6f5a43210.apps.cluster.example.com`

The `gw-` prefix produces a hostname that is a subdomain of `<base-domain>`. With the shortened 26-character namespace, the first DNS label (e.g., `gw-openshell-a1b2c3d4e5f67890` at 29 chars) is well within the 63-character DNS label limit, eliminating the need for truncation and hash suffixes. The cluster's existing wildcard certificate for `*.<base-domain>` covers all per-tenant Gateway listeners without requiring per-tenant certificate issuance.

---

## Requirements

### Requirement: Gateway as API Resource

Gateway SHALL be a first-class HyperShell resource kind, persisted in PostgreSQL and exposed via the REST API. Each Gateway declares an OpenShell gateway deployment with specific configuration, while the API server owns its namespace assignment.

#### Scenario: Create a Gateway via hsctl apply

- GIVEN a kustomize overlay containing a `gateway.yaml`:
  ```yaml
  kind: Gateway
  name: openshell-gateway
  image: ghcr.io/nvidia/openshell/gateway:21da343c9f838bd9ac85dc61bf44889de1a72873
  ```
- WHEN a user runs `hsctl apply -k overlays/tenant-a/`
- THEN the CLI SHALL render the kustomization and POST the Gateway resource to the API server
- AND the API server SHALL persist the Gateway in PostgreSQL
- AND the persisted Gateway SHALL have a unique namespace derived from its identifier
- AND the API server SHALL emit a gRPC watch event for the new Gateway
- AND the GatewayReconciler SHALL receive the event and deploy gateway K8s resources to the assigned namespace

#### Scenario: Update a Gateway via overlay patch

- GIVEN a Gateway already exists for `tenant-a` with image `v0.0.70`
- AND a kustomize patch changes the image to `v0.0.71`
- WHEN a user runs `hsctl apply -k overlays/tenant-a/`
- THEN the CLI SHALL PATCH the existing Gateway resource
- AND the GatewayReconciler SHALL detect the change and update the gateway Deployment

#### Scenario: Gateway namespace does not exist yet

- GIVEN a newly persisted Gateway whose API-assigned namespace does not exist
- WHEN the GatewayReconciler processes its creation event
- THEN the GatewayReconciler SHALL create the namespace
- AND it SHALL continue reconciling the Gateway resources in that namespace

---

### Requirement: Shared Kustomize Library

The kustomize rendering engine SHALL be extracted from `hsctl apply/cmd.go` into a shared library package. This library SHALL be consumed by both the CLI (`hsctl apply`) and the PlatformReconciler.

#### Scenario: Library extraction

- GIVEN the kustomize engine currently lives in `components/hypershell-cli/cmd/hsctl/apply/cmd.go`
- WHEN the shared library is created
- THEN it SHALL be placed in a package accessible to both the CLI and the control plane (e.g., `components/hypershell-sdk/go-sdk/kustomize/`)
- AND it SHALL expose functions for: loading a kustomization directory, resolving bases, merging resources, applying strategic-merge patches, and producing a flat manifest stream
- AND the existing `hsctl apply` command SHALL be refactored to use the shared library
- AND the PlatformReconciler SHALL be updated to use the shared library for rendering

#### Scenario: Supported kinds

- GIVEN the shared kustomize library renders manifests
- THEN it SHALL support the following HyperShell resource kinds:
  - `Project`
  - `Gateway` *(new)*
- AND documents with unrecognized `kind` values SHALL be skipped with a warning

#### Scenario: Unit testability

- GIVEN the shared kustomize library
- THEN it SHALL be fully unit-testable without a running cluster or API server
- AND tests SHALL cover: base resolution, resource merging, strategic-merge patch semantics (scalar overwrite, map merge, sequence replace), `--dry-run` output, multi-document YAML, kind filtering, and error cases (missing bases, invalid YAML, circular references)

---

### Requirement: GatewayReconciler

The control plane SHALL include a GatewayReconciler in `internal/reconciler/` that watches Gateway resource events via the gRPC informer and reconciles them into Kubernetes gateway deployments.

#### Scenario: Gateway ADDED event

- GIVEN the GatewayReconciler receives a Gateway ADDED event
- WHEN the reconciler processes the event
- THEN it SHALL create the API-assigned namespace if it does not exist
- AND it SHALL validate the Gateway configuration (image reference, DNS names, TOML config)
- AND it SHALL apply all K8S resources needed for gateway provisioning to the namespace
- AND all resources SHALL carry the label `hypershell.redhat.io/managed-by=hypershell-control-plane`
- AND the reconciler SHALL use update-or-create semantics (SSA or equivalent)

#### Scenario: Gateway MODIFIED event

- GIVEN the GatewayReconciler receives a Gateway MODIFIED event
- WHEN the reconciler processes the event
- THEN it SHALL detect changes (image version, config, DNS names)
- AND it SHALL update the affected K8s resources
- AND the update SHALL be a rolling update for Deployments (zero downtime)

#### Scenario: Gateway DELETED event

- GIVEN the GatewayReconciler receives a Gateway DELETED event
- WHEN the reconciler processes the event
- THEN it SHALL delete gateway K8s resources from the namespace
- AND it SHALL NOT delete the namespace itself because legacy Gateway records may reference caller-selected or shared namespaces

#### Scenario: Validation failure

- GIVEN a Gateway resource with an invalid image reference or malformed TOML config
- WHEN the GatewayReconciler processes the event
- THEN it SHALL log a validation error with the Gateway name and assigned namespace
- AND it SHALL NOT apply any K8s resources
- AND it SHALL retry on the next reconciliation cycle

#### Scenario: Namespace creation fails

- GIVEN the GatewayReconciler receives a Gateway event whose assigned namespace does not exist
- WHEN namespace creation fails
- THEN reconciliation SHALL fail without applying namespaced resources
- AND it SHALL retry on the next reconciliation cycle

---

### Requirement: Gateway Manifest Templating

The GatewayReconciler SHALL load gateway resource manifests from the container filesystem and apply namespace-specific substitutions. This reuses the existing manifest loading and templating logic from the `internal/gateway/manifests.go` module.

#### Scenario: Load gateway manifests from filesystem

- GIVEN the HyperShell container includes gateway manifests at `/manifests/gateway/`
- WHEN the GatewayReconciler loads manifests
- THEN it SHALL read all YAML files from the manifests directory
- AND it SHALL parse each file into Kubernetes resource objects
- AND it SHALL substitute `NAMESPACE_PLACEHOLDER` with the target namespace name
- AND it SHALL substitute `DB_IMAGE_PLACEHOLDER` and `DB_STORAGE_PLACEHOLDER` before `IMAGE_PLACEHOLDER` to avoid substring collision
- AND it SHALL substitute `IMAGE_PLACEHOLDER` with the Gateway resource's `image` field

#### Scenario: Required manifest files missing

- GIVEN the `/manifests/gateway/` directory is missing or empty
- WHEN the GatewayReconciler attempts to load manifests
- THEN it SHALL log an error and fail gracefully
- AND it SHALL NOT crash the control plane

---

### Requirement: TLS Certificate Management via cert-manager

The GatewayReconciler SHALL use cert-manager for TLS certificate lifecycle management. cert-manager is a required cluster prerequisite. See [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) for full details.

**Why cert-manager:** cert-manager automates certificate lifecycle - issuance, renewal before expiry, and secret rotation - without operator intervention. cert-manager also integrates with external CAs (ACME, Vault, etc.) for production deployments.

**Cluster prerequisite:** cert-manager (v1.20+ recommended) must be installed cluster-wide by an administrator before gateways can use it.

**Coexistence with certgen job:** cert-manager handles TLS certificate lifecycle (issuance, renewal, rotation). The certgen job handles JWT key generation (`signing.pem`, `public.pem`, `kid` in the `openshell-gateway-jwt-keys` Secret). Both run: cert-manager creates TLS secrets, then certgen checks if they exist (skipping TLS) and only creates JWT keys. The certgen job remains in the deploy order for all gateways regardless of cert-manager availability.

---

### Requirement: Trusted CA Bundle Injection

Gateways with OIDC enabled need to reach the identity provider's OIDC discovery endpoint over HTTPS. In environments where the IdP is exposed through an ingress controller with a non-public CA certificate (e.g., OpenShift CRC, private PKI), the gateway pod's default trust store will not include the required CA and OIDC initialization will fail.

The control plane SHALL support an optional `gateway-trusted-ca` ConfigMap in the HyperShell namespace. When present, it is copied to each tenant namespace and mounted into the gateway Deployment so that the gateway process trusts the additional CA certificates.

#### Scenario: Trusted CA ConfigMap present in HyperShell namespace

- GIVEN a ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- AND the ConfigMap has a `ca-bundle.crt` key containing one or more PEM-encoded CA certificates
- WHEN the GatewayReconciler reconciles a gateway in a tenant namespace
- THEN it SHALL copy the `gateway-trusted-ca` ConfigMap to the tenant namespace (create-or-update pattern)
- AND it SHALL add a volume to the gateway Deployment mounting the `ca-bundle.crt` key at `/etc/pki/tls/certs/ca-bundle.crt` (read-only, using `subPath`)
- AND it SHALL add an `SSL_CERT_FILE` environment variable set to `/etc/pki/tls/certs/ca-bundle.crt` on the gateway container
- AND the mounted CA bundle SHALL be used by the gateway's TLS client for OIDC discovery and JWKS fetching

#### Scenario: Trusted CA ConfigMap absent

- GIVEN no ConfigMap named `gateway-trusted-ca` exists in the HyperShell namespace
- WHEN the GatewayReconciler reconciles a gateway
- THEN it SHALL NOT add any CA volume or `SSL_CERT_FILE` env var to the gateway Deployment
- AND the gateway SHALL use its built-in trust store (default behavior)
- AND this SHALL be the default for environments with publicly-trusted IdP certificates (e.g., production with a public CA)

#### Scenario: Trusted CA ConfigMap updated

- GIVEN a `gateway-trusted-ca` ConfigMap exists and has been updated (new certificates added or removed)
- WHEN the GatewayReconciler runs its next reconciliation cycle
- THEN it SHALL update the copy in the tenant namespace
- AND the gateway pod SHALL pick up the new CA bundle on its next restart

**Design rationale:** The OIDC issuer URL must be identical inside and outside the cluster (OpenShell requirement - see [Gateway Auth: OIDC](https://docs.nvidia.com/openshell/reference/gateway-auth#oidc)). On CRC, the external Keycloak Route uses HTTPS with the CRC ingress controller's self-signed CA. The gateway must reach this same URL, so it needs the ingress CA in its trust store. This approach generalizes to any environment where the IdP uses a private CA.

---

### Requirement: Gateway Configuration Validation

The GatewayReconciler SHALL validate Gateway resource fields before applying K8s manifests.

#### Scenario: Valid Gateway configuration

- GIVEN a Gateway with a valid image reference, RFC-1123-compliant DNS names, and valid TOML config
- WHEN the GatewayReconciler validates the configuration
- THEN validation SHALL pass and reconciliation SHALL proceed

#### Scenario: Invalid image reference

- GIVEN a Gateway with an image reference containing invalid characters
- WHEN the GatewayReconciler validates the configuration
- THEN validation SHALL fail with a descriptive error
- AND the Gateway SHALL not be reconciled until the configuration is corrected

> **GHCR image tag convention:** OpenShell gateway images on GHCR use commit-SHA tags only (no semver tags). For example, `ghcr.io/nvidia/openshell/gateway:21da343c9f838bd9ac85dc61bf44889de1a72873` corresponds to v0.0.91. The GatewayReconciler continuously reconciles the image field, so the gitops overlay must be the source of truth for the image tag - manual image changes on the Deployment will be reverted.

#### Scenario: Invalid DNS name

- GIVEN a Gateway with a `serverDnsNames` entry that violates RFC 1123
- WHEN the GatewayReconciler validates the configuration
- THEN validation SHALL fail with a descriptive error listing the invalid DNS name

---

### Requirement: Kustomize Overlay Structure for Gateways

Gateway resources SHALL be expressible in the existing `examples/` kustomize overlay structure alongside Project resources.

#### Scenario: Gateway in a tenant overlay

- GIVEN the directory `examples/overlays/tenant-a/`:
  ```
  kustomization.yaml
  project.yaml          # kind: Project
  gateway.yaml          # kind: Gateway
  ```
- AND `kustomization.yaml` references all resources:
  ```yaml
  kind: Kustomization
  bases:
    - ../../base
  resources:
    - project.yaml
    - gateway.yaml
  ```
- WHEN a user runs `hsctl apply -k examples/overlays/tenant-a/`
- THEN the Project and Gateway SHALL all be applied in order
- AND the ProjectReconciler SHALL create the Project namespace
- AND the GatewayReconciler SHALL create the Gateway's distinct API-assigned namespace
- AND the GatewayReconciler SHALL deploy the gateway into its assigned namespace

#### Scenario: Gateway base with per-tenant patches

- GIVEN a base gateway configuration in `examples/base/gateway.yaml`:
  ```yaml
  kind: Gateway
  name: openshell-gateway
  image: ghcr.io/nvidia/openshell/gateway:21da343c9f838bd9ac85dc61bf44889de1a72873
  serverDnsNames: []
  ```
- AND a tenant overlay patches the DNS names:
  ```yaml
  kind: Gateway
  name: openshell-gateway
  project: tenant-a
  serverDnsNames:
    - openshell-gateway.tenant-a.svc.cluster.local
  ```
- WHEN the kustomize engine resolves the overlay
- THEN the merged Gateway SHALL have the base image and the overlay's DNS names and project

---

### Requirement: Gateway Deployment Resources

For each Gateway resource, the GatewayReconciler SHALL deploy the following Kubernetes resources into the API-assigned namespace:

All gateway resources SHALL use fixed names (one gateway per namespace):
- Deployment: `openshell-gateway`
- Service: `openshell-gateway` (ClusterIP, ports: `grpc:8080` with `appProtocol: grpc`, `metrics:9090`)
- ServiceAccounts: `openshell-gateway`, `openshell-gateway-sandbox`, `openshell-gateway-certgen`
- ConfigMap: `openshell-gateway-config` (contains `gateway.toml`)
- Roles, RoleBindings, ClusterRole, ClusterRoleBinding (see RBAC section below)
- NetworkPolicies (see NetworkPolicy section below)

Additionally, the reconciler creates these resources based on gateway configuration:
- cert-manager Issuer and Certificate resources (see [TLS spec](./openshell-gateway-tls.spec.md))
- JWT key generation Job: `openshell-gateway-certgen` (see certgen details below)
- Database resources when `database` is configured (see [database spec](./openshell-gateway-database.spec.md))
- GRPCRoute and BackendTLSPolicy when `route` is configured (see [routing spec](./openshell-gateway-routing.spec.md))

All gateway resources SHALL carry the following labels:
- `app.kubernetes.io/name=openshell`
- `app.kubernetes.io/component=gateway`
- `app.kubernetes.io/managed-by=hypershell-control-plane`
- `hypershell.redhat.io/managed=true`

The gateway Deployment SHALL specify:
- **No init containers.** Database readiness is enforced by the control plane's `waitForDeploymentReady` check after reconciling `database.yaml`, before the gateway Deployment is created.
- **Container image:** from the Gateway resource's `image` field
- **Container args:** `--config /etc/openshell/gateway.toml`
- **SecurityContext:** `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, capabilities `drop: [ALL]`, `seccompProfile.type: RuntimeDefault`
- **Resource requests:** `cpu: 100m`, `memory: 256Mi`
- **Resource limits:** `cpu: 500m`, `memory: 512Mi`
- **Ports:** `grpc: 8080`, `health: 8081`, `metrics: 9090`
- **Probes:**
  - Startup: `GET /healthz` on `health` port (period 2s, failureThreshold 30)
  - Liveness: `GET /healthz` on `health` port (period 5s, failureThreshold 3)
  - Readiness: `GET /readyz` on `health` port (period 2s, failureThreshold 3)
- **Env vars:**
  - `OPENSHELL_DB_URL` from Secret `openshell-gateway-db-credentials` key `url`
  - `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` from Secret `openshell-gateway-credential-kek` key `key-encryption-key`
- **Volume mounts:**
  - `/etc/openshell` - ConfigMap `openshell-gateway-config` (readOnly)
  - `/etc/openshell-jwt` - Secret `openshell-gateway-jwt-keys` (readOnly)
  - `/etc/openshell-tls/server` - Secret `openshell-server-tls` (readOnly)
  - `/etc/openshell-tls/client-ca` - Secret `openshell-client-tls` (only `ca.crt` key, readOnly)

#### Scenario: Deploy gateway to assigned namespace

- GIVEN a Gateway resource has the assigned namespace `openshell-abc123`
- WHEN the GatewayReconciler reconciles
- THEN it SHALL ensure namespace `openshell-abc123` exists
- AND it SHALL apply all gateway manifests with namespace set to `openshell-abc123`
- AND it SHALL use update-or-create semantics (never create-and-ignore-AlreadyExists)

#### Scenario: Gateway already exists (idempotency)

- GIVEN `tenant-a` has an OpenShell gateway already deployed
- WHEN the GatewayReconciler reconciles again
- THEN it SHALL apply the latest configuration using SSA or equivalent
- AND it SHALL NOT create duplicate resources

---

### Requirement: Per-Gateway RBAC Resources

The GatewayReconciler SHALL create RBAC resources that grant the gateway pod and sandbox pods the permissions they need to operate.

#### Gateway ServiceAccount RBAC

The gateway pod runs as ServiceAccount `openshell-gateway` and needs:

1. **ClusterRole `openshell-gateway-node-reader`** (cluster-scoped):
   ```yaml
   rules:
   - apiGroups: ["authentication.k8s.io"]
     resources: ["tokenreviews"]
     verbs: ["create"]
   - apiGroups: [""]
     resources: ["nodes"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["namespaces"]
     verbs: ["get"]
   ```

2. **ClusterRoleBinding `openshell-gateway-node-reader-<tenant-namespace>`**: binds the ClusterRole to `ServiceAccount/openshell-gateway` in the tenant namespace. Each tenant gets a dedicated ClusterRoleBinding with a namespace-qualified name to avoid collisions across tenants. When a Gateway is deleted, the GatewayReconciler SHALL delete the corresponding ClusterRoleBinding.

3. **Role `openshell-gateway-sandbox`** (namespace-scoped):
   ```yaml
   rules:
   - apiGroups: ["agents.x-k8s.io"]
     resources: ["sandboxes", "sandboxes/status"]
     verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
   - apiGroups: [""]
     resources: ["events"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["pods"]
     verbs: ["get"]
   ```

4. **RoleBinding `openshell-gateway-sandbox`**: binds `Role/openshell-gateway-sandbox` to `ServiceAccount/openshell-gateway`.

#### Sandbox ServiceAccount

ServiceAccount `openshell-gateway-sandbox` is the identity for sandbox pods. On OpenShift, this SA receives the `privileged` SCC binding (see OpenShift-specific requirements).

#### Certgen Job RBAC

The JWT key generation Job runs as ServiceAccount `openshell-gateway-certgen` with:

1. **Role `openshell-gateway-certgen`**:
   ```yaml
   rules:
   - apiGroups: [""]
     resources: ["secrets"]
     verbs: ["get", "create"]
   ```

2. **RoleBinding `openshell-gateway-certgen`**: binds the Role to `ServiceAccount/openshell-gateway-certgen`.

---

### Requirement: JWT Key Generation Job

The GatewayReconciler SHALL create a Job (`openshell-gateway-certgen`) to generate JWT signing keys for the gateway. The certgen job creates a Secret named `openshell-gateway-jwt-keys` containing:
- `signing.pem` - ECDSA private key for signing gateway JWTs
- `public.pem` - corresponding public key
- `kid` - key identifier

#### Scenario: Certgen job runs alongside cert-manager

- GIVEN cert-manager is installed and creates TLS secrets
- WHEN the certgen job runs
- THEN it SHALL detect existing TLS secrets and skip TLS generation
- AND it SHALL only create JWT keys in `openshell-gateway-jwt-keys`

#### Scenario: Certgen job SecurityContext

- GIVEN the certgen job is created
- THEN the job container SHALL specify: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `seccompProfile.type: RuntimeDefault`, capabilities `drop: [ALL]`
- AND resource requests SHALL be `cpu: 50m`, `memory: 64Mi` with limits `cpu: 200m`, `memory: 128Mi`
- AND `backoffLimit` SHALL be `3` with `restartPolicy: OnFailure`

---

### Requirement: Gateway NetworkPolicies

The GatewayReconciler SHALL create NetworkPolicies to enforce network segmentation between the gateway, sandboxes, and external traffic.

#### Sandbox SSH NetworkPolicies

The gateway connects to sandbox pods via SSH on port 2222. Two NetworkPolicies SHALL be created to support both legacy and v2 sandbox label patterns:

1. **`openshell-gateway-sandbox-ssh`** (legacy labels):
   - Selects pods with label `openshell.ai/managed-by: openshell`
   - Allows ingress on TCP port 2222 from gateway pods (`app.kubernetes.io/name: openshell`, `app.kubernetes.io/instance: openshell-gateway`)

2. **`openshell-gateway-sandbox-ssh-v2`** (v2 labels):
   - Selects pods where label `agents.x-k8s.io/sandbox-name-hash` exists
   - Allows ingress on TCP port 2222 from gateway pods

#### Sandbox-to-Gateway NetworkPolicy

Sandbox pods need to connect back to the gateway for gRPC communication:

3. **`openshell-gateway-allow-sandbox-v2`**:
   - Selects gateway pods (`app.kubernetes.io/instance: openshell-gateway`, `app.kubernetes.io/name: openshell`)
   - Allows ingress on TCP ports 8080 and 8081 from pods with label `agents.x-k8s.io/sandbox-name-hash` (exists)

#### Router Ingress NetworkPolicy

See [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) for the `openshell-gateway-allow-router` NetworkPolicy that allows ingress from Gateway-labeled Envoy proxy pods.

#### Database NetworkPolicy

See [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) for the `openshell-gateway-db` NetworkPolicy that restricts database ingress to gateway pods only.

---

### Requirement: Gateway Configuration (gateway.toml)

The GatewayReconciler SHALL create a ConfigMap (`openshell-gateway-config`) containing a `gateway.toml` file that configures the OpenShell gateway process. The TOML is assembled from the Gateway resource fields and cluster-derived values.

#### Base Configuration

The gateway.toml SHALL always contain:

```toml
[openshell]
version = 1

[openshell.gateway]
bind_address             = "0.0.0.0:8080"
health_bind_address      = "0.0.0.0:8081"
metrics_bind_address     = "0.0.0.0:9090"
log_level                = "info"
sandbox_namespace        = "<tenant-namespace>"
default_image            = "<sandbox-default-image>"
supervisor_image         = "<supervisor-image>"
client_tls_secret_name   = "openshell-client-tls"
enable_loopback_service_http = true
policy_validation_failure_mode = "fail_closed"
server_sans              = [<serverDnsNames from Gateway resource>]

[openshell.gateway.tls]
cert_path      = "/etc/openshell-tls/server/tls.crt"
key_path       = "/etc/openshell-tls/server/tls.key"
client_ca_path = "/etc/openshell-tls/client-ca/ca.crt"

[openshell.gateway.credential_storage]
key_encryption_key_env = "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY"

[openshell.gateway.auth]
allow_unauthenticated_users = false

[openshell.gateway.gateway_jwt]
signing_key_path = "/etc/openshell-jwt/signing.pem"
public_key_path  = "/etc/openshell-jwt/public.pem"
kid_path         = "/etc/openshell-jwt/kid"
gateway_id       = "openshell-gateway"
ttl_secs         = 3600

[openshell.drivers.kubernetes]
grpc_endpoint              = "https://openshell-gateway.<namespace>.svc.cluster.local:8080"
service_account_name       = "openshell-gateway-sandbox"
supervisor_sideload_method = "image-volume"
sa_token_ttl_secs          = 3600
app_armor_profile          = "Unconfined"
topology                   = "single-cluster"

[openshell.drivers.kubernetes.sidecar]
image = "<supervisor-image>"
```

The `supervisor_image` field is configurable on the Gateway resource. If not set, it defaults to `ghcr.io/nvidia/openshell/supervisor:0.0.101`. The same image is used in both `[openshell.gateway].supervisor_image` and `[openshell.drivers.kubernetes.sidecar].image`.

#### OIDC Section (conditional)

When `oidc.issuer` is set on the Gateway resource, the reconciler injects the OIDC section. See [`openshell-gateway-oidc.spec.md`](./openshell-gateway-oidc.spec.md).

#### Auth Section

- Default: `allow_unauthenticated_users = false`
- When OIDC is enabled, the reconciler also injects the `[openshell.gateway.oidc]` section

---

### Requirement: OpenShift-Specific Gateway Provisioning

When the control plane detects that it is running on an OpenShift cluster (the `route.openshift.io` API group is available), the GatewayReconciler SHALL adjust the gateway deployment to conform to OpenShift's SecurityContextConstraints (SCC) and PodSecurity admission requirements. These adjustments follow the [NVIDIA OpenShell OpenShift deployment guide](https://docs.nvidia.com/openshell/kubernetes/openshift).

**Key difference from vanilla Kubernetes:** OpenShift enforces the `restricted` PodSecurity standard by default. The OpenShell Helm chart's hardcoded `fsGroup` and `runAsUser` values conflict with OpenShift's SCC admission controller, which assigns UIDs and GIDs from the namespace's allocated ranges. Additionally, sandbox pods require the `privileged` SCC to function correctly.

**TLS is NOT disabled.** The NVIDIA docs show `--set server.disableTls=true` for evaluation scenarios. HyperShell does NOT use this setting because BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod, which requires the gateway to serve TLS. The gateway's self-signed certificate (generated by cert-manager) is used for the backend TLS segment.

#### Scenario: SCC binding for sandbox service account

- GIVEN the GatewayReconciler is deploying a gateway to an OpenShift cluster
- AND the target namespace exists
- WHEN the reconciler applies gateway manifests
- THEN it SHALL ensure that the `privileged` SCC is bound to the `openshell-gateway-sandbox` ServiceAccount in the target namespace
- AND this binding SHALL be applied BEFORE the Deployment is created (so sandbox pods can schedule)
- AND the binding SHALL be equivalent to: `oc adm policy add-scc-to-user privileged -z openshell-gateway-sandbox -n <namespace>`
- AND the reconciler SHALL use update-or-create semantics for the SCC binding (idempotent)

#### Scenario: Pod security context adjustments for OpenShift

- GIVEN the GatewayReconciler is deploying a gateway to an OpenShift cluster
- WHEN the reconciler applies gateway manifests
- THEN it SHALL clear the `podSecurityContext.fsGroup` field (set to null/omit) so that OpenShift's SCC admission controller assigns the fsGroup from the namespace's allocated UID range
- AND it SHALL clear the `securityContext.runAsUser` field (set to null/omit) so that OpenShift's SCC admission controller assigns the UID from the namespace's allocated range
- AND all gateway containers SHALL set `securityContext.seccompProfile.type` to `RuntimeDefault` to satisfy the `restricted:latest` PodSecurity standard

#### Scenario: Gateway deployment on vanilla Kubernetes (unchanged)

- GIVEN the GatewayReconciler is deploying a gateway to a non-OpenShift cluster (e.g., Kind, EKS, GKE)
- WHEN the reconciler applies gateway manifests
- THEN it SHALL NOT modify `podSecurityContext.fsGroup` or `securityContext.runAsUser` (the chart defaults are correct for non-OpenShift)
- AND it SHALL NOT create SCC bindings (SCC is an OpenShift-only concept)
- AND the `seccompProfile.type: RuntimeDefault` SHALL still be set (it is valid on all Kubernetes clusters)

#### Scenario: Platform detection reuse

- GIVEN the GatewayReconciler already detects OpenShift for SCC/security adjustments
- AND the GatewayReconciler detects Gateway API availability for GRPCRoute provisioning
- WHEN the reconciler initializes
- THEN it SHALL reuse the same `isOpenShift` detection result for SCC/security adjustments
- AND it SHALL reuse the same `hasGatewayAPI` detection result for GRPCRoute provisioning
- AND both detections SHALL occur once at startup, not per-reconciliation

---

### Requirement: Gateway Deployment Failure Handling

When gateway deployment fails (e.g., ImagePullBackOff, insufficient permissions), the GatewayReconciler SHALL log the error and retry on subsequent reconcile cycles without crashing.

#### Scenario: Image pull failure

- GIVEN a Gateway resource specifies an image that does not exist
- WHEN Kubernetes attempts to pull the image
- THEN the Deployment SHALL enter ImagePullBackOff state
- AND the GatewayReconciler SHALL log an error with the Gateway name, project, and failure reason
- AND the GatewayReconciler SHALL retry on the next reconcile cycle

#### Scenario: Insufficient RBAC permissions

- GIVEN the CP ServiceAccount does NOT have permission to create Deployments in a namespace
- WHEN the GatewayReconciler attempts to apply gateway manifests
- THEN the Kubernetes API SHALL return a Forbidden error
- AND the GatewayReconciler SHALL log an error and continue processing other Gateway resources

---

### Requirement: Separation from Agent Configuration

Gateway provisioning SHALL be independent of agent definitions. Agent-specific configuration (schedules, prompts, policies) is out of scope for this specification.

---

### Requirement: Payload Delivery via SSH-over-gRPC

When the control plane needs to write payload files (`.mcp.json`, `CLAUDE.md`, credential configs) into a running sandbox, it SHALL use the OpenShell SSH-over-gRPC mechanism rather than `ExecSandbox`. Sandbox containers use a read-only root filesystem, so `ExecSandbox`-based writes (which run as the sandbox user) fail with "Permission denied". The SSH path routes through the supervisor's embedded SSH server (russh), which runs as root and can write to any path.

**Data path:**
```
Control Plane
  → gRPC: CreateSshSession(sandbox_id) → authorization token
  → gRPC: ForwardTcp (bidirectional stream)
      → TcpForwardInit: sandbox_id, service_id, SshRelayTarget, token
      → SSH handshake over the gRPC stream (net.Conn adapter)
      → Validate sandbox_path against allowlist regex (reject shell metacharacters, traversal)
      → SSH session: "mkdir -p '<dir>' && cat > '<path>'" with content piped to stdin
  → Repeat for each payload file over the same SSH connection
```

**Path validation:** Before constructing the shell command, each `sandbox_path` is validated against the regex `^/[a-zA-Z0-9/_.\\-]+$` and checked for `..` traversal segments. Paths that fail validation are rejected before any SSH session is opened.

**Implementation:** `internal/openshell/ssh_upload.go` - `GatewayClient.UploadPayloads()`

---

## Configuration

### Gateway Resource Schema (Consolidated)

| Field | Required | Default | Description |
|---|---|---|---|
| `name` | Yes | - | Resource name (typically `openshell-gateway`) |
| `namespace` | No | API assigned | Read-only Kubernetes namespace derived from the Gateway identifier |
| `image` | No | `ghcr.io/nvidia/openshell/gateway:0.0.101` | Gateway container image reference |
| `supervisor_image` | No | `ghcr.io/nvidia/openshell/supervisor:0.0.101` | Supervisor sidecar container image |
| `serverDnsNames` | Yes | - | DNS names for TLS certificate generation |
| `oidc` | No | - | OIDC authentication configuration (see OIDC spec) |
| `oidc.issuer` | Yes (to enable OIDC) | `""` | OIDC issuer URL; empty disables OIDC |
| `oidc.audience` | No | `"openshell-cli"` | Expected `aud` claim value in JWT |
| `oidc.jwks_ttl` | No | `3600` | JWKS key cache retention in seconds |
| `oidc.roles_claim` | No | `""` | Dot-delimited path to roles array in JWT claims |
| `oidc.admin_role` | No | `""` | Role name conferring admin access |
| `oidc.user_role` | No | `""` | Role name conferring standard user access |
| `oidc.scopes_claim` | No | `""` | Dot-delimited path to scopes array in JWT claims |
| `route` | No | - | Route configuration for external exposure |
| `route.host` | No | auto-derived | Hostname for the GRPCRoute |
| `routeAddress` | - | - | Read-only. External address populated by the control plane |
| `database` | No | - | Database backend configuration |
| `database.storageSize` | No | `5Gi` | PVC size for PostgreSQL data |
| `database.image` | No | `registry.redhat.io/rhel9/postgresql-16:latest` | PostgreSQL container image (Red Hat hardened) |
| `database.externalSecretRef` | No | - | Name of Secret with `url` key. Skips DB provisioning. Reserved (Phase 2) |

### Control Plane Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_API_BASE_DOMAIN` | auto-detected | Cluster base domain for hostname generation (read from `ingresses.config.openshift.io/cluster` `.spec.domain`) |
| `GATEWAY_API_GATEWAY_CLASS` | `openshift-default` | GatewayClass name for per-tenant Gateway resources (e.g., `cloud-provider-kind` for Kind clusters) |

### Example: Full Gateway Configuration

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:21da343c9f838bd9ac85dc61bf44889de1a72873
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
oidc:
  issuer: https://keycloak.example.com/realms/hypershell
  audience: hypershell-frontend
route: {}
database:
  storageSize: 10Gi
  image: registry.redhat.io/rhel9/postgresql-16:latest
```

---

## Data Model Changes

The Gateway kind in `data-model.spec.md` SHALL include `oidc`, `route`, `routeAddress`, and `database` fields:

```
Gateway {
    ...existing fields...
    jsonb  oidc         "nullable - OIDC authentication config: {issuer, audience, jwks_ttl, roles_claim, admin_role, user_role, scopes_claim}"
    jsonb  route        "nullable - route exposure config (host)"
    text   routeAddress "nullable - read-only external address populated by control plane"
    jsonb  database     "nullable - database backend config: {storageSize, image, externalSecretRef}"
}
```

Database migrations SHALL add the columns to the `gateways` table:

```sql
ALTER TABLE gateways ADD COLUMN oidc JSONB;
ALTER TABLE gateways ADD COLUMN route JSONB;
ALTER TABLE gateways ADD COLUMN route_address TEXT;
ALTER TABLE gateways ADD COLUMN database JSONB;
```

---

## RBAC Requirements (Consolidated)

### Control Plane ServiceAccount Permissions

The HyperShell control plane ServiceAccount SHALL have sufficient permissions to create and manage all gateway resources:

```yaml
- apiGroups: [""]
  resources: ["serviceaccounts", "secrets", "configmaps", "services", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["cert-manager.io"]
  resources: ["issuers", "certificates"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways", "grpcroutes", "backendtlspolicies"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]

- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gatewayclasses"]
  verbs: ["get", "list"]
```

### Per-Gateway RBAC Resources Created by the Reconciler

The GatewayReconciler creates RBAC resources within each tenant namespace for the gateway and sandbox pods. See the "Per-Gateway RBAC Resources" requirement above for details.

---

## Template Packaging

Gateway manifests SHALL be:
- Stored in the HyperShell codebase at `components/hypershell-control-plane/manifests/gateway/`
- Generated once during development using `helm template` (NOT Helm at runtime)
- Packaged into the HyperShell container image at build time
- Read from the container filesystem at `/manifests/gateway/` at runtime

---

## Upstream Helm Chart Provenance

HyperShell does NOT install the OpenShell gateway via Helm at runtime. The gateway manifests at `components/hypershell-control-plane/manifests/gateway/` were generated once using `helm template` from the upstream chart, then maintained as static files. Similarly, cert-manager resources and OpenShift adjustments are applied programmatically by the GatewayReconciler, not via Helm.

This section documents which upstream Helm chart values each HyperShell behavior is equivalent to, so that future configuration changes can be traced back to the upstream chart source.

### OpenShell Gateway Helm Chart

- **Chart:** `oci://ghcr.io/nvidia/openshell/helm-chart`
- **Source:** <https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell>
- **Docs:** <https://docs.nvidia.com/openshell/kubernetes/openshift>, <https://docs.nvidia.com/openshell/kubernetes/managing-certificates>

The baseline `helm template` command that produced the static manifests:

```bash
helm template openshell-gateway oci://ghcr.io/nvidia/openshell/helm-chart \
  --namespace NAMESPACE_PLACEHOLDER \
  --set "pkiInitJob.serverDnsNames={openshell-gateway.NAMESPACE_PLACEHOLDER.svc.cluster.local}"
```

| Helm `--set` value | HyperShell equivalent | Implementation location |
|---|---|---|
| `pkiInitJob.serverDnsNames={...}` | `serverDnsNames` field on the Gateway API resource; substituted into cert-manager Certificate SANs at reconcile time | `internal/reconciler/gateway_reconciler.go` |
| `certManager.enabled=true` | Auto-detected: GatewayReconciler checks for `cert-manager.io` API group at startup via `detectCertManager()`. When present, creates Issuer/Certificate resources inline | `internal/reconciler/gateway_reconciler.go` |
| `podSecurityContext.fsGroup=null` | On OpenShift only: `applyOpenShiftOverrides()` clears `fsGroup` from the Deployment pod securityContext before apply | `internal/gateway/reconciler.go` |
| `securityContext.runAsUser=null` | On OpenShift only: `applyOpenShiftOverrides()` clears `runAsUser` from container securityContext | `internal/gateway/reconciler.go` |
| `server.disableTls=true` | **NOT used.** BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod, requiring the gateway to serve TLS. TLS remains enabled on all clusters | N/A |
| `server.externalDbSecret` | PostgreSQL Secret with `url` key provisioned by default; the gateway workload receives `OPENSHELL_DB_URL` from the Secret | `internal/reconciler/gateway_reconciler.go` |
| `workload.kind=deployment` | Always Deployment - PostgreSQL is the sole backend | `internal/reconciler/gateway_reconciler.go` |
| `server.oidc.*` | `oidc` field on Gateway resource; injected into `gateway.toml` ConfigMap by `ApplyConfigOverrides` | `internal/gateway/manifests.go` |
| `replicaCount` | HyperShell uses 1 replica (Deployment default) | N/A |

### cert-manager Installation

- **Docs:** <https://docs.nvidia.com/openshell/kubernetes/managing-certificates>

HyperShell test environments install cert-manager via `kubectl apply` (not Helm) for simplicity:

```bash
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.17.1}"
kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
```

The NVIDIA docs recommend cert-manager v1.20+. HyperShell test environments currently pin `v1.17.1` (the version available when this feature was implemented). The version is configurable via the `CERT_MANAGER_VERSION` environment variable.

### OpenShift-Specific Adjustments

- **Docs:** <https://docs.nvidia.com/openshell/kubernetes/openshift>

| NVIDIA doc instruction | HyperShell equivalent |
|---|---|
| `oc adm policy add-scc-to-user privileged -z openshell-gateway-sandbox -n <ns>` | `reconcileOpenShiftSCC()` creates a RoleBinding granting `system:openshift:scc:privileged` ClusterRole to the `openshell-gateway-sandbox` ServiceAccount |
| `--set podSecurityContext.fsGroup=null` | `applyOpenShiftOverrides()` clears `fsGroup` via `unstructured.RemoveNestedField()` |
| `--set securityContext.runAsUser=null` | `applyOpenShiftOverrides()` clears `runAsUser` via `unstructured.RemoveNestedField()` |
| `--set server.disableTls=true` | **NOT used** - BackendTLSPolicy re-encrypts to the pod |

The NVIDIA docs note that the OpenShift install path is experimental and recommends `server.disableTls=true` for evaluation. HyperShell diverges from this recommendation by keeping TLS enabled, because BackendTLSPolicy re-encrypts traffic from the networking Gateway to the pod, requiring the gateway to terminate TLS on the backend segment.

---

## References

- [OpenShell Gateway Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell) - upstream chart source; consult `values.yaml` when adding new gateway configurations
- [NVIDIA OpenShell on OpenShift](https://docs.nvidia.com/openshell/kubernetes/openshift) - OpenShift-specific deployment (SCC, security context, TLS)
- [NVIDIA OpenShell Managing Certificates](https://docs.nvidia.com/openshell/kubernetes/managing-certificates) - cert-manager integration for TLS certificate lifecycle
- [NVIDIA OpenShell Kubernetes Ingress Guide](https://docs.nvidia.com/openshell/kubernetes/ingress) - GRPCRoute and Gateway setup for OpenShell
- [OpenShell Helm Gateway Template](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/templates/gateway.yaml) - Reference Gateway resource
- [OpenShell Helm GRPCRoute Template](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/templates/grpcroute.yaml) - Reference GRPCRoute resource
- [BackendTLSPolicy on OpenShift](https://www.redhat.com/en/blog/backendtlspolicy-expands-gateway-api-transport-security) - Re-encrypt TLS via Gateway API
- [BackendTLSPolicy API Reference](https://gateway-api.sigs.k8s.io/reference/api-types/policy/backendtlspolicy/) - Spec structure and fields
- [Gateway API TLS Guide](https://gateway-api.sigs.k8s.io/guides/tls/) - TLS configuration patterns
- [OpenShell OIDC User Authentication](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-user-authentication) - OIDC integration
- [OpenShell OIDC Values Reference](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-values-reference) - OIDC helm values
- [OpenShell Helm Chart - `server.externalDbSecret`](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell) - External PostgreSQL integration
- [OpenShell Kubernetes Setup - External DB](https://docs.nvidia.com/openshell/latest/kubernetes/setup) - External DB documentation
- [cert-manager Helm Chart](https://artifacthub.io/packages/helm/cert-manager/cert-manager) - cert-manager installation via Helm (alternative to kubectl apply)
