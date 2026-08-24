# OpenShell Gateway Helm Chart Adoption

**Date:** 2026-08-24
**Status:** Draft
**JIRA:** [HYPERSHELL-146](https://redhat.atlassian.net/browse/HYPERSHELL-146)
**Related:** `openshell-gateway.spec.md` - current provisioning; `control-plane.spec.md` - CP architecture
**Upstream:** [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell); [Helm Go SDK](https://helm.sh/docs/sdk/); [PR #2728 (BackendTLSPolicy)](https://github.com/NVIDIA/OpenShell/pull/2728)

---

## Purpose

The control plane SHALL shift from applying static YAML manifests (generated once via `helm template` and maintained as embedded files) to installing OpenShell gateways using the upstream Helm chart at runtime via the Helm Go SDK. This eliminates drift between HyperShell and upstream, reduces maintenance burden, and gives automatic access to new chart features.

### Current State

The GatewayReconciler loads static YAML files from `/manifests/gateway/` at startup, substitutes placeholders (`NAMESPACE_PLACEHOLDER`, `IMAGE_PLACEHOLDER`), and applies them with SSA. cert-manager resources, RBAC overlays, ingress routes, console, database, and credential resources are created programmatically in Go. The static manifests were originally generated from the upstream Helm chart, but are now maintained independently — creating an ongoing drift risk.

### Target State

The GatewayReconciler SHALL use the Helm Go SDK to run `helm install` when a gateway is created and `helm uninstall` when a gateway is deleted. Gateway upgrades (image changes, config changes) are not handled at this time. The old SSA-based deployment code will be removed entirely — there is only one code path.

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
  │        ServiceAccounts, RBAC, certgen Job,
  │        credential KEK Secret,
  │        cert-manager Issuers/Certificates,
  │        GRPCRoute, BackendTLSPolicy, Route,
  │        BackendCA ConfigMap (via certgen)
  │        (NetworkPolicy disabled — see decision below)
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

### Helm SDK Integration

```
GatewayReconciler
  │
  ├─ HelmClient (new internal package)
  │   ├─ LoadChart()   — load chart once at startup
  │   ├─ BuildValues() — map Gateway resource → chart values
  │   ├─ Install()     — install release for new gateway
  │   └─ Uninstall()   — clean removal on gateway deletion
  │
  └─ Uses: helm.sh/helm/v3/pkg/action (Install, Uninstall)
           helm.sh/helm/v3/pkg/chart/loader
           helm.sh/helm/v3/pkg/cli
```

---

## Requirements

### Requirement: Helm Go SDK Integration

The control plane SHALL use the [Helm Go SDK](https://helm.sh/docs/sdk/) (`helm.sh/helm/v3`) to manage gateway Helm releases programmatically. The SDK replaces the current static manifest loading and SSA-based application.

#### Scenario: Helm client initialization

- GIVEN the control plane starts up
- WHEN the GatewayReconciler initializes
- THEN it SHALL create a Helm action configuration targeting each managed cluster's kubeconfig
- AND it SHALL load the chart once at startup and reuse it for all subsequent installs
- AND the Helm storage driver SHALL be `secrets` (the Helm default) so release state is stored as Secrets in the gateway namespace

#### Scenario: New gateway provisioning (Helm install)

- GIVEN a Gateway ADDED event is received
- AND no Helm release exists in the gateway namespace
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `action.Install` with the computed values
- AND the release name SHALL be `openshell-gateway`
- AND the release namespace SHALL be the gateway's API-assigned namespace
- AND `Install.CreateNamespace` SHALL be `false` (the reconciler creates the namespace itself)

#### Scenario: Retry after failed install

- GIVEN a previous Helm install failed (e.g. certgen job timed out waiting for cert-manager)
- WHEN the GatewayReconciler processes the next reconciliation event
- THEN it SHALL detect the failed release and run `helm upgrade` to retry
- AND this is the only scenario where `helm upgrade` is invoked

#### Scenario: Gateway deletion (Helm uninstall + namespace deletion)

- GIVEN a Gateway DELETED event is received
- WHEN the GatewayReconciler processes the event
- THEN it SHALL clean up non-chart resources (database, Keycloak clients)
- AND if a Helm release exists in the gateway namespace, it SHALL call `action.Uninstall`
- AND it SHALL delete the gateway namespace

---

### Requirement: Chart Sourcing

The control plane SHALL load the upstream OpenShell Helm chart from a `.tgz` archive embedded in the container image. An OCI registry override is available for development.

#### Scenario: Embedded chart (default)

- GIVEN the chart archive is vendored into the control plane container image at `/charts/openshell.tgz`
- WHEN the reconciler starts up
- THEN it SHALL use `loader.LoadArchive()` to load the chart once
- AND the loaded chart SHALL be reused for all gateway installs without reloading
- AND the chart version SHALL be pinned in the Dockerfile build script (e.g. `helm pull oci://ghcr.io/nvidia/openshell/helm-chart --version <version> --destination /charts/`)
- AND upgrading the chart version SHALL require a control plane image rebuild
- AND this ensures the chart version is always coupled to the control plane release — a given control plane image always deploys the same chart version

#### Scenario: OCI registry override (development only)

- GIVEN the environment variable `HELM_CHART_REGISTRY` is set (e.g. `oci://ghcr.io/nvidia/openshell/helm-chart`)
- WHEN the reconciler starts up
- THEN it SHALL pull from the OCI registry instead of the embedded path
- AND the chart version SHALL be read from `HELM_CHART_VERSION` (required when registry is set)
- AND the chart SHALL be loaded once at startup and cached for the lifetime of the process
- AND this mode SHALL NOT be used in production — it introduces a runtime dependency on registry availability

---

### Requirement: Values Mapping

The GatewayReconciler SHALL translate Gateway resource fields and cluster-derived configuration into Helm chart `values.yaml` overrides. This mapping replaces the current manifest placeholder substitution and Go-based config patching.

#### Core Values

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| Gateway `image` field | `image.repository`, `image.tag` | Split at last `:` |
| Gateway `supervisor_image` field | `supervisorImage.repository`, `supervisorImage.tag` | Split at last `:` |
| Always `deployment` | `workload.kind` | PostgreSQL backend, never StatefulSet |
| `1` | `replicaCount` | Single replica per gateway |
| Gateway namespace | `server.sandboxNamespace` | Sandboxes run in gateway NS |
| Gateway `serverDnsNames` | `pkiInitJob.serverDnsNames` | TLS certificate SANs |
| `true` | `serviceAccount.create` | Chart creates gateway SA |
| `true` | `sandboxServiceAccount.create` | Chart creates sandbox SA |
| `false` | `networkPolicy.enabled` | Disabled — see NetworkPolicy Decision below |
| cert-manager detected | `certManager.enabled` | Auto-detect at startup |
| DB credentials Secret name | `server.externalDbSecret` | `openshell-gateway-db-credentials` |
| Trusted CA ConfigMap name | `server.oidc.caConfigMapName` | `gateway-trusted-ca` (when present) |

#### OIDC Values (conditional)

| Gateway OIDC Config | Helm Value |
|---|---|
| `oidc.issuer` | `server.oidc.issuer` |
| `oidc.audience` | `server.oidc.audience` |
| `oidc.roles_claim` | `server.oidc.rolesClaim` |
| `oidc.admin_role` | `server.oidc.adminRole` |
| `oidc.user_role` | `server.oidc.userRole` |
| `oidc.scopes_claim` | `server.oidc.scopesClaim` |

#### Credential Driver Values (conditional)

| CP Config | Helm Value |
|---|---|
| KEK mode (default) | `credentialDrivers.kubernetesSecrets.enabled=false` |
| `kubernetes-secrets` driver | `credentialDrivers.kubernetesSecrets.enabled=true` |
| Vault driver | `credentialDrivers.vault.enabled=true`, `credentialDrivers.vault.*` |

#### Ingress Values (conditional)

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| Gateway has `route` config + Gateway API available | `grpcRoute.enabled=true` | Enable GRPCRoute creation |
| Route hostname | `grpcRoute.hostnames` | `[gw-<ns>.<base-domain>]` |
| Gateway API Gateway ref | `grpcRoute.gateway.name`, `grpcRoute.gateway.namespace` | Cross-namespace parentRef |
| BackendTLSPolicy support (PR #2728) | `grpcRoute.backendTLSPolicy.enabled=true` | Requires PR #2728 |
| mTLS toggle (PR #2728) | `server.tls.enableMtls=false` | Required when BackendTLSPolicy is used |
| Gateway has `route` config + no Gateway API (OpenShift < 4.22) | `openshiftRoute.enabled=true` | Fallback to `route.openshift.io/v1` Route |

#### OpenShift Values (conditional)

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
- Existing gateways are not migrated — they will be reprovisioned when needed
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
| ClusterRole `openshell-gateway-node-reader` | `manifests/gateway/rbac.yaml` | `clusterrole.yaml` | Always created |
| ClusterRoleBinding `...-node-reader-<ns>` | `manifests/gateway/rbac.yaml` | `clusterrolebinding.yaml` | Always created |
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

### Gap Table: Control-Plane-Managed Resources Within the Gateway Namespace

The Helm chart covers the OpenShell gateway deployment itself. The only resource the control plane must create within the gateway namespace outside of the chart is the OpenShift SCC binding:

| # | Resource | Kind | Why the Control Plane Handles It |
|---|---|---|---|
| 1 | `openshell-sandbox-privileged-scc` | RoleBinding | OpenShift SCC binding granting the `privileged` SCC to the sandbox ServiceAccount. The chart handles `podSecurityContext` values but has no concept of OpenShift SCC grants. This is documented as an out-of-chart step in the [upstream chart README](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/README.md#install-on-openshift). |

The trusted CA ConfigMap (`gateway-trusted-ca`) is still copied from the CP namespace into the tenant namespace by the control plane, but its injection into the Deployment is handled by the chart via `server.oidc.caConfigMapName` — no post-Helm SSA patch is needed.

All other resources the control plane manages (CNPG database provisioning, DB credentials, console, Keycloak clients) are HyperShell platform infrastructure deployed outside the context of the OpenShell gateway itself. The chart's `server.externalDbSecret` value references the DB credentials Secret that the control plane provisions before the Helm install, but the chart does not deploy databases.

---

## NetworkPolicy Decision: Do Not Install

The control plane SHALL NOT install any NetworkPolicies for gateway namespaces, and the Helm chart SHALL be configured with `networkPolicy.enabled=false`.

**Rationale:** On OVN-Kubernetes (the network plugin used on OpenShift), the default posture is allow-all. Pods within the same namespace — and across namespaces — can communicate freely. NetworkPolicies are additive deny: creating any NetworkPolicy that selects a pod with `policyTypes: [Ingress]` switches that pod to deny-by-default, requiring explicit allow rules for all legitimate traffic.

In practice, the existing gateway NetworkPolicies created a self-defeating cycle:

1. `allow-router` selected the gateway pod and permitted cross-namespace ingress from `openshift-ingress` — but this isolated the gateway pod for all other ingress
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
- The DB credentials Secret (`openshell-gateway-db-credentials`) must be created before the Helm install because the chart's Deployment references it via `server.externalDbSecret`. If the Secret does not exist at install time, the pod will fail to start with a missing Secret error.
- The OpenShift SCC binding must be created before the Helm install so that sandbox pods can schedule when the chart creates the Deployment.

---

## Code Changes

### New Package: `internal/helm/`

| File | Purpose |
|---|---|
| `client.go` | Helm action configuration, release state queries |
| `values.go` | Gateway resource → Helm values map translation |
| `chart.go` | Chart loading (embedded filesystem or OCI pull, loaded once at startup) |

### Modified Files

| File | Change |
|---|---|
| `internal/reconciler/gateway_reconciler.go` | Replace `deployGateway()` call with `helmClient.Install()`; remove manifest loading; keep pre/post-Helm steps; add namespace deletion on gateway delete |
| `internal/reconciler/gateway_reconciler.go` | Remove inline cert-manager, credential KEK, credential RBAC reconciliation (chart handles) |
| `go.mod` | Add `helm.sh/helm/v3` dependency |
| `Dockerfile` / build | Embed chart archive |

### Removed

| Item | Reason |
|---|---|
| `/manifests/gateway/*.yaml` (embedded static manifests) | Replaced by Helm chart |
| `internal/gateway/manifests.go` (manifest loading) | No longer needed |
| `reconcileCertManager()` logic | Chart handles cert-manager resources |
| `reconcileCredentialKEK()` logic | Chart handles credential KEK Secret |
| `reconcileCredentialDriverRBAC()` logic | Chart handles credential driver RBAC |
| All NetworkPolicy reconciliation code | No longer installed (see NetworkPolicy Decision) |
| `manifests/gateway/networkpolicy.yaml` | No longer installed |
| Trusted CA SSA patch logic | Chart handles via `server.oidc.caConfigMapName` |
| GRPCRoute/BackendTLSPolicy/Route Go reconciliation | Chart handles via values |
| BackendCA ConfigMap Go reconciliation | Chart certgen hook handles |

---

## Configuration

### New Environment Variables

| Variable | Default | Description |
|---|---|---|
| `HELM_CHART_PATH` | `/charts/openshell.tgz` | Path to embedded chart archive in the container |
| `HELM_CHART_REGISTRY` | *(empty)* | OCI registry URL (dev only); when set, overrides `HELM_CHART_PATH` |
| `HELM_CHART_VERSION` | *(empty)* | Chart version to pull from OCI registry; required when `HELM_CHART_REGISTRY` is set |

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Helm install failure (e.g. certgen timeout) | Gateway not provisioned | Detect failed release on next reconcile and retry via `helm upgrade` |
| Chart upgrade changes resource names/labels | Orphaned resources, broken selectors | Pin chart version in container image; test upgrades in dev-cluster before production |
| PR #2728 not merged | BackendTLSPolicy and BackendCA ConfigMap gaps remain | Keep Go-based reconciliation as fallback; gate on chart version |
| Helm SDK version drift | API incompatibilities | Pin `helm.sh/helm/v3` in `go.mod`; track upstream releases |

---

## References

- [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [OpenShell Helm Chart README — Install on OpenShift](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/README.md#install-on-openshift) — documents SCC binding as out-of-chart step
- [Helm Go SDK Documentation](https://helm.sh/docs/sdk/)
- [PR #2728: BackendTLSPolicy support](https://github.com/NVIDIA/OpenShell/pull/2728)
- [Current gateway spec](./openshell-gateway.spec.md)
- [Gateway routing spec](./openshell-gateway-routing.spec.md)
- [Gateway database spec](./openshell-gateway-database.spec.md)
- [Gateway console spec](./openshell-gateway-console.spec.md)
- [Gateway credentials spec](./openshell-gateway-credentials.spec.md)
