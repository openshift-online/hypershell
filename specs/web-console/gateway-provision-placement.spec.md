# Gateway Provision Placement Specification

## Purpose

Define the placement choices presented while provisioning an OpenShell gateway and the server-side resolution of those choices to a concrete `ManagedCluster`. The provision form SHALL let a user choose exactly one network visibility: VPN or public. Public placement SHALL support AWS and IBM Cloud. VPN placement SHALL support AWS only. The client SHALL submit placement intent rather than a cluster identifier. The backend SHALL randomly select a matching eligible cluster and persist its identifier on the created gateway.

## Requirements

### Requirement: GP-01 -- Placement Intent Contract

The gateway provision request SHALL contain the gateway name and one of these placement intents:

- `{ network: "vpn", provider: "aws" }`
- `{ network: "public", provider: "aws" | "ibm" }`
- `{ mode: "local-kind" }`

The client SHALL NOT submit a user-selected `cluster_id`. The backend SHALL resolve and assign the concrete `cluster_id` before creating the gateway.

#### Scenario: Public IBM placement is submitted as intent

- GIVEN the user enters a valid gateway name
- AND selects Public
- AND selects IBM Cloud
- WHEN the user submits the form
- THEN the request SHALL contain `network: public` and `provider: ibm`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: VPN placement is submitted as AWS intent

- GIVEN the user enters a valid gateway name
- AND selects VPN
- WHEN the user submits the form
- THEN the request SHALL contain `network: vpn` and `provider: aws`
- AND the request SHALL NOT contain a concrete `cluster_id`

### Requirement: GP-02 -- Network Visibility Selection

The provision form SHALL present VPN and Public as mutually exclusive choices. The form SHALL require exactly one network visibility before submission. Selecting one visibility SHALL clear the other visibility.

The form SHALL NOT present the former cluster selector.

#### Scenario: Network visibility is required

- GIVEN the gateway name is valid
- AND neither VPN nor Public is selected
- WHEN the user activates Provision gateway
- THEN the form SHALL identify network visibility as required
- AND no provision request SHALL be sent

#### Scenario: VPN and Public cannot both be selected

- GIVEN Public is selected
- WHEN the user selects VPN
- THEN VPN SHALL become selected
- AND Public SHALL become unselected

### Requirement: GP-03 -- Provider Selection

The provision form SHALL present AWS and IBM Cloud as mutually exclusive choices. The form SHALL require exactly one provider before submission.

The provider choices SHALL be labeled Amazon Web Services and IBM Cloud and include the following user guidance:

- IBM Cloud: "The default home for gateways. General-purpose workloads with no special network or data needs."
- Amazon Web Services: "For workloads that rely heavily on AWS services or data."

The VPN network choice SHALL display the label "Red Hat VPN required" above the guidance "For gateways that need to reach GitLab and other internal Red Hat services."

IBM Cloud SHALL be selected by default when Public becomes active. When VPN becomes active, AWS SHALL be selected and IBM Cloud SHALL be disabled. The user SHALL NOT be able to submit VPN placement with IBM Cloud.

#### Scenario: Public defaults to IBM Cloud

- GIVEN the user selects Public
- WHEN the provider controls render
- THEN IBM Cloud SHALL be selected
- AND Amazon Web Services SHALL be unselected

#### Scenario: VPN supports AWS only

- GIVEN the user selects VPN
- WHEN the provider controls render
- THEN Amazon Web Services SHALL be selected
- AND IBM Cloud SHALL be disabled

#### Scenario: Provider is required

- GIVEN a network visibility is selected
- AND neither AWS nor IBM Cloud is selected
- WHEN the user activates Provision gateway
- THEN the form SHALL identify provider as required
- AND no provision request SHALL be sent

### Requirement: GP-04 -- Placement Availability

The form SHALL obtain placement availability from current managed-cluster data. Each managed cluster used for placement SHALL identify its provider and supported network visibility in backend-owned placement data. AWS SHALL support Public and VPN placement. IBM Cloud SHALL support Public placement only. For each supported network and provider pair, availability SHALL require at least one registered cluster with a connected control plane that matches both choices.

