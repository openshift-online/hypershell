# Gateway Version Visibility Console Specification

**Date:** 2026-10-12
**Status:** Draft
**Applies to:** `packages/gateway-management-ui` (gateway list and detail pages, application ports, probes), `components/web-console` (host routes, API adapter, composition root, BFF)
**Parent:** `web-console/architecture.spec.md` -- console stack and trust boundaries
**Related:** `platform/openshell-gateway-health.spec.md` -- how the control plane observes a gateway's running `gateway_version`; `web-console/user_flows.md` -- the gateway Details tab; `standards/ui/hexagonal-architecture.spec.md`, `standards/ui/patternfly.spec.md`, `standards/ui/domain-observability.spec.md`, `standards/ui/content-localization.spec.md`, `standards/ui/accessibility.spec.md`

---

## Purpose

Make the OpenShell gateway version visible in the management web console. A
gateway's **running version** is the runtime version the gateway pod reports on
its in-cluster `/health` endpoint, observed by the control plane and stored in
`gateways.gateway_version` (`openshell-gateway-health.spec.md`). It reaches the
console as `GatewayRecord.gatewayVersion`.

HyperShell does not provide in-place gateway upgrades, so this surface is
informational. No upgrade affordance or "upgrade available" wording appears
anywhere.

---

## Requirements

### Requirement: WEB-VER-01 -- Running Version List Column

The gateway list page SHALL present the running version as its own sortable
column. The column SHALL render `GatewayRecord.gatewayVersion` for each gateway,
and SHALL render a localized placeholder (for example a dash) for a gateway that
has no observed version yet. The list SHALL NOT render a version chip on list
rows; the running version appears on the list only as this column.

Sorting by the version column SHALL be a server-side sort, consistent with the
other list columns, and SHALL add a `version` sort field to the gateway list
query contract.

#### Scenario: WEB-VER-01.01 -- Version column renders and sorts

- GIVEN the gateway list returns gateways with and without an observed version
- WHEN the list renders
- THEN each row SHALL show its running version in the version column
- AND a gateway with no observed version SHALL show the localized placeholder
- AND activating the column header SHALL request the list sorted by `version`

#### Scenario: WEB-VER-01.02 -- No chip on list rows

- GIVEN the gateway list renders
- WHEN a row shows its running version
- THEN the running version SHALL appear only in the version column
- AND no version chip SHALL appear on the row

### Requirement: WEB-VER-02 -- Running Version on the Details Page

The gateway details page SHALL present the running version in exactly two places
and nowhere else:

- As a chip in the page header, next to the health/status indicator that sits
  beside the gateway name.
- As a field in the Details tab.

Both placements SHALL render `GatewayRecord.gatewayVersion`. When no running
version has been observed, the header chip SHALL be omitted and the Details-tab
field SHALL show a localized placeholder.

#### Scenario: WEB-VER-02.01 -- Header chip and Details-tab field render

- GIVEN a gateway with an observed running version
- WHEN the details page renders
- THEN the page header SHALL show a version chip next to the health/status indicator
- AND the Details tab SHALL show the running version as a field

#### Scenario: WEB-VER-02.02 -- No observed version hides the header chip

- GIVEN a gateway with no observed running version
- WHEN the details page renders
- THEN the header SHALL NOT show a version chip
- AND the Details tab version field SHALL show the localized placeholder

### Requirement: WEB-VER-04 -- Version Facts Behind the Application Boundary

The running version SHALL be exposed to the UI through application-owned ports,
not read from raw SDK or infrastructure values inside React components. The API
adapter SHALL map the gateway read model's `gateway_version` onto
`GatewayRecord.gatewayVersion`, and the list and detail surfaces SHALL render
that one value. Workflow and dependency facts for version observation SHALL be
published through the existing domain-probe fan-out port rather than through
raw console or telemetry calls.

#### Scenario: WEB-VER-04.01 -- The adapter maps the running version

- GIVEN the gateway read model returns `gateway_version: "v0.1.2-rhaiv.7"`
- WHEN the API adapter builds the gateway record
- THEN `GatewayRecord.gatewayVersion` SHALL be `v0.1.2-rhaiv.7`
- AND React components SHALL render that value rather than reading the API response

### Requirement: WEB-VER-05 -- Accessibility and Localization

The version column header, header version chip, Details-tab version field, and
the no-version placeholder SHALL be localized. The header chip SHALL expose an
accessible name. New message IDs SHALL be added to the gateway-management-ui
message catalog and `locales/en.json` regenerated.

#### Scenario: WEB-VER-05.01 -- The version chip has an accessible name

- GIVEN a gateway with an observed running version
- WHEN a keyboard or assistive-technology user inspects the header
- THEN the version chip SHALL expose a localized accessible name that includes the version

## Non-Goals

- In-place gateway upgrades or any upgrade affordance, action, or "upgrade available" wording.
- A version chip on gateway list rows (the list shows the version only as a sortable column).
- Any Details-page placement of the running version beyond the header chip and the Details-tab field.
- Operational-dashboard version distribution widgets.

## Retired IDs

- `WEB-VER-03`
