# Gateway Sandbox Events

**Status:** Draft
**Version:** 0.1.0
**Created:** 2026-10-20
**Applies to:** `components/control-plane`, `components/api-server`, `components/sdk-typescript`
**Related:** `platform/openshell-gateway-sandbox-count.spec.md`, `platform/gateway-sandbox-attention-status.spec.md`, `platform/managed-cluster-registration.spec.md`, `platform/data-model.spec.md`, `platform/control-plane-observability.spec.md` (OBS-CPL-06), `security/rbac-enforcement.spec.md`, `standards/control-plane/conventions.spec.md`

## Purpose

Gateway-bound users need recent Kubernetes pod events to diagnose a sandbox that is pending, crash-looping, or has failed without direct cluster access. This specification defines a periodic control-plane observation and push path, a short-lived API-server cache, and a gateway-scoped REST read API. The data is advisory and recent. It is not durable resource state.

## Goals

- Surface the most recent Kubernetes Events for each sandbox of a gateway through the HyperShell REST API.
- Include events for active and terminated sandbox pods while Kubernetes retains them.
- Keep the control plane as the only spoke-to-hub writer and enforce managed-cluster scope on that write.
- Restrict reads to callers bound to the requested Gateway.
- Bound memory, event count, and data lifetime without adding a PostgreSQL table or telemetry series.

## Non-Goals

- A web-console page, UI mockup, or any other UI surface.
- A per-sandbox REST endpoint, streaming API, or an event-driven push protocol.
- Per-user filtering inside one Gateway. Every authorized Gateway user sees all event groups for that Gateway.
- PostgreSQL persistence, OTLP metrics, Prometheus metrics, or telemetry attributes containing sandbox names, namespaces, Event messages, or Event reasons.
- Multi-replica API-server support. Version 1 supports one API-server replica only; high availability requires a shared cache backing such as Valkey or Redis.
- Events for sandboxes in namespaces without a live Gateway.
- A Kubernetes Event history guarantee after Kubernetes has deleted an Event, after the cache TTL expires, or after an API-server restart.

## Domain Vocabulary

- **Sandbox pod** - a Kubernetes Pod that has the `agents.x-k8s.io/sandbox-name-hash` label.
- **Sandbox name** - the name of the Sandbox custom resource that owns a sandbox pod. The owner reference is the primary correlation. A same-namespace, same-name fallback is allowed only when the owner reference is absent.
- **Gateway namespace** - the namespace assigned to a live Gateway. It is the tenancy key between a sandbox observation and its Gateway.
- **Sandbox Event** - a Kubernetes Event whose involved object is a correlated sandbox pod. The API representation contains `type`, `reason`, `message`, `first_observed_at`, `last_observed_at`, `count`, `involved_pod_name`, and `involved_pod_uid`. A missing source value is represented as absent, not fabricated.
- **Event snapshot** - the complete, bounded set of most recent Events for all correlated Pods of one `(gateway namespace, sandbox name)` at one reconcile tick. It replaces, rather than merges with, the preceding value.
- **Event limit (N)** - the configured finite maximum number of Events retained for one sandbox. The default SHALL be `10`; a valid value SHALL be from `5` through `10` inclusive.
- **Sandbox event cache** - the API-server process-local, TTL-bound store of Event snapshots, keyed by Gateway namespace and sandbox name.

## Actors

- **Control plane** - the registered managed-cluster control plane that observes Kubernetes resources and reports snapshots for Gateways assigned to its cluster.
- **API server** - receives authenticated snapshots, keeps them in its in-memory cache, and serves the REST read endpoint.
- **Gateway-bound user** - an authenticated caller with a `gateway:viewer`, `gateway:admin`, or `gateway:owner` binding on the requested Gateway.
- **Kubernetes API** - provides sandbox Pods, Sandbox custom resources, and Kubernetes Events.

## Requirements

### Requirement: OSG-SEVT-01 -- Periodic Sandbox Event Observation

The control plane SHALL run a sandbox-events reconciler on a periodic tick. Its default cadence SHALL be two minutes, matching the sandbox attention reconciler cadence. The reconciler SHALL use a bounded Kubernetes-list context and SHALL continue on its next tick after a failed observation or push.

The reconciler SHALL scope its Kubernetes Event list calls to the live gateway namespaces of its managed cluster and SHALL NOT list Events cluster-wide. A shared cluster hosts other tenants' environments whose event volume is unbounded from this controller's perspective, and a cluster-wide list is an unbounded decode cost for the memory-limited control plane.

On each tick, the reconciler SHALL observe all sandbox Pods carrying `agents.x-k8s.io/sandbox-name-hash` in namespaces assigned to live Gateways on its managed cluster. It SHALL include Pods in `Pending`, `Running`, `Succeeded`, `Failed`, and `Unknown` phases. It SHALL NOT use the active-sandbox definition from `openshell-gateway-sandbox-count.spec.md` to exclude terminated Pods.

