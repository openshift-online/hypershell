# OpenShell Gateway Database Specification

**Status:** Draft

## Purpose

Each HyperShell execution controller uses one configured PostgreSQL server to
create a separate SQL database and login role for each gateway it owns. The
installation system owns the PostgreSQL server. The HyperShell API owns gateway
placement on execution clusters, but does not select or register database
servers. The application model has no `ManagedDatabase` entity or Gateway
`database_id` field. This intentionally breaking release supports fresh
installations and explicit teardown followed by recreation.

## Scope and Terminology

- **PostgreSQL server:** the RDS instance, CNPG Cluster, or development PostgreSQL
  server supplied by the installation system.
- **Gateway database:** a SQL database and login role inside that server. It is
  not a separate RDS instance or a separate CNPG Cluster.
- **Hub instance:** a HyperShell instance with an API server and web console. It
  can also run a local execution controller.
- **Spoke instance:** an execution controller that registers with a parent hub
  and runs gateway workloads without its own HyperShell API server or console.
  This use of hub is distinct from the shared Keycloak identity hub.

This is the database contract for the [data model](./data-model.spec.md),
[control plane](./control-plane.spec.md), and
[gateway lifecycle](./openshell-gateway.spec.md). The API server's own PostgreSQL
database and Keycloak databases are separate installation dependencies.

In-place upgrades, data migration, old API clients, and mixed old/new runtimes
are unsupported for this transition. There is no v2 API or legacy compatibility
runtime in this design. An installation that remains on its existing release
SHALL NOT receive this change automatically. Remote gRPC transport implementation is a
separate dependency. The execution ownership requirements below apply even when
all controllers use the same Kubernetes cluster.

## Requirements

### Requirement: The Installation System Owns the Server

The installation system SHALL create, configure, back up, and delete PostgreSQL
server infrastructure according to the installation policy. HyperShell SHALL
NOT create, resize, or delete RDS instances, CNPG Clusters, PostgreSQL Deployments,
or their persistent volumes as part of gateway reconciliation.

Hypbox SHALL retain RDS and CNPG as installation choices. Both SHALL supply the
same PostgreSQL connection contract to the application. The application SHALL
NOT require a cloud provider setting or CNPG APIs to create a gateway database.
Local development and CI SHALL supply a PostgreSQL server through their setup
manifests or scripts, with no database-registration API call.

#### Scenario: Create a gateway with either backend

- GIVEN hypbox has supplied a ready RDS or CNPG PostgreSQL server
- WHEN the assigned controller provisions a gateway
- THEN it SHALL create the gateway database and role on that server
- AND it SHALL NOT create a second PostgreSQL server

#### Scenario: Delete the last gateway

- GIVEN a server has one remaining gateway database
- WHEN the gateway is deleted and cleanup completes
- THEN the gateway database and role SHALL be absent
- AND the server and its persistent storage SHALL remain present

### Requirement: Each Controller Has Explicit Local Configuration

Each execution controller SHALL receive an explicit reference to an
administrative credential Secret in its Kubernetes cluster. The Secret SHALL
provide the server address, port, administrative database, login credentials,
and TLS trust configuration. The reference SHALL be deployment configuration,
not a field accepted from a gateway API request. Credentials SHALL stay outside
Git, public API responses, events, logs, and metrics.

Secret access SHALL be limited to the declared namespace and Secret. A missing
reference, invalid configuration, unavailable server, or insufficient SQL
privileges SHALL produce a clear dependency error. The controller SHALL NOT
fall back to another server or report affected gateways as ready. SQL operations SHALL have bounded timeouts. Temporary
failures SHALL be retried with bounded backoff. Dependency failure SHALL be
visible through controller readiness and status without restarting healthy
processes in a loop.

Production administrative and gateway connections SHALL use TLS and verify
the server identity. Required CA material SHALL reach the gateway workload.
Explicit plaintext configuration MAY be used for isolated local development;
production configuration SHALL NOT silently downgrade TLS verification.

#### Scenario: Credentials are not yet available

- GIVEN the configured Secret has not been synchronized
- WHEN the controller starts or receives a gateway event
- THEN database provisioning SHALL wait with a redacted dependency error
- AND it SHALL retry when credentials become available
- AND it SHALL NOT provision against a default server

