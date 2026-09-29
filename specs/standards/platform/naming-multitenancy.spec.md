# HyperShell Naming & Multi-Tenancy Standard

**Related operational document:** The [DNS strategy](https://github.com/openshift-online/hypershell-gitops/blob/main/dns-strategy.md)
defines the public DNS hierarchy, delegation, and name inventory.

## Abstract

HyperShell employs a hub-and-spoke GitOps architecture in which multiple control
plane instances (e.g., `hyp0`, `hyp1`, `stage`, `uat`) may be deployed
onto a single shared OpenShift cluster. Because Kustomize's global `namePrefix`
and `nameSuffix` transformers rewrite strings without understanding resources
created outside the build (e.g., externally synced database Secrets) or
cluster-scope boundaries, this standard defines the explicit naming rules that prevent
cross-instance stomping and broken volume mounts.

The rules are normative (Sections 1-5). Section 6 is a visual reference that must
stay consistent with them.

## 1. Namespaced Resources (Deployments, Services, Secrets, PVCs)

**Rule: Use native namespace isolation, no prefixes or suffixes.**

Namespaced resources MUST NOT be mutated with `namePrefix` or `nameSuffix` in the
GitOps overlay. They rely entirely on the Kubernetes `Namespace` for isolation, so
`hyp0/hypershell-db-app` and `hyp1/hypershell-db-app` coexist without collision.

* **API Server:** `hypershell-api-server`
* **Controller:** `hypershell-controller`
* **Web Console:** `hypershell-web-console`
* **Platform Database Secret:** `hypershell-db-app` (connection details for the
  externally provisioned platform PostgreSQL database)

**Why:** Upstream deployments hardcode volume mounts and environment variables
(e.g., `secretName: hypershell-db-app`). The Secret is created outside the
Kustomize build (by the installer or an external secret sync), so a Kustomize
prefix would rewrite the reference to `hyp0-hypershell-db-app` while the Secret
keeps its original name, producing `CreateContainerConfigError` or `FailedMount`.
The name is constant; the namespace provides isolation.

## 2. Cluster-Scoped Resources (ClusterRoles, ClusterRoleBindings)

**Rule: Explicit per-instance prefixing required.**

Cluster-scoped resources are global to the OpenShift cluster. If `hyp0` and `hyp1`
both deploy a `ClusterRoleBinding` named `hypershell-controller`, the last instance
to sync via ArgoCD overwrites the binding, stealing permissions from the other
instance. These resources MUST be uniquely named per instance by prefixing the
instance namespace:

* **Controller Role:** `<instance-namespace>-hypershell-controller` (e.g., `hyp0-hypershell-controller`)
* **Controller Binding:** `<instance-namespace>-hypershell-controller`
* **SCC Bindings:** `<instance-namespace>-hypershell-controller-scc`, `<instance-namespace>-hypershell-controller-scc-bind`, `<instance-namespace>-hypershell-sandbox-scc`

**Implementation:** GitOps overlays MUST use Kustomize `replacements` that inject
the instance `Namespace` name as a prefix (`delimiter: '-'`, `index: 0`) into the
`metadata.name` of every `ClusterRole` and `ClusterRoleBinding`, and into every
`ClusterRoleBinding`'s `roleRef.name`. A `name-references.yaml` `nameReference`
config makes each `ClusterRole` rename propagate to the `roleRef.name` fields that
reference it. The prefix MUST be scoped to cluster-scoped kinds only; never to
namespaced resources (§1). Combined with the subject rewrite in §3, each instance
keeps its own binding and cannot have its permissions stolen by another instance.

## 3. RBAC Subject Alignment

**Rule: Dynamic subject namespaces.**

When a `ClusterRoleBinding` is deployed for an instance, its `subjects[0].namespace`
MUST point to that instance's namespace:

```yaml
subjects:
- kind: ServiceAccount
  name: hypershell-controller
  namespace: hyp0  # <-- MUST match the deployed instance
```

**Implementation:** GitOps overlays MUST use Kustomize `replacements` to map the
overlay's `Namespace` name into `subjects[0].namespace` of the ClusterRoleBinding.
If it is left as the base default (`hypershell-system`), the controller
ServiceAccount in the instance namespace is not actually bound, so the controller
runs with zero permissions and fails to provision gateways.

## 4. Dynamically Provisioned Resources (Gateways, Databases, Sandboxes)

**Rule: Controller-generated hashes; databases live outside the cluster.**

Resources the control plane provisions at runtime follow programmatic naming to
prevent tenant collisions across the cluster. The API server assigns the gateway
namespace name in its `BeforeCreate` hook; the control plane mirrors the prefix in
`components/control-plane/internal/gateway/namespace.go`.

* **Tenant (gateway) namespace:** `openshell-<hash>`, 16 hex chars
  (e.g., `openshell-bdd12bb523f166db`). Prefix `openshell-`.
* **Per-gateway database:** a PostgreSQL database and login role, both named
  `gw_<gateway-id>`, created by the control plane on the externally provisioned
  gateway database server whose admin credentials are mounted into the controller
  (`hypershell-gateway-database-admin`). No Kubernetes resource represents it, and
  one server backs every gateway of the installation.
* **Database credentials in the tenant namespace:** only the
  `openshell-gateway-db-credentials` Secret is written into `openshell-<hash>`; the
  database itself never runs in the cluster.
* **Gateway workload:** `openshell-gateway` (Deployment, ServiceAccount) in the
  tenant namespace; NetworkPolicies `openshell-gateway-*`.

## 5. Shared Platform Services

**Rule: Singleton deployments.**

Services that manage cluster-wide operators or external integrations that do not
require per-instance control-plane isolation are deployed once per cluster:

* **Keycloak / SSO:** `keycloak-system`. A single Keycloak instance serves all
  `hyp*` environments via distinct Realms or Clients. Its database is an
  externally provisioned PostgreSQL database.

## 6. Visual Reference

### 6.1 Topology Graph

```text
[ Shared OpenShift Cluster ]
 │
 ├── Shared Platform Singletons (one per cluster - §5)
 │   └── Namespace: keycloak-system
 │       └── Deployment: keycloak  (database: external PostgreSQL)
 │
 ├── Cluster-Scoped RBAC (prefixed per instance - §2/§3)
 │   ├── ClusterRole:        hyp0-hypershell-controller
 │   ├── ClusterRoleBinding: hyp0-hypershell-controller
 │   │   └── subjects[0].namespace ── binds to ──┐
 │   ├── ClusterRole:        hyp1-hypershell-controller
 │   └── ClusterRoleBinding: hyp1-hypershell-controller
 │       └── subjects[0].namespace ── binds to ──────┐
 │                                                    │
 ├── Control Plane Instance 0 (native isolation - §1) │
 │   └── Namespace: hyp0  <──────────────────────────┘ (from hyp0 binding)
 │       ├── ServiceAccount: hypershell-controller
 │       ├── Secret:         hypershell-db-app  (external platform DB connection)
 │       │                                  ▲ (hardcoded volumeMount)
 │       ├── Secret:         hypershell-gateway-database-admin  (gateway DB server admin + CA)
 │       │                                  ▲ (volumeMount /etc/hypershell/gateway-database)
 │       ├── Deployment: hypershell-api-server ──┤
 │       ├── Deployment: hypershell-controller ──┴──┘
 │       └── Deployment: hypershell-web-console
 │
 ├── Control Plane Instance 1
 │   └── Namespace: hyp1
 │       ├── ServiceAccount: hypershell-controller
 │       ├── Secret:         hypershell-db-app
 │       └── Deployment:     hypershell-api-server, hypershell-controller, ...
 │
 └── Dynamic Tenant Gateways (created by the controller at runtime - §4)
     ├── Namespace: openshell-bdd12bb523f166db (Tenant A)
     │   ├── Deployment:     openshell-gateway
     │   ├── ServiceAccount: openshell-gateway
     │   ├── Secret:         openshell-gateway-db-credentials  (points at gw_<id> on the external server)
     │   └── NetworkPolicies: openshell-gateway-*
     │
     └── Namespace: openshell-c22557dbb511cdcb (Tenant B)
         └── ...
```

### 6.2 Resource Naming Table

| Name | Kind | Scope | Description |
| :--- | :--- | :--- | :--- |
| `keycloak-system` | Namespace | Shared Platform | Singleton namespace shared across all instances. |
| `keycloak` | Deployment | Shared Platform | Singleton deployment inside the shared namespace. |
| `<instance>-hypershell-controller` | ClusterRole | Cluster-Scoped RBAC | Prefixed to prevent multi-tenant overwrites (e.g., `hyp0-`). |
| `<instance>-hypershell-controller` | ClusterRoleBinding | Cluster-Scoped RBAC | Links the ClusterRole to the instance's ServiceAccount; `subjects[0].namespace` = instance namespace. |
| `<instance>-hypershell-controller-scc`, `<instance>-hypershell-controller-scc-bind`, `<instance>-hypershell-sandbox-scc` | SCC Bindings | Cluster-Scoped RBAC | OpenShift SCC access for the instance's controller and sandbox SAs. |
| `<instance>` (e.g., `hyp0`, `hyp1`) | Namespace | Control Plane Instance | Isolates the control plane instance. |
| `hypershell-controller` | ServiceAccount | Control Plane Instance | Runs the controller; subject of the prefixed ClusterRoleBinding. |
| `hypershell-db-app` | Secret | Control Plane Instance | Connection details for the externally provisioned platform database; hardcoded in volume mounts. **No suffix**; must not be renamed (§1). |
| `hypershell-gateway-database-admin` | Secret | Control Plane Instance | Administrative connection to the gateway database server: `host`/`port`/`user`/`password`/`sslrootcert` (+ optional `dbname`, `sslmode=verify-full`). Mounted read-only into `hypershell-controller` at `/etc/hypershell/gateway-database`; the control plane reads it from the filesystem and never looks a database Secret up by name through the API. The name is deployment configuration and MAY be changed in an overlay together with the volume. Not created, modified or deleted by HyperShell. See [`openshell-gateway-database.spec.md`](../../platform/openshell-gateway-database.spec.md). |
| `hypershell-api-server`, `hypershell-controller`, `hypershell-web-console` | Deployment | Control Plane Instance | Core components; no suffix (§1). |
| `gw_<gateway-id>` | PostgreSQL database and role | Gateway database server | Per-gateway database and login role on the external server named by `hypershell-gateway-database-admin` (§4). |
| `openshell-<hash>` | Namespace | Tenant Gateway | Gateway workload namespace; generated by the controller at runtime (§4). |
| `openshell-gateway` | Deployment, ServiceAccount | Tenant Gateway | The gateway application and its identity. |
| `openshell-gateway-db-credentials` | Secret | Tenant Gateway | DB credentials (`uri` with `sslmode=require`, no CA - the Helm chart has no CA-mount path for this connection) written into the tenant namespace; the DB itself lives on the external server. |
| `openshell-gateway-*` | NetworkPolicy | Tenant Gateway | Tenant-specific network isolation policies. |
