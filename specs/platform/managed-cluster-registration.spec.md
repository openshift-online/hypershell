# Managed Cluster Self-Registration

**Date:** 2026-09-09
**Status:** Draft
**Ticket:** HYPERSHELL-326, HYPERSHELL-333
**Related:** `security/rbac-enforcement.spec.md` (managed-cluster-registrar role), `platform/data-model.spec.md` (ManagedCluster entity), `platform/control-plane.spec.md` (mandatory cluster identity, startup flow, gRPC transport security), `platform/hub-grpc-tls.spec.md` (hub-side TLS Route every control plane's `HYPERSHELL_GRPC_SERVER_ADDR` dials in production)

---

## Purpose

Every control plane must determine its own `cluster_id` at runtime without requiring a human to pre-register it and distribute the resulting KSUID out-of-band. The `/registration` sub-resource within the `managedClusters` plugin enables a control plane to register itself using its OIDC `client_credentials` identity and receive a stable `cluster_id` in return. This applies to every control plane, including the one co-located with the hub API server: there is no unregistered mode (`control-plane.spec.md`, "Requirement: Mandatory Cluster Identity").

The same endpoint serves as a health ping. The control plane calls it on a loop; registration calls after the first are no-ops that update `last_seen_at`, giving the hub passive fleet-health visibility with no additional infrastructure.

Because every control plane is identified, the API server can enforce cluster scoping on the gRPC watch streams instead of trusting the `cluster_id` a caller asks for. That binding is specified here too.

---

## API

### POST /api/hypershell/v1/managed_clusters/registration

Idempotent. Creates a `ManagedCluster` record on first call; returns the existing record on subsequent calls from the same OIDC identity. Updates `last_seen_at` on every call.

**Authentication:** OIDC `client_credentials` JWT. The caller must carry the `managed-cluster-registrar` role in `realm_access.roles`. See `security/rbac-enforcement.spec.md`.

**Identity resolution:** The API server extracts the `sub` claim from the validated JWT and uses it as `oidc_subject` on the `ManagedCluster` record. The caller never supplies `oidc_subject` directly.

**Request:**

```json
{
  "name": "hyp0-mc1",
  "description": "optional"
}
```

**Response (201 Created on first call, 200 OK on subsequent calls):**

```json
{
  "cluster_id": "<KSUID>"
}
```

**Upsert key:** `(oidc_subject, name)`. Both must match for the call to be idempotent. If the same OIDC subject re-registers with a different `name`, or the requested `name` already belongs to a record with a different (or empty) `oidc_subject`, the API returns 409 Conflict. A control plane may not change its registered name, and may not take over a name, without admin intervention.

---

## Data Model Impact

`ManagedCluster` gains two fields to support self-registration and fleet health:

| Field | Type | Description |
|-------|------|-------------|
| `oidc_subject` | string | OIDC `sub` claim of the service account that registered this cluster. Set server-side; not writable via PATCH. Unique index with `name`. |
| `last_seen_at` | timestamp | Updated on every `/registration` call. Null until first registration. |

`name` SHALL be unique across all `ManagedCluster` records, registered or not, so that discovery by name (seed scripts, e2e, operators) is unambiguous.

`last_seen_at` enables passive fleet health without active probing. Hub-side consumers (dashboard, alerting) derive spoke health from staleness:

| `last_seen_at` age | Derived health |
|--------------------|---------------|
| < 5 min | Healthy |
| 5 - 30 min | Unknown |
| > 30 min | Offline |

These thresholds are informational and may be tuned per deployment. The `status` field on `ManagedCluster` continues to reflect the control-plane reconciler's view of cluster state; `last_seen_at` is a separate, spoke-reported liveness signal.

**Manually created records.** `POST /api/hypershell/v1/managed_clusters` still creates a record with an empty `oidc_subject`. No control plane serves such a record: gateways assigned to it are never reconciled, and the control plane that later wants that name receives 409 until an operator deletes the manual record. Manual creation is therefore an inert placeholder, not a way to add a cluster to the fleet; the fleet grows only through registration.

---

## Control Plane Startup and Loop

```
startup:
  cluster_id = POST /registration { name: HYPERSHELL_MANAGED_CLUSTER_NAME }
  open WatchGateways(cluster_id=cluster_id)

loop every 60s:
  POST /registration { name: HYPERSHELL_MANAGED_CLUSTER_NAME }
  # returns same cluster_id; API updates last_seen_at
```

Gitops configuration required for every control plane, the hub's co-located one included:

| Env var | Description |
|---------|-------------|
| `HYPERSHELL_MANAGED_CLUSTER_NAME` | Human-readable name, unique per control plane (e.g. `hyp0-mc1` for a remote spoke, `hyp0-hub` for the control plane co-located with hub `hyp0`; `local-kind` / `local-openshift` in the development overlays) |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | One OIDC client per control plane, holding `managed-cluster-registrar`; no new secret types |
| `HYPERSHELL_API_SERVER_URL` | REST endpoint used for `/registration` (e.g. `https://api.hyp0.infra.hypershell.app`; the in-cluster `http://hypershell-api-server:8000` only where TLS is off) |
| `HYPERSHELL_GRPC_SERVER_ADDR` | gRPC endpoint the watch streams dial. On a production hub every control plane, co-located or remote, dials the hub's external passthrough Route (e.g. `grpc.hyp0.infra.hypershell.app:443`) with TLS, because the hub's gRPC listener is TLS-only (`hub-grpc-tls.spec.md`). In development environments with gRPC TLS off, an in-cluster address (e.g. `hypershell-api-server:9000`) dialed with plaintext. The control plane selects the transport from this address -- see `control-plane.spec.md` ("gRPC Transport Security"). |

`HYPERSHELL_CLUSTER_ID` is resolved at runtime and SHALL NOT appear in gitops.

On the hub api-server, `RBAC_SERVICE_ACCOUNTS` SHALL list the service-account username of every control plane (for example `service-account-hypershell-control-plane` for the co-located one), because the gRPC RBAC interceptor grants the control-plane status writes (`UpdateGateway`, sandbox counts) only to allowlisted service accounts.

---

## RBAC

The `managed-cluster-registrar` role is required on both the initial registration call and every subsequent loop call. A control plane without the role receives 403 on its first call and cannot start.

**Enforcement mechanism:** `managed-cluster-registrar` is a JWT-direct role. The `isAuthorized` function in the HTTP authorization middleware has a dedicated case for `POST managed_clusters/registration` that checks the JWT claim directly, bypassing the `hasGatewayCreator` fallback. The role is NOT in `JWTSyncedRoles` and has no DB RoleBinding lifecycle. See `security/rbac-enforcement.spec.md` for the implementation contract.

An administrator assigns `managed-cluster-registrar` to each control plane's OIDC client in Keycloak before that control plane is deployed. In production this is an explicit, out-of-band admin step. The development realms (`deploy/base/keycloak`, the Kind and OpenShift overlays) SHALL ship the role already assigned to the `hypershell-control-plane` client, so `make kind-up` and `make openshift-up` produce a control plane that registers without manual steps. Keycloak is the trusted source of truth; the API server does not re-verify role assignment beyond reading the JWT claim.

**Isolation guarantee:** In production (`RBAC_DEFAULT_ROLES=`, `RBAC_ENFORCE=true`), a control plane holding only `managed-cluster-registrar` has no gateway permissions through the HTTP API. All permissions flow exclusively from Keycloak. A control plane gains gateway access only if an administrator also explicitly grants `gateway:creator` or a gateway-scoped binding in Keycloak.

---

## Requirements

### Requirement: Idempotent Registration

`POST /managed_clusters/registration` SHALL be idempotent on the `(oidc_subject, name)` key.

- On first call: create a `ManagedCluster` record with a new KSUID, set `oidc_subject` from the JWT `sub` claim, set `last_seen_at` to now. Return 201 with `cluster_id`.
- On subsequent calls with the same subject and name: update `last_seen_at` to now. Return 200 with the existing `cluster_id`.
- If the same OIDC subject supplies a different `name` than the one already registered: return 409 Conflict.

The upsert SHALL use database-level locking to handle concurrent first-time requests safely.

#### Scenario: First-time registration

- GIVEN a control plane with `managed-cluster-registrar` that has never registered
- WHEN it calls `POST /managed_clusters/registration` with `name: hyp0-mc1`
- THEN a new `ManagedCluster` record is created with a stable KSUID
- AND `oidc_subject` is set to the JWT `sub` claim
- AND `last_seen_at` is set to now
- AND the response is 201 with `cluster_id`

#### Scenario: Re-registration is idempotent

- GIVEN a control plane that previously registered and received `cluster_id: X`
- WHEN it calls `POST /managed_clusters/registration` again with the same `name`
- THEN no new record is created
- AND `last_seen_at` is updated to now
- AND the response is 200 with `cluster_id: X`

#### Scenario: Name change rejected

- GIVEN a control plane already registered as `hyp0-mc1`
- WHEN it calls `POST /managed_clusters/registration` with `name: hyp0-mc2`
- THEN the response is 409 Conflict
- AND no record is created or modified

### Requirement: Name Collision Is a Conflict

A registration whose `name` matches an existing `ManagedCluster` record with a different `oidc_subject`, or with an empty `oidc_subject` (a record created through `POST /managed_clusters`), SHALL return 409 Conflict and SHALL NOT create, adopt, or modify any record. The conflict message SHALL name the existing record's id and state that an operator must delete it (and re-point any gateways that reference it) before this control plane can register under that name.

This is deliberate: registration never silently takes over a record, because gateways may already reference its id and a takeover would move them to a different control plane without anyone deciding so.

#### Scenario: Name held by a manually created record

- GIVEN a `ManagedCluster` named `local-openshift` created via `POST /managed_clusters` (empty `oidc_subject`)
- WHEN a control plane calls `POST /managed_clusters/registration` with `name: local-openshift`
- THEN the response is 409 Conflict naming the existing record's id
- AND no record is created or modified

#### Scenario: Name held by another control plane

- GIVEN a `ManagedCluster` named `hyp0-mc1` registered by OIDC subject `A`
- WHEN a control plane with OIDC subject `B` calls `POST /managed_clusters/registration` with `name: hyp0-mc1`
- THEN the response is 409 Conflict
- AND no record is created or modified

#### Scenario: Operator resolves a collision

- GIVEN the 409 from the scenario above
- WHEN an operator deletes the manually created `local-openshift` record
- AND the control plane retries registration
- THEN the response is 201 with a new `cluster_id`

### Requirement: Role Enforcement

The `/registration` endpoint SHALL require the `managed-cluster-registrar` role in the caller's JWT. A caller without the role SHALL receive 403 Forbidden before any database operation.

#### Scenario: Missing role rejected

- GIVEN a control plane service account without `managed-cluster-registrar` in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the response is 403 Forbidden
- AND no `ManagedCluster` record is created

### Requirement: last_seen_at Updated on Every Call

Every successful call to `/registration` SHALL update `last_seen_at` on the matching `ManagedCluster` record, including calls that are no-ops for the registration data itself.

#### Scenario: Heartbeat loop updates last_seen_at

- GIVEN a registered control plane calling `/registration` every 60 seconds
- WHEN each call completes successfully
- THEN `last_seen_at` on the `ManagedCluster` record is updated to the call time
- AND the response returns the same `cluster_id` each time

### Requirement: oidc_subject Is Server-Assigned and Immutable

`oidc_subject` SHALL be set by the API server from the validated JWT `sub` claim. It SHALL NOT be accepted as an input field on any request. It SHALL NOT be modifiable via `PATCH /managed-clusters/{id}`.

### Requirement: Gateways Reference a Registered Cluster

`POST /api/hypershell/v1/gateways` and any `PATCH` that sets `cluster_id` SHALL reject (400 Bad Request) an empty `cluster_id` and a `cluster_id` that does not reference a `ManagedCluster` with a non-empty `oidc_subject`. Because every control plane is registered and filters by its own id, a gateway without a registered cluster would never be reconciled by anyone. Callers that want "the local cluster" discover it by name from `GET /managed_clusters`; on any running environment the co-located control plane's record always exists.

#### Scenario: Empty cluster_id rejected

- GIVEN a gateway create request with `cluster_id: ""`
- WHEN it is submitted
- THEN the response is 400 Bad Request naming `cluster_id`
- AND no Gateway is created

#### Scenario: Unregistered cluster rejected

- GIVEN a `ManagedCluster` created via `POST /managed_clusters` (empty `oidc_subject`)
- WHEN a gateway create request references its id
- THEN the response is 400 Bad Request stating the cluster has no registered control plane
- AND no Gateway is created

### Requirement: Watch Stream Caller Binding

The five gRPC watch RPCs (`WatchGateways`, `WatchGatewayReleases`, `WatchManagedClusters`, `WatchGatewayNetworks`, `WatchRoleBindings`) SHALL require a valid JWT; they SHALL NOT appear in `--auth-bypass-methods` in any environment that runs the control plane with OIDC credentials, and never on a hub that exposes gRPC externally (`hub-grpc-tls.spec.md`).

For `WatchGateways`, `ListGateways`, `WatchRoleBindings`, and `ListRoleBindings`, when the caller's JWT `sub` matches the `oidc_subject` of a registered `ManagedCluster`, the request SHALL carry `cluster_id` equal to that record's id: a missing `cluster_id` SHALL be rejected with `INVALID_ARGUMENT` and a different one with `PERMISSION_DENIED`. The check runs in the gRPC RBAC interceptor before the handler subscribes to the event broker, and applies regardless of `RBAC_SERVICE_ACCOUNTS`. Callers whose `sub` is not a registered cluster (users, `hsctl`) are unaffected by this rule and continue through the existing role-binding authorization.

For `WatchRoleBindings` and `ListRoleBindings` the scoping key is the `cluster_id` of the binding's gateway: under a `cluster_id` filter the api-server SHALL deliver (in the watch replay, in live watch events, and in list results) only bindings whose gateway is assigned to that cluster, and SHALL NOT deliver global bindings (no `gateway_id`). A binding delete is attributed through its gateway even when that gateway is already soft-deleted, so the delete still reaches the owning cluster; a binding whose gateway cannot be found at all is not delivered to any filtered stream.

#### Scenario: Registered control plane watches its own cluster

- GIVEN a control plane registered as `cluster_id: X`
- WHEN it opens `WatchGateways` with its bearer token and `cluster_id: X`
- THEN the stream is accepted
- AND only events for gateways with `cluster_id: X` are delivered

#### Scenario: Registered control plane watches role bindings

- GIVEN a control plane registered as `cluster_id: X`
- AND gateways assigned to `X` and to `Y`, each with a RoleBinding, and a global RoleBinding
- WHEN it opens `WatchRoleBindings` with its bearer token and `cluster_id: X`
- THEN the stream is accepted
- AND only bindings for gateways assigned to `X` are delivered, in the replay and as live events
- AND bindings for gateways assigned to `Y` and global bindings are not delivered

#### Scenario: Registered control plane asks for another cluster

- GIVEN a control plane registered as `cluster_id: X`
- WHEN it opens `WatchGateways` with `cluster_id: Y`
- THEN the api-server returns `PERMISSION_DENIED`

#### Scenario: Registered control plane omits the filter

- GIVEN a control plane registered as `cluster_id: X`
- WHEN it opens `WatchGateways` with no `cluster_id`
- THEN the api-server returns `INVALID_ARGUMENT`

#### Scenario: Anonymous watch rejected

- GIVEN a gRPC client with no bearer token
- WHEN it opens any `Watch*` RPC
- THEN the api-server returns `UNAUTHENTICATED`

### Requirement: Fail-Closed Startup

The control plane SHALL NOT open the `WatchGateways` gRPC stream until a successful `/registration` response has been received. On registration failure at startup, the control plane SHALL retry with exponential backoff indefinitely, logging the error on each attempt. It SHALL NOT proceed with an unresolved `cluster_id`.

The exceptions are the non-retryable responses. On 403 Forbidden the control plane lacks the required Keycloak role; on 409 Conflict its name is taken or its subject is registered under another name. In both cases retrying will not help: the control plane SHALL log the error and exit, surfacing a clear message (the missing `managed-cluster-registrar` role, or the API's conflict message naming the record an operator must remove).

#### Scenario: Transient failure retried with backoff

- GIVEN the API server is temporarily unreachable (network partition, restart)
- WHEN the control plane attempts to register at startup
- THEN it SHALL retry with exponential backoff
- AND it SHALL NOT open `WatchGateways` until registration succeeds

#### Scenario: 403 exits immediately

- GIVEN the API server returns 403 (role not yet assigned in Keycloak)
- WHEN the control plane attempts to register at startup
- THEN it SHALL NOT retry
- AND it SHALL log a clear error identifying the missing `managed-cluster-registrar` role and exit

#### Scenario: 409 exits immediately

- GIVEN the API server returns 409 (name collision)
- WHEN the control plane attempts to register at startup
- THEN it SHALL NOT retry
- AND it SHALL log the API's conflict message and exit

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Every control plane registers; no unregistered mode | The hub's co-located control plane is physically a spoke already (it deploys only into its own cluster with in-cluster credentials). Giving it an identity removes the one code path that could not be scoped server-side and makes registration, filtering, and authenticated watches run in every environment. |
| Name collision is a 409, not an adoption | A silent takeover would re-home the gateways referencing the old record. Failing loudly makes the operator decide, and the seed scripts simply stop creating the record so the collision never happens on a fresh environment. |
| `name` unique across all records | Seed scripts, e2e discovery, and operators look clusters up by name. Two records with one name would make that lookup nondeterministic. |
| Manual `POST /managed_clusters` records are inert | Nothing consumes `kubeconfig_secret`, and no control plane filters on a record it did not register. Keeping the endpoint avoids an API break for `hsctl`, but the spec is explicit that it does not add a cluster to the fleet. |
| Gateways must reference a registered cluster | With every control plane filtering, an unassigned gateway is orphaned forever. Rejecting it at create time turns a silent no-op into an immediate, actionable error. |
| Watch-stream binding checks the caller's registered cluster, not a role | The identity that matters is "which cluster is this", and registration already established it. A role would only prove the caller is some control plane. |
| Single `/registration` endpoint for both register and heartbeat | Eliminates a separate heartbeat endpoint. The idempotent registration call already has all the information needed to update `last_seen_at`. Fewer endpoints, simpler RBAC surface. |
| Narrow response body (`{ "cluster_id" }` only, not full ManagedCluster) | The control plane needs exactly one thing from registration: its stable `cluster_id` to use as the `WatchGateways` filter. Returning the full ManagedCluster object would expose fields the control plane cannot and should not act on. The narrow shape is intentional and differs from the standard `GET /managed_clusters/{id}` response by design. |
| 403 and 409 exit immediately; other failures retry with backoff | A 403 means the Keycloak role is absent and a 409 means an operator must remove a record; retrying either is pointless and delays operator awareness. Network or 5xx errors are transient; exponential backoff recovers automatically without operator intervention. |
| `(oidc_subject, name)` upsert key | `oidc_subject` alone allows a control plane to change its human name between deployments. Requiring both prevents accidental name changes and makes conflicts explicit rather than silent. |
| `last_seen_at` as passive liveness, not a status field | Keeps the control plane's self-reported liveness separate from the hub reconciler's view of cluster state. The `status` field remains the reconciler's domain. |
| No active health probing from the hub | Control planes call in; the hub does not need to reach out. Avoids hub-to-spoke credential management and works across network topologies where the hub cannot initiate connections to spokes. |
| Role assigned by admin in production, pre-assigned in dev realms | The `managed-cluster-registrar` role is a privilege gate. A Keycloak admin must explicitly grant it before a production control plane can self-register, providing a human control point for fleet membership. Development realms are code-owned, so the grant ships with them. |
| JWT-direct enforcement, not DB-synced | `managed-cluster-registrar` is not in `JWTSyncedRoles` because it should never be auto-assigned (unlike `gateway:creator`) and does not need a DB binding lifecycle. A live JWT claim check in `isAuthorized` is sufficient and avoids polluting the sync table with a role that applies to a narrow class of service accounts. |