#### Scenario: Administrative credentials change

- GIVEN the Secret receives valid replacement credentials for the same server
- WHEN the controller retries a database operation
- THEN it SHALL use the replacement credentials
- AND it SHALL preserve existing gateway database names and login passwords

### Requirement: The API Has No Database Placement Entity

The API SHALL accept valid gateway creation without `database_id`. It SHALL
retain gateway execution placement through `cluster_id`. It SHALL NOT select,
create, or query a database resource during gateway creation.

The REST and gRPC contracts SHALL have no ManagedDatabase operations, watch
stream, or Gateway database-selection fields. Protobuf numbers and names retired
from those contracts SHALL remain reserved and SHALL NOT be reused. Generated
SDKs, CLI commands, UI workflows, dashboards, fixtures, and documentation SHALL
use this model. There SHALL be no database registration prerequisite or central
cluster-to-database mapping entity. ManagedCluster and its registration remain
part of the API.

The release SHALL use the existing REST `/api/hypershell/v1` and gRPC
`hypershell.v1` namespaces with an explicitly documented breaking contract.
Only matching release clients and controllers are supported. This release SHALL
NOT advertise compatibility with prior clients based on those namespace names.

A fresh application schema SHALL have no active ManagedDatabase table or Gateway
`database_id` column. Historical migration records SHALL NOT be rewritten to
conceal the transition. Existing application schemas SHALL remain unchanged when
this release rejects them; the startup gate below applies before any migrations.

#### Scenario: No database record exists

- GIVEN a fresh API installation and an eligible execution target
- WHEN a client creates a gateway using the new contract
- THEN the API SHALL store the gateway and publish its work event
- AND it SHALL NOT require a database record or database identifier

### Requirement: Reconciliation Creates One Database Per Gateway

The assigned controller SHALL create a separate PostgreSQL database and login
role for each gateway. The database and role names SHALL be `gw_<lowercase-gateway-id>`, derived from
the stable gateway ID, not its display name. Each gateway SHALL receive only its own login credentials.
Other gateway roles SHALL NOT have access to that database through PUBLIC
permissions. The administrative credential SHALL NOT be supplied to gateway
pods.

The controller SHALL publish `openshell-gateway-db-credentials` in the gateway
namespace with `host`, `port`, `dbname`, `user`, `password`, `sslmode`, and `uri`.
The URI SHALL encode credentials correctly. TLS trust material SHALL be supplied
where needed. Passwords SHALL use at least 256 bits of cryptographic randomness.
SQL identifiers and values SHALL be escaped or bound safely. Administrative
access SHALL require only the privileges needed to manage gateway databases and
roles, including RDS-supported privileges; PostgreSQL superuser access SHALL NOT
be required. Provisioning SHALL verify connection readiness before it starts the
gateway workload. Explicit password rotation follows the
[secret rotation contract](./openshell-gateway-secret-rotation.spec.md).

Reconciliation SHALL preserve an existing database, its contents, and valid
credentials. Repeated events, partial SQL completion, process restarts, and
concurrent attempts SHALL converge without dropping data or creating duplicate
databases. A persisted gateway credential Secret SHALL allow reuse of the
password. If creation stops after SQL operations but before Secret persistence,
a retry SHALL repair the incomplete provisioning safely.

#### Scenario: Restart after successful provisioning

- GIVEN a gateway database contains application data
- WHEN its controller restarts and receives the gateway again
- THEN the existing database and role SHALL be reused
- AND the data and gateway password SHALL remain unchanged

#### Scenario: Retry after partial creation

- GIVEN the SQL role exists but the database or credential Secret is absent
- WHEN the same gateway is reconciled again
- THEN provisioning SHALL complete using the same gateway identity
- AND existing SQL objects SHALL NOT be replaced destructively

### Requirement: Database Destination Is Stable

A gateway SHALL remain bound to the configured server on which its database was
created. The controller SHALL retain enough durable local information to detect
a different destination before it changes gateway credentials or deletes data.
This information SHALL NOT become a central database inventory or mapping API.
Changing a gateway's execution cluster or its controller's database destination
while it has provisioned data SHALL be rejected as an unsupported migration.
Credential renewal for the same destination SHALL remain supported.

#### Scenario: Configuration points to a different server