The form SHALL disable a provider for the selected network when no eligible cluster matches that pair. When neither provider is available for a network, the form SHALL disable that network choice. Disabled controls SHALL communicate why the placement is unavailable.

Placement availability SHALL NOT depend on CPU, memory, pod usage, or other capacity data.

The form SHALL load placement availability automatically when it opens. The form SHALL refresh placement availability after a placement-related submission error. The form SHALL NOT require a separate manual refresh or Retry control.

A placement pair that becomes unavailable before submission SHALL be rejected by the backend. The form SHALL present the rejection as a recoverable provisioning error.

#### Scenario: AWS is unavailable for Public

- GIVEN no eligible AWS cluster matches Public
- AND an eligible IBM Cloud cluster matches Public
- WHEN the user selects Public
- THEN AWS SHALL be disabled
- AND IBM Cloud SHALL remain selectable

#### Scenario: IBM Cloud is unsupported for VPN

- GIVEN the user selects VPN
- WHEN the provider controls render
- THEN IBM Cloud SHALL be disabled regardless of IBM Cloud cluster availability
- AND the form SHALL communicate that IBM Cloud does not support VPN placement

#### Scenario: Both managed networks are unavailable

- GIVEN no eligible AWS or IBM Cloud cluster matches Public or VPN
- WHEN the provision form renders
- THEN Public and VPN SHALL be disabled
- AND the form SHALL explain that no managed placement is currently available
- AND the Provision gateway button SHALL remain enabled

#### Scenario: Capacity data does not affect availability

- GIVEN an eligible IBM Cloud cluster matches Public
- AND capacity data for that cluster is unavailable
- WHEN the provider controls render
- THEN IBM Cloud SHALL remain selectable

### Requirement: GP-05 -- Local-kind Development Placement

When the platform is running in the Kind development environment and the registered `local-kind` cluster is connected, and no managed placement is available, the form SHALL expose a **Use local-kind** option. The option SHALL NOT be shown when an AWS or IBM Cloud managed placement is available, or outside the Kind development environment.

Local-kind SHALL be mutually exclusive with VPN and Public. Local-kind SHALL be selected by default only when no managed placement is available.

Selecting local-kind SHALL submit `{ placement: { mode: "local-kind" } }`.

#### Scenario: Local-kind appears for Kind development

- GIVEN the platform is running in the Kind development environment
- AND the registered `local-kind` cluster is connected
- WHEN the provision form renders
- THEN Use local-kind SHALL be visible
- AND Use local-kind SHALL be available for selection

#### Scenario: Local-kind is hidden with managed placement

- GIVEN a managed placement pair is available
- AND the platform is running in the Kind development environment
- AND the registered `local-kind` cluster is connected
- WHEN the provision form renders
- THEN Use local-kind SHALL NOT be visible
- AND managed placement choices SHALL remain available

#### Scenario: Local-kind is hidden outside Kind development

- GIVEN no managed placement pair is available
- AND the platform is not running in the Kind development environment
- WHEN the provision form renders
- THEN Use local-kind SHALL NOT be visible
- AND the form SHALL explain that no managed placement is currently available

### Requirement: GP-06 -- Form Submission Validation

The Provision gateway button SHALL remain enabled regardless of whether the form is complete, valid, or displaying validation errors. Activating the button SHALL run form validation and show field-level errors for invalid or missing values. The form SHALL NOT send a provision request until validation succeeds.

The existing gateway-name validation and normalization rules SHALL remain unchanged.

#### Scenario: Invalid form does not submit

- GIVEN the gateway name or placement choices are invalid
- WHEN the user activates Provision gateway
- THEN the corresponding field-level errors SHALL be shown
- AND no provision request SHALL be sent

#### Scenario: Valid form submits

- GIVEN the gateway name and placement choices are valid
- WHEN the user activates Provision gateway
- THEN the placement-intent request SHALL be sent
- AND the button MAY show a loading state while the request is pending

