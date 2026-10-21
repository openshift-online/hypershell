# Gateway Provision Placement Specification

## Purpose

Define the placement choices presented while provisioning an OpenShell gateway and the server-side resolution of those choices to a concrete `ManagedCluster`. ManagedClusters self-declare independent capability labels, and placement uses match-all label selectors rather than a hard-coded provider-by-network matrix. The provision form SHALL keep the user-facing provider and network choices, map them to selectors, and submit placement intent rather than a cluster identifier. The backend SHALL select the gateway version for the resolved cluster. The backend SHALL randomly select a matching eligible cluster and persist its identifier on the created gateway. When no eligible cluster matches, the backend SHALL NOT reject the create: the gateway is created unplaced and late-bound to a matching cluster when one appears (WEB-PLC-07), which lets a declarative Gateway that declares only a selector import before any matching cluster is connected.

The production design-review states are represented by the `Default`, `LocalDevelopmentOnly`, and `VpnUnavailable` stories in `components/web-console/app/features/gateways/gateway-create.stories.tsx`. The implementation mockup and parity story are `specs/web-console/mockups/gateways/provision-gateway.stories.tsx` and `specs/web-console/mockups/gateways/provision-gateway.parity.stories.tsx`.

## Requirements

### Requirement: WEB-PLC-01 -- Placement Intent Contract

The gateway provision request SHALL contain the gateway name and one of these placement intents:

- `{ selector: ["network/vpn", "provider/aws"] }`
- `{ selector: ["network/public", "provider/aws" | "provider/ibm"] }`
- `{ mode: "local-kind" }`

The provision form SHALL NOT submit a `gateway-version` label. The placement contract remains compatible with a managed selector that carries at most one `gateway-version/<tag>` label for non-form clients. The `local-kind` mode SHALL NOT carry a version selector.

A managed-placement intent MAY also carry `excludedSelector`, a non-empty array of capability labels that a candidate cluster SHALL NOT advertise. The required selector and excluded selector SHALL both be validated, normalized, and persisted. The `local-kind` mode SHALL NOT carry a version or excluded selector. This negative selector is required for workflows such as homepage Quick create that must choose IBM Cloud public placement without Kata.

The client SHALL NOT submit a user-selected `cluster_id`. The backend SHALL resolve and assign the concrete `cluster_id` before creating the gateway.

#### Scenario: WEB-PLC-01.01 -- Provision form omits the version label

- GIVEN the user enters a valid gateway name
- AND selects Public and Amazon Web Services
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/public", "provider/aws"]`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: WEB-PLC-01.02 -- Version selector remains API-compatible

- GIVEN a non-form client submits a valid managed-placement selector
- AND that selector contains one `gateway-version/<tag>` label
- WHEN the backend processes the request
- THEN the backend SHALL accept the existing selector shape

#### Scenario: WEB-PLC-01.03 -- Public IBM placement is submitted as intent

- GIVEN the user enters a valid gateway name
- AND selects Public
- AND selects IBM Cloud
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/public", "provider/ibm"]`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: WEB-PLC-01.04 -- VPN placement is submitted as AWS intent

- GIVEN the user enters a valid gateway name
- AND selects VPN
- WHEN the user submits the form
- THEN the request SHALL contain `selector: ["network/vpn", "provider/aws"]`
- AND the request SHALL NOT contain a concrete `cluster_id`

#### Scenario: WEB-PLC-01.05 -- Public IBM placement excludes Kata

- GIVEN a workflow requires Public IBM Cloud placement without Kata
- WHEN the client submits a managed placement intent
- THEN the request SHALL contain `selector: ["network/public", "provider/ibm"]`
- AND the request SHALL contain `excludedSelector: ["runtime/kata"]`

### Requirement: WEB-PLC-02 -- Network Visibility Selection

The provision form SHALL present the question "Does the gateway need to reach GitLab or other internal Red Hat services?" with mutually exclusive choices labeled "No" and "Yes (Requires Red Hat VPN)". The form SHALL require exactly one answer before submission. "No" SHALL map to Public and "Yes" SHALL map to VPN. When both Public and VPN are available and the user has not made a choice, "No" SHALL be selected by default. When only one network is available, the corresponding answer SHALL be selected automatically.

