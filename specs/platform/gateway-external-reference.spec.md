# Gateway External Reference Specification

## Purpose

An external control plane needs one stable Gateway for each resource that it manages. The Gateway API accepts a caller-scoped reference so the control plane can recover from a lost response without a duplicate Gateway.

## Requirements

### Requirement: Caller-scoped creation

The REST Gateway create request MAY contain `external_reference`. The value MUST contain 1 to 255 bytes. It MUST NOT contain control characters or surrounding whitespace. The API SHALL derive the scope from the authenticated caller. The caller MUST NOT supply or change that scope.

An authenticated caller with create permission SHALL receive the same live Gateway when it repeats a create request with the same reference. A repeated request SHALL NOT update the Gateway configuration. Requests without a reference SHALL retain independent creation behavior.

#### Scenario: Lost response

- GIVEN an external control plane creates a Gateway with reference `control/instance/project/incarnation`
- AND the create response does not reach the external control plane
- WHEN the external control plane repeats the create request with the same caller identity and reference
- THEN the API returns status 201 and the original Gateway identifier and namespace
- AND the API does not create another database, owner binding, or Gateway event

#### Scenario: Concurrent creation

- GIVEN no Gateway exists for a caller and reference
- WHEN multiple create requests arrive at the same time
- THEN all successful responses identify one Gateway
- AND one database and one owner binding exist for that Gateway

#### Scenario: Another caller uses the same reference

- GIVEN one caller owns a Gateway with reference `control/instance/project/incarnation`
- WHEN another authorized caller creates a Gateway with that reference
- THEN the second caller receives a separate Gateway
- AND the first caller's Gateway and access bindings do not change

### Requirement: Atomic creation and access

The Gateway, its database placement, its owner binding, and their create events SHALL commit together. Failure to create owner access SHALL leave no partial resources. A retry SHALL NOT restore an access binding that an administrator removed.

#### Scenario: Owner access fails

- GIVEN the owner binding cannot be stored
- WHEN a caller creates a Gateway with an external reference
- THEN the API returns an error
- AND the Gateway, database, owner binding, and create events do not persist
- AND a later retry can create the complete resource set

#### Scenario: Access is revoked

- GIVEN an administrator removed the creator's access to a Gateway
- WHEN that creator repeats the create request
- THEN the API returns status 403
- AND the API does not restore access

### Requirement: Exact recovery lookup

`GET /api/hypershell/v1/gateways?external_reference=<value>` SHALL return only live Gateways with an exact reference match in the authenticated creator's scope. Normal Gateway visibility rules SHALL also apply. Missing, deleted, or inaccessible matches SHALL produce an empty list. An empty reference or multiple reference parameters SHALL produce a validation error.

### Requirement: Stable binding

The external reference and its caller scope SHALL be immutable. REST updates and internal whole-row updates SHALL NOT reassign them. The Gateway response, generated SDKs, and gRPC Gateway message SHALL expose the external reference. The caller scope SHALL remain private. Reference-based creation SHALL use the authenticated REST API.

A deleted Gateway SHALL retain its reference reservation. A create retry for that caller and reference SHALL return status 409. A new external resource incarnation MUST use a new reference.


### Requirement: Observable deletion completion

`GET /api/hypershell/v1/gateways/deletion?external_reference=<value>` SHALL return the Gateway identifier, external reference, and state. The states SHALL be `active`, `requested`, or `completed`. A requested deletion SHALL include `deletion_requested_at`. A completed deletion SHALL also include `deletion_completed_at`. The timestamps SHALL use RFC 3339.

Only the immutable reference creator SHALL have access to this status. Gateway role removal SHALL NOT remove the creator's access to this limited status. Status access SHALL NOT restore Gateway access. A missing reference in the caller's scope SHALL return status 404. A caller SHALL NOT infer another caller's resource from this endpoint.

An accepted DELETE or a missing live Gateway SHALL NOT prove completion. The control plane SHALL record completion only after it confirms that the Gateway namespace and its dedicated database namespace are absent. External database objects, credential RBAC, cluster RBAC, and Keycloak clients SHALL be removed before completion. Cleanup failures SHALL remain pending and SHALL be retried. Finalizers SHALL keep deletion pending until the resource is absent.

The API SHALL retain pending deletion records across restarts. An allowlisted control plane identity SHALL recover those records through the Gateway watch replay mode. Replay SHALL use keyset pagination so concurrent completion does not skip records. Completed records SHALL be omitted. Only an allowlisted control plane identity SHALL submit completion or read deleted ManagedDatabase configuration. Gateway owners and platform administrators SHALL NOT obtain these internal permissions through their roles.

#### Scenario: Deletion is accepted while resources remain

- GIVEN a Gateway namespace has a finalizer
- WHEN the Gateway deletion is accepted
- THEN status remains `requested`
- WHEN the namespace and all other owned resources are absent
- THEN the control plane records `completed`
- AND a repeated completion report does not change the completion timestamp

#### Scenario: Control plane restarts during deletion

- GIVEN a Gateway with an external reference was deleted while the control plane was unavailable
- WHEN the control plane reconnects
- THEN it receives the pending deletion record
- AND it retries cleanup until all owned resources are absent
- AND it records completion

#### Scenario: Revoked creator checks deletion

- GIVEN the Gateway creator's access binding was removed
- WHEN the creator reads deletion status using its original reference
- THEN the API returns the limited deletion status
- AND the API does not restore Gateway access
- AND another caller receives status 404 for that reference