The reconciler SHALL correlate a selected Pod to its Sandbox custom resource using a `Sandbox` owner reference in the same namespace. When that owner reference is absent, it MAY use a same-namespace, same-name Sandbox custom resource fallback. A Pod for which no sandbox name can be resolved SHALL NOT be reported. The label identifies a candidate Pod; it SHALL NOT by itself select an unrelated Event.

The reconciler SHALL group correlated Pods by Gateway namespace and sandbox name before it selects Events. For every Pod in a group, it SHALL select Kubernetes Events whose involved object identifies that Pod by UID when available, and otherwise by the Pod name and namespace. It SHALL combine the matching Events for the group, sort them from newest to oldest using Event time, then the Kubernetes series or deprecated last-observed time, then creation time as available, and report one snapshot containing at most the most recent `N` Events for that sandbox. The selection SHALL be deterministic when timestamps tie.

#### Scenario: OSG-SEVT-01.01 -- Failed sandbox events are included

- GIVEN a live Gateway namespace contains a sandbox Pod with the sandbox label
- AND the Pod phase is `Failed`
- AND Kubernetes retains two Events for that Pod
- WHEN the sandbox-events reconciler runs
- THEN it SHALL select the two Events for that sandbox
- AND it SHALL report them in the sandbox Event snapshot

#### Scenario: OSG-SEVT-01.02 -- Most recent N Events are selected

- GIVEN a correlated sandbox Pod has `N + 3` matching Kubernetes Events
- WHEN the sandbox-events reconciler runs
- THEN it SHALL report exactly the newest `N` Events
- AND it SHALL not report the three older Events

#### Scenario: OSG-SEVT-01.03 -- A non-sandbox Pod Event is ignored

- GIVEN a Pod in a live Gateway namespace has no `agents.x-k8s.io/sandbox-name-hash` label
- WHEN the sandbox-events reconciler runs
- THEN it SHALL NOT report Events for that Pod

#### Scenario: OSG-SEVT-01.04 -- Sandbox Pods share one complete snapshot

- GIVEN two correlated Pods in one live Gateway namespace have the same sandbox name
- AND Kubernetes retains matching Events for both Pods
- WHEN the sandbox-events reconciler runs
- THEN it SHALL send one snapshot for that sandbox name
- AND the snapshot SHALL contain the newest `N` Events across both Pods

---

### Requirement: OSG-SEVT-02 -- Cluster-Scoped Full-Snapshot gRPC Report

The Gateway gRPC service SHALL expose `ReportSandboxEvents`. Its request SHALL contain the Gateway namespace, the correlated sandbox name, and a repeated bounded Event representation. The representation SHALL include the Event fields needed by the REST response and SHALL NOT include Kubernetes credentials, Secret data, or an unbounded arbitrary object payload.

`ReportSandboxEvents` SHALL have SET semantics. A successful request SHALL atomically replace the cached Event snapshot for the request key with the complete request value. Repeating the same request SHALL have the same result. An empty Event list SHALL replace a prior snapshot with an empty snapshot, so Events removed from Kubernetes do not remain until an unrelated overwrite. The API server SHALL enforce the per-sandbox `N` limit on received input even when the caller sends more items.

The control plane SHALL bound each `ReportSandboxEvents` call with a ten-second RPC timeout. A failed or timed-out call SHALL not stop subsequent sandbox reports or the next periodic tick.

A registered control-plane identity SHALL be authorized to call `ReportSandboxEvents` only through the explicit `GatewayService` allowlist in the default-deny `authorizeControlPlaneGRPC` switch. That switch SHALL validate the request type and SHALL call `requireNamespaceOnCluster` with the request namespace and caller cluster identity. A caller for another cluster SHALL be denied. A missing gateway namespace MAY remain a no-op, consistent with `requireNamespaceOnCluster`; it SHALL NOT create an orphaned-namespace cache entry.

The protobuf definition and generated gRPC stubs SHALL be regenerated with `make proto` when this RPC changes.

#### Scenario: OSG-SEVT-02.01 -- A complete snapshot replaces stale Events

- GIVEN the cache holds Events `A` and `B` for namespace `openshell-a` and sandbox `writer`
- WHEN the control plane reports a successful snapshot containing only Event `C` for that key
- THEN a later read SHALL contain Event `C`
- AND it SHALL NOT contain Events `A` or `B`

#### Scenario: OSG-SEVT-02.02 -- A foreign control plane cannot report Events

- GIVEN namespace `openshell-a` belongs to cluster `cluster-a`
- AND a registered control plane for `cluster-b` calls `ReportSandboxEvents` for `openshell-a`
- WHEN gRPC authorization evaluates the request
- THEN the call SHALL be denied
- AND the cache SHALL remain unchanged

