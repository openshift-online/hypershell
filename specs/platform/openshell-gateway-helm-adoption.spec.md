# OpenShell Gateway Helm Chart Adoption

**Date:** 2026-08-24
**Status:** Draft
**JIRA:** [HYPERSHELL-146](https://redhat.atlassian.net/browse/HYPERSHELL-146)
**Related:** `openshell-gateway.spec.md` - current provisioning; `control-plane.spec.md` - CP architecture
**Upstream:** [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell); [Helm Go SDK](https://helm.sh/docs/sdk/); [PR #2728 (BackendTLSPolicy)](https://github.com/NVIDIA/OpenShell/pull/2728)

---

## Purpose

The control plane SHALL shift from applying static YAML manifests (generated once via `helm template` and maintained as embedded files) to installing and upgrading OpenShell gateways using the upstream Helm chart at runtime via the Helm Go SDK. This eliminates drift between HyperShell and upstream, reduces maintenance burden, and gives automatic access to new chart features.

### Current State

The GatewayReconciler loads static YAML files from `/manifests/gateway/` at startup, substitutes placeholders (`NAMESPACE_PLACEHOLDER`, `IMAGE_PLACEHOLDER`), and applies them with SSA. cert-manager resources, RBAC overlays, ingress routes, console, database, and credential resources are created programmatically in Go. The static manifests were originally generated from the upstream Helm chart, but are now maintained independently — creating an ongoing drift risk.

### Target State

The GatewayReconciler SHALL use the Helm Go SDK to install or upgrade a Helm release per gateway namespace, passing computed values that map gateway configuration to upstream chart values. Resources not covered by the chart (database provisioning, console, trusted CA, OpenShift SCC, extra network policies, ingress) remain under direct Go reconciliation.

---

## Architecture

### Reconciliation Flow (Updated)

```
Gateway event (gRPC watch)
  │
  ▼
GatewayReconciler
  ├─ 1. Create namespace (if absent)
  ├─ 2. Reconcile CNPG database resources              ← Go (unchanged)
  ├─ 3. Reconcile credential KEK / driver RBAC          ← Helm chart handles
  ├─ 4. Reconcile cert-manager resources                 ← Helm chart handles
  ├─ 5. Reconcile Keycloak client                        ← Go (unchanged)
  ├─ 6. Copy trusted CA ConfigMap                        ← Go (unchanged)
  ├─ 7. Install/Upgrade Helm release                     ← NEW (replaces deployGateway)
  │     └─ Chart deploys: Deployment, Service, ConfigMap,
  │        ServiceAccounts, RBAC, certgen Job,
  │        credential KEK Secret,
  │        cert-manager Issuers/Certificates,
  │        GRPCRoute, BackendTLSPolicy (PR #2728)
  │        (NetworkPolicy disabled — see decision below)
  ├─ 8. Reconcile OpenShift SCC binding                  ← Go (unchanged)
  ├─ 9. Reconcile ingress (BackendCA ConfigMap)          ← Go (if not covered by chart)
  └─ 10. Reconcile console                               ← Go (unchanged)
```

### Helm SDK Integration

```
GatewayReconciler
  │
  ├─ HelmClient (new internal package)
  │   ├─ LoadChart()        — load chart from embedded or OCI registry
  │   ├─ BuildValues()      — map Gateway resource → chart values
  │   ├─ InstallOrUpgrade() — idempotent release management
  │   └─ Uninstall()        — clean removal on gateway deletion
  │
  └─ Uses: helm.sh/helm/v3/pkg/action (Install, Upgrade, Uninstall)
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
- AND the Helm storage driver SHALL be `secrets` (the Helm default) so release state is stored as Secrets in the gateway namespace

#### Scenario: New gateway provisioning (Helm install)

- GIVEN a Gateway ADDED event is received
- AND no Helm release exists in the gateway namespace
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `action.Install` with the computed values
- AND the release name SHALL be `openshell-gateway`
- AND the release namespace SHALL be the gateway's API-assigned namespace
- AND `Install.CreateNamespace` SHALL be `false` (the reconciler creates the namespace itself)

#### Scenario: Gateway update (Helm upgrade)

- GIVEN a Gateway MODIFIED event is received
- AND a Helm release already exists in the gateway namespace
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `action.Upgrade` with the updated values
- AND the upgrade SHALL be atomic (`Upgrade.Atomic = true`) to auto-rollback on failure
- AND the upgrade SHALL wait for resources to become ready (`Upgrade.Wait = true`)

#### Scenario: Gateway deletion (Helm uninstall)

- GIVEN a Gateway DELETED event is received
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `action.Uninstall` to remove chart-managed resources
- AND it SHALL separately clean up non-chart resources (database, console, SCC binding, extra network policies, Keycloak clients)
- AND it SHALL NOT delete the namespace (consistent with current behavior)

#### Scenario: Idempotent reconciliation

- GIVEN a Gateway reconciliation is triggered (event or periodic resync)
- WHEN the current Helm release values match the computed values
- THEN the SDK upgrade SHALL be a no-op (Helm detects no diff)
- AND unnecessary Kubernetes API calls SHALL be avoided

---

### Requirement: Chart Sourcing

The control plane SHALL load the upstream OpenShell Helm chart from a `.tgz` archive embedded in the container image. An OCI registry override is available for development.

#### Scenario: Embedded chart (default)

- GIVEN the chart archive is vendored into the control plane container image at `/charts/openshell.tgz`
- WHEN the reconciler loads the chart
- THEN it SHALL use `loader.LoadArchive()` to load from the embedded path
- AND the chart version SHALL be pinned in the Dockerfile build script (e.g. `helm pull oci://ghcr.io/nvidia/openshell/helm-chart --version <version> --destination /charts/`)
- AND upgrading the chart version SHALL require a control plane image rebuild
- AND this ensures the chart version is always coupled to the control plane release — a given control plane image always deploys the same chart version

#### Scenario: OCI registry override (development only)

- GIVEN the environment variable `HELM_CHART_REGISTRY` is set (e.g. `oci://ghcr.io/nvidia/openshell/helm-chart`)
- WHEN the reconciler loads the chart
- THEN it SHALL pull from the OCI registry instead of the embedded path
- AND the chart version SHALL be read from `HELM_CHART_VERSION` (required when registry is set)
- AND the chart SHALL be cached locally after the first pull
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
| Gateway has `route` config | `grpcRoute.enabled=true` | Enable GRPCRoute creation |
| Route hostname | `grpcRoute.hostname` | `gw-<ns>.<base-domain>` |
| Gateway API Gateway ref | `grpcRoute.parentRefs[0]` | Cross-namespace parentRef |
| BackendTLSPolicy support (PR #2728) | `grpcRoute.backendTLSPolicy.enabled=true` | Requires PR #2728 |
| mTLS toggle (PR #2728) | `server.tls.enableMtls=false` | Required when BackendTLSPolicy is used |

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

#### Scenario: Sandbox preflight disabled

- GIVEN the control plane manages sandbox CRD installation separately
- WHEN computing Helm values
- THEN `agentSandbox.preflight.enabled` SHALL be set to `false`
- AND the chart SHALL not fail if the sandbox CRD API is not yet served

---

### Requirement: Migration Path

Existing gateway deployments use directly-applied resources (SSA). Transitioning to Helm-managed releases requires adopting existing resources into the Helm release without downtime.

#### Scenario: Adopt existing resources into Helm release

- GIVEN a gateway namespace has resources created by the current SSA-based reconciler
- WHEN the control plane is upgraded to use Helm
- THEN the first Helm install SHALL use `Install.Replace = true` to adopt existing resources
- AND the reconciler SHALL annotate existing resources with `meta.helm.sh/release-name` and `meta.helm.sh/release-namespace` and label them with `app.kubernetes.io/managed-by=Helm` before the first install
- AND no resources SHALL be deleted or recreated during migration
- AND the gateway pod SHALL NOT be restarted unless the Deployment spec actually changes

#### Scenario: Rollback capability

- GIVEN a Helm-managed gateway release
- WHEN an upgrade fails
- THEN the `Atomic` flag SHALL cause automatic rollback to the previous release revision
- AND the reconciler SHALL log the failure and retry on the next reconciliation cycle

#### Scenario: Mixed-state during rolling upgrade

- GIVEN a fleet of gateways across multiple clusters
- WHEN the control plane is upgraded incrementally
- THEN gateways on clusters not yet upgraded SHALL continue to use SSA-based reconciliation
- AND gateways on upgraded clusters SHALL use Helm-based reconciliation
- AND both modes SHALL produce functionally identical resource sets

---

## Coverage Gap Analysis

The following table documents resources that the control plane currently creates but are NOT covered by the upstream Helm chart. These resources SHALL continue to be reconciled directly in Go after the Helm release is installed.

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
| ~~NetworkPolicy `openshell-gateway-sandbox-ssh`~~ | `manifests/gateway/networkpolicy.yaml` | `networkpolicy.yaml` | `networkPolicy.enabled=false` (disabled) |
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
| Route (OpenShift) | Go (`reconcileRoute`) | `route.yaml` | `openshiftRoute.enabled` |

### Gap Table: Control-Plane-Managed Resources Within the Gateway Namespace

The Helm chart covers the OpenShell gateway deployment itself. The following resources are deployed by the control plane into the gateway namespace because they are HyperShell platform concerns outside of OpenShell's scope:

| # | Resource | Kind | Why the Control Plane Handles It |
|---|---|---|---|
| 1 | `openshell-sandbox-privileged-scc` | RoleBinding | OpenShift SCC binding granting the `privileged` SCC to the sandbox ServiceAccount. The chart handles `podSecurityContext` values but has no concept of OpenShift SCC grants. |
| 2 | `gateway-trusted-ca` | ConfigMap | CA bundle for private-CA environments (e.g. Keycloak behind OpenShift ingress). Copied from the CP namespace and mounted into the gateway Deployment via a post-Helm SSA patch. |

All other resources the control plane manages (CNPG database provisioning, DB credentials, console, Keycloak clients, backend CA ConfigMap) are HyperShell platform infrastructure deployed outside the context of the OpenShell gateway itself. The chart's `server.externalDbSecret` value references the DB credentials Secret that the control plane provisions before the Helm install, but the chart does not deploy databases.

---

## Trusted CA Injection Strategy

The trusted CA ConfigMap is not managed by the Helm chart. After the Helm release is installed, the reconciler must overlay the Deployment to add the CA volume, volume mount, and `SSL_CERT_FILE` env var.

#### Scenario: Post-Helm trusted CA overlay

- GIVEN the Helm release has been installed/upgraded
- AND a `gateway-trusted-ca` ConfigMap exists in the CP namespace
- WHEN the reconciler applies trusted CA injection
- THEN it SHALL patch the `openshell-gateway` Deployment using SSA to add:
  - A volume referencing the `gateway-trusted-ca` ConfigMap
  - A volumeMount at `/etc/pki/tls/certs/ca-bundle.crt` (subPath `ca-bundle.crt`)
  - An env var `SSL_CERT_FILE=/etc/pki/tls/certs/ca-bundle.crt`
- AND the SSA field manager SHALL be `hypershell-control-plane` (distinct from `helm`) to avoid conflict with Helm's field ownership

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

**Existing NetworkPolicies will be removed** from gateway namespaces during migration to Helm. The control plane will stop creating them, and any existing ones will not be adopted into Helm releases.

---

## Ordering Constraints

Resources have ordering dependencies that the reconciler must respect:

```
1. Namespace                          (must exist first)
2. CNPG Database + credentials Secret (must exist before Helm install)
3. Trusted CA ConfigMap copy          (must exist before Helm install if present)
4. Helm install/upgrade               (creates core workload + chart-managed resources)
5. OpenShift SCC binding              (can run after Helm, before pod scheduling)
6. Trusted CA Deployment overlay      (must run after Helm, patches the Deployment)
7. Console resources                  (can run after Helm)
8. Keycloak clients                   (can run after Helm)
```

The key constraint: the DB credentials Secret (`openshell-gateway-db-credentials`) must be created before the Helm install because the chart's Deployment references it via `server.externalDbSecret`. If the Secret does not exist at install time, the pod will fail to start with a missing Secret error.

---

## Code Changes

### New Package: `internal/helm/`

| File | Purpose |
|---|---|
| `client.go` | Helm action configuration, release state queries |
| `values.go` | Gateway resource → Helm values map translation |
| `chart.go` | Chart loading (embedded filesystem or OCI pull) |
| `migration.go` | One-time adoption of existing SSA-managed resources |

### Modified Files

| File | Change |
|---|---|
| `internal/reconciler/gateway_reconciler.go` | Replace `deployGateway()` call with `helmClient.InstallOrUpgrade()`; remove manifest loading; keep pre/post-Helm steps |
| `internal/gateway/manifests.go` | Remove or deprecate — static manifest loading no longer needed |
| `internal/reconciler/gateway_reconciler.go` | Remove inline cert-manager, credential KEK, credential RBAC reconciliation (chart handles) |
| `go.mod` | Add `helm.sh/helm/v3` dependency |
| `Dockerfile` / build | Embed chart archive or configure OCI pull |

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
| Helm release state corruption | Gateway stuck in failed state | Use `Atomic` flag for auto-rollback; add release health check to reconcile loop |
| Chart upgrade changes resource names/labels | Orphaned resources, broken selectors | Pin chart version; test upgrades in dev-cluster before production |
| Field ownership conflict (Helm vs SSA patches) | Unexpected field reversion on upgrade | Use distinct field managers; limit SSA patches to fields Helm does not manage |
| PR #2728 not merged | BackendTLSPolicy gap remains | Keep Go-based BackendTLSPolicy reconciliation as fallback; gate on chart version |
| Helm SDK version drift | API incompatibilities | Pin `helm.sh/helm/v3` in `go.mod`; track upstream releases |

---

## Implementation Phases

### Phase 1: Helm SDK Integration + Values Mapping

- Add `helm.sh/helm/v3` dependency
- Implement `internal/helm/` package (client, values, chart loader)
- Unit test values mapping against snapshot of current manifests
- Embed chart in container image build

### Phase 2: Migration + Dual-Mode

- Implement resource adoption (annotate existing resources for Helm)
- Add feature flag `HELM_DEPLOY_ENABLED` (default `false`)
- Run both modes in dev-cluster; compare resource output
- Validate no-downtime migration on existing gateways

### Phase 3: Cut Over

- Enable Helm deployment by default (`HELM_DEPLOY_ENABLED=true`)
- Remove static manifest files and loading code
- Remove Go-based cert-manager, credential KEK, credential RBAC reconciliation
- Remove all NetworkPolicy creation code and delete existing NetworkPolicies from gateway namespaces
- Update `openshell-gateway.spec.md` to reflect new architecture

---

## References

- [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [Helm Go SDK Documentation](https://helm.sh/docs/sdk/)
- [PR #2728: BackendTLSPolicy support](https://github.com/NVIDIA/OpenShell/pull/2728)
- [Current gateway spec](./openshell-gateway.spec.md)
- [Gateway routing spec](./openshell-gateway-routing.spec.md)
- [Gateway database spec](./openshell-gateway-database.spec.md)
- [Gateway console spec](./openshell-gateway-console.spec.md)
- [Gateway credentials spec](./openshell-gateway-credentials.spec.md)
