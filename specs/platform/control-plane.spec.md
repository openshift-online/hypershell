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

The Watcher establishes gRPC streaming connections to the API server for each resource Kind (Gateways, ManagedClusters, RoleBindings, and AgentRuntimes). On each event (create, update, delete), it dispatches to the Reconciler. Gateway events are dispatched through a per-gateway reconcile queue drained by a bounded worker pool: work for a single gateway is serialized while distinct gateways reconcile concurrently up to a configurable cap. The pool size and its configuration are defined in [`gateway-reconcile-concurrency.spec.md`](./gateway-reconcile-concurrency.spec.md). AgentRuntime events are dispatched to the AgentRuntime reconciler (see "Requirement: AgentRuntime Reconciliation" below).

### Reconciler

The Reconciler receives resource events from the Watcher and converges the Kubernetes state on its cluster to match. Key responsibilities:

- Deploy/update Gateway workloads on this cluster
- Provision per-gateway PostgreSQL databases and roles on the platform's gateway database server, using the admin credential Secret mounted into the controller
- Configure TLS certificates via cert-manager
- Create GRPCRoute and BackendTLSPolicy for external gateway exposure
- Inject OIDC authentication configuration into gateway deployments
- Provision and reconcile OpenShellGatewayServiceAccount identities in Keycloak through an internal in-cluster gRPC service
- Reconcile AgentRuntime resources into cluster scheduling resources and gateway-side workspace objects (see "AgentRuntime Reconciliation" below)
- Run the `RepositoryReconciler` (the declarative `.hypershell` import reconciler): for each registered `Repository`, read only its `import_path` subtree through the GitHub API (no git clone), render that subtree in memory with the shared kustomize library, and reconcile (create/update/prune) the rendered `Repository`-owned ext resources into the API over the gRPC write path (see "Declarative `.hypershell` Import Reconciliation" below)
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
(`openshell-gateway-namespace-gc.spec.md`), and the RoleBinding-to-Keycloak role sync.
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

#### Scenario: Startup seeds gateways created while the control plane was down
- GIVEN gateways for cluster `X` that were created while its control plane was not running
- WHEN the control plane starts and its `WatchGateways` subscription header arrives
- THEN it SHALL list cluster `X`'s gateways and queue each one for reconciliation before, and independently of, any watch event
- AND this is verified by `TestWatchGateways_StartupSeedsExistingGateways` (control-plane watcher) and `TestGRPCControlPlaneStartupListsExistingGateways` (api-server integration)

### Requirement: AgentRuntime Watch Stream

The control plane SHALL watch `AgentRuntime` resources over a `WatchAgentRuntimes`
gRPC stream, scoped to its own registered `cluster_id`, and SHALL seed that stream
from an initial `ListAgentRuntimes` call on every (re)connect. The stream SHALL
reuse the same reconnect-with-backoff, HTTP/2 keepalive, idle watchdog, and
seed-on-reconnect machinery as the existing watch streams ("gRPC Watch Streams",
"Watch Stream Liveness"). Every RPC SHALL carry the control plane's OIDC bearer
token as per-RPC credentials, like every other watch stream.

An `AgentRuntime` MAY omit `cluster_id`, in which case its scheduling cluster is
derived from its referenced `Gateway`'s capability-label placement (see
[`data-model.spec.md`](./data-model.spec.md), "AgentRuntime as a Top-Level
Resource"). The API server SHALL resolve this derived placement server-side and
SHALL deliver on each control plane's stream only the `AgentRuntime` records whose
effective cluster matches that control plane's `cluster_id`, exactly as it binds
the `WatchGateways` and `WatchRoleBindings` streams to the caller's cluster
(`managed-cluster-registration.spec.md`, "Watch Stream Caller Binding"). As
defense in depth, the control plane SHALL re-resolve each delivered
`AgentRuntime`'s effective cluster and SHALL skip, without error, any record that
does not resolve to its own `cluster_id`.

#### Scenario: AgentRuntime watch is opened with the control plane's own cluster filter

- GIVEN a control plane registered as `cluster_id: X`
- WHEN it opens `WatchAgentRuntimes`
- THEN the request SHALL carry `cluster_id: X` and the control plane's bearer token
- AND `AgentRuntime` records whose effective cluster is not `X` SHALL NOT be delivered