---

### Requirement: OSG-SEVT-03 -- TTL-Bound Sandbox Event Cache

The API server SHALL provide a small `SandboxEventStore` interface with operations to replace and read a snapshot by `(gateway namespace, sandbox name)`, and to list unexpired snapshots for one Gateway namespace. The default implementation SHALL be registered through `environments.Services` and SHALL keep snapshots only in API-server process memory. Handlers and gRPC services SHALL depend on the interface, not on the in-memory implementation, so a Valkey or Redis look-aside implementation can replace the backing store later.

The store key SHALL contain both the Gateway namespace and sandbox name. It SHALL NOT key only by sandbox name. Each stored value SHALL contain at most `N` Events. A successful replacement SHALL refresh the entry TTL. The default TTL SHALL be ten minutes and SHALL be configurable as a finite positive duration.

The in-memory implementation SHALL evict expired entries before returning them and SHALL periodically sweep expired entries. It SHALL have a configured finite maximum entry count. When a replacement would exceed that count, it SHALL evict an expired entry first, or otherwise the least-recently-refreshed entry. A cache read of an expired or evicted entry SHALL behave as no Event snapshot, not as an error.

The cache SHALL NOT write sandbox Events to PostgreSQL, emit them as OTLP or Prometheus data, or place sandbox identifiers, namespaces, Event messages, or Event reasons in metric labels or telemetry attributes. This enforces OBS-CPL-06 and keeps string Event payloads outside cardinality-sensitive telemetry.

Version 1 SHALL run with one API-server replica. A process restart, a cache eviction, or a request sent to a different replica may produce an empty response until the next report. Deployments requiring multiple replicas SHALL use a shared cache implementation before enabling this feature on more than one replica.

#### Scenario: OSG-SEVT-03.01 -- Cache key separates equal sandbox names

- GIVEN Gateway namespace `openshell-a` and Gateway namespace `openshell-b` each have a sandbox named `writer`
- WHEN the API server receives different snapshots for each namespace
- THEN a read for the first Gateway SHALL return only the `openshell-a` snapshot
- AND a read for the second Gateway SHALL return only the `openshell-b` snapshot

#### Scenario: OSG-SEVT-03.02 -- TTL expiry removes a snapshot

- GIVEN the cache stores a snapshot for a sandbox
- AND its TTL expires without a successful replacement
- WHEN an authorized user reads that Gateway's sandbox Events
- THEN the response SHALL not include that sandbox snapshot

---

### Requirement: OSG-SEVT-04 -- Gateway-Scoped REST Read API

The API server SHALL expose `GET /api/hypershell/v1/gateways/{gateway_id}/sandbox_events`. The response SHALL group Event snapshots by sandbox name. Each group SHALL include the sandbox name and its bounded ordered Event list. Every Event SHALL use the Sandbox Event representation in Domain Vocabulary. The Event list SHALL preserve the newest-to-oldest order reported by OSG-SEVT-01. The response MAY include an observation time for each snapshot so callers can determine its freshness.

An authorized read with no current cache entries SHALL return HTTP `200` and an empty group collection. It SHALL NOT return stale data after a TTL expiry, a cache eviction, or an API-server restart. The endpoint SHALL be REST only in version 1; it SHALL NOT add a browser UI, UI mockup, per-sandbox route, or streaming endpoint.

The OpenAPI document SHALL define the route, response schemas, Event fields, empty response, and HTTP error responses. `make generate` SHALL regenerate the Go and TypeScript SDK clients after that document changes.

#### Scenario: OSG-SEVT-04.01 -- Read groups Events by sandbox

- GIVEN a Gateway has cached snapshots for sandboxes `writer` and `reviewer`
- WHEN an authorized caller sends `GET /api/hypershell/v1/gateways/{gateway_id}/sandbox_events`
- THEN the response SHALL be HTTP `200`
- AND it SHALL contain one group named `writer`
- AND it SHALL contain one group named `reviewer`

#### Scenario: OSG-SEVT-04.02 -- Empty cache is a successful empty read

- GIVEN an authorized caller's Gateway has no unexpired sandbox Event snapshots
- WHEN the caller sends the sandbox-events GET request
- THEN the response SHALL be HTTP `200`
- AND its group collection SHALL be empty

---

### Requirement: OSG-SEVT-05 -- Gateway-Binding Authorization and Existence Hiding

The sandbox-events route SHALL be classified as a Gateway-scoped nested route in `extractResourceInfoFromRoute`, mirroring the gateway access facade. The authorization layer SHALL extract `{gateway_id}` as the parent Gateway identity before it evaluates the request.

