# Fleet Dashboard Map Hub-Spine and Provider-Lane Layout

**Status:** Active
**Applies to:** `packages/fleet-dashboard-ui` map domain model and layout (`buildMapModel`, `computeLayout`, `orderLanes`) and map presentation; `components/fleet-dashboard` Argo CD Application matching for the promotion plane (`argoByInstance`)
**Tracks:** [HYPERSHELL-418](https://redhat.atlassian.net/browse/HYPERSHELL-418), [HYPERSHELL-423](https://redhat.atlassian.net/browse/HYPERSHELL-423)

## Purpose

The standalone Fleet dashboard
(`@openshift-online/hypershell-fleet-dashboard-ui`, the separate operational
dashboard, not the web-console built-in view) renders a promotion-topology map:
one column per promotion stage, a `change` diamond on the left, a promotion gate
to the right of each stage, and a terminal gate past the final stage. Each stage
has exactly one HUB cluster that governs the stage; its SPOKE clusters attach to
the same stage.

The map reads correctly only when two groupings hold at once:

1. The HUB clusters form a single straight horizontal line - the hub spine -
   through the vertical center of the diagram: a direct shot from the `change`
   diamond, through every HUB card and gate, to the final gate.
2. Clusters are grouped into visible provider lanes (AWS, IBM, ...), one lane per
   cloud provider, so a reader can see at a glance which provider each cluster
   runs on.

These two groupings are compatible only under the single-cloud-hub assumption
stated below. This specification makes both the centered hub spine and the
provider-lane grouping behavior contracts, and records the assumption that keeps
them coherent.

The layout is pure, deterministic geometry over the map model: it takes columns
(promotion order), the per-node HUB/SPOKE role, the per-node provider lane, and
the node set, and returns absolute coordinates with no DOM, no measurement, and
no baked-in provider or hub list. Every placement rule below SHALL be derived from
the role and provider signals already present on each node and lane, never from a
hard-coded cluster name, provider name, or hub table.

## Single-Cloud-Hub Assumption

This specification assumes that every stage HUB runs on a single cloud provider -
today IBM. Under this assumption a centered hub spine and provider-lane grouping
do not conflict: because all HUBs share one provider, they all fall in one
provider lane, and that lane can be centered so the spine stays centered while
every other provider's clusters group into their own lanes above and below it.

The hub-hosting provider SHALL be derived at runtime from the role signal (the
lane whose clusters carry the HUB role), not hard-coded to IBM or any provider
name. The assumption constrains the data the dashboard is expected to receive; it
is not a license to embed a provider name in the layout.

**Known tension (multi-cloud hubs).** If HUBs ever span more than one cloud
provider, a centered hub spine and per-provider lane grouping become mutually
exclusive: the HUBs would belong to different provider lanes at different vertical
positions, so they could not all sit on one centered spine while each HUB stayed
in its own provider lane. This specification does not resolve that tension; it is
out of scope until multi-cloud hubs are a real requirement. When that day comes,
one grouping SHALL be chosen as primary and this spec amended accordingly.

## Requirements

### Requirement: FLT-MAP-01 -- HUB clusters sit on one centered horizontal spine

The layout SHALL place the governing HUB of every column on one shared horizontal
line - the hub spine - and that line SHALL sit at the vertical center of the map.
Under the single-cloud-hub assumption every HUB shares one provider lane, and the
layout SHALL center that hub-hosting lane so the spine stays at the vertical
center. The vertical center SHALL be computed so that the lanes above the spine
and the lanes below it are balanced: the spine SHALL NOT be pushed to the top or
bottom of the diagram by the presence, count, or cloud provider of any cluster.

#### Scenario: FLT-MAP-01.01 -- Every column's hub shares one spine Y

- GIVEN a promotion map with several stages, each with one HUB cluster on the
  single hub-hosting provider
- WHEN the layout is computed
- THEN every HUB node's vertical center SHALL equal the single hub-spine Y
- AND the hub-spine Y SHALL equal the vertical center of the laid-out map

#### Scenario: FLT-MAP-01.02 -- Spine stays centered while spokes group by provider

- GIVEN a map whose HUBs run on one provider and whose SPOKE clusters run on
  other providers
- WHEN the layout is computed
- THEN every HUB node SHALL share the one centered hub-spine Y
- AND grouping the SPOKE clusters into their provider lanes SHALL NOT move the hub
  spine off the vertical center

### Requirement: FLT-MAP-07 -- Clusters group into visible provider lanes

The map SHALL render one provider lane per cloud provider (the provider string, or
an "unassigned" lane when the server omits it) as a visible grouping, so each
cluster is shown within its provider's lane. The lane set SHALL be derived from
the per-node provider signal, with no baked-in provider list, and SHALL be
deterministic for a given model.

#### Scenario: FLT-MAP-07.01 -- Two providers render as two lanes

- GIVEN a map with clusters on the AWS provider and clusters on the IBM provider
- WHEN the map is rendered
- THEN the map SHALL show an AWS provider lane and an IBM provider lane as visible
  groupings
- AND each cluster SHALL appear within the lane of the provider it runs on

#### Scenario: FLT-MAP-07.02 -- Unassigned provider

- GIVEN a cluster whose server record carries no provider
- WHEN the map is rendered
- THEN that cluster SHALL be grouped into a single "unassigned" lane
- AND no provider name SHALL be invented for it

### Requirement: FLT-MAP-02 -- The promotion spine rides the hub line

The `change` diamond, every per-stage promotion gate (the `hypershell-analysis`
gates), and the terminal gate SHALL be placed on the hub-spine Y, so the entire
promotion spine runs straight through the HUB cards from left to right. The map
SHALL expose this shared line (`hubSpineY`) as the vertical center, not as the top
of any single lane.

#### Scenario: FLT-MAP-02.01 -- Gates and change diamond align with the hubs

- GIVEN a promotion map with HUB clusters on the centered spine
- WHEN the layout is computed
- THEN the `change` diamond, each promotion gate, and the terminal gate SHALL be
  placed at the hub-spine Y
- AND the published `hubSpineY` SHALL equal that same Y

#### Scenario: FLT-MAP-02.02 -- No hub present

- GIVEN a map model in which no node carries the HUB role
- WHEN the layout is computed
- THEN the hub spine SHALL fall back to the vertical midpoint of the laid-out
  content
- AND the gates and `change` diamond SHALL ride that fallback line

### Requirement: FLT-MAP-03 -- HUB-first ordering within a column

Within each column the governing HUB SHALL be placed on the spine regardless of
the order its clusters appear in the model, server response, or source data. A
SPOKE that appears before its HUB in the input SHALL NOT take the spine position
or push its HUB off center.

#### Scenario: FLT-MAP-03.01 -- Spoke precedes hub in input order

- GIVEN a column whose SPOKE cluster appears before its HUB in the node input
- WHEN the layout is computed
- THEN the HUB SHALL occupy the spine position
- AND the SPOKE SHALL be placed off the spine (above or below it)

### Requirement: FLT-MAP-04 -- Provider lanes stack around the centered hub spine

The hub-hosting provider lane SHALL sit on the centered spine, and the remaining
provider lanes SHALL stack above and below it in a balanced way rather than all on
one side. The hub-hosting lane SHALL NOT be sorted to the top or bottom of the
diagram. Lane ordering and the stacking of clusters within a lane SHALL be
deterministic: the same model SHALL always yield the same lane order and the same
assignment of clusters to rows.

#### Scenario: FLT-MAP-04.01 -- Hub-hosting lane is centered, others stack around it

- GIVEN a map with a hub-hosting IBM lane and a spoke-only AWS lane
- WHEN the layout is computed
- THEN the IBM (hub-hosting) lane SHALL carry the centered hub spine
- AND the AWS lane SHALL be placed above or below the spine, not on it

#### Scenario: FLT-MAP-04.02 -- Hub-hosting provider is not sunk to the bottom

- GIVEN a provider whose clusters include the stage HUBs
- WHEN the layout is computed
- THEN that provider's lane SHALL remain centered on the hub spine
- AND it SHALL NOT be pushed below the other providers' lanes

#### Scenario: FLT-MAP-04.03 -- Deterministic placement

- GIVEN the same map model computed twice
- WHEN the layout is computed each time
- THEN every node, lane, gate, and the hub spine SHALL receive identical
  coordinates

### Requirement: FLT-MAP-05 -- Role and provider together govern vertical placement

Vertical placement SHALL be governed by the provider lane (which band a cluster
groups into) together with the HUB/SPOKE role (which cluster holds the spine
within the hub-hosting lane). Under the single-cloud-hub assumption these do not
conflict: the hub-hosting lane is centered by role, and every cluster is grouped
by provider. Provider grouping SHALL NOT override the centered hub spine, and
role SHALL NOT dissolve the visible provider lanes.

#### Scenario: FLT-MAP-05.01 -- Provider grouping and centered spine coexist

- GIVEN a map whose HUBs run on IBM and whose SPOKEs run on IBM and AWS
- WHEN the layout is computed
- THEN the IBM HUBs SHALL sit on the centered spine
- AND the AWS SPOKEs SHALL group into a visible AWS lane off the spine
- AND the IBM SPOKEs SHALL group into the IBM lane near the spine

### Requirement: FLT-MAP-08 -- Nodes on one physical cluster share a container

Every control plane, hub or spoke, SHALL have its own node on the map. A hub and
the spokes co-located with it run on the same physical cluster. The map SHALL show
each co-located spoke as its own node, and SHALL wrap every node of a column that
runs on the same physical cluster in one parent container that represents that
cluster.

A co-located spoke is a managed cluster the hub's topology document lists under
`spokes`. When the spoke has no promotion environment of its own, the map SHALL
still create its node from the topology document. That node SHALL take its
release and Argo state from its hub, because GitOps delivers it from the hub's
cluster. It SHALL carry no promotion gates of its own.

A remote spoke (listed in the hub's topology under `remote_spokes`) that has no
promotion environment SHALL also get its own node in its hub's column. No
promotion environment reports its release, so the node SHALL leave the release
unknown and SHALL NOT raise the "health unavailable" alarm. Its cluster, provider
and Argo state come from Argo CD when FLT-MAP-09 finds them; otherwise they stay
unknown and the node groups into the unassigned provider lane. No gate waits for it, so the map SHALL NOT draw a
change line or a promotion gate line to or from it.

A node's physical cluster is its `cluster` delivery label. A hub, or a co-located
spoke, with no such label SHALL group with its hub. Only a cluster that runs two or
more of a column's nodes in one provider lane SHALL get a container.

#### Scenario: FLT-MAP-08.01 -- A co-located spoke gets its own card and counts

- GIVEN a hub whose topology lists spoke `S` as co-located, and `S` has no
  promotion environment
- AND the hub's fleet breakdown reports gateways for managed clusters `S` and the
  hub's own controller
- WHEN the map model is built
- THEN the hub's column SHALL contain a spoke node `S`
- AND node `S` SHALL show only the gateways and sandboxes reported for `S`
- AND the hub node SHALL no longer count the gateways and sandboxes of `S`

#### Scenario: FLT-MAP-08.02 -- Grouped nodes sit in adjacent rows inside one box

- GIVEN a hub and a co-located spoke on the same physical cluster, and remote
  spokes on other clusters in the same provider lane
- WHEN the layout is computed
- THEN the hub SHALL stay on the hub spine
- AND the co-located spoke SHALL sit in the row directly below the hub
- AND one container box SHALL enclose both cards and no other card

#### Scenario: FLT-MAP-08.03 -- The container names the cluster

- GIVEN a container for physical cluster `C`
- WHEN the map is rendered
- THEN the container SHALL be labelled `C`
- AND the cards inside it SHALL NOT repeat `C` as their own cluster label

#### Scenario: FLT-MAP-08.04 -- A cluster with one node gets no container

- GIVEN a column whose nodes each run on a different physical cluster
- WHEN the map model is built
- THEN no container SHALL be created for that column

#### Scenario: FLT-MAP-08.05 -- A remote spoke without an environment gets its own node

- GIVEN a hub whose topology lists spoke `R` under `remote_spokes`, and `R` has no
  promotion environment
- AND the hub's fleet breakdown reports gateways for managed cluster `R`
- WHEN the map model is built
- THEN the hub's column SHALL contain a spoke node `R` showing only the gateways
  and sandboxes reported for `R`
- AND the hub node SHALL NOT count the gateways and sandboxes of `R`
- AND node `R` SHALL carry no release and SHALL NOT count toward the
  "health unavailable" alarm

#### Scenario: FLT-MAP-08.06 -- No promotion lines for a box outside promotion

- GIVEN a column holding a hub and a remote spoke `R` that has no promotion
  environment
- WHEN the map is rendered
- THEN the column's change and gate lines SHALL connect the hub
- AND no change or gate line SHALL connect node `R`

### Requirement: FLT-MAP-09 -- Each node finds its Argo CD Application by where it deploys

A node's delivery labels (cluster, provider, role, env), Argo health and links come
from the Argo CD Application that deploys its instance. The dashboard backend SHALL
match an Application to an instance by where the Application deploys, not by a
naming rule alone:

1. An Application whose destination namespace is the instance, and whose name is
   `<namespace>-<instance>` or just `<instance>`, SHALL match that instance.
2. An Application named `<namespace>-<instance>` SHALL match that instance,
   whatever its destination namespace.

When several Applications match one instance, the backend SHALL pick, in order:
the one with a delivery `role` label, then the `<namespace>-<instance>` name, then
the bare `<instance>` name, then the alphabetically first. A helper Application
that deploys into an instance's namespace under another name SHALL NOT match.

A node's physical cluster SHALL be its `cluster` delivery label. When the
Application has no such label, it SHALL be the Application's destination cluster
name, except Argo CD's `in-cluster` alias.

The promotion payload SHALL list, under `unpromoted`, every instance an
Application deploys (rule 1) that is not a promotion environment, with what the
Application reports. Only control-plane Applications SHALL qualify: the
Application SHALL name a physical cluster, and SHALL either carry a delivery
`role` label or be named just `<instance>`. The map SHALL use it for a remote spoke without a promotion
environment (FLT-MAP-08.05): the node takes its cluster, provider, capabilities,
Argo state and links from it, and still carries no release and no promotion lines.

#### Scenario: FLT-MAP-09.01 -- An Application named after the instance matches

- GIVEN promotion environment `B`
- AND an Application named `B` whose destination namespace is `B`
- WHEN the promotion payload is built
- THEN environment `B` SHALL carry that Application's labels and health

#### Scenario: FLT-MAP-09.02 -- The core Application wins over helpers

- GIVEN three Applications that deploy into namespace `A`: a helper with another
  name, an unlabelled one named `A`, and a labelled one named `<namespace>-A`
- WHEN the promotion payload is built
- THEN environment `A` SHALL carry the labelled Application's labels and health

#### Scenario: FLT-MAP-09.03 -- The cluster falls back to the destination cluster

- GIVEN an Application with no `cluster` delivery label that deploys to cluster `C`
- WHEN the promotion payload is built
- THEN its instance SHALL report cluster `C`
- AND an Application that deploys to `in-cluster` SHALL report no cluster

#### Scenario: FLT-MAP-09.04 -- Instances outside promotion are reported

- GIVEN an Application that deploys instance `G` on cluster `C`, and `G` is not a
  promotion environment
- WHEN the promotion payload is built
- THEN `unpromoted` SHALL hold `G` with cluster `C` and its Argo health
- AND an Application that matches only by name SHALL NOT appear there
- AND neither SHALL an Application on Argo CD's own cluster, nor an unlabelled
  Application named `<namespace>-<add-on>`

#### Scenario: FLT-MAP-09.05 -- A remote spoke box shows its physical cluster

- GIVEN a remote spoke `R` without a promotion environment
- AND `unpromoted` holds `R` with cluster `C`
- WHEN the map model is built
- THEN node `R` SHALL report cluster `C` and its Argo health and link
- AND node `R` SHALL still carry no release

### Requirement: FLT-MAP-06 -- Derived, deterministic, baked-in-free geometry

The layout SHALL remain pure geometry: deterministic from the model alone, with no
DOM access and no measurement, and SHALL derive the spine, the provider lanes, and
the stacking entirely from the HUB/SPOKE role and provider signals already present
on each node and lane. It SHALL NOT embed a static ENV-to-HUB map, a provider
allowlist, a hub-hosting provider name (including IBM), or a hub cluster name list.

#### Scenario: FLT-MAP-06.01 -- No baked-in hub or provider list

- GIVEN a map model whose HUB clusters and providers are not known ahead of time
- WHEN the layout is computed
- THEN the hub spine, the provider lanes, and the spoke stacking SHALL be derived
  from the per-node role and provider signals
- AND no hard-coded cluster name or provider name (including the hub-hosting
  provider) SHALL be consulted