- GIVEN a controller has provisioned gateway databases
- WHEN its database destination changes without teardown
- THEN it SHALL report the unsupported change
- AND it SHALL NOT create empty replacement databases or redirect gateway pods
- AND it SHALL NOT execute cleanup on the replacement server

### Requirement: Execution Ownership Applies to Database Operations

Only the controller assigned to a gateway SHALL create, repair, or delete its
SQL resources. Initial list operations, watch events, retries, and deletion
recovery SHALL all preserve this boundary. A hub controller SHALL NOT process a
spoke's gateways merely because it uses the same API server.

Managed-cluster self-registration SHALL remain the source of spoke identity.
Hypbox SHALL NOT seed duplicate managed-cluster records. Authentication and
server-side authorization SHALL bind a remote controller to its registered
identity; a caller-supplied `cluster_id` alone SHALL NOT establish ownership.

#### Scenario: Hub and spoke both run controllers

- GIVEN the hub and spoke have separate configured PostgreSQL servers
- WHEN the API assigns a gateway to the spoke
- THEN only the spoke SHALL provision its database
- AND the hub's PostgreSQL server SHALL remain unchanged

### Requirement: Cleanup Is Durable and Retryable

Gateway deletion SHALL remove only the gateway's SQL database, login role, and
local credentials. A shared PostgreSQL server or another gateway's objects
SHALL NOT be removed by this operation. The controller SHALL retain durable
cleanup intent and the destination reference until cleanup is complete.
Neither an in-memory cache nor receipt of one watch event is sufficient.

A temporary database outage or controller restart SHALL NOT lose pending
cleanup. Missing SQL objects SHALL count as already removed. Cleanup failures
SHALL remain visible and retryable; they SHALL NOT be logged once and forgotten.
Teardown SHALL keep the server and administrative credentials available until
all pending gateway cleanup completes or the operator explicitly selects full
installation data deletion.

#### Scenario: Database is unavailable during deletion

- GIVEN gateway deletion has started and PostgreSQL becomes unavailable
- WHEN the controller restarts before SQL cleanup completes
- THEN pending cleanup SHALL survive and resume when PostgreSQL is available
- AND the deletion SHALL NOT be reported as fully cleaned up before completion

### Requirement: Hypbox Generates the Complete Database Configuration

Hypbox SHALL generate the PostgreSQL infrastructure, credential delivery,
controller configuration, network access, TLS trust, and startup dependencies.
It SHALL NOT generate `ManagedDatabase` seed Jobs, seed-only Keycloak clients,
or seed-only credentials and access grants.

Spoke installation SHALL target only hypbox-managed clusters. Users SHALL be
able to add spokes to a cluster running this new contract or create a fresh
cluster with spokes. Adopting arbitrary existing clusters is outside scope.
Single and main/canary modes SHALL both be supported. Each spoke SHALL have its
own gateway PostgreSQL server and administrative credentials. A spoke SHALL
not get an application database or Keycloak database for absent components.

Each spoke SHALL inherit its parent's pinned controller image. Generated parent
image changes SHALL include the affected spoke pins. Main and canary SHALL
remain independently pinned. A parent update SHALL NOT create an incompatible
API/controller combination.

#### Scenario: Paired managed instances

- GIVEN parent canary `hyp0` and parent main `hyp1`
- WHEN hypbox generates paired spokes `hyp0-mc` and `hyp1-mc`
- THEN each spoke SHALL register with its corresponding parent
- AND each SHALL receive its own PostgreSQL server and credentials
- AND canary SHALL inherit the canary controller image
- AND main SHALL inherit the main controller image
- AND generation SHALL produce the necessary parent and spoke changes in one PR

### Requirement: Delivery Requires Teardown and Recreation

This transition SHALL use a coordinated API, controller, SDK, manifest, and
tooling release. An existing installation selected for replacement SHALL first
be removed through its existing teardown workflow. Replacement SHALL use a fresh
application schema and fresh gateway storage. Teardown is an explicit destructive
operator action; installation or a binary upgrade SHALL NOT perform it implicitly.
Existing installations that are not selected for replacement SHALL stay on their
current release and resources.

