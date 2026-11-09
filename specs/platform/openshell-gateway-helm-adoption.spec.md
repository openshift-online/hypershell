# OpenShell Gateway Helm Chart Adoption

**Date:** 2026-08-24
**Status:** Draft
**JIRA:** [HYPERSHELL-146](https://redhat.atlassian.net/browse/HYPERSHELL-146)
**Related:** `openshell-gateway.spec.md` - current provisioning; `control-plane.spec.md` - CP architecture
**Upstream:** [OpenShell Helm Chart](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell); [Helm CLI](https://helm.sh/docs/helm/); [PR #2728 (BackendTLSPolicy)](https://github.com/NVIDIA/OpenShell/pull/2728); [PR #2939 (namespace-scoped ClusterRole names)](https://github.com/NVIDIA/OpenShell/pull/2939)

---

## Purpose

The control plane SHALL deploy OpenShell gateways with the upstream OpenShell Helm chart, run at reconcile time through the Helm CLI. Using the upstream chart keeps HyperShell aligned with upstream and gives it new chart features without in-tree manifest maintenance.

The GatewayReconciler manages one Helm release per gateway:

1. When a gateway is created, it runs `helm install`.
2. When the gateway's desired spec changes, it runs `helm upgrade` with the newly computed values.
3. When the gateway is deleted, it runs `helm uninstall`.

The Helm release is the only way the control plane deploys the gateway workload.

---

## Architecture

### Reconciliation Flow

```
Gateway ADDED or spec-changed event (gRPC watch)
  │
  ▼
GatewayReconciler
  ├─ 1. Create namespace (if absent)
  ├─ 2. Provision the per-gateway database and role      ← Go
  ├─ 3. Reconcile Keycloak client                       ← Go
  ├─ 4. Copy trusted CA ConfigMap (if present)           ← Go
  ├─ 5. Reconcile OpenShift SCC binding                  ← Go (before Helm)
  ├─ 6. Helm install or upgrade                          ← Helm chart (see OSG-HELM-01.03)
  │     └─ Chart deploys: Deployment, Service, ConfigMap,
  │        ServiceAccounts, RBAC (ClusterRole/CRB scoped
  │        by namespace -- see Multi-Tenant section),
  │        certgen Job, credential KEK Secret,
  │        cert-manager Issuers/Certificates,
  │        GRPCRoute, BackendTLSPolicy, Route,
  │        BackendCA ConfigMap (via certgen)
  │        (NetworkPolicy disabled -- see decision below)
  └─ 7. Reconcile console                               ← Go
```

```
Gateway DELETED event (gRPC watch)
  │
  ▼
GatewayReconciler
  ├─ 1. Helm uninstall (if release exists)   ← first, so the gateway pod stops
  │                                            holding database connections
  ├─ 2. Clean up non-chart resources (ClusterRoleBinding, Keycloak clients,
  │     then the per-gateway database and role)
  └─ 3. Delete namespace
```

### Helm CLI Integration

```
GatewayReconciler
  │
  ├─ ShellClient (internal/helm package)
  │   ├─ Install()          -- shell out to `helm install` for new gateway
  │   ├─ Upgrade()          -- shell out to `helm upgrade` for spec changes and failed releases
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

### Requirement: OSG-HELM-01 -- Helm Integration

The control plane SHALL use the Helm CLI to manage gateway Helm releases programmatically. The `internal/helm` package shells out to the `helm` binary (avoiding Go dependency conflicts between the Helm SDK and the project's Kubernetes client packages).

#### Scenario: OSG-HELM-01.01 -- Helm client initialization

- GIVEN the control plane starts up
- WHEN the GatewayReconciler initializes
- THEN it SHALL verify the `helm` binary is available and is version 3.x
- AND it SHALL verify the chart archive exists at the configured path
- AND it SHALL create a `ShellClient` configured with the chart path and helm binary location
- AND the Helm storage driver SHALL be `secrets` (the Helm default) so release state is stored as Secrets in the gateway namespace

#### Scenario: OSG-HELM-01.02 -- New gateway provisioning (Helm install)

- GIVEN a Gateway ADDED event is received
- AND no Helm release exists in the gateway namespace
- WHEN the GatewayReconciler processes the event
- THEN it SHALL run `helm install` with the computed values
- AND the release name SHALL be `openshell-gateway`
- AND the release namespace SHALL be the gateway's API-assigned namespace
- AND `--create-namespace=false` SHALL be set (the reconciler creates and labels the namespace before the Helm install, applying labels `app.kubernetes.io/managed-by: hypershell-control-plane` and `hypershell.redhat.io/managed: "true"` that the upstream chart does not support)

#### Scenario: OSG-HELM-01.03 -- Existing release reconciled by its status

- GIVEN a Helm release already exists in the gateway namespace
- WHEN the GatewayReconciler provisions the gateway
- THEN it SHALL query the release status via `helm status`
- AND if the status is `deployed` or `failed`, it SHALL run `helm upgrade --reset-values` with the full computed values
- AND if the status starts with `pending-` (an install, upgrade or rollback that was interrupted, for example by a controller restart), it SHALL run `helm uninstall` and then `helm install`, because Helm rejects every new operation on a pending release
- AND for any other status, it SHALL run `helm upgrade --reset-values`

The reconciler provisions a gateway only while its desired spec is not converged (`observed_generation` is behind `generation`; see `control-plane.spec.md`, CTRL-CORE-13). A converged gateway is not upgraded again. A new chart or image default in the control plane therefore reaches an existing gateway only with its next spec change.

#### Scenario: OSG-HELM-01.05 -- Gateway spec change upgrades the release

- GIVEN a gateway whose release status is `deployed`
- WHEN its desired spec changes (for example a new `image`)
- THEN the GatewayReconciler SHALL run `helm upgrade --reset-values` with values computed from the new spec

#### Scenario: OSG-HELM-01.06 -- Failed Helm operation is retried

- GIVEN a Helm install, upgrade or reinstall fails (for example the chart times out waiting for cert-manager)
- WHEN the GatewayReconciler handles the error
- THEN it SHALL set the `GatewayDeployed` condition and the gateway `phase` to `Failed` (see `gateway-provisioning-progress.spec.md`, GTW-PROG-02)
- AND it SHALL return the error to the gateway reconcile queue
- AND the queue SHALL retry the reconcile with capped exponential backoff and no attempt limit, until it succeeds or the gateway is deleted
- AND the next attempt SHALL reconcile the release by its status (OSG-HELM-01.03)

#### Scenario: OSG-HELM-01.04 -- Gateway deletion (Helm uninstall + namespace deletion)

- GIVEN a Gateway DELETED event is received
- WHEN the GatewayReconciler processes the event
- THEN if a Helm release exists in the gateway namespace, it SHALL run `helm uninstall` first, so the gateway pod stops holding database connections before the database is dropped
- AND it SHALL then clean up non-chart resources (ClusterRoleBinding, Keycloak clients, and the per-gateway database and role)
- AND it SHALL delete the gateway namespace
- AND if the Helm uninstall fails, the reconciler proceeds to delete the gateway namespace (which removes all namespaced chart resources)
- AND if the per-gateway database cleanup fails, it SHALL record a `PostgreSQLDatabase` orphan and return an error so the delete is retried (see [`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md))

