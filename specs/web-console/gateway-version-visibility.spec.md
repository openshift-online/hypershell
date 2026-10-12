# Gateway Version Visibility Console Specification

**Date:** 2026-10-12
**Status:** Draft
**Applies to:** `packages/gateway-management-ui` (gateway list and detail pages, application ports, probes), `components/web-console` (host routes, API adapter, composition root, BFF)
**Parent:** `web-console/architecture.spec.md` -- console stack and trust boundaries
**Related:** `platform/managed-cluster-registration.spec.md` -- the `gateway-version` capability a spoke advertises; `platform/openshell-gateway-health.spec.md` -- how the control plane observes a gateway's running `gateway_version`; `web-console/gateway-provision-placement.spec.md` -- version-based placement in the create form; `web-console/user_flows.md` -- the gateway Details tab; `standards/ui/hexagonal-architecture.spec.md`, `standards/ui/patternfly.spec.md`, `standards/ui/domain-observability.spec.md`, `standards/ui/content-localization.spec.md`, `standards/ui/accessibility.spec.md`

---

## Purpose

Make OpenShell gateway versions visible in the management web console. Two
distinct version concepts exist and this surface presents both:

- A gateway's **running version** is the runtime version the gateway pod reports
  on its in-cluster `/health` endpoint, observed by the control plane and stored
  in `gateways.gateway_version` (`openshell-gateway-health.spec.md`). It already
  reaches the console as `GatewayRecord.gatewayVersion` but is not displayed.
- A spoke's **provisioning version** is the gateway version a control plane hands
  to new gateways. It is the single-valued `gateway-version` capability the spoke
  advertises on registration (`managed-cluster-registration.spec.md`).

These two values can legitimately differ: a gateway may run an older version
while its spoke now provisions a newer one, or a gateway may be pinned to an
explicit image and run a version unrelated to its spoke's default. HyperShell
does not provide in-place gateway upgrades, so this surface is informational: a
gateway that is behind is replaced by creating a new one, not upgraded. No
upgrade affordance or "upgrade available" wording appears anywhere.

This spec covers only the display of the running version and the neutral
drift indicator. Choosing a version at provision time is specified in
`gateway-provision-placement.spec.md` (GP-11).

---

## Requirements

### Requirement: GVV-01 -- Running Version List Column

The gateway list page SHALL present the running version as its own sortable
column. The column SHALL render `GatewayRecord.gatewayVersion` for each gateway,
and SHALL render a localized placeholder (for example a dash) for a gateway that
has no observed version yet. The list SHALL NOT render a version chip on list
rows; the running version appears on the list only as this column.

Sorting by the version column SHALL be a server-side sort, consistent with the
other list columns, and SHALL add a `version` sort field to the gateway list
query contract.

#### Scenario: Version column renders and sorts

- GIVEN the gateway list returns gateways with and without an observed version
- WHEN the list renders
- THEN each row SHALL show its running version in the version column
- AND a gateway with no observed version SHALL show the localized placeholder
- AND activating the column header SHALL request the list sorted by `version`

#### Scenario: No chip on list rows

- GIVEN the gateway list renders
- WHEN a row shows its running version
- THEN the running version SHALL appear only in the version column
- AND no version chip SHALL appear on the row

### Requirement: GVV-02 -- Running Version on the Details Page

The gateway details page SHALL present the running version in exactly two places
and nowhere else:

- As a chip in the page header, next to the health/status indicator that sits
  beside the gateway name.
- As a field in the Details tab.

Both placements SHALL render `GatewayRecord.gatewayVersion`. When no running
version has been observed, the header chip SHALL be omitted and the Details-tab
field SHALL show a localized placeholder.

#### Scenario: Header chip and Details-tab field render

- GIVEN a gateway with an observed running version
- WHEN the details page renders
- THEN the page header SHALL show a version chip next to the health/status indicator
- AND the Details tab SHALL show the running version as a field

#### Scenario: No observed version hides the header chip

- GIVEN a gateway with no observed running version
- WHEN the details page renders
- THEN the header SHALL NOT show a version chip
- AND the Details tab version field SHALL show the localized placeholder

### Requirement: GVV-03 -- Newer-Version-Available Indicator

When a gateway's running version is older than the provisioning version its spoke
advertises, the details page SHALL show a neutral "Newer Version Available"
indicator alongside the header version chip. The indicator SHALL be neutral in
tone (not a warning or error) and SHALL carry a help tooltip clarifying that
existing gateways are not upgraded in place and that the newer version applies
only to newly created gateways. The indicator and its tooltip SHALL NOT present
an upgrade action, link, or "upgrade available" wording.