#### Scenario: Derived placement routes an AgentRuntime to the Gateway's cluster

- GIVEN an `AgentRuntime` with no `cluster_id` whose referenced `Gateway` is placed on `cluster_id: Y`
- WHEN control planes registered as `X` and `Y` each open `WatchAgentRuntimes`
- THEN only the control plane registered as `Y` SHALL receive the `AgentRuntime` event
- AND should the event reach `X` anyway, `X` SHALL skip it without error

#### Scenario: Seed on reconnect recovers missed AgentRuntime state

- GIVEN a `WatchAgentRuntimes` stream that went silent and was torn down by the idle watchdog
- WHEN the control plane reconnects the stream
- THEN it SHALL re-run `ListAgentRuntimes` scoped to its `cluster_id` before resuming the stream
- AND any `AgentRuntime` create or update missed during the silence SHALL be reconciled

### Requirement: AgentRuntime Reconciliation

The control plane SHALL reconcile each `AgentRuntime` delivered on its watch stream
into the cluster scheduling resources and gateway-side workspace objects defined by
[`data-model.spec.md`](./data-model.spec.md) ("AgentRuntime Cluster Resource
Provisioning", "Gateway-Side Provisioning", "WorkspaceMembership", and "AgentRuntime
Status Lifecycle"). Reconciliation SHALL converge, not create-and-skip: re-running
against unchanged desired state SHALL be idempotent and SHALL update mutable fields
in place.

On reconcile the control plane SHALL, using its in-cluster credentials, create or
update on the target cluster the `AgentRuntime`'s `Namespace`, `ServiceAccount`
(with `automountServiceAccountToken: false`), default-deny-ingress `NetworkPolicy`,
the coordinator `gateway-identity` wiring (the OIDC identity the coordinator uses to
authenticate to its gateway, from the `gateway-identity` `SecretSource` material read
cluster-local), and the coordinator `CronJob`. The control plane SHALL read each
referenced `SecretSource`'s durable material from the configured secret store through
its read-only `pkg/secretstore` client; for provider material it SHALL NOT render the
credential as a raw Kubernetes `Secret`, SHALL NOT create an `ExternalSecret`, and
SHALL NOT rely on External Secrets Operator. It SHALL then provision the gateway-side
`AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`, and `InferenceRoute`
objects through the gateway gRPC API, the same idempotent mechanism it uses for
gateway database provisioning, and SHALL realize each `ProviderBinding` as an OpenShell
**provider** on the agent's gateway/workspace from the binding's `provider_spec_id`,
its `refresh_strategy`, and the material read cluster-local. OpenShell owns rotation of
the realized credential through the provider `refresh_strategy` (`app-installation-token`,
`google-service-account-jwt`, `static-token`); the control plane SHALL NOT mint,
refresh, or rotate the usable credential and SHALL NOT place or rotate a raw `Secret`.

The control plane SHALL drive `AgentRuntime.status` through
`Pending -> Provisioning -> Running -> Degraded -> Terminating` as the data-model
status lifecycle defines, and SHALL write the status back to the API server. When a
`SecretSource` material cannot be read from the store, or gateway-side provisioning
(including provider realization) fails, the control plane SHALL set `status`
to `Degraded` and SHALL surface the failing `SecretSource` name or the gateway-side
error in the status message.

A reference-only `SecretSource` whose material has not yet been uploaded to the assigned
cluster's local store is a benign waiting state, NOT a failure (the late-binding model
in [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md)). When
the store is reachable and reports the material missing, the control plane SHALL set a
durable, observable `SecretPending` condition on the `AgentRuntime`, SHALL NOT create
the dependent OpenShell provider(s) and SHALL NOT configure a provider with empty or
partial material, SHALL NOT mark the workspace `Ready` (the runtime holds at
`Provisioning`, not `Running` or `Degraded`), SHALL NOT crash-loop, and SHALL retry
idempotently on its reconcile cadence, converging the moment the material appears with
no manual trigger. The `SecretPending` condition SHALL name the resolved `ManagedCluster`
and the exact store mount and path to upload the material to, phrased as an actionable
instruction rather than a bare error. Once the material appears the control plane
realizes the provider and updates it in place on subsequent reconciles; keeping the
usable credential current (rotation) is owned by OpenShell, not the control plane (see
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md), "Rotation
Owned by OpenShell"). A store that is unreachable or errors SHALL still degrade the
`AgentRuntime` as above.