---

### Requirement: OSG-HELM-02 -- Chart Sourcing

The control plane SHALL load the upstream OpenShell Helm chart from a `.tgz` archive embedded in the container image. An OCI registry override is available for development.

#### Scenario: OSG-HELM-02.01 -- Embedded chart (default)

- GIVEN the chart archive is vendored into the control plane container image at `/charts/openshell.tgz`
- WHEN the reconciler starts up
- THEN it SHALL verify the chart exists at the embedded path and pass it to the Helm CLI for install operations
- AND the chart source repository and tag SHALL be declared in the top-level `OPENSHELL_VERSION` file (`OPENSHELL_CHART_REPO` and `OPENSHELL_TAG`) as provenance and re-vendor coordinates, not as build-time inputs
- AND the chart source SHALL be committed to the repository under `charts/openshell/` and vendored via `make vendor-openshell-chart` (which clones the upstream tag, replaces the directory, and strips forbidden characters); the Dockerfile COPYs this committed tree and runs `helm package` with no network access, satisfying Konflux hermetic build requirements
- AND upgrading the chart version SHALL require updating `OPENSHELL_VERSION`, running `make vendor-openshell-chart` to re-vendor the chart source, and rebuilding the control plane image
- AND this ensures the chart version is always coupled to the control plane release -- a given control plane image always deploys the same chart version

#### Scenario: OSG-HELM-02.02 -- OCI registry override (development only)

- GIVEN the environment variable `HELM_CHART_REGISTRY` is set (e.g. `oci://ghcr.io/nvidia/openshell/helm-chart`)
- WHEN the reconciler starts up
- THEN it SHALL pull from the OCI registry instead of the embedded path
- AND the chart version SHALL be read from `HELM_CHART_VERSION` (required when registry is set)
- AND the chart SHALL be loaded once at startup and cached for the lifetime of the process
- AND this mode SHALL NOT be used in production -- it introduces a runtime dependency on registry availability

