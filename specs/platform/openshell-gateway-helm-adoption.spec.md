# OpenShell Gateway Helm Chart Adoption

**Date:** 2026-08-24
**Status:** Draft
**JIRA:** [HYPERSHELL-146](https://redhat.atlassian.net/browse/HYPERSHELL-146)
**Related:** `openshell-gateway.spec.md` - current provisioning; `control-plane.spec.md` - CP architecture
**Upstream:** [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell); [Helm CLI](https://helm.sh/docs/helm/); [PR #2728 (BackendTLSPolicy)](https://github.com/NVIDIA/OpenShell/pull/2728); [PR #2939 (namespace-scoped ClusterRole names)](https://github.com/NVIDIA/OpenShell/pull/2939)

---

## Purpose

The control plane SHALL shift from applying static YAML manifests (generated once via `helm template` and maintained as embedded files) to installing OpenShell gateways using the upstream Helm chart at runtime via the Helm CLI. This eliminates drift between HyperShell and upstream, reduces maintenance burden, and gives automatic access to new chart features.

### Previous Approach

The GatewayReconciler loads static YAML files from `/manifests/gateway/` at startup, substitutes placeholders (`NAMESPACE_PLACEHOLDER`, `IMAGE_PLACEHOLDER`), and applies them with SSA. cert-manager resources, RBAC overlays, ingress routes, console, database, and credential resources are created programmatically in Go. The static manifests were originally generated from the upstream Helm chart, but are now maintained independently -- creating an ongoing drift risk.

### New Approach

The GatewayReconciler SHALL use the Helm CLI to run `helm install` when a gateway is created and `helm uninstall` when a gateway is deleted. Gateway upgrades (image changes, config changes) are not handled at this time. The old SSA-based deployment code will be removed entirely -- there is only one code path.

---

## Architecture

### Reconciliation Flow (Updated)

```
Gateway ADDED event (gRPC watch)
  │
  ▼
GatewayReconciler
  ├─ 1. Create namespace (if absent)
  ├─ 2. Reconcile CNPG database resources              ← Go (unchanged)
  ├─ 3. Reconcile Keycloak client                       ← Go (unchanged)
  ├─ 4. Copy trusted CA ConfigMap (if present)           ← Go (unchanged)
  ├─ 5. Reconcile OpenShift SCC binding                  ← Go (before Helm install)
  ├─ 6. Helm install                                     ← NEW (replaces deployGateway)
  │     └─ Chart deploys: Deployment, Service, ConfigMap,
  │        ServiceAccounts, RBAC (ClusterRole/CRB scoped
  │        by namespace -- see Multi-Tenant section),
  │        certgen Job, credential KEK Secret,
  │        cert-manager Issuers/Certificates,
  │        GRPCRoute, BackendTLSPolicy, Route,
  │        BackendCA ConfigMap (via certgen)
  │        (NetworkPolicy disabled -- see decision below)
  └─ 7. Reconcile console                               ← Go (unchanged)
```

```
Gateway DELETED event (gRPC watch)
  │
  ▼
GatewayReconciler
  ├─ 1. Clean up non-chart resources (database, Keycloak clients)
  ├─ 2. Helm uninstall (if release exists)
  └─ 3. Delete namespace
```

### Helm CLI Integration

```
GatewayReconciler
  │
  ├─ ShellClient (internal/helm package)
  │   ├─ Install()          -- shell out to `helm install` for new gateway
  │   ├─ Upgrade()          -- shell out to `helm upgrade` for retry
  │   ├─ Uninstall()        -- shell out to `helm uninstall` on deletion
  │   └─ GetReleaseStatus() -- shell out to `helm status` to query state
  │
  ├─ ValuesBuilder (internal/helm package)
  │   └─ Build()            -- map Gateway resource → chart values map
  │
  └─ Shells out to the `helm` CLI binary (avoids Go dependency
     conflicts between the Helm SDK and k8s client packages)
```

---

## Requirements

### Requirement: Helm Integration

The control plane SHALL use the Helm CLI to manage gateway Helm releases programmatically. The `internal/helm` package shells out to the `helm` binary (avoiding Go dependency conflicts between the Helm SDK and the project's Kubernetes client packages). This replaces the previous static manifest loading and SSA-based application.

#### Scenario: Helm client initialization

- GIVEN the control plane starts up
- WHEN the GatewayReconciler initializes
- THEN it SHALL verify the `helm` binary is available and is version 3.x
- AND it SHALL verify the chart archive exists at the configured path
- AND it SHALL create a `ShellClient` configured with the chart path and helm binary location
- AND the Helm storage driver SHALL be `secrets` (the Helm default) so release state is stored as Secrets in the gateway namespace

#### Scenario: New gateway provisioning (Helm install)

- GIVEN a Gateway ADDED event is received
- AND no Helm release exists in the gateway namespace
- WHEN the GatewayReconciler processes the event
- THEN it SHALL run `helm install` with the computed values
- AND the release name SHALL be `openshell-gateway`
- AND the release namespace SHALL be the gateway's API-assigned namespace
- AND `--create-namespace=false` SHALL be set (the reconciler creates and labels the namespace before the Helm install, applying labels `app.kubernetes.io/managed-by: hypershell-control-plane` and `hypershell.redhat.io/managed: "true"` that the upstream chart does not support)

#### Scenario: Retry after failed install

- GIVEN a previous Helm install failed (e.g. certgen job timed out waiting for cert-manager)
- WHEN the GatewayReconciler processes the next reconciliation event
- THEN it SHALL query Helm release status via `helm status`
- AND if release status is `failed` or `pending-install`, it SHALL run `helm upgrade --reset-values` with the full computed values to retry
- AND it SHALL implement exponential backoff (1m, 2m, 4m, 8m, max 15m between retries)
- AND it SHALL mark the Gateway status as `State: "Failed"` after 5 consecutive failures
- AND this is the only scenario where `helm upgrade` is invoked

#### Scenario: Gateway deletion (Helm uninstall + namespace deletion)

- GIVEN a Gateway DELETED event is received
- WHEN the GatewayReconciler processes the event
- THEN it SHALL clean up non-chart resources (database, Keycloak clients)
- AND if a Helm release exists in the gateway namespace, it SHALL run `helm uninstall`
- AND it SHALL delete the gateway namespace
- AND if the Helm uninstall fails, the reconciler proceeds to delete the gateway namespace (which removes all namespaced chart resources)

---

### Requirement: Chart Sourcing

The control plane SHALL load the upstream OpenShell Helm chart from a `.tgz` archive embedded in the container image. An OCI registry override is available for development.

#### Scenario: Embedded chart (default)

- GIVEN the chart archive is vendored into the control plane container image at `/charts/openshell.tgz`
- WHEN the reconciler starts up
- THEN it SHALL verify the chart exists at the embedded path and pass it to the Helm CLI for install operations
- AND the chart source repository SHALL be declared in `charts/CHART_REPO` and the Git ref in `charts/CHART_REF`
- AND the Dockerfile SHALL clone the chart source at the specified ref, package it with `helm package`, and embed the resulting `.tgz` archive
- AND upgrading the chart version SHALL require updating `charts/CHART_REF` (and optionally `charts/CHART_REPO`) and rebuilding the control plane image
- AND this ensures the chart version is always coupled to the control plane release -- a given control plane image always deploys the same chart version

#### Scenario: OCI registry override (development only)

- GIVEN the environment variable `HELM_CHART_REGISTRY` is set (e.g. `oci://ghcr.io/nvidia/openshell/helm-chart`)
- WHEN the reconciler starts up
- THEN it SHALL pull from the OCI registry instead of the embedded path
- AND the chart version SHALL be read from `HELM_CHART_VERSION` (required when registry is set)
- AND the chart SHALL be loaded once at startup and cached for the lifetime of the process
- AND this mode SHALL NOT be used in production -- it introduces a runtime dependency on registry availability

---

### Requirement: Values Mapping

The GatewayReconciler SHALL translate Gateway resource fields and cluster-derived configuration into Helm chart `values.yaml` overrides. This mapping replaces the current manifest placeholder substitution and Go-based config patching.

#### Core Values

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| `ReleaseName` constant (`openshell-gateway`) | `fullnameOverride` | Upstream chart name is `helm-chart`; override pins all resource names to `openshell-gateway` |
| Gateway `image` field | `image.repository`, `image.tag` | Split at last `:` |
| Gateway `supervisor_image` field | `supervisorImage.repository`, `supervisorImage.tag` | Split at last `:` |
| Always `deployment` | `workload.kind` | PostgreSQL backend, never StatefulSet |
| `1` | `replicaCount` | Single replica per gateway |
| Gateway namespace | `server.sandboxNamespace` | Sandboxes run in gateway NS |
| Gateway `serverDnsNames` | `pkiInitJob.serverDnsNames` | TLS certificate SANs |
| `true` | `serviceAccount.create` | Chart creates gateway SA |
| `true` | `sandboxServiceAccount.create` | Chart creates sandbox SA |
| `false` | `networkPolicy.enabled` | Disabled -- see NetworkPolicy Decision below |
| cert-manager detected | `certManager.enabled` | Auto-detect at startup |
| DB credentials Secret name | `server.externalDbSecret` | `openshell-gateway-db-credentials` |
| Trusted CA ConfigMap name | `server.oidc.caConfigMapName` | `gateway-trusted-ca` (when present) |

#### OIDC Values (conditional)

Set only when the Gateway resource includes OIDC configuration.

| Gateway OIDC Config | Helm Value |
|---|---|
| `oidc.issuer` | `server.oidc.issuer` |
| `oidc.audience` | `server.oidc.audience` |
| `oidc.roles_claim` | `server.oidc.rolesClaim` |
| `oidc.admin_role` | `server.oidc.adminRole` |
| `oidc.user_role` | `server.oidc.userRole` |
| `oidc.scopes_claim` | `server.oidc.scopesClaim` |

#### Credential Driver Values (conditional)

Set based on the Gateway's credential driver configuration.

| CP Config | Helm Value |
|---|---|
| KEK mode (default) | `credentialDrivers.kubernetesSecrets.enabled=false` |
| `kubernetes-secrets` driver | `credentialDrivers.kubernetesSecrets.enabled=true` |
| Vault driver | `credentialDrivers.vault.enabled=true`, `credentialDrivers.vault.*` |

#### Ingress Values (conditional)

Set only when the Gateway has route configuration enabled.

The control plane supports two mutually exclusive ingress modes depending on cluster capabilities. The TLS architecture differs fundamentally between them.

##### GRPCRoute + BackendTLSPolicy Mode (Gateway API -- OpenShift 4.22+)

```
Client ──TLS──▶ Shared Gateway (terminates with wildcard cert)
                   ──re-encrypt──▶ Gateway Pod (internal self-signed cert)
                   BackendTLSPolicy validates pod cert against internal CA
```

The shared Gateway terminates external TLS with its admin-provisioned wildcard certificate. BackendTLSPolicy re-encrypts to the pod and validates the pod's cert against the internal CA. External clients never see the pod's certificate, so the chart's default self-signed CA chain is sufficient.

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| HyperShell Gateway has `route` config + Kubernetes Gateway API available | `grpcRoute.enabled=true` | Enable GRPCRoute creation |
| Route hostname | `grpcRoute.hostnames` | `[gw-<ns>.<base-domain>]` where `<base-domain>` is from `GATEWAY_API_BASE_DOMAIN` (e.g. `gw-openshell-abc123.openshell.example.com`) |
| Kubernetes Gateway API Gateway reference | `grpcRoute.gateway.name`, `grpcRoute.gateway.namespace` | Cross-namespace parentRef to the shared Kubernetes Gateway API `Gateway` resource (e.g. `name=openshell-gateway`, `namespace=openshift-ingress`) |
| BackendTLSPolicy support (PR #2728) | `grpcRoute.backendTLSPolicy.enabled=true` | Requires PR #2728 |
| mTLS toggle (PR #2728) | `server.tls.enableMtls=false` | Required when BackendTLSPolicy is used |

##### Route Passthrough Mode (OpenShift < 4.22)

```
Client ──TLS──▶ HAProxy Route (SNI passthrough, no termination)
                   ──encrypted──▶ Gateway Pod (client sees this cert directly)
```

HAProxy does NOT terminate TLS -- it forwards the encrypted connection end-to-end based on SNI. The client sees the gateway pod's certificate directly. This means the pod's certificate must:

1. **Include the external hostname as a SAN** (e.g. `gw-openshell-abc123.apps.example.com`)
2. **Be signed by an externally trusted CA** (e.g. Let's Encrypt via an ACME ClusterIssuer) -- a self-signed internal CA will cause TLS verification failures for CLI clients

The upstream Helm chart supports this via `certManager.serverIssuerRef`: when set, the chart creates a **second** Certificate resource (`openshell-gateway-server-external`) signed by the operator-provided issuer, with only externally-resolvable SANs from `certManager.serverDnsNames`. The internal cert (signed by the chart's self-signed CA) still exists for supervisor/sandbox traffic. The gateway uses SNI to present the correct certificate based on the incoming hostname.

**Cluster prerequisite for Route mode:** An administrator must pre-provision a ClusterIssuer (or namespaced Issuer) for externally trusted certificates (e.g. ACME/Let's Encrypt, Vault PKI, or an organization CA). The control plane references this issuer by name -- it does not create it.

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| HyperShell Gateway has `route` config + no Kubernetes Gateway API | `openshiftRoute.enabled=true` | Fallback to `route.openshift.io/v1` Route |
| Route hostname | `openshiftRoute.host` | `gw-<ns>.<base-domain>` |
| HAProxy timeout | `openshiftRoute.annotations` | `{"haproxy.router.openshift.io/timeout": "3600s"}` |
| External CA issuer name | `certManager.serverIssuerRef.name` | Pre-provisioned ClusterIssuer (e.g. `letsencrypt-prod`) |
| External CA issuer kind | `certManager.serverIssuerRef.kind` | `ClusterIssuer` (or `Issuer` for namespace-scoped) |
| Route hostname (as cert SAN) | `certManager.serverDnsNames` | `[gw-<ns>.<base-domain>]` -- must be externally resolvable |

The chart validates this configuration at install time:
- Fails if `openshiftRoute.enabled=true` and `server.disableTls=true` (passthrough requires the pod to terminate TLS)
- Fails if `serverIssuerRef.name` is set but `serverDnsNames` is empty
- Fails if `serverDnsNames` contains internal-only names (e.g. `localhost`, `*.svc.cluster.local`) when using an external issuer -- ACME CAs reject these per CA/Browser Forum baseline requirements
- Fails if `openshiftRoute.host` is set with `serverIssuerRef` but the host is not covered by `serverDnsNames`

#### OpenShift Values (conditional)

Set only when the target cluster is detected as OpenShift.

| Platform Detection | Helm Value | Notes |
|---|---|---|
| OpenShift detected | `podSecurityContext.fsGroup=null` | Let SCC assign |
| OpenShift detected | `securityContext.runAsUser=null` | Let SCC assign |

#### Scenario: Values computation

- GIVEN a Gateway resource with image, OIDC config, route, and credential driver settings
- WHEN the GatewayReconciler computes Helm values
- THEN it SHALL produce a `map[string]interface{}` that maps all Gateway fields to chart values
- AND unset optional fields (e.g. no OIDC) SHALL be omitted from the values map
- AND the computed values SHALL be deterministic for the same Gateway state

---

### Requirement: Migration Path

The platform is in beta. The migration path is intentionally simple.

- New gateway creations SHALL use the Helm chart
- Existing gateways are not migrated -- they will be reprovisioned when needed
- The old SSA-based deployment code SHALL be removed entirely; there is no dual-mode or feature flag

---

## Coverage Gap Analysis

### Resources Covered by the Helm Chart

| Resource | Current Source | Chart Template | Chart Value Controls |
|---|---|---|---|
| Deployment `openshell-gateway` | `manifests/gateway/deployment.yaml` | `deployment.yaml` | `workload.kind=deployment`, `image.*`, `resources.*` |
| ConfigMap `openshell-gateway-config` | `manifests/gateway/configmap.yaml` | `gateway-config.yaml` | `server.*` values render `gateway.toml` |
| Service `openshell-gateway` | `manifests/gateway/service.yaml` | `service.yaml` | `service.type`, `service.port` |
| ServiceAccount `openshell-gateway` | `manifests/gateway/serviceaccount.yaml` | `serviceaccount.yaml` | `serviceAccount.create` |
| ServiceAccount `openshell-gateway-sandbox` | `manifests/gateway/serviceaccount.yaml` | `serviceaccount.yaml` | `sandboxServiceAccount.create` |
| ClusterRole `...-node-reader-<ns>` | `manifests/gateway/rbac.yaml` | `clusterrole.yaml` | Always created; name scoped by `.Release.Namespace` (PR #2939) |
| ClusterRoleBinding `...-node-reader-<ns>` | `manifests/gateway/rbac.yaml` | `clusterrolebinding.yaml` | Always created; name scoped by `.Release.Namespace` (PR #2939) |
| Role `openshell-gateway-sandbox` | `manifests/gateway/rbac.yaml` | `role.yaml` | `workspaceMode=shared` |
| RoleBinding `openshell-gateway-sandbox` | `manifests/gateway/rbac.yaml` | `rolebinding.yaml` | `workspaceMode=shared` |
| Job `openshell-gateway-certgen` + RBAC | `manifests/gateway/certgen-job.yaml` | `certgen.yaml` | `pkiInitJob.enabled` |
| Secret `openshell-gateway-credential-kek` | Go (`reconcileCredentialKEK`) | `credential-storage-key-encryption-key-secret.yaml` | No credential driver enabled |
| Role/RoleBinding credential-secrets | Go (`reconcileCredentialDriverRBAC`) | `credential-secrets-role.yaml` / `credential-secrets-rolebinding.yaml` | `credentialDrivers.kubernetesSecrets.enabled` |
| Issuer `openshell-selfsigned` | Go (`reconcileCertManager`) | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-ca` | Go (`reconcileCertManager`) | `cert-manager-pki.yaml` | `certManager.enabled` |
| Issuer `openshell-ca-issuer` | Go (`reconcileCertManager`) | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-server` | Go (`reconcileCertManager`) | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-client` | Go (`reconcileCertManager`) | `cert-manager-pki.yaml` | `certManager.enabled` |
| GRPCRoute `openshell-gateway` | Go (`reconcileGRPCRoute`) | `grpcroute.yaml` | `grpcRoute.enabled` |
| BackendTLSPolicy `openshell-gateway` | Go (`reconcileBackendTLSPolicy`) | `backend-tls-policy.yaml` (PR #2728) | `grpcRoute.backendTLSPolicy.enabled` |
| BackendCA ConfigMap | Go (`reconcileBackendCA`) | Created by certgen hook (PR #2728) | `grpcRoute.backendTLSPolicy.enabled` |
| Route (OpenShift) | Go (`reconcileRoute`) | `route.yaml` | `openshiftRoute.enabled` |
| Trusted CA volume/mount/env | Go (post-Helm SSA patch) | `_gateway-workload.tpl` | `server.oidc.caConfigMapName` |

### Gap Table: Control-Plane-Managed Resources

The Helm chart covers the OpenShell gateway deployment itself. The control plane must create or manage the following resources outside of the chart:

| # | Resource | Kind | Scope | Why the Control Plane Handles It |
|---|---|---|---|---|
| 1 | `openshell-sandbox-privileged-scc` | RoleBinding | Namespace | OpenShift SCC binding granting the `privileged` SCC to the sandbox ServiceAccount. The chart handles `podSecurityContext` values but has no concept of OpenShift SCC grants. This is documented as an out-of-chart step in the [upstream chart README](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/README.md#install-on-openshift). |

The trusted CA ConfigMap (`gateway-trusted-ca`) is still copied from the CP namespace into the tenant namespace by the control plane, but its injection into the Deployment is handled by the chart via `server.oidc.caConfigMapName` -- no post-Helm SSA patch is needed. The control plane normalizes the ConfigMap key to `ca.crt` (the key the chart expects at the mount path) when copying, regardless of the source key name.

### Multi-Tenant: Namespace-Scoped ClusterRole Names

The upstream Helm chart appends `.Release.Namespace` to the ClusterRole and ClusterRoleBinding names ([PR #2939](https://github.com/NVIDIA/OpenShell/pull/2939)), producing per-release resources like `openshell-gateway-node-reader-<ns>`. Each Helm release owns its own cluster-scoped resources with no annotation conflicts, so multiple gateways on the same cluster install and uninstall independently. The rules are identical and small -- the duplication is harmless.

All other resources the control plane manages (CNPG database provisioning, DB credentials, console, Keycloak clients) are HyperShell platform infrastructure deployed outside the context of the OpenShell gateway itself. The chart's `server.externalDbSecret` value references the DB credentials Secret that the control plane provisions before the Helm install, but the chart does not deploy databases.

---

## NetworkPolicy Decision: Do Not Install

The control plane SHALL NOT install any NetworkPolicies for gateway namespaces, and the Helm chart SHALL be configured with `networkPolicy.enabled=false`.

**Rationale:** On OVN-Kubernetes (the network plugin used on OpenShift), the default posture is allow-all. Pods within the same namespace -- and across namespaces -- can communicate freely. NetworkPolicies are additive deny: creating any NetworkPolicy that selects a pod with `policyTypes: [Ingress]` switches that pod to deny-by-default, requiring explicit allow rules for all legitimate traffic.

In practice, the existing gateway NetworkPolicies created a self-defeating cycle:

1. `allow-router` selected the gateway pod and permitted cross-namespace ingress from `openshift-ingress` -- but this isolated the gateway pod for all other ingress
2. `allow-sandbox-v2` was then required to restore sandbox→gateway gRPC connectivity (same namespace, already open without policies)
3. `sandbox-ssh-v2` was required to restore gateway→sandbox SSH connectivity
4. `allow-console` was required to restore console→gateway connectivity

**Verified empirically:** On namespace `openshell-3eaf1474f52f561c`, removing all 7 NetworkPolicies had no impact on end-to-end connectivity (local machine → OpenShift ingress → gateway → sandbox exec). Conversely, installing only `allow-router` broke sandbox connectivity by isolating the gateway pod.

**Future consideration:** If a restrictive network policy posture becomes a platform requirement, we will revisit this decision and implement a comprehensive policy set that covers all legitimate traffic directions.

---

## Ordering Constraints

Resources have ordering dependencies that the reconciler must respect:

```
1. Namespace                          (must exist first)
2. CNPG Database + credentials Secret (must exist before Helm install)
3. Trusted CA ConfigMap copy          (must exist before Helm install if present)
4. OpenShift SCC binding              (must run before Helm install)
5. Helm install                       (creates core workload + chart-managed resources)
6. Console resources                  (can run after Helm)
7. Keycloak clients                   (can run after Helm)
```

The key constraints:
- The DB credentials Secret (`openshell-gateway-db-credentials`) must be created before the Helm install because the chart's Deployment references it via `server.externalDbSecret`. The reconciler ensures this by resolving and provisioning database resources (step 2) before invoking Helm (step 5). If for any reason the Secret is missing at install time, Kubernetes will eventually start the pod once the Secret appears, but the Helm install may report a timeout.
- The OpenShift SCC binding must be created before the Helm install so that sandbox pods can schedule when the chart creates the Deployment.

---

## Implementation Architecture

### Error Handling (HyperShell Convention)

The reconciler SHALL follow HyperShell error handling conventions for all Helm operations:

- Helm install/upgrade/uninstall errors SHALL be wrapped with context: `fmt.Errorf("helm install gateway %s: %w", gateway.Name, err)`
- Gateway status SHALL be updated on Helm failure with `State: "Failed"` and error message
- Reconciler SHALL return explicit errors -- NEVER `panic()` on Helm failures
- Partial failures (e.g. Helm install succeeded but console reconciliation failed) SHALL be collected and returned as a multi-error
- All error paths SHALL propagate to the caller or be logged with full context

### Helm Package (`internal/helm/`)

The control plane SHALL provide a dedicated package for Helm operations with these capabilities:

| Capability | Description |
|---|---|
| Release management | Install, upgrade, uninstall, and query release status via the Helm CLI |
| Values translation | Map Gateway resource fields to Helm chart values (`map[string]interface{}`) |
| Chart verification | Verify chart archive exists at the embedded path; pull from OCI registry in dev mode |

### Responsibilities Transferred to Chart

The following responsibilities SHALL be delegated to the upstream Helm chart and removed from in-tree control plane code:

| Responsibility | Chart Mechanism |
|---|---|
| Static YAML manifests (Deployment, Service, ConfigMap, ServiceAccount, RBAC) | Chart templates |
| cert-manager Issuers and Certificates | `certManager.enabled` value |
| Credential KEK Secret and driver RBAC | Credential driver values |
| NetworkPolicies | Disabled (`networkPolicy.enabled=false`) |
| Trusted CA volume/mount/env injection | `server.oidc.caConfigMapName` value |
| GRPCRoute, BackendTLSPolicy, Route | Ingress values |
| BackendCA ConfigMap | Chart certgen hook |

---

## Configuration

### New Environment Variables

| Variable | Default | Description |
|---|---|---|
| `HELM_CHART_PATH` | `/charts/openshell.tgz` | Path to embedded chart archive in the container |
| `HELM_CHART_REGISTRY` | *(empty)* | OCI registry URL (dev only); when set, overrides `HELM_CHART_PATH` |
| `HELM_CHART_VERSION` | *(empty)* | Chart version to pull from OCI registry; required when `HELM_CHART_REGISTRY` is set |
| `EXTERNAL_CA_ISSUER_NAME` | *(empty)* | cert-manager issuer name for externally trusted certificates (e.g. `letsencrypt-prod`); required for Route passthrough mode |
| `EXTERNAL_CA_ISSUER_KIND` | `ClusterIssuer` | Kind of the external CA issuer (`ClusterIssuer` or `Issuer`) |

### Validation at Startup

The control plane SHALL validate configuration at startup and fail fast with clear error messages:

- Control plane SHALL validate `EXTERNAL_CA_ISSUER_NAME` is set when any managed cluster lacks Gateway API support (Route passthrough mode will be used)
- Control plane SHALL fail fast with error message: `"EXTERNAL_CA_ISSUER_NAME required for Route passthrough mode on cluster <name>"` if validation fails
- When `HELM_CHART_REGISTRY` is set, `HELM_CHART_VERSION` SHALL be required (fail if missing)
- Chart version compatibility SHALL be verified if pulling from OCI registry (log warning if chart version is not tested)

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Helm install failure (e.g. certgen timeout) | Gateway not provisioned | Detect failed release on next reconcile and retry via `helm upgrade --reset-values` |
| Chart upgrade changes resource names/labels | Orphaned resources, broken selectors | Pin chart version in container image; test upgrades in dev-cluster before production |
| PR #2728 not merged | BackendTLSPolicy and BackendCA ConfigMap gaps remain | Gate on upstream chart version that includes these features |
| Helm CLI binary missing or incompatible | Control plane cannot deploy gateways | Verify helm binary availability and version at startup; fail fast with clear error |
| Namespace-scoped ClusterRole names | Upgrading from fixed-name chart leaves orphaned old ClusterRole | Old `openshell-gateway-node-reader` can be cleaned up manually; new per-namespace resources are independent |

---

## References

- [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [OpenShell Helm Chart README -- Install on OpenShift](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/README.md#install-on-openshift) -- documents SCC binding as out-of-chart step
- [Helm CLI Documentation](https://helm.sh/docs/helm/)
- [PR #2728: BackendTLSPolicy support](https://github.com/NVIDIA/OpenShell/pull/2728)
- [PR #2939: namespace-scoped ClusterRole names](https://github.com/NVIDIA/OpenShell/pull/2939)
- [Current gateway spec](./openshell-gateway.spec.md)
- [Gateway routing spec](./openshell-gateway-routing.spec.md)
- [Gateway database spec](./openshell-gateway-database.spec.md)
- [Gateway console spec](./openshell-gateway-console.spec.md)
- [Gateway credentials spec](./openshell-gateway-credentials.spec.md)
