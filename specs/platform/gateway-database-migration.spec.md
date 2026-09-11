# Gateway Database Migration and Compatibility Specification

**Status:** Draft

## Purpose

Existing installations can adopt controller-local PostgreSQL without losing data,
recreating gateways, or replacing existing clients. The system supports the
existing provider runtime and the
[controller-local runtime](./openshell-gateway-database.spec.md), with an explicit
transfer of database ownership. The API server and Keycloak application databases
are outside the gateway data transfer, but their data must also survive upgrades.

## Requirements

### Requirement: Existing Interfaces Remain Functional

REST `/api/hypershell/v1`, gRPC `hypershell.v1`, existing SDK interfaces, and CLI
commands SHALL retain their existing request, response, validation, authorization,
pagination, watch, and error behavior. `ManagedDatabase`, Gateway `database_id`,
and their protobuf tags SHALL remain present where the existing contract defines
them. An upgrade SHALL NOT replace an operation with a success response that does
nothing, an empty response, or an unsupported-operation error.

The v1 schema includes database metadata, name, provider, region, engine,
engine version, instance class, connection Secret reference, status, and namespace.
Its [ManagedDatabase protobuf](../../components/api-server/proto/hypershell/v1/managed_databases.proto)
and [Gateway protobuf](../../components/api-server/proto/hypershell/v1/gateways.proto)
retain their wire identities. Existing API field names and generated client
symbols SHALL remain available, including optional fields and their semantics.

The v2 DTOs without database-selection fields SHALL use REST `/api/hypershell/v2` and gRPC `hypershell.v2`.
The v2 gateway path SHALL assign an execution target without database selection.
V1 SHALL remain supported; this change sets no removal date. New SDKs SHALL keep
v1 exports and expose v2 separately. Existing CLI syntax and API-version settings
SHALL remain valid. Capability negotiation SHALL allow new tooling to use the
existing workflow on an older server; it SHALL NOT misreport v2 as available.

V1 projections of migrated or v2-created gateways SHALL retain the database
metadata needed by existing consumers. The compatibility layer SHALL provide
stable IDs and accurate destination metadata. It SHALL NOT make the v2 placement
path depend on a user-created database registration. Writes through either API
version SHALL reach the resource's current owner, with the same authorization
and without duplicate SQL or infrastructure operations.

#### Scenario: Existing client operates after upgrade

- GIVEN an existing SDK or CLI uses v1 to create, read, update, and delete resources
- WHEN the API and controller are upgraded
- THEN those operations SHALL retain their existing behavior
- AND v1 database reads and watches SHALL return valid records
- AND new clients MAY use v2 without a database-registration step

### Requirement: Existing Provider Behavior Remains Available

Until ownership transfer completes, `DATABASE_PROVIDER` and per-record provider
settings SHALL retain their existing meanings. Changing a startup default SHALL
NOT retarget an existing gateway. The compatibility runtime SHALL support all
three existing providers:

| Provider | Existing placement and lifecycle |
|---|---|
| `deployment` | The default when unset; each gateway gets a dedicated ManagedDatabase and PostgreSQL Deployment, Service, PVC, and credentials. Explicit gateway deletion removes its dedicated infrastructure. |
| `cnpg` | Creation selects the sole eligible database record, or reports the existing zero/multiple-record error. The database record owns a CNPG Cluster; gateways use Database and DatabaseRole resources with their existing names and credentials. |
| `external` | Creation selects the oldest eligible registration. HyperShell creates gateway SQL objects, but does not own the external server. Later registrations do not move existing gateways. |

ManagedDatabase CRUD SHALL preserve provider immutability and deletion protection:
a referenced record cannot be deleted. Delete events SHALL retain the provider,
namespace, and ID in replayable tombstones. Existing tombstone capability checks,
replay authorization, and retries SHALL remain supported. External connection
references SHALL remain bare DNS-1123 namespace names with the
`hypershell-managed-db-` prefix and the fixed Secret name
`hypershell-managed-db-credentials`;
compatibility SHALL NOT broaden Secret access.

