# Audit Capture

**ID prefix:** SEC-AUD

**Status:** Draft
**Related:** `security/rbac-enforcement.spec.md` (request authorization and the platform-admin audit requirement SEC-RBAC-12), `platform/openshell-gateway-service-accounts.spec.md` (the separate service-account lifecycle audit, OSG-SACC-15), `platform/api-server-observability.spec.md` (tracing and log correlation), `standards/security/security.spec.md` (secret handling and log hygiene)
**Upstream:** TRex audit capture (rh-trex-ai spec SEC-004), adopted through the pinned `github.com/openshift-online/rh-trex-ai/components/api-server` dependency.

## Purpose

The HyperShell API server SHALL keep a request-level audit trail that answers "who
did what, to which resource, when, and with what result" for every mutating request,
every refused request, and every failed login, on both the REST and the gRPC
interfaces.

The trail is produced by the TRex framework audit capture (upstream spec SEC-004) so
that every plugin is covered by one cross-cutting mechanism and no plugin has to write
an audit line by hand. This specification defines the HyperShell-specific decisions
that make the captured events useful: which sensitive read kinds opt in, how the real
client address is resolved behind the router and the web console BFF, how audit lines
are routed and retained, and how integration tests observe audit events.

This audit trail is distinct from two existing records that are retained unchanged:
the TRex `events` table, which is a controller and gRPC watch work queue and is not an
audit log; and the OpenShellGatewayServiceAccount lifecycle audit table
(`open_shell_gateway_service_account_audit_events`, OSG-SACC-15), which is a
domain-specific record of service-account lifecycle events.

## Requirements

### Requirement: SEC-AUD-01 -- Framework-Level Request Audit Capture

The API server SHALL emit one audit event for each completed request through the TRex
framework audit capture (upstream spec SEC-004), on both the REST and the gRPC
interfaces, without per-plugin code.

An audit event SHALL be emitted for each of:

- every create, update, patch, delete, and custom action;
- every denied request;
- every authorizer error;
- every failed login.

Each audit event SHALL identify:

- the actor;
- the action;
- the resource kind and ID;
- the outcome;
- the HTTP status or gRPC code;
- the source IP;
- the operation ID;
- the trace ID.

An audit event SHALL NOT contain request bodies, query strings, tokens, or
credentials.

Because capture is a framework concern, a plugin that adds a new mutating route or
gRPC method SHALL be covered without adding an audit call.

#### Scenario: SEC-AUD-01.01 -- Change over REST emits one event

- GIVEN audit capture is enabled
- WHEN an authenticated caller updates a gateway over REST and the request completes
- THEN the API server SHALL emit exactly one audit event for that request
- AND the event SHALL carry the actor, the action, the resource kind and ID, the outcome, the HTTP status, the source IP, the operation ID, and the trace ID

#### Scenario: SEC-AUD-01.02 -- Denied request emits one event

- GIVEN audit capture is enabled
- AND a caller lacks permission to delete a gateway
- WHEN the caller attempts the delete and the request is refused
- THEN the API server SHALL emit one audit event with a denied outcome

#### Scenario: SEC-AUD-01.03 -- Change over gRPC emits one event

- GIVEN audit capture is enabled
- WHEN a platform administrator deletes a gateway over gRPC and the request completes
- THEN the API server SHALL emit one audit event with action `delete`, resource kind `gateways`, the administrator as actor, and the gRPC code

#### Scenario: SEC-AUD-01.04 -- Events carry no secrets or bodies

- GIVEN audit capture is enabled
- WHEN any audited request is processed, including one that carries a token, a credential, or a request body
- THEN the emitted audit event SHALL NOT contain the request body, the query string, any token, or any credential

### Requirement: SEC-AUD-02 -- Audit Output, Schema, and Enable Flag

By default the TRex recorder SHALL write one JSON line per audit event to stdout, each
line carrying `schema_version: 1`. Application logs SHALL remain on stderr. The default
recorder SHALL write nothing to the database.

Audit capture SHALL be controllable with the `--enable-audit` flag. When a downstream
audit destination is not yet available a deployment MAY run with `--enable-audit=false`,
and SHALL remove that override once a destination exists.

#### Scenario: SEC-AUD-02.01 -- One JSON line per event on stdout

- GIVEN the default recorder and audit capture enabled
- WHEN an audited request completes
- THEN the API server SHALL write one JSON line with `schema_version: 1` to stdout
- AND application log lines SHALL continue to go to stderr

#### Scenario: SEC-AUD-02.02 -- Default recorder does not touch the database

- GIVEN the default recorder
- WHEN audited requests are processed
- THEN no audit event SHALL be written to the database

### Requirement: SEC-AUD-03 -- Recorded Read Kinds

TRex SHALL NOT record reads by default. HyperShell SHALL opt in read recording only for
the sensitive kinds secret sources (`secret_sources`) and service accounts
(`service_accounts`). No other kind SHALL opt in to read recording.

Each opted-in kind name SHALL match a registered route or gRPC method so that the
startup warning about an audited kind with no route or gRPC method does not appear.

#### Scenario: SEC-AUD-03.01 -- Reads of sensitive kinds are recorded

- GIVEN audit capture is enabled
- WHEN an authenticated caller reads a `secret_sources` resource or a `service_accounts` resource
- THEN the API server SHALL emit an audit event for that read

#### Scenario: SEC-AUD-03.02 -- Reads of other kinds are not recorded

- GIVEN audit capture is enabled
- WHEN an authenticated caller reads a `gateways` resource
- THEN the API server SHALL NOT emit an audit event for that read

