# Gateway Managed-Cluster Attribution

**Status:** Active
**Applies to:** `components/api-server` gateway Prometheus collectors and DAO, `components/fleet-dashboard` BFF Prometheus source

## Purpose

Attribute gateway and active-sandbox counts to the **managed cluster** (spoke) a
gateway actually runs on, rather than lumping every count under the hub instance
that emits the metric. A single hub `api-server` manages many managed clusters;
today both `hypershell_gateways_total` and
`hypershell_gateways_active_sandboxes_total` are emitted as hub-wide database
aggregates with no spoke dimension, so a gateway running on a spoke is
mis-reported as running on the hub. This specification adds a `managed_cluster`
label to both gauges, sourced from the ManagedCluster the gateway references, so
counts can be grouped per spoke end to end.

### Three identities, kept distinct

HyperShell topology involves three separate identities that this spec must not
conflate:

| Identity | Example | Where it appears |
| --- | --- | --- |
| Physical OCP cluster | `hypc-ibm-00` | GitOps directory / `topology.json` |
| Hub instance | `hyp0` | Prometheus `namespace` label; the scrape target |
| Managed cluster (spoke) | `hyp0-spoke0` | ManagedCluster registry `Name` |

The `cluster`, `provider`, and `region` labels already present on hub metrics are
**scrape-injected hub identity** (always the hub, e.g. `hyp0` → `hypc-ibm-00`),
not the spoke a gateway runs on. The `managed_cluster` label introduced here is
**application-emitted** and names the spoke. Managed clusters self-register from
GitOps, so a ManagedCluster registry `Name` equals its GitOps spoke name; that
name is therefore the stable join key between these metrics and the fleet
topology.

### Relationship to other specifications

| Concern | This spec | Related spec |
| --- | --- | --- |
| Gateway phase counts (fleet-wide) | Adds the per-spoke dimension | `platform/gateway-metrics-dashboard.spec.md` (DASH-01, DASH-05) |
| Active sandbox counts (fleet-wide) | Adds the per-spoke dimension | `platform/gateway-sandbox-active-trends.spec.md` |
| Managed cluster inventory (registry totals) | Out of scope | `platform/platform-inventory.spec.md` |
| Map rendering of spokes (nest co-located / link remote) | Out of scope; this spec delivers the data only | `web-console/operational-dashboard.spec.md` |

This spec is additive: it does not rename either metric, remove any existing
label, or change fleet-wide totals. Rendering per-spoke counts on the map -
including resolving co-located versus remote spokes against topology - is
downstream work that consumes the data this spec exposes.

## Requirements

### Requirement: GMCA-01 -- Managed-Cluster Label on the Gateway Phase Gauge

The API server gateway phase collector SHALL add a `managed_cluster` label to
`hypershell_gateways_total`, in addition to the existing `phase` label. The label
value SHALL be the `Name` of the ManagedCluster referenced by each gateway's
`cluster_id`.

The collector SHALL continue to query the database once per scrape and SHALL
group gateway counts by both managed cluster and phase. For every managed cluster
that has at least one gateway, the collector SHALL emit all canonical phases from
`gatewayhealth.PhaseStrings()` for that managed cluster - including phases whose
count is zero - so a per-spoke phase series never has gaps while that spoke has
any gateway. The collector SHALL also emit the `other` phase bucket per managed
cluster for gateways whose stored phase is outside the canonical vocabulary, so
the per-spoke total never silently under-reports.

A gateway whose `cluster_id` does not resolve to a registered ManagedCluster
(for example a soft-deleted or unknown cluster) SHALL be attributed to
`managed_cluster="unknown"` rather than dropped, so the sum across managed
clusters always equals the fleet total.

When the database query fails, the collector SHALL emit
`prometheus.NewInvalidMetric` so the scrape registers as failed, preserving the
behavior of `platform/gateway-metrics-dashboard.spec.md` DASH-01.

#### Scenario: Gateways on different spokes are attributed separately

- GIVEN three `Running` gateways reference ManagedCluster `hyp0-spoke0` and two reference `hyp0-spoke1`
- WHEN Prometheus scrapes the API server `/metrics` endpoint
- THEN the response SHALL contain `hypershell_gateways_total{phase="Running",managed_cluster="hyp0-spoke0"}` with value `3`
- AND `hypershell_gateways_total{phase="Running",managed_cluster="hyp0-spoke1"}` with value `2`

#### Scenario: Canonical phases are gapless per spoke

- GIVEN managed cluster `hyp0-spoke0` has gateways only in the `Running` phase
- WHEN Prometheus scrapes the metrics endpoint
- THEN `hypershell_gateways_total{phase="Failed",managed_cluster="hyp0-spoke0"}` SHALL be present with value `0`
- AND every other canonical phase SHALL be present for `hyp0-spoke0`

#### Scenario: Gateway on an unresolved cluster is bucketed, not dropped

- GIVEN a gateway references a `cluster_id` with no matching ManagedCluster row
- WHEN Prometheus scrapes the metrics endpoint
- THEN that gateway SHALL be counted under `managed_cluster="unknown"`
- AND the sum of `hypershell_gateways_total` across all `managed_cluster` values SHALL equal the fleet total

---

### Requirement: GMCA-02 -- Managed-Cluster Label on the Active-Sandbox Gauge