CNPG password rotation SHALL remain available through its existing operator path
until transfer. Existing deployment and external rotation annotations SHALL stay
inactive until the operator enables controller-local rotation. A previously inert
annotation SHALL NOT cause an unexpected password change during migration.

#### Scenario: Upgrade an installation with several database providers

- GIVEN gateways use CNPG, dedicated Deployments, and external PostgreSQL servers
- WHEN the API is upgraded before migration
- THEN each gateway SHALL retain its provider, server, SQL names, and credentials
- AND existing create, rotation, and delete workflows SHALL remain functional
- AND no server SHALL be replaced because a new default became available

### Requirement: Schema Upgrades Preserve Data and Rollback

Schema upgrades SHALL be additive and repeatable. They SHALL preserve existing
rows, IDs, database references, deletion tombstones, and migration history.
The ManagedDatabase table and v1 fields SHALL remain available to compatibility
consumers. A backfill SHALL record progress and resume safely after interruption.
New physical schema fields SHALL NOT make an older compatible binary unable to
read or write its existing resources.

Fresh installations SHALL expose the same compatibility interfaces. The new
runtime SHALL use local configuration; compatibility rows are not its database
placement authority. An upgrade SHALL NOT require empty tables or cluster teardown.

#### Scenario: Schema backfill is interrupted

- GIVEN an installation contains live gateways and deletion tombstones
- WHEN a schema backfill stops and is restarted
- THEN it SHALL resume without duplicate resources or changed identifiers
- AND existing clients SHALL remain able to access their resources

### Requirement: Migration Has an Explicit Plan

Migration SHALL be an operator-selected workflow with a read-only preview. Hypbox
SHALL support migration of existing hypbox-managed installations. Application
migration tooling SHALL also support existing installations without requiring
adoption into hypbox. This does not add arbitrary-cluster creation to hypbox.

The plan SHALL identify each gateway, its source server, SQL database and role,
credential references, current owner, destination, and proposed ownership change.
It SHALL show required backup and restore checks, capacity, PostgreSQL version and
extension compatibility, TLS and network checks, expected downtime, rollback
steps, and resources retained after completion. Credentials SHALL be redacted.
Missing ownership evidence, name collisions, incompatible extensions, or inadequate
privileges SHALL block the affected transfer before mutation.

The workflow SHALL support adoption on the same server and copying to a new RDS
or CNPG server. Multiple existing servers and per-gateway Deployments SHALL have
a path to the controller's configured destination. It SHALL NOT assume all
existing databases already share one server or use the new SQL naming rule.

#### Scenario: Preview consolidation of dedicated databases

- GIVEN two gateways use separate PostgreSQL Deployments
- WHEN the operator previews migration to one configured server
- THEN the plan SHALL identify both source databases and any naming conflicts
- AND it SHALL show data transfer, downtime, verification, and rollback steps
- AND it SHALL NOT change resources, credentials, or data

### Requirement: One Owner Controls Each Gateway

The API and migration workflow SHALL enforce one active database reconciler per
gateway. Initial lists, watches, retries, deletion, rotation, and garbage collection
SHALL observe that ownership. A persisted transfer marker and fencing mechanism
SHALL stop the old owner before the new owner can mutate SQL or credentials.
An older controller that does not understand migration SHALL receive only work
it can safely own. A caller-supplied filter is not an ownership boundary.

CNPG adoption SHALL prevent the operator from deleting adopted SQL objects or
resetting their passwords. Reclaim policies and ownership references SHALL be
made safe and verified before Database, DatabaseRole, or password resources are
released. Server, PVC, and Secret ownership SHALL transfer without GitOps pruning
or garbage collection deleting live data. Adoption SHALL preserve SQL names,
credentials, grants, extensions, and contents unless the approved plan changes them.