The indicator SHALL fire only for a gateway that tracks its spoke's default
image. It SHALL NOT fire for a gateway pinned to an explicit `image`, because
such a gateway may intentionally run a version that differs from -- and may even
be ahead of -- its spoke's advertised provisioning version. "Provisioning
version" is the cluster default for new gateways, not a guarantee for every
existing gateway.

The comparison SHALL be a real "is older than" version comparison over gateway
version tags that carry dots and a release suffix (for example `v0.1.2-rhaiv.7`),
not a plain string inequality. The indicator SHALL fire only when the running
version is unambiguously older than the advertised provisioning version under
that comparison. When the two versions are equal, when the running version is the
same or newer, when either value is missing, or when either value cannot be
parsed by the comparison, the indicator SHALL NOT fire.

#### Scenario: Behind gateway shows the neutral indicator

- GIVEN a gateway running `v0.1.1-rhaiv.3` that tracks its spoke default
- AND its spoke advertises `gateway-version/v0.1.2-rhaiv.7`
- WHEN the details page renders
- THEN a neutral "Newer Version Available" indicator SHALL appear by the version chip
- AND its tooltip SHALL explain that existing gateways are not upgraded in place
- AND no upgrade action or "upgrade available" wording SHALL appear

#### Scenario: Pinned-image gateway never shows the indicator

- GIVEN a gateway pinned to an explicit image running `v0.1.1-rhaiv.3`
- AND its spoke advertises `gateway-version/v0.1.2-rhaiv.7`
- WHEN the details page renders
- THEN the "Newer Version Available" indicator SHALL NOT appear

#### Scenario: Up-to-date gateway shows no indicator

- GIVEN a gateway running `v0.1.2-rhaiv.7` that tracks its spoke default
- AND its spoke advertises `gateway-version/v0.1.2-rhaiv.7`
- WHEN the details page renders
- THEN the "Newer Version Available" indicator SHALL NOT appear

#### Scenario: Unparseable or missing versions suppress the indicator

- GIVEN a gateway whose running version or spoke provisioning version is missing or unparseable
- WHEN the details page renders
- THEN the "Newer Version Available" indicator SHALL NOT appear

### Requirement: GVV-04 -- Version Facts Behind the Application Boundary

The running version, the spoke's advertised provisioning version for the
gateway's cluster, whether the gateway pins an explicit image, and the derived
drift verdict SHALL be exposed to the UI through application-owned ports, not
computed from raw SDK or infrastructure values inside React components. The
drift verdict SHALL be derived once in the application layer from the running
version, the provisioning version, and the pinned-image flag, so the list and
detail surfaces render a single authoritative result. Workflow and dependency
facts for version observation and drift SHALL be published through the existing
domain-probe fan-out port rather than through raw console or telemetry calls.

The gateway read model SHALL provide the provisioning version advertised by the
gateway's cluster and the pinned-image signal needed for GVV-03, derived from the
gateway's `cluster_id` and that cluster's `gateway-version` capability. This
changes the gateway API contract and SHALL be reflected consistently in the
OpenAPI document, the generated Go and TypeScript SDKs, the proto stubs, and the
`hsctl` TUI descriptor.

#### Scenario: Drift is derived in the application layer

- GIVEN the gateway read model exposes running version, provisioning version, and the pinned-image flag
- WHEN the list and detail surfaces render version information for the same gateway
- THEN the drift verdict SHALL be computed once in the application layer
- AND React components SHALL consume the derived verdict rather than recomputing it

### Requirement: GVV-05 -- Accessibility and Localization

The version column header, header version chip, Details-tab version field, the
"Newer Version Available" indicator, its tooltip, and the no-version placeholder
SHALL be localized. The chip and indicator SHALL expose accessible names, and the
indicator's clarifying text SHALL be reachable by keyboard and assistive
technology. New message IDs SHALL be added to the gateway-management-ui message
catalog and `locales/en.json` regenerated.

#### Scenario: Indicator is reachable without a pointer

- GIVEN a gateway showing the "Newer Version Available" indicator
- WHEN a keyboard or assistive-technology user inspects the header
- THEN the indicator SHALL expose an accessible name
- AND its clarifying text SHALL be reachable without pointer input

## Non-Goals

- In-place gateway upgrades or any upgrade affordance, action, or "upgrade available" wording.
- A version chip on gateway list rows (the list shows the version only as a sortable column).
- Any Details-page placement of the running version beyond the header chip and the Details-tab field.
- Surfacing drift for a gateway pinned to an explicit image.
- Choosing a version at provision time (specified in `gateway-provision-placement.spec.md`, GP-11).
- Operational-dashboard version distribution widgets.
