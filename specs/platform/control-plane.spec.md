# Control Plane

**Date:** 2026-08-03
**Status:** Active
**Related:** `managed-cluster-registration.spec.md` (registration API, watch-stream caller binding), `hub-grpc-tls.spec.md` (hub-side gRPC TLS Route and certificate, HYPERSHELL-333), `security/rbac-enforcement.spec.md` (`managed-cluster-registrar`), `gateway-access-management.spec.md` (GAM-02 Keycloak Role Bridge union reconciliation)

## Overview

The HyperShell control plane is a Go service that watches the API server via gRPC streaming RPCs and reconciles the desired state (Gateway and related resources in the database) into actual Kubernetes resources on the cluster it runs in. It follows the informer-reconciler pattern without depending on controller-runtime.

Every control plane is a registered spoke of exactly one hub. There is one control plane per managed cluster, it runs inside that cluster, and it reconciles only the gateways assigned to its own `cluster_id`. The control plane co-located with the hub API server is not special: it registers, filters, and dials the hub exactly like a control plane on a remote cluster.

## Architecture

```
API Server (PostgreSQL)                      -- hub
  │  gRPC watch streams per Kind, filtered by cluster_id
  ▼
Control Plane (Watcher + Reconciler)         -- one per managed cluster
  │  reconciles into K8s resources with in-cluster credentials
  ▼
Managed Cluster (Gateway pods, Services, Configs)
```

## Components

### Watcher

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Gateways, GatewayReleases, ManagedClusters, GatewayNetworks). On each event (create, update, delete), it dispatches to the Reconciler. Gateway events are dispatched through a per-gateway reconcile queue drained by a bounded worker pool: work for a single gateway is serialized while distinct gateways reconcile concurrently up to a configurable cap. The pool size and its configuration are defined in [`gateway-reconcile-concurrency.spec.md`](./gateway-reconcile-concurrency.spec.md).

### Reconciler

The Reconciler receives resource events from the Watcher and converges the Kubernetes state on its cluster to match. Key responsibilities:

- Deploy/update Gateway workloads on this cluster
- Provision per-gateway PostgreSQL databases and roles on the platform's gateway database server, using the admin credential Secret mounted into the controller
- Configure TLS certificates via cert-manager
- Create GRPCRoute and BackendTLSPolicy for external gateway exposure
- Inject OIDC authentication configuration into gateway deployments
- Configure network meshes between gateways
- Manage release rollouts (including canary strategies)
- Provision and reconcile OpenShellGatewayServiceAccount identities in Keycloak through an internal in-cluster gRPC service
- Update resource status back to the API server

#### Gateway Provisioning Specifications

Gateway reconciliation is defined in detail across dedicated sub-specs:

| Sub-Spec | Scope |
|---|---|
| [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) | Core provisioning: GatewayReconciler, manifest templating, deployment resources, RBAC, OpenShift adjustments |
| [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) | Per-gateway PostgreSQL provisioning from the mounted admin credential Secret (admin TLS `verify-full`, gateway `require`), credential security, cleanup |
| [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) | TLS certificate management via cert-manager, SAN management, cert rotation |
| [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) | External connectivity: Gateway API (GRPCRoute + BackendTLSPolicy), NetworkPolicy |
| [`openshell-gateway-oidc.spec.md`](./openshell-gateway-oidc.spec.md) | OIDC authentication, role validation, gateway.toml injection |
| [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) | Phase lifecycle, workload-readiness gating, continuous health reconciliation |
| [`gateway-version-selection.spec.md`](./gateway-version-selection.spec.md) | Database-backed version selection: resolving `release_id` to a GatewayRelease image and its precedence over a direct image |
| [`gateway-release-reconciliation.spec.md`](./gateway-release-reconciliation.spec.md) | GatewayRelease reconciliation: image validation, deterministic release status, change propagation to referencing gateways |
| [`gateway-release-rollout.spec.md`](./gateway-release-rollout.spec.md) | Safe release rollout: last-good workload preserved until the new revision passes health gates, revision-aware readiness, observed-release and rollout-state reporting, timeout/failure handling |
| [`gateway-network-reconciliation.spec.md`](./gateway-network-reconciliation.spec.md) | GatewayNetwork reconciliation: topology vocabulary, topology/hub coherence and hub-reference validation, deterministic network status write-back |

### Config