#### Scenario: Adopt a CNPG database on the same server

- GIVEN CNPG resources currently manage a gateway database and role
- WHEN migration transfers their ownership to the local SQL controller
- THEN the operator SHALL stop mutating those objects before the controller starts
- AND removing old control resources SHALL NOT drop data or reset credentials
- AND the existing gateway SHALL reconnect to the same verified database

### Requirement: Data Transfer Is Resumable and Verifiable

Cross-server migration SHALL preserve all committed gateway data, schema,
sequences, large objects, required extensions, grants, and role ownership. The
workflow SHALL account for active sessions and prevent writes to both source and
destination during cutover. A bounded maintenance window MAY be used; continuous
availability is not required. The operator SHALL see its expected impact first.

Durable progress SHALL record the authoritative source, transfer phase, validation
results, credential version, and active owner. Before cutover, the workflow SHALL
verify the copied data and application behavior. It SHALL switch the destination
and credentials only after validation and write fencing succeed. Restarting any
participant SHALL resume from the recorded phase without repeating destructive
operations. Concurrent deletion or rotation SHALL wait or be rejected clearly
while transfer owns that gateway.

#### Scenario: Transfer fails before cutover

- GIVEN a gateway still uses the source and a destination copy is incomplete
- WHEN transfer fails or the migration process restarts
- THEN the source SHALL remain authoritative and its data SHALL remain intact
- AND the workflow SHALL report failure and support retry or safe cancellation
- AND cancellation before cutover SHALL restore source writes when it is safe
- AND it SHALL NOT start the gateway against the incomplete copy

### Requirement: Rollback Preserves Writes

The source server, required credentials, and verified recovery material SHALL
remain available through an explicit rollback window. Before destination writes
start, rollback MAY restore the original owner and configuration. After writes
start on the destination, rollback SHALL first stop writes and synchronize or
restore all committed destination changes to the source. It SHALL NOT switch
clients back to a stale copy. A failed rollback SHALL preserve the authoritative
copy and report the recovery steps; it SHALL NOT claim success.

Migration completion SHALL NOT delete source data. Source retirement SHALL be a
separate previewed and confirmed operation after verification and expiry of the
agreed rollback window. Finalizers, reclaim policies, Terraform state, and GitOps
ownership SHALL be checked before retirement so shared or active resources remain.

#### Scenario: Rollback after destination writes

- GIVEN a migrated gateway has committed new data on the destination
- WHEN the operator requests rollback
- THEN the workflow SHALL fence writes and preserve those changes on the source
- AND it SHALL validate the restored state before switching gateway connections
- AND a failed restore SHALL leave the destination data available for recovery

### Requirement: Rollout and Compatibility Are Verified

Release checks SHALL cover the previous released API, controllers, SDKs, CLI, and
web console against the upgraded system. The API SHALL be upgraded first while
existing controllers remain operational. New controllers SHALL detect capabilities
before entering the new runtime. Unsupported combinations SHALL leave existing
workloads operational and report the required upgrade order.

Checks SHALL cover both API versions, fresh and existing schema, all three source
providers, same-server adoption, cross-server transfer to RDS and CNPG, partial
failure, process restart, interrupted ownership transfer, rollback after writes,
and deletion after migration. Tests SHALL include stored gateway data and prove
its preservation. Passing empty-database tests alone is insufficient.

Existing dashboard builds SHALL retain their database inventory fields, metrics,
and endpoint behavior. New dashboards MAY omit these widgets. Removing a widget
from the new UI SHALL NOT remove the interface used by an older dashboard.

#### Scenario: New controller meets an older API

- GIVEN the API does not advertise controller-local migration capabilities
- WHEN a new controller starts for an existing installation
- THEN it SHALL use its compatible existing runtime
- AND it SHALL NOT migrate data, drop fields, or stop existing gateway workloads