The form SHALL NOT present the former cluster selector.

#### Scenario: WEB-PLC-02.01 -- Network visibility is required

- GIVEN the gateway name is valid
- AND neither VPN nor Public is selected
- WHEN the user activates Provision gateway
- THEN the form SHALL identify network visibility as required
- AND no provision request SHALL be sent

#### Scenario: WEB-PLC-02.02 -- Internal-services answers are mutually exclusive

- GIVEN "No" is selected
- WHEN the user selects "Yes (Requires Red Hat VPN)"
- THEN "Yes" SHALL become selected
- AND "No" SHALL become unselected

### Requirement: WEB-PLC-03 -- Provider Selection

The provision form SHALL present AWS and IBM Cloud as mutually exclusive choices. The form SHALL require exactly one provider before submission.

The provider choices SHALL be labeled "IBM (default)" and "AWS (For workloads that rely heavily on AWS services or data)" and include the following preview-card guidance:

- IBM Cloud: "General-purpose workloads with no special network or data needs."
- AWS: "For publicly accessible workloads that rely heavily on AWS services or data."
- Kata: "For workloads that require special security considerations."

The internal-services "Yes" choice SHALL communicate "Requires Red Hat VPN", and the VPN preview SHALL use the feature label "Red Hat VPN required".

IBM Cloud SHALL be selected by default when Public becomes active when its selector is available. When VPN becomes active, the form SHALL select an available provider. A provider SHALL be disabled only when no connected cluster matches the corresponding provider and network selector.

When Public is the default network, IBM Cloud SHALL be the default provider when available. When only one provider is available for the selected network, that provider SHALL be selected automatically.

#### Scenario: WEB-PLC-03.01 -- Public defaults to IBM Cloud

- GIVEN the user selects Public
- AND a connected cluster matches `provider/ibm` and `network/public`
- WHEN the provider controls render
- THEN IBM Cloud SHALL be selected
- AND Amazon Web Services SHALL be unselected

#### Scenario: WEB-PLC-03.02 -- VPN provider choices follow capabilities

- GIVEN the user selects VPN
- WHEN the provider controls render
- THEN each provider SHALL be enabled only when a connected cluster matches its provider and `network/vpn` labels
- AND the form SHALL select an available provider when one exists

#### Scenario: WEB-PLC-03.03 -- Provider is required

- GIVEN a network visibility is selected
- AND neither AWS nor IBM Cloud is selected
- WHEN the user activates Provision gateway
- THEN the form SHALL identify provider as required
- AND no provision request SHALL be sent

### Requirement: WEB-PLC-04 -- Placement Availability

The form SHALL obtain placement availability from current managed-cluster capability data. It SHALL send the finite set of selectors rendered by the form in one batch request. A request selector MAY include an excluded-selector set. The response SHALL contain one result per normalized selector, including whether at least one registered cluster with a connected control plane contains every required label and none of the excluded labels. The response SHALL NOT expose an enumerated `aws_public`, `aws_vpn`, `ibm_public`, or `ibm_vpn` matrix.

The form SHALL disable a provider for the selected network when no eligible cluster matches that pair. When neither provider is available for a network, the form SHALL disable that network choice. Disabled controls SHALL communicate why the placement is unavailable.

When VPN is unavailable but Public is available, VPN SHALL remain visible as a disabled choice with its unavailability reason. This state is represented by the production `VpnUnavailable` story.

Placement availability SHALL NOT depend on CPU, memory, pod usage, or other capacity data.

The form SHALL load placement availability automatically when it opens. The form SHALL refresh placement availability after a placement-related submission error. The form SHALL NOT require a separate manual refresh or Retry control.

A placement pair that becomes unavailable before submission SHALL NOT be rejected by the backend; the Gateway SHALL be created unplaced in the `Pending` phase and late-bound when a matching cluster appears (WEB-PLC-07). The form MAY surface the resulting awaiting-placement status so the user understands the Gateway is waiting for a matching cluster.

#### Scenario: WEB-PLC-04.01 -- AWS is unavailable for Public

- GIVEN no eligible AWS cluster matches Public
- AND an eligible IBM Cloud cluster matches Public
- WHEN the user selects Public
- THEN AWS SHALL be disabled
- AND IBM Cloud SHALL remain selectable