Holds connection configuration for the API server (REST URL for registration, gRPC address for watch streams), the control plane's own cluster name and OIDC client credentials, Kubernetes client initialization, and the internal service-account provisioner. The provisioner listens on an in-cluster gRPC port that a NetworkPolicy restricts to the API server pod. The API server does not receive Keycloak administrator credentials. `HYPERSHELL_GRPC_SERVER_ADDR` selects the gRPC transport credentials (plaintext vs. TLS) as described in "Requirement: gRPC Transport Security" below.

## Requirements

### Requirement: Mandatory Cluster Identity

Every control plane SHALL have a cluster identity, resolved at startup by calling
`POST /api/hypershell/v1/managed_clusters/registration` with its OIDC
`client_credentials` token. There is no unidentified or unfiltered mode of operation:
the control plane co-located with the hub API server registers exactly like a
control plane on a remote cluster.

`HYPERSHELL_MANAGED_CLUSTER_NAME` (unique per control plane, set in gitops) and the
OIDC client credentials (`OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`) are
required configuration. When either is missing the control plane SHALL log a fatal
error naming the missing variable and exit non-zero before opening any Kubernetes or
gRPC connection. The control plane SHALL NOT read a cluster id from configuration:
`HYPERSHELL_CLUSTER_ID` SHALL NOT be consulted and SHALL NOT appear in gitops.

Each control plane SHALL use its own OIDC client, so that its JWT `sub` claim is
unique across the fleet; the registration upsert key and the watch-stream caller
binding (`managed-cluster-registration.spec.md`) both depend on that uniqueness.