---

### Requirement: OSG-HELM-03 -- Values Mapping

The GatewayReconciler SHALL translate Gateway resource fields and cluster-derived configuration into Helm chart `values.yaml` overrides.

#### Core Values

| Gateway / CP Config | Helm Value | Notes |
|---|---|---|
| `ReleaseName` constant (`openshell-gateway`) | `fullnameOverride` | Upstream chart name is `helm-chart`; override pins all resource names to `openshell-gateway` |
| Gateway `image` field | `gateway.image.{registry,repository,tag,digest}` | Parsed into parts; a configured digest is preserved. See OSG-HELM-05 |
| Gateway `supervisor_image` field | `supervisor.image.{registry,repository,tag,digest}` | Parsed into parts; a configured digest is preserved. See OSG-HELM-05 |
| Gateway `sandbox_runtime_image` (from `GATEWAY_SANDBOX_RUNTIME_IMAGE`) | `sandboxRuntime.image.{registry,repository,tag,digest}` | Parsed into parts; a configured digest is preserved. See OSG-HELM-05 |
| Always `deployment` | `workload.kind` | PostgreSQL backend, never StatefulSet |
| `1` | `replicaCount` | Single replica per gateway |
| `GATEWAY_RESOURCES` (else defaults) | `resources` | Default requests `cpu: 100m`, `memory: 512Mi`; limits `cpu: 500m`, `memory: 1Gi`. Always set: chart default is `{}` (no limits) |
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

#### Scenario: OSG-HELM-03.01 -- Values computation

- GIVEN a Gateway resource with image, OIDC config, route, and credential driver settings
- WHEN the GatewayReconciler computes Helm values
- THEN it SHALL produce a `map[string]interface{}` that maps all Gateway fields to chart values
- AND unset optional fields (e.g. no OIDC) SHALL be omitted from the values map
- AND the computed values SHALL be deterministic for the same Gateway state

---

### Requirement: OSG-HELM-05 -- Image Digest Preservation

The GatewayReconciler SHALL preserve an explicitly configured container image
digest when it builds Helm values for the gateway, supervisor and sandbox-runtime
images. A configured image digest identifies the exact content that was tested and
approved; the values builder SHALL NOT drop it, replace a digest reference with a
tag, or fall back to `latest` when a digest is present.

The values builder SHALL parse each image reference into its registry, repository,
optional tag and optional digest parts, handling a registry port (for example
`registry.example:5000/repo`) correctly, and SHALL set `gateway.image.digest`,
`supervisor.image.digest` and `sandboxRuntime.image.digest` whenever the
corresponding reference carries a digest. When a reference carries both a tag and a
digest, the rendered image SHALL use the digest. The chart gives
`image.digest` priority over `image.tag` (see the `openshell.image` helper in
`charts/openshell/templates/_helpers.tpl`); the values builder SHALL supply the
digest so this priority takes effect.

An image reference that is malformed, or that cannot be parsed into valid parts,
SHALL be reported as a validation error. The values builder SHALL NOT silently
drop or replace parts of an unparseable reference.

This requirement constrains only how an operator-selected image reference is carried
through to the rendered deployment. Fleet upgrade scheduling and release promotion
policy are out of scope.

#### Scenario: OSG-HELM-05.01 -- Tag-only reference keeps working

- GIVEN a configured image `quay.io/opendatahub/odh-openshell-gateway:v0.1.5` with no digest
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL set `gateway.image.registry` to `quay.io`, `gateway.image.repository` to `opendatahub/odh-openshell-gateway` and `gateway.image.tag` to `v0.1.5`
- AND it SHALL NOT set `gateway.image.digest`
- AND the rendered gateway image SHALL be `quay.io/opendatahub/odh-openshell-gateway:v0.1.5`

#### Scenario: OSG-HELM-05.02 -- Tag-plus-digest reference renders with the digest

- GIVEN a configured image `quay.io/opendatahub/odh-openshell-gateway:v0.1.5@sha256:abc...` with both a tag and a digest
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL set `gateway.image.digest` to `sha256:abc...`
- AND the rendered gateway image SHALL be `quay.io/opendatahub/odh-openshell-gateway@sha256:abc...`
- AND the rendered reference SHALL NOT use the tag `v0.1.5`

#### Scenario: OSG-HELM-05.03 -- Digest-only reference never falls back to a tag