#### Scenario: WEB-PLC-04.02 -- IBM Cloud VPN availability is data-driven

- GIVEN no connected cluster has both `provider/ibm` and `network/vpn`
- AND the user selects VPN
- WHEN the provider controls render
- THEN IBM Cloud SHALL be disabled
- AND the form SHALL communicate that no eligible cluster matches the choice

#### Scenario: WEB-PLC-04.03 -- A new capability combination needs no schema change

- GIVEN a connected cluster registers `provider/ibm` and `network/vpn`
- WHEN the form requests availability for that selector
- THEN the selector SHALL be available
- AND no gateway API schema or placement code change SHALL be required

#### Scenario: WEB-PLC-04.04 -- Both managed networks are unavailable

- GIVEN no eligible AWS or IBM Cloud cluster matches Public or VPN
- WHEN the provision form renders
- THEN Public and VPN SHALL be disabled
- AND the form SHALL explain that no managed placement is currently available
- AND the Provision gateway button SHALL remain enabled

#### Scenario: WEB-PLC-04.05 -- Capacity data does not affect availability

- GIVEN an eligible IBM Cloud cluster matches Public
- AND capacity data for that cluster is unavailable
- WHEN the provider controls render
- THEN IBM Cloud SHALL remain selectable

### Requirement: WEB-PLC-05 -- Local-kind Development Placement

When the platform is running in the Kind development environment and the registered `local-kind` cluster is connected, and no managed placement is available, the form SHALL expose a **Kind** option under **Local development**. The option SHALL NOT be shown when an AWS or IBM Cloud managed placement is available, or outside the Kind development environment.

Local-kind SHALL be mutually exclusive with VPN and Public. Local-kind SHALL be selected by default only when no managed placement is available.

When local-kind is the only available placement, Kind SHALL be the only placement choice rendered. The managed network and provider groups SHALL NOT be rendered in that state.

When local-kind is the only available placement, the provision preview card
SHALL NOT render a default managed-placement recommendation.

Selecting local-kind SHALL submit `{ placement: { mode: "local-kind" } }`.

#### Scenario: WEB-PLC-05.01 -- Local-kind appears for Kind development

- GIVEN the platform is running in the Kind development environment
- AND the registered `local-kind` cluster is connected
- WHEN the provision form renders
- THEN Kind SHALL be visible under Local development
- AND Kind SHALL be available for selection
- AND Public, VPN, Amazon Web Services, and IBM Cloud SHALL NOT be rendered
- AND the provision preview card SHALL NOT render

#### Scenario: WEB-PLC-05.02 -- Local-kind is hidden with managed placement

- GIVEN a managed placement pair is available
- AND the platform is running in the Kind development environment
- AND the registered `local-kind` cluster is connected
- WHEN the provision form renders
- THEN Kind SHALL NOT be visible
- AND managed placement choices SHALL remain available

#### Scenario: WEB-PLC-05.03 -- Local-kind is hidden outside Kind development

- GIVEN no managed placement pair is available
- AND the platform is not running in the Kind development environment
- WHEN the provision form renders
- THEN Kind SHALL NOT be visible
- AND the form SHALL explain that no managed placement is currently available

### Requirement: WEB-PLC-06 -- Form Submission Validation

The Provision gateway button SHALL remain enabled regardless of whether the form is complete, valid, or displaying validation errors. Activating the button SHALL run form validation and show field-level errors for invalid or missing values. The form SHALL NOT send a provision request until validation succeeds.

The existing gateway-name validation and normalization rules SHALL remain unchanged.

#### Scenario: WEB-PLC-06.01 -- Invalid form does not submit

- GIVEN the gateway name or placement choices are invalid
- WHEN the user activates Provision gateway
- THEN the corresponding field-level errors SHALL be shown
- AND no provision request SHALL be sent

#### Scenario: WEB-PLC-06.02 -- Valid form submits

- GIVEN the gateway name and placement choices are valid
- WHEN the user activates Provision gateway
- THEN the placement-intent request SHALL be sent
- AND the button MAY show a loading state while the request is pending

### Requirement: WEB-PLC-07 -- Backend Placement Resolution

