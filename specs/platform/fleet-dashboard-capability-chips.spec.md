# Fleet Dashboard Capability-Label Chips and Capabilities Drawer

**ID prefix:** FLT-CAP
**Status:** Active
**Applies to:** `packages/fleet-dashboard-ui` promotion domain, map model, wire
adapter, and map presentation (card + node drawer); `components/fleet-dashboard`
BFF promotion source (delivery capabilities-annotation sourcing).
**Tracks:** [HYPERSHELL-437](https://redhat.atlassian.net/browse/HYPERSHELL-437)

## Purpose

The standalone Fleet dashboard
(`@openshift-online/hypershell-fleet-dashboard-ui`, the separate operational
dashboard, not the web-console built-in view) draws every cluster on its
promotion-topology map as an SVG card and, when a node is selected, in a details
drawer. Today a card shows only the cloud **provider** and the **hub/spoke role**
as two pills, and the drawer never lists what else the cluster is. Clusters
self-declare a richer set of **capability labels** at registration
(`namespace/value` strings, see `PLAT-CLUS-06`), but none of them reach either
surface.

This specification makes the following behavior contracts:

1. The map card SHALL carry the cluster's **runtime** and **gateway-version**
   capabilities as a second chip row below the existing provider/role row.
2. All card chips SHALL share one consistent, measured size system so chip text
   never spills past its pill, and the card's text labels SHALL truncate at the
   card edge rather than overflow it.
3. The node drawer SHALL gain a **Capabilities** section listing the cluster's
   full self-declared capability set.
4. The `capabilities` set SHALL be carried end to end as an optional field on the
   promotion environment, sourced by the BFF from the delivery
   `delivery.hypershell.app/capabilities` annotation already present on the
   resolved Argo `Application`, so the BFF stays low-privilege.

Capabilities are **opaque data**: the UI SHALL NOT bake in a closed vocabulary.
An unknown namespace or value SHALL still render. The capability chips and the
drawer section SHALL derive colour, order, and casing from the capability
strings themselves, never from a hard-coded cluster, provider, or capability
list, consistent with the identity firewall of `FLT-MAP-06`.

## Relationship to existing specs

- `PLAT-CLUS-06` is the authoritative model for the `namespace/value` capability
  string, its recognized namespaces (`provider`, `network`, `region`, `runtime`,
  `environment`, `gateway-version`), and which namespaces are single-valued. This
  spec consumes that model for display; it does not redefine it.
- `FLT-MAP-06` requires the pure map **layout** (`computeLayout`) to stay free of
  DOM access and measurement. Chip text measurement in this spec happens in the
  **presentation** layer (the SVG card), not in the layout geometry; the layout
  contributes only the card width and height constants. This spec does not relax
  `FLT-MAP-06`.
- `FLT-SYNC-01`/`FLT-SYNC-03` establish the low-privilege BFF sourcing pattern
  (read from the Argo CD `Application` objects already resolved per instance; no
  new Kubernetes query or cluster credential) and the end-to-end threading
  pattern (`wire -> promotion -> MapNode`). The `capabilities` field follows both.

## Requirements

### Requirement: FLT-CAP-01 -- Capabilities carried end to end from the delivery annotation

The promotion environment SHALL carry an optional `capabilities` set of
`namespace/value` strings, threaded unchanged through the wire adapter, the
promotion domain, and the map node, mirroring how `provider` is carried today.
The set SHALL be sourced from the single, comma-joined
`delivery.hypershell.app/capabilities` annotation on the Argo CD `Application` by
the fleet-dashboard BFF promotion source, using data the BFF already resolves per
instance; the BFF SHALL NOT authenticate to the api-server and SHALL NOT
introduce any new Kubernetes query, list, watch, or cluster credential to obtain
it. Entries SHALL be validated against the recognized capability namespaces
(`PLAT-CLUS-06`) and emitted in a stable order; unknown or malformed entries
SHALL be dropped rather than rendered. The field SHALL be optional at every hop:
a node with no capability annotation SHALL carry an empty set, not an error, so
existing test factories and payloads that omit it stay valid.

#### Scenario: FLT-CAP-01.01 -- Capabilities thread from wire to map node

- GIVEN a promotion environment whose wire payload carries
  `capabilities: ["provider/aws", "runtime/kata", "gateway-version/v0.1.2-rhaiv.7"]`
- WHEN the wire adapter maps it into the promotion domain and the map node is built
- THEN the map node SHALL carry the same three capability strings in order
- AND no capability value SHALL be altered, dropped, or invented in transit

#### Scenario: FLT-CAP-01.02 -- Absent capabilities are an empty set, not an error

- GIVEN a promotion environment whose wire payload omits `capabilities`
- WHEN the wire adapter maps it and the map node is built
- THEN the map node's capabilities SHALL be an empty set
- AND the mapping SHALL NOT fail or require the field to be present

#### Scenario: FLT-CAP-01.03 -- Sourcing stays low-privilege

- GIVEN the fleet-dashboard BFF promotion source resolving each instance's Argo CD `Application`
- WHEN it emits the `capabilities` field for a promotion environment
- THEN the value SHALL be read from the `delivery.hypershell.app/capabilities` annotation on the already-resolved object
- AND the BFF SHALL NOT open a new Kubernetes query, watch, or cluster credential, and SHALL NOT call the api-server, to obtain capabilities

### Requirement: FLT-CAP-02 -- Capability string helpers

The dashboard SHALL provide pure helpers over the capability strings that bake in
no vocabulary: one that returns the value of the first capability in a given
namespace (or nothing when absent), one that splits a capability into its
namespace and value on the first slash only (later slashes stay in the value),
and one that returns the capabilities in a stable lead-namespace order
(`provider`, `runtime`, `gateway-version`, `network`, `region`, then any other
namespace alphabetically). An unknown namespace SHALL be ordered after the lead
namespaces and SHALL never be discarded.

#### Scenario: FLT-CAP-02.01 -- Value lookup by namespace

- GIVEN the capability set `["runtime/kata", "region/us-east-1"]`
- WHEN the value of the `runtime` namespace is requested
- THEN the helper SHALL return `kata`
- AND requesting the value of a namespace not present SHALL return nothing

#### Scenario: FLT-CAP-02.02 -- Split on the first slash only

- GIVEN the capability string `gateway-version/v0.1.2-rhaiv.7`
- WHEN it is split into namespace and value
- THEN the namespace SHALL be `gateway-version`
- AND the value SHALL be `v0.1.2-rhaiv.7`, with its internal characters intact

#### Scenario: FLT-CAP-02.03 -- Lead-namespace ordering keeps unknowns

- GIVEN the capability set `["zone/a", "region/eu-de", "runtime/runc", "provider/ibm"]`
- WHEN the set is ordered
- THEN the order SHALL be `provider/ibm`, `runtime/runc`, `region/eu-de`, `zone/a`
- AND the unknown `zone` namespace SHALL appear after the lead namespaces, not be dropped

### Requirement: FLT-CAP-03 -- Map card second chip row for runtime and gateway-version

The map card SHALL render a second chip row directly below the provider/role row,
containing the cluster's `runtime` capability chip and its `gateway-version`
capability chip. The two rows SHALL be two intentional lines that do not wrap. A
capability that the cluster does not declare SHALL drop its chip, and the row
SHALL collapse around the chips that remain; when the cluster declares neither
runtime nor gateway-version the second row SHALL be absent.

#### Scenario: FLT-CAP-03.01 -- Both capabilities present render two chips

- GIVEN a card whose cluster declares `runtime/runc` and `gateway-version/v0.1.2-rhaiv.7`
- WHEN the card is rendered
- THEN a second chip row below the provider/role row SHALL show a runtime chip and a gateway-version chip
- AND the two chip rows SHALL remain on their own single lines without wrapping

#### Scenario: FLT-CAP-03.02 -- Absent capability drops its chip

- GIVEN a card whose cluster declares `gateway-version/v0.1.2-rhaiv.7` but no `runtime`
- WHEN the card is rendered
- THEN the second row SHALL show only the gateway-version chip
- AND no empty placeholder SHALL be drawn for the missing runtime chip

#### Scenario: FLT-CAP-03.03 -- Neither capability omits the row

- GIVEN a card whose cluster declares neither runtime nor gateway-version
- WHEN the card is rendered
- THEN no second chip row SHALL be drawn

### Requirement: FLT-CAP-04 -- One measured chip system, no overflow

**Verification:** Inspection -- SVG glyph width has no synchronous measurement
API under jsdom, so pixel-level fit is confirmed by visual review of the card;
the truncation and width helpers themselves are unit-tested through their
non-DOM fallback.

Every chip on the card (provider, role, runtime, gateway-version) SHALL share one
size system: the same height, font size, font weight, horizontal padding, corner
radius, and inter-chip gap. Each chip's width SHALL be derived by measuring its
rendered text, not estimated per character, so no chip text spills past its pill.
The card's cluster label, environment name, and alias SHALL be truncated with a
trailing ellipsis so none of them overflows the card edge. The measurement SHALL
use a cached offscreen text-measurement path and SHALL fall back to a slight
over-estimate where no measurement API is available (server-side rendering or
tests), so a chip never clips.

#### Scenario: FLT-CAP-04.01 -- A long gateway-version fits inside its pill

- GIVEN a card whose cluster declares `gateway-version/v0.1.2-rhaiv.7`
- WHEN the card is rendered
- THEN the gateway-version chip's pill SHALL be wide enough to contain its text
- AND the text SHALL NOT extend past the pill's edges

#### Scenario: FLT-CAP-04.02 -- A long environment name truncates at the card edge

- GIVEN a card whose environment name is longer than the available text width
- WHEN the card is rendered
- THEN the name SHALL be shortened with a trailing ellipsis so it fits the card
- AND the name SHALL NOT overflow the card edge

#### Scenario: FLT-CAP-04.03 -- Chips share one size family

- GIVEN a card rendering a provider, role, runtime, and gateway-version chip
- WHEN the chips are drawn
- THEN all four SHALL use the same height, font, padding, and corner radius
- AND only each chip's width and colour SHALL differ

### Requirement: FLT-CAP-05 -- Chip colour and casing semantics

Chip appearance SHALL read as a hierarchy derived from the capability values, with
no baked-in vocabulary. The provider chip SHALL use a brand hue for known public
clouds and a deterministic hash hue otherwise, with a dark fill and bright
same-hue text. The role chip SHALL be the one inverted pill: a light same-hue
fill with dark text, so hub/spoke reads as structural rather than as another
coloured capability. The runtime and gateway-version chips SHALL use a value-
derived hue one notch dimmer than the provider chip, so the second row reads as
secondary. The runtime chip label SHALL be upper-cased; the gateway-version chip
label SHALL keep its natural case, because upper-casing a version string reads
wrong. An unrecognized runtime or version value SHALL still render with a hash
hue; the dashboard SHALL NOT require the value to be in a known set.

#### Scenario: FLT-CAP-05.01 -- Second-row chips are dimmer than the provider chip

- GIVEN a card with a provider chip and a runtime chip
- WHEN the chips are drawn
- THEN the runtime chip SHALL use a dimmer value of the hue than the provider chip
- AND the provider/role row SHALL read as primary over the runtime/version row

#### Scenario: FLT-CAP-05.02 -- Version keeps natural case, runtime is upper-cased

- GIVEN a runtime value `runc` and a gateway-version value `v0.1.2-rhaiv.7`
- WHEN the chips are rendered
- THEN the runtime chip SHALL read `RUNC`
- AND the gateway-version chip SHALL read `v0.1.2-rhaiv.7` unchanged

#### Scenario: FLT-CAP-05.03 -- Unknown value still renders

- GIVEN a runtime value the dashboard has no brand hue for
- WHEN the chip is rendered
- THEN the chip SHALL be drawn with a deterministic hash hue
- AND the dashboard SHALL NOT drop the chip or require the value to be known

### Requirement: FLT-CAP-06 -- Node drawer Capabilities section

The node drawer's Details tab SHALL include a Capabilities section that lists the
cluster's full self-declared capability set. The section SHALL be rendered as a
PatternFly label group of compact, outline labels, with the extras collapsed
behind the built-in "N more" toggle. Each label SHALL pair a muted namespace, a
decorative `/` separator, and an emphasised value, because the value is the part
that varies. The labels SHALL be shown in the lead-namespace order of
`FLT-CAP-02`. The section SHALL be omitted entirely when the cluster declares no
capabilities. The section title SHALL be a localized message.

#### Scenario: FLT-CAP-06.01 -- Capabilities listed in lead-namespace order

- GIVEN a selected node whose capabilities are `["region/eu-de", "provider/ibm", "runtime/runc"]`
- WHEN the drawer's Details tab is shown
- THEN a Capabilities section SHALL list the three capabilities as compact outline labels
- AND the labels SHALL be ordered `provider/ibm`, `runtime/runc`, `region/eu-de`

#### Scenario: FLT-CAP-06.02 -- Extra capabilities collapse behind a toggle

- GIVEN a selected node with more capabilities than the section's visible limit
- WHEN the drawer's Details tab is shown
- THEN the section SHALL show the first labels and collapse the remainder behind an "N more" toggle

#### Scenario: FLT-CAP-06.03 -- No capabilities omits the section

- GIVEN a selected node whose cluster declares no capabilities
- WHEN the drawer's Details tab is shown
- THEN the Capabilities section SHALL NOT be rendered