On `AgentRuntime` deletion the control plane SHALL delete the cluster resources it
created (`Namespace`, `ServiceAccount`, `NetworkPolicy`, the coordinator
`gateway-identity` wiring, and the `CronJob`) and SHALL delete the gateway-side
`AgentWorkspace`, its OpenShell providers, and its sub-resources through the gateway
gRPC API.

#### Scenario: New AgentRuntime is provisioned end to end

- GIVEN a new `AgentRuntime` with status `Pending` whose effective cluster is this control plane's `cluster_id`
- WHEN the control plane reconciles it
- THEN it SHALL read each `SecretSource`'s material cluster-local and create the `Namespace`, `ServiceAccount`, `NetworkPolicy`, the coordinator `gateway-identity` wiring, and `CronJob` on this cluster, rendering no raw credential `Secret`
- AND it SHALL transition the `AgentRuntime` status to `Provisioning`
- AND after it has provisioned the gateway-side `AgentWorkspace` and realized its OpenShell providers from the material, it SHALL transition the status to `Running`

#### Scenario: AgentRuntime change re-applies affected resources

- GIVEN an `AgentRuntime` with status `Running`
- WHEN its `cron` field changes
- THEN the control plane SHALL update the coordinator `CronJob` on the target cluster
- AND reconciliation SHALL remain idempotent for the unchanged resources

#### Scenario: Pending SecretSource material holds, converges when uploaded

- GIVEN an `AgentRuntime` whose reference-only `SecretSource` material has not yet been uploaded to the assigned cluster's reachable local store
- WHEN the control plane reconciles the `AgentRuntime`
- THEN it SHALL set a `SecretPending` condition naming the resolved `ManagedCluster` and the exact store mount and path to upload to
- AND it SHALL NOT create the dependent OpenShell provider, SHALL NOT mark the workspace `Ready`, and SHALL NOT set `Degraded`
- AND after the material is uploaded, the next reconcile SHALL realize the provider, clear `SecretPending`, and converge the `AgentRuntime` to `Running` with no manual trigger

#### Scenario: SecretSource read failure degrades the AgentRuntime

- GIVEN an `AgentRuntime` with status `Running`
- WHEN one of its `SecretSource` materials cannot be read from the store, or provider realization fails on the gateway
- THEN the control plane SHALL set the `AgentRuntime` status to `Degraded`
- AND the status message SHALL identify the failing `SecretSource` by name

#### Scenario: AgentRuntime deletion removes cluster and gateway-side resources

- GIVEN an `AgentRuntime` with status `Running`
- WHEN a delete event arrives on the watch stream
- THEN the control plane SHALL delete the `Namespace`, `ServiceAccount`, `NetworkPolicy`, the coordinator `gateway-identity` wiring, and `CronJob` from the cluster
- AND it SHALL delete the gateway-side `AgentWorkspace`, its OpenShell providers, and its sub-resources through the gateway gRPC API

### Requirement: Secret Store and Import Reconciler Enablement

The declarative import and `SecretSource` material resolution SHALL be enabled through
configuration, not code (separate configuration from code). The control plane SHALL
run the `RepositoryReconciler` import loop ONLY when `HYPERSHELL_REPOSITORY_IMPORT_ENABLED`
is `true`; when it is unset or `false` the control plane SHALL log that the import
reconciler is disabled and SHALL NOT run the loop.

