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
