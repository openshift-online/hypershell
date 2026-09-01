# OpenShell Gateway Quota Specification

**Date:** 2026-08-27
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `data-model.spec.md` - GatewayProfile entity definition

---

## Purpose

This specification defines resource quota enforcement for OpenShell gateway namespaces. The control plane applies Kubernetes ResourceQuota and LimitRange objects to gateway namespaces based on GatewayProfile definitions, preventing runaway resource consumption and enforcing tenant-level resource limits.

---

## Architecture

### Resource Flow

```
GatewayProfile (API resource)
    │  Defines quota limits: CPU, memory, storage, pod/PVC counts, container defaults
    ▼
ManagedCluster.profile_id (default profile for cluster)
    │  Cluster administrator assigns a default profile
    ▼
Gateway.profile_id (client-supplied or cluster default)
    │  API server uses client-supplied value, or falls back to cluster's profile_id
    ▼
Control Plane - GatewayReconciler.fetchQuotaConfig()
    │  Fetches GatewayProfile via gRPC
    │  Translates to QuotaConfig struct
    ▼
Control Plane - ReconcileNamespaceQuota()
    │  Creates/updates ResourceQuota (namespace-level limits)
    │  Creates/updates LimitRange (container-level defaults/max)
    ▼
Kubernetes - Admission Control
    │  Enforces quota on pod creation
    │  Injects default requests, rejects over-limit containers
```

### Profile Assignment Strategy

Gateway `profile_id` is **required**. A gateway cannot be created without a profile. The API server resolves it during gateway creation:

1. Client creates Gateway with `cluster_id`, optionally supplying `profile_id`
2. If `profile_id` is present in the request, the API server honors that value
3. If `profile_id` is absent, the API server looks up `ManagedCluster.profile_id` and assigns it
4. If neither the client nor the cluster provides a `profile_id`, the API server rejects the creation request with HTTP 400

After creation, `profile_id` is **immutable to clearing** but **reassignable** to a different existing GatewayProfile via PATCH. Reassignment is validated (the target profile must exist and `profile_id` cannot be set to null or empty), and re-triggers reconciliation for that gateway through the existing gateway watch stream.

### Changing Quota: Reassignment, Not Profile Reconciliation

The control plane does **not** watch or reconcile GatewayProfile changes. Editing a GatewayProfile in place does **not** retroactively re-apply to gateways already using it. This is a deliberate design choice: profiles are modified rarely, so maintaining a permanent profile watch and fleet-wide fan-out for a rare operation is not worth the standing cost and concentrated blast radius.

To change the resource limits a gateway is subject to, reassign the gateway to a different profile:

1. Create (or select) the target GatewayProfile with the desired limits
2. PATCH each gateway's `profile_id` to the target profile
3. Each PATCH is a gateway update, which the control plane already watches and reconciles - the new quota is applied per gateway