The registration endpoint is idempotent; the returned `cluster_id` is stable across
restarts. The control plane SHALL use this `cluster_id` as the cluster filter for
every gateway-scoped operation for the lifetime of the process: the `WatchGateways`
stream, the initial `ListGateways` seed, the health reconciler, the sandbox-count
reconciler, the instance-label backfill, the orphaned-namespace garbage collector
(`openshell-gateway-namespace-gc.spec.md`), the GatewayRelease change fan-out
(`gateway-release-reconciliation.spec.md`), and the RoleBinding-to-Keycloak role sync.
None of these SHALL ever run without the filter. The control plane SHALL open the
`WatchRoleBindings` stream, and SHALL call `ListRoleBindings`, with its `cluster_id`;
the api-server delivers only bindings whose gateway is assigned to that cluster, and
no global bindings (`managed-cluster-registration.spec.md`, "Watch Stream Caller
Binding"). As defense in depth, the control plane SHALL additionally resolve each
binding's gateway and verify it is assigned to its own `cluster_id` before touching
Keycloak, since only that control plane owns the gateway's Keycloak client.

After startup, the control plane SHALL call `/registration` on a regular interval
(default: 60 seconds) to update `last_seen_at` on the hub. These subsequent calls are
no-ops for registration data and return the same `cluster_id`. See
`managed-cluster-registration.spec.md` for full registration semantics.

#### Scenario: Successful startup registration

- GIVEN the control plane's OIDC client has `managed-cluster-registrar` in Keycloak
- AND `HYPERSHELL_MANAGED_CLUSTER_NAME` is set
- WHEN the control plane starts
- THEN it calls `POST /managed_clusters/registration` before opening `WatchGateways`
- AND uses the returned `cluster_id` to filter the watch stream, the seed list, and every reconciler to this cluster's gateways

#### Scenario: Missing cluster name refuses to start

- GIVEN `HYPERSHELL_MANAGED_CLUSTER_NAME` is unset
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming `HYPERSHELL_MANAGED_CLUSTER_NAME` and exit non-zero
- AND it SHALL NOT open any watch stream

#### Scenario: Missing OIDC credentials refuse to start

- GIVEN `HYPERSHELL_MANAGED_CLUSTER_NAME` is set
- AND `OIDC_ISSUER` or `OIDC_CLIENT_SECRET` is unset
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming the missing variable and exit non-zero
- AND it SHALL NOT open any watch stream

#### Scenario: Transient failure retried with backoff

- GIVEN the API server is temporarily unreachable at startup
- WHEN the control plane attempts to register
- THEN it SHALL retry with exponential backoff
- AND it SHALL NOT open `WatchGateways` until registration succeeds

#### Scenario: 403 exits immediately

- GIVEN the API server returns 403 (managed-cluster-registrar not assigned in Keycloak)
- WHEN the control plane attempts to start
- THEN it SHALL NOT retry and SHALL NOT open `WatchGateways`
- AND it SHALL log a clear error identifying the missing role and exit

#### Scenario: 409 exits immediately

- GIVEN the API server returns 409 (the name is taken by a record this OIDC subject does not own, or this subject is registered under another name)
- WHEN the control plane attempts to start
- THEN it SHALL NOT retry and SHALL NOT open `WatchGateways`
- AND it SHALL log a clear error quoting the API's conflict message and exit

#### Scenario: RoleBinding for another cluster's gateway is ignored

- GIVEN control planes registered as `cluster_id: X` and `cluster_id: Y`
- AND a gateway assigned to `Y`
- WHEN a RoleBinding for that gateway is created
- THEN only the control plane registered as `Y` SHALL assign the Keycloak client roles
- AND the api-server SHALL NOT deliver the event on the `WatchRoleBindings` stream of the control plane registered as `X`
- AND should such an event reach `X` anyway, `X` SHALL skip it without error

#### Scenario: Re-registration after restart returns same cluster_id

- GIVEN a control plane that previously registered and received `cluster_id: X`
- WHEN it restarts and calls `/registration` again
- THEN the response is 200 with the same `cluster_id: X`
- AND `last_seen_at` is updated on the `ManagedCluster` record

### Requirement: gRPC Watch Streams

The control plane SHALL connect to the API server via gRPC watch streams for each resource Kind. Every RPC, including the long-lived watch streams, SHALL carry the control plane's OIDC bearer token as per-RPC credentials; the API server does not exempt watch streams from authentication (`hub-grpc-tls.spec.md`, "Requirement: Authenticated Watch Streams Before Exposure"). On connection failure, it SHALL reconnect with exponential backoff.

#### Scenario: Watch Reconnection
- GIVEN the API server becomes unreachable
- WHEN the gRPC stream disconnects
- THEN the watcher SHALL retry with exponential backoff
- AND resume processing from the last known state

#### Scenario: Watch is opened with the control plane's own cluster filter
- GIVEN a control plane registered as `cluster_id: X`
- WHEN it opens `WatchGateways`
- THEN the request SHALL carry `cluster_id: X` and the control plane's bearer token
- AND events for gateways assigned to other clusters SHALL NOT be delivered

### Requirement: Watch Stream Liveness

A watch stream SHALL NOT be trusted to stay healthy merely because it has not
returned an error. A long-lived stream can go silent -- deliver neither events
nor an error -- when an intermediary (a Route, a load balancer, or the API
server event broker) drops the server-to-client flow of an otherwise idle
stream. A blocked receive on such a stream never reconnects, never re-lists, and
so stops reconciling without any visible failure. The control plane SHALL detect
this condition at two layers and recover from it without operator intervention.

At the connection layer, the control plane SHALL configure HTTP/2 keepalive on
the gRPC client connection it dials to the API server, so a wedged or half-open
connection surfaces as a transport error rather than an indefinite silent block.
The keepalive SHALL send a ping on an idle connection and SHALL permit the ping
while no stream is active, so a connection carrying only idle watch streams is
still probed. All watch streams multiplex over this one connection, so the
keepalive protects every stream.

At the stream layer, each watch stream SHALL run under an idle watchdog. When a
stream delivers neither an event nor an error for a configured maximum idle
interval, the control plane SHALL cancel that stream attempt and reconnect
through the existing backoff path, rather than remain blocked on the receive.
Each received event SHALL reset the watchdog. The idle interval SHALL be long
enough that an ordinarily quiet stream (for example, a RoleBinding stream with no
membership changes) is not torn down needlessly, and SHALL be configurable.

A reconnect caused by a keepalive failure or an idle-watchdog timeout SHALL be
treated like any other watch-stream disconnect: logged and retried with
exponential backoff, and SHALL re-run the stream's seed or resync on reconnect
so any state missed while the stream was silent is recovered.

#### Scenario: Idle connection is kept alive

- GIVEN the control plane has dialed the API server and holds only idle watch streams
- WHEN no events flow for longer than the keepalive interval
- THEN the client SHALL send an HTTP/2 keepalive ping
- AND a connection that no longer carries traffic end-to-end SHALL surface as a transport error that the watchers retry with backoff

#### Scenario: Silent stream is detected and reconnected

- GIVEN a watch stream that has stopped delivering events but has returned no error
- WHEN no event arrives for the configured maximum idle interval
- THEN the control plane SHALL cancel the stream attempt and reconnect with backoff
- AND it SHALL re-run the stream's seed or resync so state missed during the silence is recovered

#### Scenario: Active stream is not torn down

- GIVEN a watch stream that is delivering events within the configured idle interval
- WHEN each event resets the idle watchdog
- THEN the control plane SHALL NOT cancel the stream
- AND it SHALL continue to process events on the same stream

### Requirement: Role Binding Resync

The assignment of a gateway's owner Keycloak client roles SHALL NOT depend solely
on a single long-lived `WatchRoleBindings` stream. The RoleBinding stream is
almost always idle (it sends an event only when a binding changes), so a single
missed or silently-dropped event would otherwise leave the gateway creator
without `openshell-admin` and `openshell-user`, and the gateway console would
refuse the user with `role 'openshell-user' required`. Unlike the gateway watch,
which re-lists its inventory on every reconnect to recover state the stream never
replays, the owner grant has no equivalent list-driven recovery path.

The control plane SHALL seed the RoleBinding reconciler from a `ListRoleBindings`
call, scoped to its own `cluster_id` (`managed-cluster-registration.spec.md`,
"Watch Stream Caller Binding"), on every `WatchRoleBindings` (re)connect, and
SHALL additionally re-list on a regular configurable interval while the stream is
connected. Each listed binding SHALL be reconciled through the same union
reconciliation as a live event (`gateway-access-management.spec.md`, GAM-02), so
the bound user's Keycloak client roles converge to the correct set. The resync
SHALL be idempotent: a binding whose roles are already assigned SHALL produce no
change.

Because a plain `ListRoleBindings` reports only surviving bindings, the resync
SHALL close the list-watch gap the same way the gateway watch does: it SHALL wait
for the watch subscription to be live before listing (so an event that fires
during the list is not lost), and it SHALL rely on the stream, not the list, to
deliver deletions. The resync restores missing assignments; it SHALL NOT be the
sole mechanism for revoking roles a deleted binding no longer backs.

#### Scenario: Owner grant recovered after a missed create event

- GIVEN a gateway was created on this control plane's cluster and its `gateway:owner` RoleBinding exists
- AND the `WatchRoleBindings` stream never delivered the create event (it was silent)
- WHEN the control plane reconnects the stream, or the periodic resync interval elapses
- THEN the control plane SHALL list the role bindings for its cluster and reconcile the owner binding
- AND SHALL assign `openshell-admin` and `openshell-user` to the owner on the gateway's Keycloak client
- AND the gateway console SHALL no longer refuse the owner with `role 'openshell-user' required`

#### Scenario: Resync is idempotent

- GIVEN every RoleBinding for this cluster is already reflected in Keycloak
- WHEN the periodic resync lists and reconciles them again
- THEN the control plane SHALL make no change to any user's Keycloak client roles

#### Scenario: Resync does not run without the cluster filter

- GIVEN the control plane's registered `cluster_id` is empty
- WHEN the RoleBinding seed or resync attempts to list
- THEN it SHALL be refused rather than issue an unfiltered list (`managed-cluster-registration.spec.md`)

### Requirement: gRPC Transport Security

The control plane SHALL select gRPC transport credentials from `HYPERSHELL_GRPC_SERVER_ADDR`
at dial time, without a separate TLS-enable flag:

- The address is **in-cluster** (plaintext, `insecure.NewCredentials()`) when its host is
  `localhost` or a loopback IP, contains no dot at all (a bare Kubernetes Service short
  name such as `hypershell-api-server:9000`), ends with `.svc`, or contains `.svc.`
  (namespace-qualified Service names under any cluster domain, for example
  `hypershell-api-server.hypershell-system.svc.cluster.local:9000`). None of these names
  resolve outside their own cluster, so they are exactly as trustworthy as each other.
- Any other address is **external** (TLS, `credentials.NewTLS()` with the system trust store
  and standard hostname verification) -- for example
  `grpc.hyp0.infra.hypershell.app:443`, the passthrough Route address every control plane
  of a production hub dials, including the control plane co-located with that hub. See
  `hub-grpc-tls.spec.md` for the hub-side Route and certificate this depends on.

This is a dial-time classification of one address, not a global mode switch: a control plane
dials exactly one `HYPERSHELL_GRPC_SERVER_ADDR` for its lifetime, so no additional
configuration knob is introduced. In-cluster plaintext addresses are a development
convenience for environments whose api-server runs with gRPC TLS disabled (Kind,
`make openshift-up` namespaces); a production hub's gRPC listener is TLS-only.

A TLS handshake failure (invalid or expired certificate, hostname mismatch) SHALL be treated
like any other watch-stream connection failure: logged and retried with exponential backoff,
not fatal. It is not fail-closed the way missing `managed-cluster-registrar` is (see
`managed-cluster-registration.spec.md`), since a certificate renewal on the hub should not
require an operator to restart the control plane.

#### Scenario: In-cluster address dials plaintext

- GIVEN `HYPERSHELL_GRPC_SERVER_ADDR=hypershell-api-server.hypershell-system.svc.cluster.local:9000`
- WHEN the control plane dials the gRPC connection
- THEN it SHALL use `insecure.NewCredentials()`

#### Scenario: Namespace-qualified Service address dials plaintext

- GIVEN `HYPERSHELL_GRPC_SERVER_ADDR=hypershell-api-server.hypershell-system.svc:9000`
- WHEN the control plane dials the gRPC connection
- THEN it SHALL use `insecure.NewCredentials()`

#### Scenario: Bare Service short name dials plaintext

- GIVEN `HYPERSHELL_GRPC_SERVER_ADDR=hypershell-api-server:9000`
- WHEN the control plane dials the gRPC connection
- THEN it SHALL use `insecure.NewCredentials()`

#### Scenario: External address dials TLS

- GIVEN `HYPERSHELL_GRPC_SERVER_ADDR=grpc.hyp0.infra.hypershell.app:443`
- WHEN the control plane dials the gRPC connection
- THEN it SHALL use `credentials.NewTLS()` with standard certificate and hostname verification

#### Scenario: Control plane completes handshake against an external TLS endpoint

- GIVEN a control plane configured with an external `HYPERSHELL_GRPC_SERVER_ADDR`
- AND the hub api-server presents a valid certificate for that hostname
- WHEN the control plane opens `WatchGateways`
- THEN the TLS handshake SHALL complete
- AND the watch stream SHALL deliver events identically to an in-cluster plaintext connection

#### Scenario: TLS handshake failure retries, does not exit

- GIVEN a control plane configured with an external `HYPERSHELL_GRPC_SERVER_ADDR`
- AND the hub's certificate is expired or does not match the configured hostname
- WHEN the control plane attempts to open a watch stream
- THEN the connection attempt SHALL fail
- AND the control plane SHALL log the error and retry with exponential backoff
- AND it SHALL NOT exit the process

### Requirement: Gateway Reconciliation

The control plane SHALL reconcile Gateway resources assigned to its own `cluster_id` into Kubernetes Deployments, Services, ConfigMaps, and supporting resources on the cluster it runs in, using its in-cluster credentials. It SHALL NOT hold or use credentials for any other cluster. Full provisioning details are defined in the [gateway sub-specs](./openshell-gateway.spec.md).

#### Scenario: New Gateway Created
- GIVEN a new Gateway resource with this control plane's `cluster_id` appears via the watch stream
- WHEN the reconciler processes it
- THEN it SHALL create the corresponding K8s resources on this cluster:
  - A per-gateway PostgreSQL database and role on the gateway database server (admin credentials read from the mounted Secret) and the tenant credentials Secret - see [database spec](./openshell-gateway-database.spec.md)
  - cert-manager Issuer and Certificate resources for TLS - see [TLS spec](./openshell-gateway-tls.spec.md)
  - JWT key generation Job (`openshell-gateway-certgen`)
  - Gateway Deployment, Service, ServiceAccounts, Roles, RoleBindings, ConfigMap, NetworkPolicies
  - GRPCRoute and BackendTLSPolicy (when `route` field is set) - see [routing spec](./openshell-gateway-routing.spec.md)
  - OIDC configuration in gateway.toml (when `oidc.issuer` is set) - see [OIDC spec](./openshell-gateway-oidc.spec.md)
- AND set the Gateway's `phase` to `Provisioning` while applying manifests, and to `Running` only after the `openshell-gateway` Deployment is observed Ready - see [health spec](./openshell-gateway-health.spec.md)

### Requirement: Gateway Database Admin Credentials

The control plane SHALL read the gateway database server's administrative connection from the Secret mounted at `GATEWAY_DATABASE_ADMIN_DIR` (default `/etc/hypershell/gateway-database`), SHALL validate the mounted files at startup and refuse to start when they are invalid, and SHALL NOT model the database server as an API resource. There is no database reconciler: per-gateway databases are provisioned and cleaned up by the GatewayReconciler.

#### Scenario: Controller starts with valid admin credential files
- GIVEN the admin Secret is mounted with `host`, `port`, `user`, `password` and a PEM `sslrootcert`
- WHEN the control plane starts
- THEN it SHALL validate the files without connecting to the server
- AND it SHALL serve watch streams for Gateways, GatewayReleases, ManagedClusters and GatewayNetworks

#### Scenario: Controller refuses to start on invalid admin credential files
- GIVEN a required admin credential file is missing, or `sslmode` is present with a value other than `verify-full`
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming the file and exit non-zero

See [database spec](./openshell-gateway-database.spec.md) for full provisioning details.

### Requirement: Resource Cleanup

When a Gateway is deleted, the control plane SHALL clean up all associated Kubernetes resources on its cluster.

#### Scenario: Gateway Deletion
- GIVEN a Gateway deletion event from the watch stream
- WHEN the reconciler processes it
- THEN it SHALL delete all K8s resources associated with that Gateway
- AND confirm cleanup completion

### Requirement: Status Synchronization

The control plane SHALL continuously reconcile the `phase` and `status` fields of Gateway resources in the API server to reflect actual cluster state, even after a Gateway has reached `Running`. The provisioning gate that prevents redundant re-provisioning SHALL NOT suppress these health updates, and SHALL gate re-application on convergence (`observed_generation == generation`) rather than on `phase`, so that a desired-spec change falls through the gate and triggers re-application regardless of phase. Full lifecycle semantics are defined in the [health spec](./openshell-gateway-health.spec.md).

#### Scenario: Gateway Health Check
- GIVEN a Gateway with `phase` `Running` on this cluster
- WHEN the control plane observes its `openshell-gateway` Deployment health
- THEN it SHALL update the Gateway's `status` in the API server
- AND set `phase` to `Degraded` when ready replicas fall below desired
- AND set `phase` back to `Running` when the workload recovers

#### Scenario: Spec Change to a Running Gateway
- GIVEN a converged Gateway with `phase` `Running` (`observed_generation == generation`)
- WHEN its desired spec changes and the API server advances `generation`
- THEN the control plane SHALL re-apply the gateway manifests despite the `Running` phase
- AND set `observed_generation` to the applied `generation` upon success

## Known Limitations

- **Service-account provisioner is hub-local.** The API server reaches the internal OpenShellGatewayServiceAccount provisioner at one in-cluster address, so only the control plane co-located with the hub can serve it. Gateways hosted on a remote cluster cannot yet have service accounts provisioned; see the Non-Goals of `hub-grpc-tls.spec.md`.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| gRPC watch streams (not polling) | Real-time event delivery, efficient resource usage |
| Separate module from API server | Independent lifecycle, separate deployment |
| No controller-runtime dependency | Lightweight, custom reconciliation without CRD overhead |
| Every control plane is a registered spoke; no single-cluster mode | One code path and one deployment shape. The hub's co-located control plane exercises registration, cluster filtering, authenticated watches, and TLS on every hub and in every development environment, so those paths are tested continuously instead of only at the first remote spoke. An unfiltered mode would also make server-side enforcement of cluster scoping impossible, because the server could not tell an unidentified hub controller from a misbehaving spoke. |
| Cluster identity is resolved by registration, never configured | A configured `HYPERSHELL_CLUSTER_ID` can drift from the record the hub holds, and a copy-pasted value silently makes two control planes fight over one cluster. Registration ties the id to the OIDC subject, which the hub can verify. |
| One OIDC client per control plane | The JWT `sub` is the registration key and the identity the watch-stream binding checks. Sharing a client across control planes would collapse them into one subject and break both. |
| One control plane per cluster, in-cluster credentials only | No hub holds a spoke kubeconfig, so a compromised hub cannot reach into spokes and a spoke keeps reconciling its own gateways while the hub is unreachable. Matches the platform GitOps pull model in `global-architecture.spec.md`. |
| Transport security inferred from the dial address, not a separate `HYPERSHELL_GRPC_TLS` flag | The address already tells you whether the hop leaves the cluster. A second flag could drift from the address (TLS off against an external host, or on against an in-cluster one) with no compile-time or admission check to catch it. |
| TLS handshake failure retries like any other disconnect, rather than exiting | A control plane should tolerate a hub certificate renewal without operator intervention. Contrast with the `managed-cluster-registrar` 403 and the name-conflict 409, which are fail-closed because retrying truly cannot help. |