### Requirement: GP-07 -- Backend Placement Resolution

The backend SHALL reject a VPN IBM Cloud placement intent as unsupported. The backend SHALL resolve every supported managed placement intent to the set of registered clusters with connected control planes that match the requested network and provider.

When one eligible cluster matches, the backend SHALL select that cluster. When multiple eligible clusters match, the backend SHALL select one at random. Each matching eligible cluster SHALL have an equal probability of selection.

The backend SHALL NOT inspect, score, or rank clusters by CPU, memory, pod usage, or other capacity data. The client SHALL neither calculate nor submit the selected cluster.

If no eligible cluster matches the requested network and provider, the backend SHALL return a placement-unavailable error. The backend SHALL NOT fall back to another network, another provider, or local-kind.

The backend SHALL persist the selected cluster identifier on the Gateway record. Gateway reconciliation SHALL use the persisted identifier.

#### Scenario: Backend randomly selects a matching cluster

- GIVEN two eligible AWS clusters match a public AWS intent
- WHEN the backend creates the gateway
- THEN the backend SHALL randomly select one of the two clusters
- AND the persisted Gateway record SHALL contain the selected cluster's identifier

#### Scenario: Backend does not use capacity

- GIVEN two eligible IBM Cloud clusters match a public IBM Cloud intent
- AND one cluster has more available capacity than the other
- WHEN the backend creates the gateway
- THEN both clusters SHALL remain eligible for random selection

#### Scenario: Backend re-evaluates stale availability

- GIVEN the client submits a valid placement intent
- AND the cluster available when the form rendered is no longer eligible
- WHEN the backend resolves placement
- THEN the backend SHALL select another eligible matching cluster
- OR the backend SHALL return a placement-unavailable error

### Requirement: GP-08 -- Local-kind Placement Resolution

When local-kind intent is submitted, the backend SHALL resolve it to the registered `local-kind` `ManagedCluster` in the Kind development environment.

The backend SHALL reject local-kind placement with a typed client-visible error when the platform is not running in the Kind development environment, `local-kind` is not registered, or `local-kind` is not connected.

#### Scenario: Local-kind resolves to the local cluster

- GIVEN the client submits a valid local-kind intent
- AND the registered `local-kind` cluster is connected
- WHEN the backend creates the gateway
- THEN the gateway SHALL be assigned to `local-kind`

### Requirement: GP-09 -- API and Persistence Compatibility

The API contract, generated clients or adapters, authorization boundaries, and gateway provisioning tests SHALL be updated consistently for the placement-intent request. Existing gateway records and read APIs SHALL continue to expose their resolved `cluster_id`.

The change SHALL NOT require a database migration unless the implementation introduces persisted placement-intent fields. Placement intent MAY remain request-scoped after the backend resolves `cluster_id`.

#### Scenario: Existing gateway placement remains readable

- GIVEN a gateway was created through the placement-intent flow
- WHEN a gateway detail or list API is read
- THEN the response SHALL expose the resolved cluster using the existing gateway placement representation

### Requirement: GP-10 -- Accessibility and Localization

The network, provider, availability, disabled-state, validation, local-kind, and error messages SHALL be localized. The controls SHALL expose accessible names and selected or disabled state through semantic PatternFly form controls. Keyboard users SHALL be able to select and change network, provider, and local-kind placement without pointer input.

#### Scenario: Disabled provider is announced

- GIVEN VPN is selected
- WHEN a keyboard or assistive-technology user inspects the provider controls
- THEN IBM Cloud SHALL be exposed as disabled
- AND the unsupported reason SHALL be conveyed in localized text or an accessible description

## Non-Goals

- Allowing users to choose both VPN and Public.
- Supporting IBM Cloud VPN placement.
- Allowing users to select a concrete managed cluster from the provision form.
- Capacity-aware cluster selection or placement scoring.
- Changing existing gateway naming rules.
- Changing the resolved `cluster_id` representation on existing gateway records.