The control plane SHALL configure its read-only `pkg/secretstore` client from the
`HYPERSHELL_SECRET_STORE_*` environment, following the two resolution paths defined in
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) ("Two
Material Resolution Paths"), NOT a single shared global store:

- For an `AgentRuntime` `SecretSource` material (distributed path), each control plane's
  read client SHALL point at its OWN cluster-local Vault, so the material it reads to
  configure providers was uploaded directly into that cluster's Vault by the per-cluster
  writer and never transited the hub.
- For the `Repository` `secret_source_id` import credential (hub-side path), the hub
  control plane SHALL read the hub-reachable store that the api-server write client
  populates; for a GitHub `Repository` the stored material is the App PEM, from which
  the `RepositoryReconciler` mints the short-lived installation token.
- The api-server write client and a control plane's read client MAY share one
  `HYPERSHELL_SECRET_STORE_BACKEND` and address ONLY when they are co-located (the same
  cluster's store, for example `hyp0`). In the general multi-cluster fleet the
  `AgentRuntime` `SecretSource` material SHALL NOT be resolvable through a single store
  that the hub api-server writes and every spoke reads.

The secret-store Vault client SHALL reach the per-cluster Vault as that cluster serves
it (private-CA trust and the cluster's auth method, Kubernetes auth where the cluster
Vault uses it), under a policy scoped to the agent-secret path, as defined in
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md)
("Agent-Secret Vault Client Capabilities"). When the secret store is unset the control
plane SHALL fail provider realization loudly rather than configure a provider with empty
material.

#### Scenario: Import loop runs only when enabled

- GIVEN a control plane with `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=false`
- WHEN the control plane starts
- THEN it SHALL log that the import reconciler is disabled and SHALL NOT run the import loop
- WHEN instead `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=true` and the secret store is wired
- THEN it SHALL run the import reconcile loop and SHALL resolve the `Repository` `secret_source_id` import credential material through its read-only `pkg/secretstore` client, minting the installation token from the App PEM

#### Scenario: AgentRuntime material is read from the control plane's own cluster-local store

- GIVEN a spoke control plane not co-located with the hub api-server
- WHEN it realizes providers for an `AgentRuntime` from its `SecretSource` material
- THEN its read client SHALL read the material from its own cluster-local Vault
- AND the material SHALL NOT be resolved through a store the hub api-server writes

#### Scenario: Client reaches a privately-signed cluster Vault with Kubernetes auth

- GIVEN a per-cluster Vault served with a private CA and configured for Kubernetes auth
- WHEN the control plane reads `SecretSource` material from it
- THEN the `pkg/secretstore` client SHALL trust the configured private-CA bundle and authenticate through the Vault Kubernetes auth mount
- AND it SHALL read only under the `secret/data/hypershell/secret-sources/*`-scoped policy

### Requirement: Declarative `.hypershell` Import Reconciliation

The control plane SHALL host a reconciler named the `RepositoryReconciler` that
reconciles the `Repository` Kind by performing the declarative `.hypershell` import
process defined in [`data-model.spec.md`](./data-model.spec.md) ("Declarative
`.hypershell` Import"). This is the system's SINGLE git-to-API sync component: it is
the same component referred to as the declarative `.hypershell` import reconciler
elsewhere, and there is no separate "PlatformReconciler" or second GitOps sync
engine. The `RepositoryReconciler` does not exist in the code yet; it is desired
state to be built. The import SHALL NOT be hosted by the API server: the API server
has no read access to the secret store and no repository read access, whereas the
control plane holds read-only access to the secret store (through its
`pkg/secretstore` client) and the platform GitHub App credential used to read the
`import_path` subtree over the GitHub API.

The `RepositoryReconciler`'s job is bounded to the import: it reads the declared
subtree, renders it, and upserts the ext resources it finds - the canonical set of
importable Kinds defined in [`data-model.spec.md`](./data-model.spec.md)
("Declarative `.hypershell` Import"), which includes `Gateway` - into the API. It
does NOT provision any of those resources cluster-side. Each imported Kind is subsequently reconciled into the
cluster by ITS OWN per-Kind reconciler - for example the `GatewayReconciler`
reconciles `Gateway` resources into Kubernetes and the `AgentRuntimeReconciler`
reconciles `AgentRuntime` resources into cluster scheduling and gateway-side
objects. This division of labor is the established control-plane pattern: the
`RepositoryReconciler` turns git into API desired state, and the per-Kind
reconcilers turn API desired state into cluster state.

The `RepositoryReconciler` SHALL run as a reconcile LOOP: it SHALL periodically
enumerate every registered `Repository` and reconcile each one, and it SHALL be
idempotent so repeated runs over unchanged repository state are side-effect free.
The reconciler SHALL NOT clone the repository, SHALL NOT write any working
directory or persistent disk (no PVC), SHALL NOT shell out to a `git`, `kustomize`,
or `hsctl` binary, and SHALL NOT dispatch a Kubernetes `Job`. The entire read and
render SHALL happen in-process and in-memory.

For each registered `Repository` the control plane SHALL:

1. Resolve the credential used to read the `Repository` subtree. The platform
   GitHub App credential is the DEFAULT transport and reads any GitHub-hosted
   repository subtree. When the `Repository` declares a `secret_source_id`, that
   per-`Repository` credential takes precedence and supplies a repository-specific
   git credential - for example a private or non-GitHub-App-covered source; the
   control plane reads its value from the secret store through its read-only
   `pkg/secretstore` client. This is the same read-only store access the control
   plane uses to place `AgentRuntime` secrets; it does not create an `ExternalSecret`
   and does not depend on External Secrets Operator.
2. Read ONLY the `import_path` subtree (default `.hypershell`, the Kustomize root)
   through the GitHub API at the `Repository`'s `default_branch` (or the branch
   otherwise specified), authenticating with the credential resolved in step 1 - the
   per-`Repository` `secret_source_id` credential when one is configured, otherwise
   the default platform GitHub App credential.
   The reconciler SHALL fetch only that subtree, NOT the whole repository, and
   SHALL NOT perform a git clone or checkout, SHALL NOT create a working directory,
   and SHALL NOT use any persistent disk or PVC.
3. Render that subtree IN MEMORY using the shared kustomize library: the exported
   `Build` of the `pkg/kustomize` package that lives in `components/cli` and is
   consumed by exactly two callers, `hsctl` and the control plane's
   `RepositoryReconciler`, so there is exactly ONE kustomize implementation in the
   stack. The library SHALL render through an in-memory
   filesystem built from the fetched subtree. The control plane SHALL NOT invoke an
   external `kustomize` or `hsctl` binary, SHALL NOT embed a second, divergent
   Kustomize implementation, and SHALL NOT dispatch a `Job` to render.
4. Reconcile the rendered ext resources into the API over the generated gRPC write
   path - create new declarations, update changed declarations, and prune
   declarations removed from the repository. Each written resource SHALL be
   ownership-marked with its originating `Repository` (`repository_id`).

Reconciliation SHALL converge, not create-and-skip, and SHALL be idempotent
against unchanged repository state: a re-run over an unchanged `Repository` SHALL
make no create, update, or prune write. Imported resources SHALL be owned by the
originating `Repository`, and pruning SHALL be scoped to that `Repository`, so one
`Repository`'s import SHALL NOT delete resources owned by another `Repository` or
created directly through the REST API or `hsctl apply`.

**Bounded write exception.** The `RepositoryReconciler` is the SINGLE, clearly-bounded
control-plane path that writes HyperShell API desired-state resources. Outside it
the control plane remains a reader of desired state: it writes back only derived
resource `status` and provisions gateway-side objects through the gateway gRPC API.
The `RepositoryReconciler` SHALL write ONLY ext resources owned by the originating
`Repository`, SHALL perform only create / update / prune on those Repository-owned
resources, and SHALL mark each written resource with its owning `Repository`. This
resolves the apparent conflict with the "control plane is a reader of desired
state" principle: the principle holds everywhere except this one named,
ownership-scoped reconciler.

**gRPC write authorization for the import.** The import-write Kinds
(`SandboxTemplate`, `ProviderSpec`, `ProviderBinding`,
`InferenceRoute`, `SecretSource`) had their gRPC Create / Update / Delete methods
held at default-deny when their gRPC surface was introduced. They SHALL now be
authorized for the control-plane identity, bounded to the `RepositoryReconciler`'s
`Repository` scope, and remain denied for every other caller (users, spokes acting
outside the import). The remaining importable Kinds, `Gateway` and `AgentRuntime`
(see the canonical importable-Kinds list in
[`data-model.spec.md`](./data-model.spec.md)), are cluster-assigned: their gRPC
`Create` / `Update` / `Delete` are authorized for the control-plane identity on the
same `Repository`-scoped import terms (a write carrying the reconciled-`Repository`
binding is set and enforced as that `Repository`'s ownership server-side), while a
`Gateway` or `AgentRuntime` write WITHOUT that binding stays on its existing
cluster-scoped non-import path (health and status write-backs). Because the import
reconciler must enumerate and prune a `Repository`'s gateways and agent runtimes
across clusters, a `ListGateways` / `ListAgentRuntimes` read that carries the
reconciled-`Repository` binding AND sets a matching `repository_id` filter for an
existing `Repository` is exempted from the own-cluster watch-stream caller binding and
served (and filtered) across clusters; every other read keeps the own-cluster binding.
The `RepositoryReconciler` upserts and prunes all seven importable Kinds under the same
`Repository`-scoped ownership mark.
`Repository` is NOT on the import-write path. The `RepositoryReconciler` reconciles
the `Repository` Kind but SHALL NEVER upsert `Repository` resources discovered in an
import, so `Repository` is excluded from the canonical importable set in
[`data-model.spec.md`](./data-model.spec.md) to prevent recursive or transitive
import (infinite recursion). Whether `Repository`'s own gRPC Create / Update / Delete
methods are authorized for any identity is a separate gRPC-authorization (RBAC)
concern governed by
[`security/rbac-enforcement.spec.md`](../../security/rbac-enforcement.spec.md); it
does NOT make `Repository` importable, and the import SHALL NOT write the `Repository`
Kind.
The authorization contract is specified in
[`security/rbac-enforcement.spec.md`](../../security/rbac-enforcement.spec.md),
"Requirement: gRPC Authorization". In particular:

- On create, the server SHALL set `repository_id` from the `Repository` the
  reconciler is reconciling and SHALL ignore (not trust) any client-supplied
  `repository_id` on the request, so a caller cannot forge ownership.
- These writes SHALL be authorized as the control-plane identity, `Repository`-scoped
  through the ownership mark, and SHALL NOT be an open bootstrap-service-account
  bypass. The bootstrap-SA interceptor exemption (which allows a fully-trusted
  internal identity to reach gRPC write handlers) is distinct from this import
  authorization: the import write path derives its authority from the reconciled
  `Repository` ownership, not from the bootstrap allowlist.

#### Scenario: Import reconciles declared resources through the API

- GIVEN a registered `Repository` whose `.hypershell` directory declares a `SandboxTemplate` and an `AgentRuntime`
- WHEN the control-plane import reconciler runs
- THEN it SHALL read only the `import_path` subtree through the GitHub API at the `default_branch`, without cloning the repository
- AND it SHALL render that subtree in memory through the shared `pkg/kustomize` `Build` and upsert both resources into the API over the gRPC write path
- AND the resources SHALL be owned by the `Repository`

#### Scenario: Removed declaration is pruned within the owning Repository

- GIVEN a `Repository` that previously imported an `AgentRuntime`, and another `AgentRuntime` created directly through the REST API
- WHEN that imported `AgentRuntime` is removed from `.hypershell` and the import re-runs
- THEN the control-plane import reconciler SHALL delete the imported `AgentRuntime` through the API
- AND it SHALL NOT delete the directly-created `AgentRuntime`

#### Scenario: Import is idempotent against unchanged repository state

- GIVEN a `Repository` whose `.hypershell` contents are unchanged since the last import
- WHEN the control-plane import reconciler runs again
- THEN it SHALL make no create, update, or prune write to the API

#### Scenario: Import reads the credential from the store, not via ExternalSecret

- GIVEN a private `Repository` whose `secret_source_id` references a stored git credential
- WHEN the control-plane import reconciler needs to read the `import_path` subtree
- THEN it SHALL read the credential value through its read-only `pkg/secretstore` client
- AND it SHALL NOT create an `ExternalSecret` or rely on External Secrets Operator

#### Scenario: Import renders in memory through the shared kustomize library

- GIVEN a `Repository` whose `import_path` subtree is a Kustomize root
- WHEN the control-plane import reconciler renders the declared resources
- THEN it SHALL render in memory through the exported `Build` of `pkg/kustomize` (the package in `components/cli` shared with `hsctl`)
- AND it SHALL NOT clone the repository, write to disk, invoke an external `kustomize` or `hsctl` binary, or dispatch a `Job`
- AND it SHALL NOT embed a second, divergent Kustomize implementation in the control plane

#### Scenario: Import loop is in-process with no disk or external process

- GIVEN the control-plane distroless runtime image, which ships no `git`, `kustomize`, or `hsctl` binary and mounts no persistent disk
- WHEN the import reconcile loop runs
- THEN it SHALL fetch the subtree over the GitHub API and render it entirely in process through an in-memory filesystem
- AND it SHALL require no working directory, no PVC, and no child process

#### Scenario: Server sets ownership on import create and ignores a client-supplied value

- GIVEN the import reconciler creates an imported ext resource whose request carries a client-supplied `repository_id`
- WHEN the gRPC create is authorized for the control-plane identity
- THEN the server SHALL set `repository_id` from the reconciled `Repository`
- AND it SHALL ignore the client-supplied `repository_id`

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

The keepalive ping interval SHALL NOT be more aggressive than the API server's
gRPC keepalive enforcement policy. A client that pings an idle connection more
often than the server's minimum permitted interval is struck by the server and,
after a few strikes, receives a GOAWAY (`ENHANCE_YOUR_CALM`, `too_many_pings`)
that closes the shared connection and drops every watch stream. The connection
layer is only the backstop for a dead or half-open connection; detecting a stream
that stays open but goes silent is the idle watchdog's job, so the keepalive
interval does not need to be short.

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
- AND it SHALL serve watch streams for Gateways, ManagedClusters, RoleBindings and AgentRuntimes

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
| AgentRuntime effective cluster resolved server-side, not client-side | An `AgentRuntime` may omit `cluster_id` and derive placement from its Gateway. If each control plane resolved placement and filtered client-side, every control plane would receive every AgentRuntime and the cluster scoping could not be enforced at the hub. Resolving the derived cluster server-side keeps the watch-stream caller binding authoritative, exactly as for Gateways and RoleBindings. The control plane re-resolves only as defense in depth. |
| AgentRuntime AND the declarative `.hypershell` import both reconciled by the control plane | The control plane holds the in-cluster credentials needed to apply AgentRuntime scheduling resources and already dials each gateway's gRPC API, so cluster-side and gateway-side provisioning belong there. The declarative `.hypershell` import also belongs there: it must read each `Repository`'s credential from the secret store and read the `import_path` subtree over the GitHub API, and the control plane has read-only secret-store access (`pkg/secretstore`) and the platform GitHub App credential while the api-server has neither store read access nor repository read access. The import is the single, bounded path by which the control plane writes API desired state, scoped to create/update/prune of `Repository`-owned ext resources; everywhere else the control plane stays a reader of desired state (see "Declarative `.hypershell` Import Reconciliation" and [`data-model.spec.md`](./data-model.spec.md), "Declarative `.hypershell` Import"). |
| Import reads the `import_path` subtree over the GitHub API, not a git clone | The control-plane distroless runtime image ships no `git` binary and mounts no writable working directory or PVC. Reading only the `import_path` subtree over the GitHub API with the platform GitHub App credential keeps the import in-process and in-memory, avoids a full repository checkout, and needs no on-disk working copy. It also narrows the blast radius: the reconciler fetches only the declarative root, not the whole repository. |
| Kustomize rendered in-process by a shared Go LIBRARY, reversing the earlier "no kustomize in control plane" | This deliberately reverses the prior decision that kept Kustomize out of the control plane. The reversal admits the kustomize Go LIBRARY (not a binary, not a `Job`) into the control plane so rendering happens in-process in-memory. The single shared implementation SHALL be `pkg/kustomize` (exported `Build`) in `components/cli`, to be built and then consumed by exactly two callers: `hsctl` and the control plane's `RepositoryReconciler`; `hsctl apply -k` is refactored off its external-`kustomize`-binary shell-out onto this package, and the control-plane module imports the cli module. One implementation renders for `hsctl` and the `RepositoryReconciler`, so CLI and server render identically and there is no second, divergent renderer to drift. The `GatewayReconciler` is NOT a consumer of this library: it renders the gateway workload through Helm. The accepted trade-off is a kustomize library dependency in the control-plane module. |