The backend SHALL validate every required and excluded selector label using the ManagedCluster capability vocabulary, including a `gateway-version/<tag>` label under the relaxed version-value syntax of `managed-cluster-registration.spec.md`. It SHALL normalize order and duplicates, reject malformed or unknown labels, and resolve a valid selector to registered clusters with connected control planes whose capability sets contain every required label and none of the excluded labels. Selector matching SHALL be literal and case-sensitive after validation; partial, wildcard, and match-any semantics SHALL NOT apply. A required selector that omits `gateway-version` SHALL match an eligible cluster regardless of the version it advertises; a required selector that includes `gateway-version/<tag>` SHALL match only a cluster advertising that exact label.

When one eligible cluster matches, the backend SHALL select that cluster. When multiple eligible clusters match, the backend SHALL select one at random. Each matching eligible cluster SHALL have an equal probability of selection.

The backend SHALL NOT inspect, score, or rank clusters by CPU, memory, pod usage, or other capacity data. The client SHALL neither calculate nor submit the selected cluster.

If no eligible cluster matches the selector, the backend SHALL NOT reject the create: the Gateway is LATE-BOUND. The backend SHALL create the Gateway with an empty `cluster_id`, hold it in the `Pending` phase with a clear awaiting-placement `status`, and bind it when a matching cluster appears, per [`data-model.spec.md`](../platform/data-model.spec.md) ("Requirement: Gateway Placement and Late Binding"). The backend SHALL NOT drop selector labels or fall back to another selector or local-kind. This late-binding model lets a declarative `.hypershell` Gateway (which declares a selector and no `cluster_id`) import successfully even when no matching cluster is connected yet.

The backend SHALL persist BOTH the `placement` required and excluded selectors (so it can re-resolve for late binding) and, once resolved, the selected cluster identifier on the Gateway record. Gateway reconciliation SHALL use the persisted `cluster_id`. The backend SHALL re-resolve placement for an unplaced Gateway when the eligible-cluster set changes, assigning `cluster_id` as soon as a match appears.

#### Scenario: WEB-PLC-07.01 -- Backend randomly selects a matching cluster

- GIVEN two eligible AWS clusters match a public AWS intent
- WHEN the backend creates the gateway
- THEN the backend SHALL randomly select one of the two clusters
- AND the persisted Gateway record SHALL contain the selected cluster's identifier

#### Scenario: WEB-PLC-07.02 -- Backend does not use capacity

- GIVEN two eligible IBM Cloud clusters match a public IBM Cloud intent
- AND one cluster has more available capacity than the other
- WHEN the backend creates the gateway
- THEN both clusters SHALL remain eligible for random selection

#### Scenario: WEB-PLC-07.03 -- Backend re-evaluates stale availability

- GIVEN the client submits a valid placement intent
- AND the cluster available when the form rendered is no longer eligible
- WHEN the backend resolves placement
- THEN the backend SHALL select another eligible matching cluster
- OR the backend SHALL create the Gateway unplaced in the `Pending` phase, awaiting late binding

#### Scenario: WEB-PLC-07.04 -- Selector with no eligible cluster creates an unplaced Gateway

- GIVEN a valid placement selector that no connected cluster matches
- WHEN the backend creates the gateway
- THEN the Gateway SHALL be created with an empty `cluster_id` and `phase` `Pending`
- AND the create SHALL NOT return a placement-unavailable error
- AND the backend SHALL assign `cluster_id` when a matching cluster later becomes eligible

#### Scenario: WEB-PLC-07.05 -- Excluded capability removes a candidate

- GIVEN two connected clusters match `network/public` and `provider/ibm`
- AND one advertises `runtime/kata`
- WHEN the backend resolves an intent that excludes `runtime/kata`
- THEN the backend SHALL select the cluster that does not advertise `runtime/kata`

### Requirement: WEB-PLC-08 -- Local-kind Placement Resolution

When local-kind intent is submitted, the backend SHALL resolve it to the registered `local-kind` `ManagedCluster` in the Kind development environment.

The backend SHALL reject local-kind placement with a typed client-visible error when the platform is not running in the Kind development environment, `local-kind` is not registered, or `local-kind` is not connected.

#### Scenario: WEB-PLC-08.01 -- Local-kind resolves to the local cluster

- GIVEN the client submits a valid local-kind intent
- AND the registered `local-kind` cluster is connected
- WHEN the backend creates the gateway
- THEN the gateway SHALL be assigned to `local-kind`