- GIVEN a configured image `quay.io/opendatahub/odh-openshell-gateway@sha256:abc...` with a digest and no tag
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL set `gateway.image.digest` to `sha256:abc...`
- AND it SHALL NOT set `gateway.image.tag`
- AND the rendered gateway image SHALL be `quay.io/opendatahub/odh-openshell-gateway@sha256:abc...`
- AND the rendered reference SHALL NOT use `latest`, the global image tag, or the chart app version

#### Scenario: OSG-HELM-05.04 -- Registry port is parsed correctly

- GIVEN a configured image `registry.example:5000/openshell/gateway:v1@sha256:abc...`
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL set `gateway.image.registry` to `registry.example:5000`, `gateway.image.repository` to `openshell/gateway` and `gateway.image.digest` to `sha256:abc...`
- AND the rendered gateway image SHALL be `registry.example:5000/openshell/gateway@sha256:abc...`

#### Scenario: OSG-HELM-05.05 -- Digest preserved for supervisor and sandbox-runtime images

- GIVEN a configured supervisor image and sandbox-runtime image that each carry a digest
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL set `supervisor.image.digest` and `sandboxRuntime.image.digest` to the respective digests
- AND the rendered supervisor and sandbox-runtime image references SHALL use those digests
- AND any gateway-image consumer rendered by the chart (for example the certificate-generation job) SHALL use the gateway digest

#### Scenario: OSG-HELM-05.06 -- Malformed reference is a validation error

- GIVEN a configured image reference that cannot be parsed into valid registry, repository, tag and digest parts
- WHEN the GatewayReconciler builds Helm values
- THEN it SHALL return a validation error that names the invalid reference
- AND it SHALL NOT silently drop or replace any part of the reference

---

## Chart Coverage

### Resources Rendered by the Helm Chart