#### Scenario: SEC-AUD-03.03 -- No startup warning for an audited kind

- GIVEN read recording is opted in for `secret_sources` and `service_accounts`
- WHEN the server starts
- THEN no warning SHALL be emitted about an audited kind that has no route or gRPC method

### Requirement: SEC-AUD-04 -- Trusted Proxies and Client Source Address

Requests reach the API server through the OpenShift router and through the web console
BFF. The API server SHALL resolve `source.ip` to the real client address by trusting the
proxy hops configured with `--audit-trusted-proxies`, set to the network range or ranges
of the router and the BFF, and SHALL record the received forwarded chain in
`source.forwarded_for`. The immediate peer address SHALL always be available as
`source.peer_ip`.

Without trusted-proxy configuration, `source.ip` SHALL be the address of the nearest
proxy rather than the client. Deployments SHALL set the trusted proxy ranges that are
correct for each cluster type, using a per-cluster overlay (for example ROSA, ROKS, and
Kind) when the ranges differ.

#### Scenario: SEC-AUD-04.01 -- Router traffic resolves the real client

- GIVEN `--audit-trusted-proxies` includes the router or pod network range
- WHEN a client sends a request through the OpenShift router
- THEN the audit event SHALL record the real client address in `source.ip`
- AND SHALL record the received hop chain in `source.forwarded_for`

#### Scenario: SEC-AUD-04.02 -- Per-cluster trusted ranges

- GIVEN the router or pod network range differs between ROSA, ROKS, and Kind
- WHEN the API server is deployed to each cluster type
- THEN each deployment SHALL set `--audit-trusted-proxies` to the range correct for that cluster through its overlay

### Requirement: SEC-AUD-05 -- Console Client Address Forwarding

The web console BFF calls the API server with the user's token, so the audit actor is
already correct for console-originated requests. To record the real client address
rather than the BFF pod address, the BFF SHALL add the originating client address to the
`X-Forwarded-For` header on each call it makes to the API server, and the BFF pod
network range SHALL be included in `--audit-trusted-proxies`.

#### Scenario: SEC-AUD-05.01 -- Console request records the real client

- GIVEN the BFF forwards the client address in `X-Forwarded-For`
- AND the BFF pod range is in `--audit-trusted-proxies`
- WHEN a user drives a mutating action through the web console
- THEN the audit event SHALL record the user as actor and the real client address in `source.ip`

### Requirement: SEC-AUD-06 -- Audit Log Routing, De-duplication, and Retention

Until a SIEM destination exists, stdout and stderr share one container log, so audit
lines have the same access and retention as application logs. The log pipeline SHALL
route audit lines, identified by the `schema_version` key or by the stdout stream, to a
separate audit destination. It SHALL retain audit lines at least as long as the security
standard requires. It SHALL de-duplicate audit lines using `event_id`.

#### Scenario: SEC-AUD-06.01 -- Audit lines routed to a separate destination

- GIVEN the log pipeline is configured for audit
- WHEN the API server writes audit lines and application lines to the container log
- THEN the pipeline SHALL route the audit lines, identified by `schema_version` or the stdout stream, to the audit destination

#### Scenario: SEC-AUD-06.02 -- Duplicate audit lines are collapsed by event_id

- GIVEN the same audit line is delivered more than once
- WHEN the pipeline ingests the lines
- THEN it SHALL keep one record per `event_id`

### Requirement: SEC-AUD-07 -- Pluggable Recorder and Test Assertions

The audit recorder SHALL be pluggable through the optional `OverrideAudit` environment
method. The integration-test environment SHALL override the recorder with an in-memory
recorder so that tests can assert on audit events and test output stays clean.

An integration test SHALL assert that a platform administrator deleting a gateway over
gRPC produces a `delete` event for kind `gateways` with the administrator as actor.

#### Scenario: SEC-AUD-07.01 -- Integration test asserts on a gRPC gateway delete event

- GIVEN the integration-test environment uses the in-memory recorder
- WHEN a test deletes a gateway as a platform administrator over gRPC
- THEN the test SHALL find a `delete` audit event for kind `gateways` with that administrator as actor

### Requirement: SEC-AUD-08 -- Relationship to Existing Audit Records

Framework-level request audit capture records platform-admin view and delete actions on
both REST and gRPC. The hand-written `platform:admin action` log lines in the gateways
REST handler SHALL be removed, because TRex now records those requests on both
interfaces, and the REST-only hand-written lines left the gRPC delete path with no
record.

The TRex `events` table SHALL remain a controller and gRPC watch work queue. It SHALL
NOT be used as an audit log and SHALL NOT be changed by this adoption.

The OpenShellGatewayServiceAccount lifecycle audit table
(`open_shell_gateway_service_account_audit_events`, OSG-SACC-15) SHALL be retained
unchanged as a separate domain-specific record.

#### Scenario: SEC-AUD-08.01 -- Hand-written admin log lines are removed

- GIVEN framework-level audit capture records gateway view and delete on REST and gRPC
- WHEN a platform administrator views or deletes a gateway
- THEN the API server SHALL rely on the captured audit event
- AND SHALL NOT emit a separate hand-written `platform:admin action` log line

#### Scenario: SEC-AUD-08.02 -- The events table is unchanged

- GIVEN the TRex `events` table serves controllers and watch streams
- WHEN audit capture is adopted
- THEN the `events` table SHALL NOT be used as an audit log and SHALL NOT be changed