### Requirement: WEB-PLC-09 -- API and Persistence Compatibility

The API contract, generated clients or adapters, authorization boundaries, and gateway provisioning tests SHALL be updated consistently for selector-based placement and batched availability. Existing gateway records and read APIs SHALL continue to expose their resolved `cluster_id`. `cluster_id` SHALL be OPTIONAL (nullable) on Gateway create: a create SHALL supply EITHER an explicit `cluster_id` (pinned) OR a `placement` selector (late-bound), and SHALL NOT accept the former provider/network request shape. The web-console provision form submits only the selector shape; the pinned shape remains available to `hsctl apply` and the declarative `.hypershell` import.

The Gateway schema SHALL carry a persisted `placement` field. The placement selector is NO LONGER merely request-scoped: it SHALL be persisted on the Gateway record so the backend can re-resolve it for late binding after create (WEB-PLC-07). A late-bound Gateway's `cluster_id` is nullable until placement resolves.

#### Scenario: WEB-PLC-09.01 -- Existing gateway placement remains readable

- GIVEN a gateway was created through the placement-intent flow
- WHEN a gateway detail or list API is read
- THEN the response SHALL expose the resolved cluster using the existing gateway placement representation

### Requirement: WEB-PLC-11 -- Backend-Selected Provisioning Version

The provision form SHALL NOT present an OpenShell version choice. The provision form's managed placement requests SHALL NOT include a `gateway-version` selector label. The backend SHALL select the gateway version from the capability advertised by the cluster on which the gateway is placed.

The provision preview card SHALL render only after placement availability has
loaded and the currently selected network/provider pair is available. It SHALL
NOT render a default managed recommendation while availability is loading, when
no managed placement is available, or for local-kind-only development. The card
SHALL NOT infer, retain, or display an OpenShell version because the version is
not known until the backend resolves placement.

Users SHALL continue to see the exact running version after provisioning
through the existing gateway list and detail views. This requirement does not
change the gateway read model, version-visibility behavior, or the
newer-version indicator.

#### Scenario: WEB-PLC-11.01 -- Managed form omits version selection

- GIVEN the user selects a network and provider with at least one eligible cluster
- AND expands Advanced options
- WHEN the user submits the form
- THEN the selector SHALL NOT carry a `gateway-version` label
- AND Advanced options SHALL contain only the Kata choice

#### Scenario: WEB-PLC-11.02 -- Recommendation omits an unknown version

- GIVEN the user selects a network and provider with at least one eligible cluster
- WHEN the provision preview card renders
- THEN the card SHALL NOT include an OpenShell version row

#### Scenario: WEB-PLC-11.03 -- Backend selects the placed cluster version

- GIVEN the backend resolves a managed placement to a connected cluster advertising `gateway-version/v0.1.2-rhaiv.7`
- WHEN the backend provisions that gateway
- THEN the gateway SHALL use the version selected by the backend for that cluster
- AND the gateway list and detail views SHALL continue to expose the observed running version

### Requirement: WEB-PLC-10 -- Accessibility and Localization

The internal-services, provider, Kata, availability, disabled-state,
validation, local-kind, recommendation, and error messages SHALL be localized.
Yes/no controls SHALL use labels that remain specific outside their visual
group context. The controls SHALL expose accessible names and selected or
disabled state through semantic PatternFly form controls. Keyboard users SHALL
be able to select and change network, provider, Kata, and local-kind
placement without pointer input.

#### Scenario: WEB-PLC-10.01 -- Disabled provider is announced

- GIVEN VPN is selected
- WHEN a keyboard or assistive-technology user inspects the provider controls
- THEN IBM Cloud SHALL be exposed as disabled
- AND the unavailable reason SHALL be conveyed in localized text or an accessible description

## Non-Goals

- Allowing users to choose both VPN and Public.
- Hard-coding support or rejection for any provider and network combination.
- Allowing users to select a concrete managed cluster from the provision form.
- User selection of the OpenShell version in the provision form.
- In-place gateway upgrades.
- Capacity-aware cluster selection or placement scoring.
- Changing existing gateway naming rules.
- Changing the resolved `cluster_id` representation on existing gateway records.