| Resource | Chart Template | Chart Value Controls |
|---|---|---|
| Deployment `openshell-gateway` | `deployment.yaml` | `workload.kind=deployment`, `image.*`, `resources.*` |
| ConfigMap `openshell-gateway-config` | `gateway-config.yaml` | `server.*` values render `gateway.toml` |
| Service `openshell-gateway` | `service.yaml` | `service.type`, `service.port` |
| ServiceAccount `openshell-gateway` | `serviceaccount.yaml` | `serviceAccount.create` |
| ServiceAccount `openshell-gateway-sandbox` | `serviceaccount.yaml` | `sandboxServiceAccount.create` |
| ClusterRole `...-node-reader-<ns>` | `clusterrole.yaml` | Always created; name scoped by `.Release.Namespace` (PR #2939) |
| ClusterRoleBinding `...-node-reader-<ns>` | `clusterrolebinding.yaml` | Always created; name scoped by `.Release.Namespace` (PR #2939) |
| Role `openshell-gateway-sandbox` | `role.yaml` | `workspaceMode=shared` |
| RoleBinding `openshell-gateway-sandbox` | `rolebinding.yaml` | `workspaceMode=shared` |
| Job `openshell-gateway-certgen` + RBAC | `certgen.yaml` | `pkiInitJob.enabled` |
| Secret `openshell-gateway-credential-kek` | `credential-storage-key-encryption-key-secret.yaml` | No credential driver enabled |
| Role/RoleBinding credential-secrets | `credential-secrets-role.yaml` / `credential-secrets-rolebinding.yaml` | `credentialDrivers.kubernetesSecrets.enabled` |
| Issuer `openshell-selfsigned` | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-ca` | `cert-manager-pki.yaml` | `certManager.enabled` |
| Issuer `openshell-ca-issuer` | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-server` | `cert-manager-pki.yaml` | `certManager.enabled` |
| Certificate `openshell-client` | `cert-manager-pki.yaml` | `certManager.enabled` |
| GRPCRoute `openshell-gateway` | `grpcroute.yaml` | `grpcRoute.enabled` |
| BackendTLSPolicy `openshell-gateway` | `backend-tls-policy.yaml` (PR #2728) | `grpcRoute.backendTLSPolicy.enabled` |
| BackendCA ConfigMap | Created by certgen hook (PR #2728) | `grpcRoute.backendTLSPolicy.enabled` |
| Route (OpenShift) | `route.yaml` | `openshiftRoute.enabled` |
| Trusted CA volume/mount/env | `_gateway-workload.tpl` | `server.oidc.caConfigMapName` |

### Gap Table: Control-Plane-Managed Resources

The Helm chart covers the OpenShell gateway deployment itself. The control plane must create or manage the following resources outside of the chart:

| # | Resource | Kind | Scope | Why the Control Plane Handles It |
|---|---|---|---|---|
| 1 | `openshell-sandbox-privileged-scc` | RoleBinding | Namespace | OpenShift SCC binding granting the `privileged` SCC to the sandbox ServiceAccount. The chart handles `podSecurityContext` values but has no concept of OpenShift SCC grants. This is documented as an out-of-chart step in the [upstream chart README](https://github.com/NVIDIA/OpenShell/blob/main/deploy/helm/openshell/README.md#install-on-openshift). |

The trusted CA ConfigMap (`gateway-trusted-ca`) is still copied from the CP namespace into the tenant namespace by the control plane, but its injection into the Deployment is handled by the chart via `server.oidc.caConfigMapName` -- no post-Helm SSA patch is needed. The control plane normalizes the ConfigMap key to `ca.crt` (the key the chart expects at the mount path) when copying, regardless of the source key name.

### Multi-Tenant: Namespace-Scoped ClusterRole Names

The upstream Helm chart appends `.Release.Namespace` to the ClusterRole and ClusterRoleBinding names ([PR #2939](https://github.com/NVIDIA/OpenShell/pull/2939)), producing per-release resources like `openshell-gateway-node-reader-<ns>`. Each Helm release owns its own cluster-scoped resources with no annotation conflicts, so multiple gateways on the same cluster install and uninstall independently. The rules are identical and small -- the duplication is harmless.

All other resources the control plane manages (per-gateway database provisioning, DB credentials, console, Keycloak clients) are HyperShell platform infrastructure deployed outside the context of the OpenShell gateway itself. The chart's `server.externalDbSecret` value references the DB credentials Secret that the control plane provisions before the Helm install, but the chart does not deploy databases.

### Gap: No Database CA Mount

The chart reads **only** the `uri` key of `server.externalDbSecret` and injects it as `OPENSHELL_DB_URL`. It offers no value for mounting an additional CA bundle into the gateway pod for the database connection: `server.oidc.caConfigMapName` and `server.credentialDrivers.vault.caConfigMapName` cover the OIDC issuer and the Vault driver respectively, and the chart has no generic `extraVolumes` / `extraVolumeMounts` / `extraEnv` escape hatch.

Consequently the gateway workload's database connection is capped at `sslmode=require` (encrypted, not certificate-verified). The control plane's own admin connection is unaffected: it reads its CA from the `hypershell-gateway-database-admin` Secret that the *controller* Deployment mounts, and stays `sslmode=verify-full`. See [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) for the full contract.

Closing this gap requires an upstream chart mechanism for a database CA volume (a `server.externalDbCaConfigMapName` value or equivalent). Until then, `verify-full` for the tenant connection SHALL NOT be claimed anywhere in the specs, code or e2e assertions.

---

## NetworkPolicy Decision: Do Not Install

The control plane SHALL NOT install any NetworkPolicies for gateway namespaces, and the Helm chart SHALL be configured with `networkPolicy.enabled=false`.

**Rationale:** On OVN-Kubernetes (the network plugin used on OpenShift), the default posture is allow-all. Pods within the same namespace -- and across namespaces -- can communicate freely. NetworkPolicies are additive deny: creating any NetworkPolicy that selects a pod with `policyTypes: [Ingress]` switches that pod to deny-by-default, requiring explicit allow rules for all legitimate traffic.

**Future consideration:** If a restrictive network policy posture becomes a platform requirement, we will revisit this decision and implement a comprehensive policy set that covers all legitimate traffic directions.

---

## Ordering Constraints

Resources have ordering dependencies that the reconciler must respect:

```
1. Namespace                          (must exist first)
2. Database + role + credentials Secret (before Helm install)
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
- On Helm failure, the `GatewayDeployed` condition and the gateway `phase` SHALL be set to `Failed`, with a user-facing message (OSG-HELM-01.06)
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

---

## Configuration

### Environment Variables

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
| Helm install failure (e.g. certgen timeout) | Gateway not provisioned | The reconcile queue retries with backoff; a `failed` release is retried with `helm upgrade --reset-values` and a `pending-*` release is reinstalled (OSG-HELM-01.03, OSG-HELM-01.06) |
| Chart upgrade changes resource names/labels | Orphaned resources, broken selectors | Pin chart version in container image; test upgrades in dev-cluster before production |
| PR #2728 not merged | BackendTLSPolicy and BackendCA ConfigMap gaps remain | Gate on upstream chart version that includes these features |
| Helm CLI binary missing or incompatible | Control plane cannot deploy gateways | Verify helm binary availability and version at startup; fail fast with clear error |
| No chart value for a database CA volume | Gateway database connection cannot be `verify-full`, only `require` | Accepted and documented (see Gap: No Database CA Mount); raise upstream for a `server.externalDbCaConfigMapName` equivalent |

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