A caller with a `gateway:viewer`, `gateway:admin`, or `gateway:owner` binding on the requested Gateway SHALL be authorized to read its sandbox Events. A caller with no binding on that Gateway SHALL receive HTTP `404`, including when the Gateway exists. `platform:admin` alone SHALL NOT authorize this endpoint because it does not grant a Gateway binding. This endpoint therefore does not disclose sandbox diagnostics or Gateway existence to non-bound users.

The authorization decision SHALL be made before the handler reads a cache entry. A denied request SHALL not reveal whether the Gateway has snapshots, sandboxes, or recent failures.

#### Scenario: OSG-SEVT-05.01 -- Gateway viewer reads Events

- GIVEN user V has a `gateway:viewer` binding on gateway `gw-1`
- AND `gw-1` has an unexpired sandbox Event snapshot
- WHEN user V sends `GET /api/hypershell/v1/gateways/gw-1/sandbox_events`
- THEN the response SHALL be HTTP `200`
- AND it SHALL include the Event group

#### Scenario: OSG-SEVT-05.02 -- Non-bound user cannot discover Events

- GIVEN user U has no binding on gateway `gw-1`
- WHEN user U sends `GET /api/hypershell/v1/gateways/gw-1/sandbox_events`
- THEN the response SHALL be HTTP `404`
- AND the response SHALL not reveal whether `gw-1` exists or has sandbox Events

---

### Requirement: OSG-SEVT-06 -- Verification

The control plane SHALL include unit tests that cover:

- selection of labeled sandbox Pods across active and terminated phases;
- Sandbox owner-reference correlation and the documented same-name fallback;
- Event association with the involved Pod and deterministic most-recent-`N` selection;
- full-snapshot replacement, including an empty snapshot clearing a prior value;
- ten-second bounded RPC context behavior and recovery on a failed report; and
- `ReportSandboxEvents` authorization through the default-deny switch, including same-cluster allow and foreign-cluster deny cases.

The API server SHALL include unit tests that cover:

- store key isolation, namespace listing, per-sandbox event cap, TTL expiry, expiry sweep, and capacity eviction;
- handler grouping, newest-to-oldest Event ordering, the defined Event representation, and an empty HTTP `200` response;
- authorization of viewer, admin, and owner bindings; and
- HTTP `404` for an unbound caller and for a `platform:admin` caller with no Gateway binding.

The generated OpenAPI and gRPC artifacts SHALL be verified as current after `make generate` and `make proto`.

#### Scenario: OSG-SEVT-06.01 -- CI rejects an unprotected nested route

- GIVEN the sandbox-events handler is registered
- WHEN API-server authorization tests run
- THEN they SHALL assert that the route resolves its parent `gateway_id`
- AND they SHALL assert that an unbound caller receives HTTP `404`

## Success Criteria

- **SC-001:** A Gateway-bound viewer can retrieve recent Events for all sandboxes of that Gateway through one REST request.
- **SC-002:** A failed or succeeded sandbox Pod can contribute retained Kubernetes Events until the cache TTL expires.
- **SC-003:** A periodic full snapshot corrects stale cache entries without requiring an Event-driven hub write.
- **SC-004:** A control plane cannot report sandbox Events for a Gateway namespace owned by another cluster.
- **SC-005:** A non-bound caller receives HTTP `404` and cannot discover sandbox diagnostics.
- **SC-006:** The feature adds no PostgreSQL persistence, metrics, telemetry payload, or UI surface.

## Assumptions

- The control plane has list permission for Pods, Sandbox custom resources, and Events in namespaces for the Gateways assigned to its managed cluster.
- Sandbox Pods retain the `agents.x-k8s.io/sandbox-name-hash` label and normally have a `Sandbox` owner reference.
- Kubernetes may garbage-collect Events and terminated Pods before the next tick; the API can expose only data that Kubernetes still provides and the cache retains.
- A one-replica API server is acceptable for version 1. A shared backing store will be selected before high availability is required.

## Alternatives Considered

| Alternative | Why rejected |
| --- | --- |
| Persist Events in PostgreSQL | Events are short-lived diagnostic observations, not durable HyperShell resource state. Persistence adds schema, retention, and deletion obligations. |
| Emit one metric or telemetry series per sandbox Event | Event text and sandbox identities have unbounded cardinality and have no per-user entitlement model. This violates OBS-CPL-06. |
| Push each Kubernetes Event as it arrives | A periodic snapshot is simpler, idempotent, self-healing, and corrects missed delivery. |
| Query the spoke cluster during each user REST request | The API server does not hold per-cluster credentials and cannot safely make a user request depend on spoke reachability. |
| Provide a per-sandbox REST endpoint first | The Gateway-scoped endpoint needs one authorization decision and provides the data needed for the first consumer. |
| Use an API-server local cache with multiple replicas | A process-local cache cannot provide a consistent response across replicas. A shared Valkey or Redis backing is required first. |
