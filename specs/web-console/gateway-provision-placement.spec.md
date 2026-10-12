# Gateway Provision Placement Specification

## Purpose

Define the placement choices presented while provisioning an OpenShell gateway and the server-side resolution of those choices to a concrete `ManagedCluster`. ManagedClusters self-declare independent capability labels, and placement uses match-all label selectors rather than a hard-coded provider-by-network matrix. The provision form SHALL keep the user-facing provider, network, and version choices, map them to selectors, and submit placement intent rather than a cluster identifier. The backend SHALL randomly select a matching eligible cluster and persist its identifier on the created gateway.

## Requirements

### Requirement: GP-01 -- Placement Intent Contract

The gateway provision request SHALL contain the gateway name and one of these placement intents:

- `{ selector: ["network/vpn", "provider/aws"] }`
- `{ selector: ["network/public", "provider/aws" | "provider/ibm"] }`
- `{ mode: "local-kind" }`

A managed-placement selector MAY additionally carry one `gateway-version/<tag>` label when the user targets a specific provisioning version, for example `{ selector: ["network/public", "provider/aws", "gateway-version/v0.1.2-rhaiv.7"] }`. When the user leaves the version choice at its default, the selector SHALL omit the `gateway-version` label, and the backend SHALL place on any eligible cluster regardless of the version it advertises. The selector SHALL carry at most one `gateway-version` label. The `local-kind` mode SHALL NOT carry a version selector.

The client SHALL NOT submit a user-selected `cluster_id`. The backend SHALL resolve and assign the concrete `cluster_id` before creating the gateway.

#### Scenario: Version choice is added to the selector

- GIVEN the user enters a valid gateway name
- AND selects Public and Amazon Web Services
- AND selects the provisioning version `v0.1.2-rhaiv.7`
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/public", "provider/aws", "gateway-version/v0.1.2-rhaiv.7"]`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: Default version omits the version label

- GIVEN the user enters a valid gateway name
- AND selects Public and Amazon Web Services
- AND leaves the provisioning version at its default
- WHEN the user submits the form
- THEN the request selector SHALL NOT contain a `gateway-version` label

#### Scenario: Public IBM placement is submitted as intent

- GIVEN the user enters a valid gateway name
- AND selects Public
- AND selects IBM Cloud
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/public", "provider/ibm"]`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: VPN placement is submitted as AWS intent

- GIVEN the user enters a valid gateway name
- AND selects VPN
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/vpn", "provider/aws"]`
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

IBM Cloud SHALL be selected by default when Public becomes active when its selector is available. When VPN becomes active, the form SHALL select an available provider. A provider SHALL be disabled only when no connected cluster matches the corresponding provider and network selector.

#### Scenario: Public defaults to IBM Cloud

- GIVEN the user selects Public
- AND a connected cluster matches `provider/ibm` and `network/public`
- WHEN the provider controls render
- THEN IBM Cloud SHALL be selected
- AND Amazon Web Services SHALL be unselected

#### Scenario: VPN provider choices follow capabilities

- GIVEN the user selects VPN
- WHEN the provider controls render
- THEN each provider SHALL be enabled only when a connected cluster matches its provider and `network/vpn` labels
- AND the form SHALL select an available provider when one exists

#### Scenario: Provider is required

- GIVEN a network visibility is selected
- AND neither AWS nor IBM Cloud is selected
- WHEN the user activates Provision gateway
- THEN the form SHALL identify provider as required
- AND no provision request SHALL be sent

### Requirement: GP-04 -- Placement Availability

The form SHALL obtain placement availability from current managed-cluster capability data. It SHALL send the finite set of selectors rendered by the form in one batch request, including the version-bearing selectors needed to populate the provisioning-version choice (GP-11). The response SHALL contain one result per normalized selector, including whether at least one registered cluster with a connected control plane contains every requested label. The response SHALL NOT expose an enumerated `aws_public`, `aws_vpn`, `ibm_public`, or `ibm_vpn` matrix.

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

#### Scenario: IBM Cloud VPN availability is data-driven

- GIVEN no connected cluster has both `provider/ibm` and `network/vpn`
- AND the user selects VPN
- WHEN the provider controls render
- THEN IBM Cloud SHALL be disabled
- AND the form SHALL communicate that no eligible cluster matches the choice

#### Scenario: A new capability combination needs no schema change

- GIVEN a connected cluster registers `provider/ibm` and `network/vpn`
- WHEN the form requests availability for that selector
- THEN the selector SHALL be available
- AND no gateway API schema or placement code change SHALL be required

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

The backend SHALL validate every selector label using the ManagedCluster capability vocabulary, including a `gateway-version/<tag>` label under the relaxed version-value syntax of `managed-cluster-registration.spec.md`. It SHALL normalize order and duplicates, reject malformed or unknown labels, and resolve a valid selector to registered clusters with connected control planes whose capability sets contain every requested label. Selector matching SHALL be literal and case-sensitive after validation; partial, wildcard, and match-any semantics SHALL NOT apply. A selector that omits `gateway-version` SHALL match an eligible cluster regardless of the version it advertises; a selector that includes `gateway-version/<tag>` SHALL match only a cluster advertising that exact label.