The API server active-sandbox collector SHALL add a `managed_cluster` label to
`hypershell_gateways_active_sandboxes_total`. The collector SHALL query the
database once per scrape and emit, for each managed cluster, the sum of
`active_sandbox_count` across that managed cluster's live gateways, treating a
NULL count as zero.

A gateway whose `cluster_id` does not resolve to a registered ManagedCluster
SHALL contribute to `managed_cluster="unknown"`. When the database query fails,
the collector SHALL emit `prometheus.NewInvalidMetric`.

#### Scenario: Active sandboxes are attributed per spoke

- GIVEN gateways on `hyp0-spoke0` have 4 active sandboxes in total and gateways on `hyp0-spoke1` have 1
- WHEN Prometheus scrapes the metrics endpoint
- THEN `hypershell_gateways_active_sandboxes_total{managed_cluster="hyp0-spoke0"}` SHALL be `4`
- AND `hypershell_gateways_active_sandboxes_total{managed_cluster="hyp0-spoke1"}` SHALL be `1`
- AND the sum across all `managed_cluster` values SHALL equal the prior fleet-wide total

---

### Requirement: GMCA-03 -- Name Resolution from the Managed-Cluster Registry

The API server SHALL resolve each gateway's `managed_cluster` label value from
the ManagedCluster registry record referenced by the gateway's `cluster_id`,
using the registry `Name`. Because managed clusters self-register from GitOps,
this `Name` equals the GitOps spoke name, making it the stable join key for
downstream topology correlation.

Name resolution SHALL run inside the single per-scrape query path and SHALL NOT
issue one registry lookup per gateway. Resolution of all referenced
`cluster_id`s to names SHALL be bounded by the number of distinct managed
clusters, not the number of gateways.

#### Scenario: Label value matches the registry name

- GIVEN a ManagedCluster registered with `Name: "hyp0-spoke0"` and a gateway referencing its `cluster_id`
- WHEN the collector emits the gateway's metric
- THEN the sample SHALL carry `managed_cluster="hyp0-spoke0"`

---

### Requirement: GMCA-04 -- Backward-Compatible Fleet Aggregation

This change SHALL be additive. Neither metric SHALL be renamed, and no existing
label SHALL be removed. Consumers that aggregate fleet-wide totals by summing
over the new `managed_cluster` label SHALL observe the same totals they observed
before this change.

The scrape-injected hub identity labels (`cluster`, `provider`, `region`) SHALL
remain untouched and SHALL NOT be reinterpreted as spoke identity. Consumers that
attribute counts to a spoke SHALL use `managed_cluster`, never the scrape-injected
`cluster` label.

#### Scenario: Existing fleet-total consumers are unaffected

- GIVEN a consumer computes `sum by (phase) (hypershell_gateways_total{...})`
- WHEN the `managed_cluster` label is added
- THEN the per-phase fleet totals SHALL be unchanged

---

### Requirement: GMCA-05 -- BFF Attribution by Managed Cluster

The fleet-dashboard BFF Prometheus source SHALL group gateway phase counts and
active-sandbox counts by the `managed_cluster` label rather than by the
scrape-injected `cluster` label. The BFF SHALL expose, per hub instance, the
per-managed-cluster gateway and active-sandbox counts so the dashboard can
attribute them to the correct spoke.

Per-instance and fleet-wide totals surfaced by the BFF SHALL remain unchanged by
this grouping. A managed-cluster breakdown with no gateways SHALL contribute no
rows rather than synthesizing zero-count spokes.

#### Scenario: BFF reports counts per managed cluster

- GIVEN Prometheus holds `hypershell_gateways_total{managed_cluster="hyp0-spoke0"}` and `{managed_cluster="hyp0-spoke1"}` under hub instance `hyp0`
- WHEN the BFF builds the fleet view for `hyp0`
- THEN it SHALL report gateway counts for `hyp0-spoke0` and `hyp0-spoke1` separately
- AND the sum of the per-managed-cluster counts SHALL equal the `hyp0` instance total

#### Scenario: Scrape-injected cluster label is not used for attribution

- GIVEN every `hyp0` sample carries the scrape-injected `cluster="hypc-ibm-00"` label
- WHEN the BFF attributes gateway counts to spokes
- THEN it SHALL group by `managed_cluster`, not `cluster`
- AND a spoke's counts SHALL NOT be attributed to the hub's physical cluster

---

### Requirement: GMCA-06 -- Verification

The API server SHALL include unit tests for the gateway phase and active-sandbox
collectors (or their DAO methods) covering: per-managed-cluster grouping, gapless
canonical phases per managed cluster, the `other` phase bucket, the
`managed_cluster="unknown"` bucket for unresolved `cluster_id`s, and that the sum
across managed clusters equals the fleet total.

The fleet-dashboard BFF SHALL include tests asserting that gateway and
active-sandbox counts are grouped by `managed_cluster` and that per-instance
totals are preserved.

#### Scenario: CI exercises per-spoke attribution

- GIVEN a fixture with gateways across two managed clusters plus one unresolved `cluster_id`
- WHEN collector and BFF tests run
- THEN they SHALL assert per-managed-cluster counts, the `unknown` bucket, and that the summed total matches the fleet total
