// Per-managed-cluster (spoke) attribution: decompose a hub instance's gateway +
// sandbox counts into the spokes they run on, classified against the hub's topology
// document. A hub's api-server reports its own population AND every managed cluster's
// under one instance (the counts are summed into the instance total), so this is the
// seam that splits that total back apart for the detail panel: the hub's own counts,
// co-located spokes (nested under the hub) and remote spokes (linked out).
//
// FIREWALL: nothing about the fleet is baked in. The hub <-> spoke relationships come
// entirely from the runtime topology document; the hub's own row is recognised by the
// product-level "<instance>-hub" controller-naming convention (a structural role
// marker, not a fleet identifier), derived from the topology's own hub instance/dns
// label at runtime - never a hard-coded name.

import type {
  GatewayClusterBreakdown,
  GatewayPhaseCounts,
  SandboxClusterCount,
} from "./fleet";
import type { InstanceTopology } from "./topology";

/** One managed cluster's merged gateway + sandbox counts. */
export interface SpokeRow {
  /** The managed-cluster (spoke) name, opaque runtime data. */
  readonly managedCluster: string;
  readonly gateways: GatewayPhaseCounts;
  readonly gatewaysTotal: number;
  readonly sandboxes: number;
}

/**
 * A hub instance's population split by where it runs. `hubOwn` is the hub's own
 * controller row (null when none is reported). `coLocated` spokes share the hub's
 * cluster (nested under it); `remote` spokes live elsewhere (linked out). `unknown`
 * holds rows topology cannot place - including EVERY row when topology is missing, so
 * the panel can still list the raw breakdown under an "attribution unavailable" label.
 */
export interface SpokeAttribution {
  readonly hubOwn: SpokeRow | null;
  readonly coLocated: readonly SpokeRow[];
  readonly remote: readonly SpokeRow[];
  readonly unknown: readonly SpokeRow[];
  /** True when a topology document was available to classify this hub against. */
  readonly hasTopology: boolean;
}

/** Busiest-first, ties broken by managed-cluster name, so the order is stable. */
function byBusiest(a: SpokeRow, b: SpokeRow): number {
  if (a.gatewaysTotal !== b.gatewaysTotal) {
    return b.gatewaysTotal - a.gatewaysTotal;
  }
  if (a.sandboxes !== b.sandboxes) {
    return b.sandboxes - a.sandboxes;
  }
  return a.managedCluster.localeCompare(b.managedCluster);
}

/**
 * Merge an instance's per-spoke gateway and sandbox rows into one row per managed
 * cluster. A spoke may appear in only one of the two series (e.g. gateways but no
 * active sandboxes); the merge keeps it with the missing side zeroed.
 */
function mergeRows(
  gateways: readonly GatewayClusterBreakdown[],
  sandboxes: readonly SandboxClusterCount[],
): SpokeRow[] {
  const byCluster = new Map<
    string,
    { gw: GatewayClusterBreakdown | null; sb: number }
  >();
  for (const g of gateways) {
    if (g.managedCluster === "") {
      continue;
    }
    const cur = byCluster.get(g.managedCluster) ?? { gw: null, sb: 0 };
    cur.gw = g;
    byCluster.set(g.managedCluster, cur);
  }
  for (const s of sandboxes) {
    if (s.managedCluster === "") {
      continue;
    }
    const cur = byCluster.get(s.managedCluster) ?? { gw: null, sb: 0 };
    cur.sb += s.count;
    byCluster.set(s.managedCluster, cur);
  }
  const rows: SpokeRow[] = [];
  for (const [managedCluster, { gw, sb }] of byCluster) {
    rows.push({
      managedCluster,
      gateways: gw?.gateways ?? {},
      gatewaysTotal: gw?.total ?? 0,
      sandboxes: sb,
    });
  }
  return rows;
}

/**
 * Whether a managed-cluster row is the hub's OWN controller row. By the product's
 * controller-naming convention the hub's own gateways carry `managed_cluster` =
 * "<instance>-hub" (each spoke carries its own name). We recognise that by stripping a
 * trailing "-hub" role marker and matching the hub's instance or DNS label - both read
 * from the topology document at runtime, so no fleet name is compiled in.
 */
function isHubOwnRow(
  managedCluster: string,
  hub: InstanceTopology["hub"],
): boolean {
  if (!hub) {
    return false;
  }
  const ids = [hub.instance, hub.dnsLabel].filter(
    (v): v is string => v !== null && v !== "",
  );
  const stripped = managedCluster.replace(/-hub$/, "");
  return ids.includes(managedCluster) || ids.includes(stripped);
}

/**
 * Classify a hub instance's gateway + sandbox breakdown into hub-own / co-located /
 * remote / unknown buckets using its topology document. Pure: same inputs -> same
 * output. When `topology` is null/absent the hub cannot be resolved, so every row
 * lands in `unknown` and `hasTopology` is false (the panel shows a flat, unattributed
 * list). Rows topology knows nothing about (e.g. a soft-deleted cluster, or the
 * `managed_cluster="unknown"` bucket) also fall through to `unknown`.
 */
export function classifySpokes(
  gateways: readonly GatewayClusterBreakdown[],
  sandboxes: readonly SandboxClusterCount[],
  topology: InstanceTopology | null,
): SpokeAttribution {
  const rows = mergeRows(gateways, sandboxes);

  if (!topology) {
    return {
      hubOwn: null,
      coLocated: [],
      remote: [],
      unknown: [...rows].sort(byBusiest),
      hasTopology: false,
    };
  }

  const coLocatedSet = new Set(
    topology.spokes.map((s) => s.name).filter((n) => n !== ""),
  );
  const remoteSet = new Set(
    (topology.hub?.remoteSpokes ?? []).filter((n) => n !== ""),
  );

  let hubOwn: SpokeRow | null = null;
  const coLocated: SpokeRow[] = [];
  const remote: SpokeRow[] = [];
  const unknown: SpokeRow[] = [];
  for (const row of rows) {
    if (coLocatedSet.has(row.managedCluster)) {
      coLocated.push(row);
    } else if (remoteSet.has(row.managedCluster)) {
      remote.push(row);
    } else if (
      hubOwn === null &&
      isHubOwnRow(row.managedCluster, topology.hub)
    ) {
      // The first row matching the hub's own controller naming is the hub-own row;
      // any further unrecognised rows are genuinely unplaceable.
      hubOwn = row;
    } else {
      unknown.push(row);
    }
  }

  return {
    hubOwn,
    coLocated: coLocated.sort(byBusiest),
    remote: remote.sort(byBusiest),
    unknown: unknown.sort(byBusiest),
    hasTopology: true,
  };
}

/** Whether an attribution carries any SPOKE row (i.e. anything beyond hub-own). */
export function hasSpokeRows(attr: SpokeAttribution): boolean {
  return (
    attr.coLocated.length > 0 ||
    attr.remote.length > 0 ||
    attr.unknown.length > 0
  );
}