When one eligible cluster matches, the backend SHALL select that cluster. When multiple eligible clusters match, the backend SHALL select one at random. Each matching eligible cluster SHALL have an equal probability of selection.

The backend SHALL NOT inspect, score, or rank clusters by CPU, memory, pod usage, or other capacity data. The client SHALL neither calculate nor submit the selected cluster.

If no eligible cluster matches the selector, the backend SHALL return a placement-unavailable error. The backend SHALL NOT drop selector labels or fall back to another selector or local-kind.

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

The API contract, generated clients or adapters, authorization boundaries, and gateway provisioning tests SHALL be updated consistently for selector-based placement and batched availability. Existing gateway records and read APIs SHALL continue to expose their resolved `cluster_id`. Gateway creation SHALL require selector-based placement and SHALL NOT accept the former provider/network request shape.

The Gateway table SHALL NOT require a migration unless the implementation introduces persisted placement-intent fields. Placement intent MAY remain request-scoped after the backend resolves `cluster_id`.

#### Scenario: Existing gateway placement remains readable

- GIVEN a gateway was created through the placement-intent flow
- WHEN a gateway detail or list API is read
- THEN the response SHALL expose the resolved cluster using the existing gateway placement representation

### Requirement: GP-11 -- Provisioning Version Selection

The provision form SHALL present a provisioning-version choice for managed placement, driven entirely by the `gateway-version` labels that connected spokes advertise for the currently selected network and provider. The form SHALL offer a default option ("Any version", or equivalent localized wording) that omits the `gateway-version` selector label, and one option per distinct `gateway-version/<tag>` advertised by an eligible cluster matching the selected network and provider. The form SHALL NOT offer a free-text version field, and SHALL NOT offer a version that no connected eligible spoke advertises.

The version choice SHALL default to "Any version". Changing the network or provider SHALL re-derive the available versions from availability data and SHALL reset the version choice to "Any version" when the previously selected version is no longer advertised for the new network and provider. The version choice SHALL have no effect on, and SHALL NOT be shown for, `local-kind` placement.

When no eligible cluster matches the selected network and provider, the form SHALL offer only "Any version". A version that becomes unavailable before submission SHALL be rejected by the backend and surfaced as a recoverable provisioning error, consistent with GP-04.

Because the single batched placement-availability request (GP-04) carries a bounded number of selectors, the form probes versions newest-first and MAY cap the number of distinct versions it offers. When a fleet advertises more distinct `gateway-version` tags than that cap, the form SHALL offer the newest capped set; an older tag advertised only by a spoke outside that set MAY be unselectable. One version per spoke makes exceeding the cap unusual. This cap bounds only the provision form's choice; it does not affect which clusters the backend resolves for an explicitly submitted version selector.

#### Scenario: Version options follow advertised capabilities

- GIVEN two connected clusters match Public and Amazon Web Services
- AND one advertises `gateway-version/v0.1.2-rhaiv.7` and the other `gateway-version/v0.1.1-rhaiv.3`
- WHEN the user selects Public and Amazon Web Services
- THEN the version choice SHALL offer "Any version", `v0.1.2-rhaiv.7`, and `v0.1.1-rhaiv.3`
- AND no other version SHALL be offered

#### Scenario: Default selects any advertised version

- GIVEN the user selects a network and provider with at least one eligible cluster
- AND leaves the version choice at "Any version"
- WHEN the user submits the form
- THEN the selector SHALL NOT carry a `gateway-version` label
- AND the backend SHALL place on any eligible cluster for that network and provider

#### Scenario: Version resets when no longer offered

- GIVEN the user selected `v0.1.2-rhaiv.7` for Public and Amazon Web Services
- AND no eligible IBM Cloud cluster advertises that version for Public
- WHEN the user changes the provider to IBM Cloud
- THEN the version choice SHALL reset to "Any version"

### Requirement: GP-10 -- Accessibility and Localization

The network, provider, version, availability, disabled-state, validation, local-kind, and error messages SHALL be localized. The controls SHALL expose accessible names and selected or disabled state through semantic PatternFly form controls. Keyboard users SHALL be able to select and change network, provider, version, and local-kind placement without pointer input.

#### Scenario: Disabled provider is announced

- GIVEN VPN is selected
- WHEN a keyboard or assistive-technology user inspects the provider controls
- THEN IBM Cloud SHALL be exposed as disabled
- AND the unavailable reason SHALL be conveyed in localized text or an accessible description

## Non-Goals

- Allowing users to choose both VPN and Public.
- Hard-coding support or rejection for any provider and network combination.
- Allowing users to select a concrete managed cluster from the provision form.
- A free-text or unvalidated version field, or offering a version no connected spoke advertises.
- In-place gateway upgrades: a version choice only targets newly created gateways.
- Capacity-aware cluster selection or placement scoring.
- Offering every distinct advertised version in the provision form when a fleet advertises more than the availability-batch cap allows: the form offers the newest capped set (GP-11).
- Changing existing gateway naming rules.
- Changing the resolved `cluster_id` representation on existing gateway records.