Hypbox's workflow SHALL remain: generate a PR, merge it, then run deployment
from the approved main revision. CI SHALL validate changes but SHALL NOT deploy
clusters. The release documentation SHALL identify the unsupported in-place
upgrade and the required teardown-and-recreate path. A matching replacement
SHALL NOT require data migration, a legacy database catalog, or a v2 API.

#### Scenario: Fresh replacement installation

- GIVEN the operator has completed teardown of the selected old installation
- WHEN the matching replacement release is deployed with fresh storage
- THEN no database-registration API call SHALL be necessary
- AND gateway provisioning SHALL use the controller-local connection contract

### Requirement: Existing Installations Are Rejected Before Mutation

The deployment preflight SHALL distinguish a fresh installation, an installation
already on the controller-local schema generation, and an old or unknown
installation. It SHALL reject an old or unknown target before changing its
Deployments, Terraform resources, credentials, or schema. Failure to inspect the
target SHALL NOT be interpreted as an empty installation. Automatic image updates
SHALL NOT bypass this preflight. A general confirmation flag such as `--yes`
SHALL NOT bypass the generation check.

The API server and every schema-initialization entry point SHALL independently
inspect the application schema before automatic migrations or schema writes.
Only an empty application schema or a recognized controller-local schema generation
may proceed. Legacy tables, legacy migration state, or an unknown generation SHALL
produce a clear unsupported-upgrade error and a non-ready result, with no schema
or data writes. An empty legacy table does not make a legacy schema fresh. Initial
schema creation and generation marking SHALL be serialized so concurrent starts
cannot bypass the check. Under that lock, a verified empty schema SHALL receive
an initialization record before other schema changes. Only a recognized record
for this generation SHALL permit an interrupted bootstrap to resume.

Deployment tooling and controllers SHALL reject an incompatible API generation
before reconciliation. They SHALL NOT fall back to the old database-provider
runtime. Failure of a new process SHALL leave the old schema and data available
to existing pods. This is a rejection safeguard, not support for mixed versions.

#### Scenario: A new API pod starts beside old pods

- GIVEN old API pods use an application schema containing database references
- WHEN a new-release API pod starts during an accidental rolling update
- THEN it SHALL reject the schema before migrations or any database writes
- AND it SHALL NOT drop tables or columns used by the old pods
- AND the existing rows and credentials SHALL remain unchanged

#### Scenario: Hypbox deploy targets an old installation

- GIVEN the target still has an old release or legacy schema
- WHEN the user runs deployment, including with `--yes`
- THEN preflight SHALL stop before apply or rollout
- AND it SHALL explain that explicit teardown and recreation are required
- AND it SHALL NOT schedule cleanup or change the existing installation

#### Scenario: Fresh bootstrap is interrupted

- GIVEN two matching API pods start against a fresh application schema
- WHEN schema initialization is interrupted and retried
- THEN initialization SHALL resume under its lock without misclassifying legacy state
- AND the installation SHALL become ready only with a valid new-generation schema

## Acceptance Coverage

The implementation SHALL demonstrate the following with automated tests:

- Fresh gateway creation without `database_id` or a database seed step.
- One SQL database and role per gateway, with cross-gateway access denied.
- Equivalent gateway behavior with a supplied RDS server and CNPG server.
- Restart, duplicate event, concurrent attempt, and partial-creation recovery.
- Redacted errors, unavailable secrets, invalid TLS trust, insufficient SQL
  privileges, and credential renewal on the same server.
- Rejection of a changed destination before any SQL or credential mutation.
- Hub/spoke isolation for initial lists, watches, retries, and deletion.
- Durable cleanup after controller restart and temporary PostgreSQL failure.
- Deletion that leaves the server, storage, and other gateways intact.
- Fresh schema and matching generated clients without database-selection fields.
- Rejection of populated and empty legacy schemas before migrations or writes.
- An old API pod still reading and writing while a new pod rejects the old schema.
- Deployment preflight rejection before apply, including when inspection fails.
- Controller rejection of an old or unknown API generation before reconciliation.
- Serialized fresh bootstrap, interrupted initialization, and safe retry.
- Single and paired hypbox generation for both RDS and CNPG, with distinct
  credentials, inherited image pins, and no seed-only resources.

Actual RDS validation SHALL include its administrative privilege constraints.
A local PostgreSQL substitute alone SHALL NOT count as that validation.