This makes migrations **progressive** (canary a few gateways, verify, continue), **reversible** (point gateways back at the previous profile, which still exists), and **auditable** (each gateway's `profile_id` records exactly which profile it runs). See `docs/gateway-quota-presentation.md` (Slide 10) for the full tradeoff rationale.

> Note: Profiles remain fully editable via the API. "Not reconciled" refers only to propagation - an in-place edit is not pushed to running gateways until each is reassigned (or otherwise re-reconciled).

---

## Requirements

### Requirement: GatewayProfile as API Resource

GatewayProfile SHALL be a first-class HyperShell resource kind, persisted in PostgreSQL and exposed via REST and gRPC APIs. A GatewayProfile defines resource quota limits that can be applied to multiple gateways.

Because a GatewayProfile is the enforcement ceiling the control plane applies, creating, updating, and deleting a profile SHALL require the `platform:admin` role. Reading profiles (list and get) SHALL be permitted for any authenticated caller holding a role binding, so that gateway creators and owners can view profiles to assign one. See the GatewayProfile Authorization requirement in [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md).

**Schema:**

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique identifier (KSUID) |
| `name` | string | Human-readable name |
| `description` | string (optional) | Profile purpose/intent |
| `cpu_request_total` | string (optional) | Total CPU requests allowed (e.g., "4", "500m") |
| `cpu_limit_total` | string (optional) | Total CPU limits allowed |
| `memory_request_total` | string (optional) | Total memory requests allowed (e.g., "8Gi", "512Mi") |
| `memory_limit_total` | string (optional) | Total memory limits allowed |
| `ephemeral_storage_total` | string (optional) | Total ephemeral storage allowed (e.g., "10Gi") |
| `pod_count` | int32 (optional) | Maximum number of pods |
| `pvc_count` | int32 (optional) | Maximum number of PersistentVolumeClaims |
| `container_cpu_request_default` | string (optional) | Default CPU request injected into containers without explicit request |
| `container_cpu_limit_max` | string (optional) | Maximum CPU limit a container can request |
| `container_memory_request_default` | string (optional) | Default memory request injected into containers |
| `container_memory_limit_max` | string (optional) | Maximum memory limit a container can request |

All quantity fields follow Kubernetes resource quantity format (e.g., "500m", "2Gi", "10").

Zero-value integers (0) are treated as "not set" and omitted from quota enforcement. Empty strings are treated as "not set".

#### Scenario: Create a GatewayProfile

- GIVEN a `platform:admin` wants to define a "small" profile
- WHEN they create a GatewayProfile:
  ```json
  {
    "name": "small",
    "description": "Small gateway profile for development environments",
    "cpu_request_total": "2",
    "cpu_limit_total": "4",
    "memory_request_total": "4Gi",
    "memory_limit_total": "8Gi",
    "ephemeral_storage_total": "10Gi",
    "pod_count": 10,
    "pvc_count": 5,
    "container_cpu_request_default": "100m",
    "container_cpu_limit_max": "2",
    "container_memory_request_default": "128Mi",
    "container_memory_limit_max": "4Gi"
  }
  ```
- THEN the API server SHALL persist the GatewayProfile
- AND the profile SHALL be available for assignment to ManagedClusters

---

### Requirement: ManagedCluster Default Profile Assignment

ManagedCluster SHALL have a `profile_id` field that references a GatewayProfile. This field is optional and mutable.

When set, all gateways created on that cluster inherit the profile. When empty, gateways on that cluster have no quota enforcement.

#### Scenario: Assign default profile to cluster

- GIVEN a ManagedCluster exists
- AND a GatewayProfile "small" exists
- WHEN an administrator updates the cluster:
  ```json
  {
    "profile_id": "<small-profile-id>"
  }
  ```
- THEN the API server SHALL store `profile_id` on the ManagedCluster
- AND new gateways created on that cluster SHALL inherit this profile

#### Scenario: Remove default profile from cluster

- GIVEN a ManagedCluster has `profile_id` set
- WHEN an administrator updates the cluster to set `profile_id` to null or empty string
- THEN the API server SHALL clear `profile_id`
- AND new gateways created on that cluster SHALL have no quota enforcement

---

### Requirement: Gateway Profile Assignment

Gateway SHALL have a `profile_id` field that is **required at creation** and **reassignable afterward**. `profile_id` cannot be cleared after a gateway is created. If the client supplies a `profile_id` on create, the API server uses it. If not, the API server falls back to `ManagedCluster.profile_id`. If neither source provides a value, the creation request is rejected with HTTP 400.

Clients:
- MAY send `profile_id` on create requests to override the cluster default
- MAY send `profile_id` on PATCH requests to reassign the gateway to a different profile
- SHALL NOT send `profile_id = null` or empty string on PATCH; the API server SHALL reject such requests with HTTP 400
- MAY read `profile_id` from GET/LIST responses

When `profile_id` is present on PATCH, the API server SHALL validate that the referenced GatewayProfile exists and SHALL reject the request with HTTP 400 if it does not. This prevents a gateway from being assigned a `profile_id` that the control plane cannot fetch (which would block its provisioning).

The reconciler uses this field to determine which quota to enforce.

#### Scenario: Create gateway on cluster with default profile

- GIVEN a ManagedCluster has `profile_id = "<small-profile-id>"`
- WHEN a client creates a Gateway on that cluster without specifying `profile_id`
- THEN the API server SHALL assign `Gateway.profile_id = "<small-profile-id>"`
- AND the control plane SHALL enforce the "small" profile quota on the gateway namespace

#### Scenario: Create gateway on cluster without default profile

- GIVEN a ManagedCluster has `profile_id = null`
- WHEN a client creates a Gateway on that cluster without supplying `profile_id`
- THEN the API server SHALL reject the request with HTTP 400: "profile_id is required; cluster has no default profile assigned"

#### Scenario: Create gateway with explicit profile_id override

- GIVEN a ManagedCluster has `profile_id = "<cluster-default-id>"`
- WHEN a client creates a Gateway on that cluster with `profile_id = "<custom-id>"`
- THEN the API server SHALL honor the client-supplied value
- AND SHALL store `Gateway.profile_id = "<custom-id>"` (cluster default is NOT applied)

#### Scenario: Reassign a gateway to a different profile

- GIVEN a Gateway with `profile_id = "<small-profile-id>"`
- AND a GatewayProfile "large" exists
- WHEN an administrator PATCHes the gateway with `profile_id = "<large-profile-id>"`
- THEN the API server SHALL store `profile_id = "<large-profile-id>"`
- AND the control plane SHALL reconcile the gateway and apply the "large" profile quota to its namespace

#### Scenario: Reassign a gateway to a nonexistent profile

- GIVEN a Gateway exists
- WHEN an administrator PATCHes the gateway with `profile_id = "<nonexistent-id>"`
- THEN the API server SHALL reject the request with HTTP 400: "gateway profile <nonexistent-id> does not exist"
- AND the gateway's `profile_id` SHALL be unchanged

#### Scenario: Attempt to clear profile_id via PATCH

- GIVEN a Gateway with `profile_id` set
- WHEN an administrator PATCHes the gateway with `profile_id = null` or `profile_id = ""`
- THEN the API server SHALL reject the request with HTTP 400: "profile_id cannot be removed from a gateway; reassign to a different profile instead"
- AND the gateway's `profile_id` SHALL be unchanged

---

### Requirement: Control Plane Quota Fetching

When reconciling a gateway, the control plane SHALL:

1. Read `gateway.profile_id` from the gRPC watch event
2. If `profile_id` is empty, skip quota enforcement (no profile assigned)
3. If `profile_id` is set, call `GatewayProfileService.GetGatewayProfile` via gRPC
4. If the gRPC call fails (profile not found, network error, timeout), **BLOCK gateway provisioning**
5. Translate the protobuf `GatewayProfile` to a `QuotaConfig` struct
6. Pass `QuotaConfig` to the reconciler

**Security Rationale:** When a gateway has `profile_id` set, quota enforcement is expected. Proceeding without quota when fetch fails would allow the gateway to run unconstrained, potentially exhausting cluster resources. Failing fast forces operators to fix the profile issue before the gateway can deploy.

#### Scenario: Fetch quota config successfully

- GIVEN a Gateway with `profile_id = "<small-profile-id>"`
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `GetGatewayProfile("<small-profile-id>")`
- AND translate the response to `QuotaConfig`
- AND apply ResourceQuota and LimitRange to the gateway namespace

#### Scenario: Fetch quota config fails - gateway provisioning blocked

- GIVEN a Gateway with `profile_id = "<nonexistent-profile-id>"`
- WHEN the GatewayReconciler processes the event
- THEN it SHALL call `GetGatewayProfile` and receive an error
- AND it SHALL return an error from the reconciliation: `failed to fetch GatewayProfile <id>: <error>`
- AND it SHALL NOT provision the gateway workload
- AND the gateway phase SHALL remain "Provisioning" (or transition to "Failed")
- AND reconciliation SHALL retry on the next loop

#### Scenario: Gateway without profile (profile_id is empty)

- GIVEN a Gateway with `profile_id = null` (legacy record predating the required-profile policy)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL skip quota fetching entirely
- AND it SHALL proceed with gateway provisioning (no quota enforcement)
- AND the gateway namespace SHALL have no ResourceQuota or LimitRange

> Note: Under the current API policy, new gateways are always created with a `profile_id`. The empty case exists for backward compatibility with gateways created before this policy was enforced.

---

### Requirement: ResourceQuota Enforcement

The control plane SHALL create or update a ResourceQuota named `hypershell-gateway-quota` in each gateway namespace when `QuotaConfig` is non-nil.

The ResourceQuota SHALL enforce:

| GatewayProfile Field | Kubernetes Resource |
|---|---|
| `cpu_request_total` | `requests.cpu` |
| `cpu_limit_total` | `limits.cpu` |
| `memory_request_total` | `requests.memory` |
| `memory_limit_total` | `limits.memory` |
| `ephemeral_storage_total` | `requests.ephemeral-storage` |
| `pod_count` | `pods` |
| `pvc_count` | `persistentvolumeclaims` |

Fields with empty string or zero value SHALL be omitted from the ResourceQuota. An empty ResourceQuota (no fields set) SHALL NOT be created.

The quota object SHALL have labels:
```yaml
labels:
  app.kubernetes.io/managed-by: hypershell-control-plane
  hypershell.redhat.io/managed: "true"
```

The reconciler SHALL use update-or-create semantics: existing ResourceQuota is updated when its spec diverges from desired state.

#### Scenario: Apply ResourceQuota to gateway namespace

- GIVEN a Gateway with a profile defining `cpu_request_total = "2"` and `memory_limit_total = "8Gi"`
- WHEN the GatewayReconciler reconciles the gateway
- THEN it SHALL create a ResourceQuota:
  ```yaml
  apiVersion: v1
  kind: ResourceQuota
  metadata:
    name: hypershell-gateway-quota
    namespace: openshell-<id-hex>
    labels:
      app.kubernetes.io/managed-by: hypershell-control-plane
      hypershell.redhat.io/managed: "true"
  spec:
    hard:
      requests.cpu: "2"
      limits.memory: 8Gi
  ```

#### Scenario: Update ResourceQuota when the gateway is reassigned

- GIVEN a Gateway namespace has a ResourceQuota with `requests.cpu: "2"` from profile "small"
- WHEN an administrator reassigns the gateway to profile "large" with `cpu_request_total = "4"`
- AND the GatewayReconciler reconciles (triggered by the gateway update)
- THEN it SHALL fetch the "large" profile
- AND update the ResourceQuota to `requests.cpu: "4"`

#### Scenario: Pod creation exceeds quota

- GIVEN a Gateway namespace has ResourceQuota with `requests.memory: "4Gi"`
- AND the namespace already has pods requesting `3Gi`
- WHEN a user attempts to create a pod requesting `2Gi`
- THEN Kubernetes admission SHALL reject the pod with: `forbidden: exceeded quota`

---

### Requirement: LimitRange Enforcement

The control plane SHALL create or update a LimitRange named `hypershell-gateway-limits` in each gateway namespace when `QuotaConfig` has container-level fields set.

The LimitRange SHALL enforce:

| GatewayProfile Field | LimitRange Field | Effect |
|---|---|---|
| `container_cpu_request_default` | `defaultRequest.cpu` | Injected into containers without `resources.requests.cpu` |
| `container_memory_request_default` | `defaultRequest.memory` | Injected into containers without `resources.requests.memory` |
| `container_cpu_limit_max` | `max.cpu` | Rejects containers with `resources.limits.cpu` > this value |
| `container_memory_limit_max` | `max.memory` | Rejects containers with `resources.limits.memory` > this value |

Fields with empty string SHALL be omitted. A LimitRange with no fields SHALL NOT be created.

The LimitRange SHALL target `type: Container`.

The reconciler SHALL use update-or-create semantics: existing LimitRange is updated when its spec diverges from desired state.

#### Scenario: Apply LimitRange to gateway namespace

- GIVEN a Gateway with a profile defining `container_cpu_request_default = "100m"` and `container_memory_limit_max = "4Gi"`
- WHEN the GatewayReconciler reconciles the gateway
- THEN it SHALL create a LimitRange:
  ```yaml
  apiVersion: v1
  kind: LimitRange
  metadata:
    name: hypershell-gateway-limits
    namespace: openshell-<id-hex>
    labels:
      app.kubernetes.io/managed-by: hypershell-control-plane
      hypershell.redhat.io/managed: "true"
  spec:
    limits:
    - type: Container
      defaultRequest:
        cpu: 100m
      max:
        memory: 4Gi
  ```

#### Scenario: Container without requests gets defaults

- GIVEN a Gateway namespace has LimitRange with `defaultRequest.cpu: "100m"`
- WHEN a user creates a pod with a container that has no `resources.requests.cpu`
- THEN Kubernetes admission SHALL inject `requests.cpu: "100m"` into the container

#### Scenario: Container exceeds max limit

- GIVEN a Gateway namespace has LimitRange with `max.memory: "4Gi"`
- WHEN a user creates a pod with a container requesting `limits.memory: "8Gi"`
- THEN Kubernetes admission SHALL reject the pod

---

### Requirement: Quota Reconciliation Idempotence

The control plane SHALL reconcile quota resources idempotently toward the desired state derived from the gateway's current `profile_id`:

- If the desired ResourceQuota is non-empty and none exists, create
- If the desired ResourceQuota is non-empty and one exists with divergent spec, update
- If the desired ResourceQuota is non-empty and one exists with matching spec, no change
- If the desired ResourceQuota is empty (no fields, or no profile), delete the managed ResourceQuota if one exists
- Same logic for LimitRange

Deletion SHALL only remove objects the control plane manages (identified by the `hypershell.redhat.io/managed: "true"` label); objects created by other actors SHALL NOT be touched.

The reconciler SHALL log quota operations at INFO level:
- `INFO created ResourceQuota hypershell-gateway-quota in namespace <ns>`
- `INFO updated ResourceQuota hypershell-gateway-quota in namespace <ns>`
- `INFO deleted ResourceQuota hypershell-gateway-quota in namespace <ns>`
- `INFO created LimitRange hypershell-gateway-limits in namespace <ns>`
- `INFO updated LimitRange hypershell-gateway-limits in namespace <ns>`
- `INFO deleted LimitRange hypershell-gateway-limits in namespace <ns>`

#### Scenario: Re-reconcile gateway with unchanged quota

- GIVEN a Gateway namespace has ResourceQuota and LimitRange matching the profile
- WHEN the GatewayReconciler reconciles the gateway again
- THEN it SHALL read existing quota resources
- AND SHALL NOT update them (spec unchanged)
- AND SHALL NOT log creation/update messages

---

### Requirement: Quota Removal When Desired State Is Empty

When the desired quota for a gateway is empty - because the gateway was reassigned to a profile that omits the corresponding fields, or (for legacy gateways) because `profile_id = null` - the control plane SHALL reconcile toward absence:

- It SHALL NOT create new quota resources for an empty desired state
- It SHALL delete any existing **managed** ResourceQuota / LimitRange (identified by the `hypershell.redhat.io/managed: "true"` label) whose desired counterpart is now empty

Because a gateway carries at most one profile at a time and the reconciler always computes the full desired set, removed fields converge to the correct namespace state without manual cleanup.

#### Scenario: Gateway without profile (legacy)

- GIVEN a Gateway with `profile_id = null` (legacy record) and no existing managed quota objects
- WHEN the GatewayReconciler reconciles the gateway
- THEN it SHALL NOT create ResourceQuota or LimitRange
- AND the gateway namespace SHALL have no quota enforcement

#### Scenario: Reassignment to a profile without container fields removes the LimitRange

- GIVEN a Gateway namespace has a managed LimitRange from a profile with container defaults
- WHEN the gateway is reassigned to a profile with no container-level fields
- AND the GatewayReconciler reconciles
- THEN it SHALL delete the managed LimitRange
- AND SHALL log `INFO deleted LimitRange hypershell-gateway-limits in namespace <ns>`

---

### Requirement: Profile Deletion Protection

A GatewayProfile SHALL NOT be deleted while it is referenced by either:

- a **ManagedCluster** via `ManagedCluster.profile_id` (the cluster's default profile), or
- a **Gateway** via `Gateway.profile_id`.

The API server SHALL enforce this constraint: a delete request for a GatewayProfile SHALL fail with HTTP 409 Conflict if any cluster or gateway references it. Only soft-deleted (non-live) referrers SHALL be ignored.

**Rationale:** A gateway that references a deleted profile would have `profile_id` set but be unable to fetch the profile, blocking its provisioning (profile assignment implies quota enforcement). A cluster that references a deleted profile would assign a dangling `profile_id` to every new gateway placed on it. Blocking deletion while either kind of referrer exists prevents both failure modes.

This mirrors the deletion-protection convention used by ManagedDatabase (see `openshell-gateway-database.spec.md`), which likewise blocks deletion while referenced by a cluster default or a gateway.

#### Scenario: Attempt to delete profile referenced by a cluster

- GIVEN a GatewayProfile "small" is the default profile for one or more ManagedClusters via `profile_id`
- WHEN an administrator attempts to delete the "small" profile
- THEN the API server SHALL reject the deletion with HTTP 409: "GatewayProfile <id> is the default profile for one or more clusters and cannot be deleted"

#### Scenario: Attempt to delete profile referenced by gateways

- GIVEN a GatewayProfile "small" is referenced by 5 gateways via `profile_id`
- WHEN an administrator attempts to delete the "small" profile
- THEN the API server SHALL reject the deletion with HTTP 409: "GatewayProfile <id> is referenced by one or more gateways and cannot be deleted"

#### Scenario: Delete profile with no references

- GIVEN a GatewayProfile referenced by no live cluster and no live gateway
- WHEN an administrator deletes the profile
- THEN the API server SHALL delete the GatewayProfile
- AND future gateways SHALL NOT be able to use this profile_id

---

### Requirement: Profile Field Validation

The API server SHALL validate GatewayProfile quantity and count fields at create and update time, so that invalid values are rejected at the API boundary rather than silently persisted and later failing gateway reconciliation.

- Each quantity field (`cpu_request_total`, `cpu_limit_total`, `memory_request_total`, `memory_limit_total`, `ephemeral_storage_total`, `container_cpu_request_default`, `container_cpu_limit_max`, `container_memory_request_default`, `container_memory_limit_max`) that is non-empty SHALL parse as a valid, non-negative Kubernetes resource quantity; otherwise the request SHALL be rejected with HTTP 400. Negative totals are rejected because a negative resource quota or limit is meaningless.
- Count fields (`pod_count`, `pvc_count`) SHALL be non-negative; a negative value SHALL be rejected with HTTP 400.
- Empty string and zero remain valid and mean "not set".

**Rationale:** Without boundary validation, a typo (e.g., `cpu_request_total: "tow"`) is accepted, assigned to gateways, and only fails later in the control plane at `resource.ParseQuantity` time - turning a single bad input into a provisioning failure discoverable only in control-plane logs.

#### Scenario: Reject profile with an invalid quantity

- GIVEN an administrator creates a GatewayProfile with `cpu_request_total = "tow"`
- WHEN the API server processes the request
- THEN it SHALL reject the request with HTTP 400 identifying the invalid field

#### Scenario: Reject profile with a negative count

- GIVEN an administrator creates a GatewayProfile with `pod_count = -1`
- WHEN the API server processes the request
- THEN it SHALL reject the request with HTTP 400

---

## Configuration Reference

No environment variables. Quota enforcement is fully API-driven via GatewayProfile resources.

---

## Configuration Examples

### Example 1: Small Development Profile

```json
{
  "name": "dev-small",
  "description": "Small profile for development gateways",
  "cpu_request_total": "1",
  "cpu_limit_total": "2",
  "memory_request_total": "2Gi",
  "memory_limit_total": "4Gi",
  "pod_count": 10,
  "container_cpu_request_default": "50m",
  "container_memory_request_default": "64Mi"
}
```

Resulting ResourceQuota:
```yaml
spec:
  hard:
    requests.cpu: "1"
    limits.cpu: "2"
    requests.memory: 2Gi
    limits.memory: 4Gi
    pods: "10"
```

Resulting LimitRange:
```yaml
spec:
  limits:
  - type: Container
    defaultRequest:
      cpu: 50m
      memory: 64Mi
```

### Example 2: Production Profile with Max Limits

```json
{
  "name": "production",
  "description": "Production gateways with strict limits",
  "cpu_request_total": "8",
  "cpu_limit_total": "16",
  "memory_request_total": "16Gi",
  "memory_limit_total": "32Gi",
  "ephemeral_storage_total": "50Gi",
  "pod_count": 50,
  "pvc_count": 10,
  "container_cpu_request_default": "500m",
  "container_cpu_limit_max": "4",
  "container_memory_request_default": "512Mi",
  "container_memory_limit_max": "8Gi"
}
```

Resulting ResourceQuota:
```yaml
spec:
  hard:
    requests.cpu: "8"
    limits.cpu: "16"
    requests.memory: 16Gi
    limits.memory: 32Gi
    requests.ephemeral-storage: 50Gi
    pods: "50"
    persistentvolumeclaims: "10"
```

Resulting LimitRange:
```yaml
spec:
  limits:
  - type: Container
    defaultRequest:
      cpu: 500m
      memory: 512Mi
    max:
      cpu: "4"
      memory: 8Gi
```

### Example 3: Minimal Profile (Pod Count Only)

```json
{
  "name": "pod-limit-only",
  "pod_count": 20
}
```

Resulting ResourceQuota:
```yaml
spec:
  hard:
    pods: "20"
```

No LimitRange is created (no container-level fields).

---

## Operational Procedures

### Assign a Profile to All Gateways on a Cluster

1. Create a GatewayProfile:
   ```bash
   hsctl create gatewayprofile small \
     --cpu-request-total=2 \
     --memory-limit-total=8Gi
   ```

2. Assign to cluster:
   ```bash
   hsctl patch managedcluster <cluster-id> \
     --profile-id=<small-profile-id>
   ```

3. New gateways inherit the profile automatically. Existing gateways retain their old `profile_id`.

### Change the Quota a Gateway Uses (Reassignment Migration)

Editing a GatewayProfile in place does **not** re-apply to gateways already using it - the control plane does not reconcile profile changes. To change the limits enforced on running gateways, reassign them to a different profile:

1. Create the target profile with the desired limits:
   ```bash
   hsctl create gatewayprofile large \
     --cpu-request-total=4 \
     --memory-limit-total=16Gi
   ```

2. Reassign gateways to it (do this progressively - canary a few first, verify, then continue):
   ```bash
   hsctl patch gateway <gateway-id> --profile-id=<large-profile-id>
   ```

3. Each reassignment is a gateway update; the control plane reconciles that gateway and applies the new quota to its namespace. To roll back, reassign the gateway to the previous profile (which still exists).

> If you instead edit an existing profile's fields, the change takes effect for a given gateway only when that gateway is next reconciled (e.g. reassigned, or the control plane restarts). Do not rely on in-place edits to retune a running fleet.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| Pod rejected with "exceeded quota" | Namespace ResourceQuota exceeded | Check `kubectl describe quota -n <ns>`, scale down or increase profile limits |
| Pod rejected with "maximum memory limit" | Container limit exceeds LimitRange max | Reduce container `resources.limits.memory` or increase profile `container_memory_limit_max` |
| Gateway has no quota despite profile_id | Profile fetch failed or profile deleted | Check control plane logs, verify profile exists |
| ResourceQuota exists but LimitRange missing | Profile has no container-level fields | Expected behavior, LimitRange only created when container defaults/max are set |
| Edited a profile but a running gateway's quota did not change | By design, profile edits are not reconciled | Reassign the gateway to the (edited or a new) profile: `hsctl patch gateway <id> --profile-id=<id>` |
| Stale quota after reassigning to a smaller profile | Old reconciler left removed limits behind | Fixed: reconciler now removes managed objects when desired state is empty; re-reconcile the gateway |

---

## References

- [Kubernetes ResourceQuotas](https://kubernetes.io/docs/concepts/policy/resource-quotas/)
- [Kubernetes LimitRanges](https://kubernetes.io/docs/concepts/policy/limit-range/)
- [Resource Quantity Format](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/)
