# Fleet Dashboard Map Hub-Spine Layout

**Status:** Active
**Applies to:** `packages/fleet-dashboard-ui` map domain layout (`computeLayout`, `orderLanes`) and map presentation
**Tracks:** [HYPERSHELL-418](https://redhat.atlassian.net/browse/HYPERSHELL-418)

## Purpose

The standalone Fleet dashboard
(`@openshift-online/hypershell-fleet-dashboard-ui`, the separate operational
dashboard, not the web-console built-in view) renders a promotion-topology map:
one column per promotion stage, a `change` diamond on the left, a promotion gate
to the right of each stage, and a terminal gate past the final stage. Each stage
has exactly one HUB cluster that governs the stage; its SPOKE clusters attach to
the same stage.

The map reads correctly only when the HUB clusters form a single straight
horizontal line through the vertical center of the diagram - a direct shot from
the `change` diamond, through every HUB card and gate, to the final gate - with
SPOKE clusters stacked symmetrically above and below that center line. This
specification makes that geometry a behavior contract so the HUB/SPOKE role, not
the cloud provider a cluster happens to run on, governs vertical placement.

The layout is pure, deterministic geometry over the map model: it takes columns
(promotion order), the per-node HUB/SPOKE role, and the node set, and returns
absolute coordinates with no DOM, no measurement, and no baked-in provider or hub
list. Every placement rule below SHALL be derived from the role signal already
present on each node and lane.

## Requirements

### Requirement: HUB clusters sit on one centered horizontal spine

The layout SHALL place the governing HUB of every column on one shared horizontal
line - the hub spine - and that line SHALL sit at the vertical center of the map.
The vertical center SHALL be computed so that the SPOKE rows above the spine and
the SPOKE rows below it are balanced: the spine SHALL NOT be pushed to the top or
bottom of the diagram by the presence, count, or cloud provider of any cluster.

#### Scenario: Every column's hub shares one spine Y

- GIVEN a promotion map with several stages, each with one HUB cluster
- WHEN the layout is computed
- THEN every HUB node's vertical center SHALL equal the single hub-spine Y
- AND the hub-spine Y SHALL equal the vertical center of the laid-out map

#### Scenario: Spine stays centered regardless of provider

- GIVEN two stages whose HUB clusters run on different cloud providers
- WHEN the layout is computed
- THEN both HUB nodes SHALL still share the one centered hub-spine Y
- AND no provider grouping SHALL move the hub spine off the vertical center

### Requirement: The promotion spine rides the hub line

The `change` diamond, every per-stage promotion gate (the `hypershell-analysis`
gates), and the terminal gate SHALL be placed on the hub-spine Y, so the entire
promotion spine runs straight through the HUB cards from left to right. The map
SHALL expose this shared line (`hubSpineY`) as the vertical center, not as the top
of any single lane.

#### Scenario: Gates and change diamond align with the hubs

- GIVEN a promotion map with HUB clusters on the centered spine
- WHEN the layout is computed
- THEN the `change` diamond, each promotion gate, and the terminal gate SHALL be
  placed at the hub-spine Y
- AND the published `hubSpineY` SHALL equal that same Y

#### Scenario: No hub present

- GIVEN a map model in which no node carries the HUB role
- WHEN the layout is computed
- THEN the hub spine SHALL fall back to the vertical midpoint of the laid-out
  content
- AND the gates and `change` diamond SHALL ride that fallback line

### Requirement: HUB-first ordering within a column

Within each column the governing HUB SHALL be placed on the spine regardless of
the order its clusters appear in the model, server response, or source data. A
SPOKE that appears before its HUB in the input SHALL NOT take the spine position
or push its HUB off center.

#### Scenario: Spoke precedes hub in input order

- GIVEN a column whose SPOKE cluster appears before its HUB in the node input
- WHEN the layout is computed
- THEN the HUB SHALL occupy the spine position
- AND the SPOKE SHALL be placed off the spine (above or below it)

### Requirement: SPOKEs stack symmetrically above and below the spine

SPOKE clusters in a column SHALL stack symmetrically around the hub spine, filling
rows above and below it in a balanced way rather than all on one side. The
stacking order SHALL be deterministic: the same model SHALL always yield the same
assignment of SPOKEs to rows.

#### Scenario: Balanced spoke stacking

- GIVEN a column with one HUB and two SPOKE clusters
- WHEN the layout is computed
- THEN one SPOKE SHALL be placed above the spine and one below it
- AND the HUB SHALL remain on the spine between them

#### Scenario: Deterministic placement

- GIVEN the same map model computed twice
- WHEN the layout is computed each time
- THEN every node, gate, and the hub spine SHALL receive identical coordinates

### Requirement: Role governs vertical placement, not provider lanes

Vertical placement SHALL be governed by the HUB/SPOKE role of each cluster. The
layout SHALL NOT sort hub-hosting cloud providers to the bottom of the diagram and
SHALL NOT let per-provider grouping determine which row a cluster occupies. Lane
or provider information MAY still label or group clusters, but it SHALL NOT
override the centered hub spine or the symmetric spoke stacking.

#### Scenario: Hub-hosting provider is not sunk to the bottom

- GIVEN a provider whose clusters include the stage HUBs
- WHEN the layout is computed
- THEN that provider's HUB clusters SHALL remain on the centered spine
- AND they SHALL NOT be pushed below the SPOKE clusters of other providers

### Requirement: Derived, deterministic, baked-in-free geometry

The layout SHALL remain pure geometry: deterministic from the model alone, with no
DOM access and no measurement, and SHALL derive the spine and stacking entirely
from the HUB/SPOKE role already present on each node and lane. It SHALL NOT embed a
static ENV-to-HUB map, a provider allowlist, or a hub cluster name list.

#### Scenario: No baked-in hub or provider list

- GIVEN a map model whose HUB clusters and providers are not known ahead of time
- WHEN the layout is computed
- THEN the hub spine and spoke stacking SHALL be derived from the per-node role
  signal
- AND no hard-coded cluster name or provider list SHALL be consulted
