// Promotion-topology MAP model. Pure projection of the promotion plane (+ fleet
// metrics) into the nodes / columns / lanes / gates the interactive map draws.
//
// FIREWALL: this module hard-codes NOTHING about the fleet - no environment
// names, no hub table, no provider list, no ordering. Columns come from the
// server's `envLabel`, lanes from `role`/`provider`, order from `promotion.order`,
// gates from the per-env gate data. A static ENV->HUB or provider table here would
// bake the topology into public source; deriving it at runtime is the whole point
// (data-architecture.spec §3.5 in the gitops repo).

import {
  findInstance,
  gatewayTone,
  totalGateways,
  ZERO_RATE,
  type FleetData,
  type GatewayPhaseCounts,
  type RateStats,
} from "../fleet";
import {
  environmentGateBadge,
  orderedEnvironments,
  promotionState,
  type PromotionData,
  type PromotionEnvironment,
  type PromotionState,
  type ReleaseBundle,
} from "../promotion";
import type { StatusBadge } from "../status";

/** The lane a node sits in: the hub spine, or a provider-grouped spoke lane. */
export type LaneKind = "hub" | "spoke";

/** External deep-links the server attaches to a node (all optional). */
export interface MapNodeLinks {
  readonly console: string | null;
  readonly argo: string | null;
  readonly pr: string | null;
  readonly analysis: string | null;
}

/** Control-plane RED metrics carried through for the drawer + service strip. */
export interface MapNodeMetrics {
  readonly rpc: RateStats;
  readonly reconcile: RateStats;
  readonly bff: RateStats;
  readonly provisionP95Ms: number | null;
}

/** One instance rendered as a card on the map. */
export interface MapNode {
  /** Instance key from the payload (runtime data, never compiled in). */
  readonly id: string;
  /** Column this node belongs to (its `envLabel`, else its own key). */
  readonly columnKey: string;
  /** Lane key: "hub" or `spoke:<provider>` - geometry is layout's job. */
  readonly laneKey: string;
  readonly laneKind: LaneKind;
  readonly role: string | null;
  readonly provider: string | null;
  readonly cluster: string | null;
  /** Stable identicon/identiname seed: the active digest, else the instance key. */
  readonly seed: string;
  readonly version: string | null;
  readonly digest: string | null;
  readonly proposedVersion: string | null;
  readonly proposedDigest: string | null;
  readonly state: PromotionState;
  readonly upToDate: boolean;
  readonly argoHealth: string | null;
  readonly argoSync: string | null;
  readonly prState: string | null;
  /** Worst-case gate badge for this node's active gates. */
  readonly gateBadge: StatusBadge;
  readonly gateways: GatewayPhaseCounts;
  readonly gatewaysTotal: number;
  readonly gatewayTone: StatusBadge["tone"];
  /** Total-gateway samples oldest -> newest for the sand spark (may be empty). */
  readonly gatewayHistory: readonly number[];
  readonly managedClusters: number | null;
  readonly users: number | null;
  readonly metrics: MapNodeMetrics;
  readonly links: MapNodeLinks;
}

/** A column of the map (a promotion environment), in server order. */
export interface MapColumn {
  readonly key: string;
  readonly index: number;
  readonly nodeIds: readonly string[];
}

/** A horizontal lane (the hub spine, or one provider's spoke row). */
export interface MapLane {
  readonly key: string;
  readonly kind: LaneKind;
  readonly provider: string | null;
}

/** A promotion gate sitting between two adjacent columns on the hub spine. */
export interface MapGate {
  readonly id: string;
  readonly fromColumnKey: string;
  readonly toColumnKey: string;
  readonly badge: StatusBadge;
  /** True when a release is actively promoting into the destination column. */
  readonly promoting: boolean;
}

export interface MapModel {
  readonly columns: readonly MapColumn[];
  readonly lanes: readonly MapLane[];
  readonly nodes: readonly MapNode[];
  readonly gates: readonly MapGate[];
  readonly frontier: ReleaseBundle | null;
}

function nonEmpty(v: string | null): string | null {
  return v !== null && v !== "" ? v : null;
}

function laneKeyFor(env: PromotionEnvironment): {
  key: string;
  kind: LaneKind;
  provider: string | null;
} {
  if ((env.role ?? "").toLowerCase() === "hub") {
    return { key: "hub", kind: "hub", provider: null };
  }
  const provider = nonEmpty(env.provider);
  return { key: `spoke:${provider ?? "none"}`, kind: "spoke", provider };
}

function buildNode(env: PromotionEnvironment, fleet: FleetData): MapNode {
  const lane = laneKeyFor(env);
  const fl = findInstance(fleet.instances, env.name);
  const gateways = fl?.gateways ?? {};
  return {
    id: env.name,
    columnKey: nonEmpty(env.envLabel) ?? env.name,
    laneKey: lane.key,
    laneKind: lane.kind,
    role: nonEmpty(env.role),
    provider: nonEmpty(env.provider),
    cluster: nonEmpty(env.cluster),
    seed: nonEmpty(env.activeDigest) ?? env.name,
    version: env.activeRelease,
    digest: env.activeDigest,
    proposedVersion: env.proposedRelease,
    proposedDigest: env.proposedDigest,
    state: promotionState(env),
    upToDate: env.upToDate,
    argoHealth: env.argoHealth,
    argoSync: env.argoSync,
    prState: env.prState,
    gateBadge: environmentGateBadge(env),
    gateways,
    gatewaysTotal: fl?.gatewaysTotal ?? totalGateways(gateways),
    gatewayTone: gatewayTone(gateways),
    gatewayHistory: fl?.gatewayHistory ?? [],
    managedClusters: fl?.managedClusters ?? null,
    users: fl?.users ?? null,
    metrics: {
      rpc: fl?.rpc ?? ZERO_RATE,
      reconcile: fl?.reconcile ?? ZERO_RATE,
      bff: fl?.bff ?? ZERO_RATE,
      provisionP95Ms: fl?.provisionP95Ms ?? null,
    },
    links: {
      console: env.consoleUrl,
      argo: env.argoUrl,
      pr: env.prUrl,
      analysis: env.analysisUrl,
    },
  };
}

/**
 * Order lanes top -> bottom with the hub spine in the middle: spoke lanes split
 * evenly above and below it, each in first-seen order so layout is stable. With
 * no hub the spokes simply stack in order.
 */
function orderLanes(nodes: readonly MapNode[]): MapLane[] {
  const seen = new Map<string, MapLane>();
  for (const n of nodes) {
    if (!seen.has(n.laneKey)) {
      seen.set(n.laneKey, {
        key: n.laneKey,
        kind: n.laneKind,
        provider: n.provider,
      });
    }
  }
  const hub = [...seen.values()].filter((l) => l.kind === "hub");
  const spokes = [...seen.values()].filter((l) => l.kind === "spoke");
  const half = Math.ceil(spokes.length / 2);
  return [...spokes.slice(0, half), ...hub, ...spokes.slice(half)];
}

/** The node that governs a column: its hub if present, else its first node. */
function governingNode(
  column: MapColumn,
  byId: ReadonlyMap<string, MapNode>,
): MapNode | null {
  let first: MapNode | null = null;
  for (const id of column.nodeIds) {
    const node = byId.get(id);
    if (!node) {
      continue;
    }
    first ??= node;
    if (node.laneKind === "hub") {
      return node;
    }
  }
  return first;
}

function buildGates(
  columns: readonly MapColumn[],
  byId: ReadonlyMap<string, MapNode>,
): MapGate[] {
  const gates: MapGate[] = [];
  for (let i = 1; i < columns.length; i++) {
    const to = columns[i];
    const from = columns[i - 1];
    if (!to || !from) {
      continue;
    }
    const governing = governingNode(to, byId);
    if (!governing) {
      continue;
    }
    gates.push({
      id: `${from.key}->${to.key}`,
      fromColumnKey: from.key,
      toColumnKey: to.key,
      badge: governing.gateBadge,
      promoting: governing.state === "promoting",
    });
  }
  return gates;
}

/**
 * Project the promotion plane (+ fleet metrics) into the map model. Columns follow
 * the server's promotion order; nodes carry their merged promotion + fleet state;
 * gates bridge adjacent columns using the destination column's governing node.
 */
export function buildMapModel(
  promotion: PromotionData,
  fleet: FleetData,
): MapModel {
  const nodes = orderedEnvironments(promotion).map((env) =>
    buildNode(env, fleet),
  );
  const byId = new Map(nodes.map((n) => [n.id, n]));

  // Columns in server order: first appearance of each columnKey wins.
  const columnOrder: string[] = [];
  const columnNodes = new Map<string, string[]>();
  for (const n of nodes) {
    let bucket = columnNodes.get(n.columnKey);
    if (!bucket) {
      bucket = [];
      columnNodes.set(n.columnKey, bucket);
      columnOrder.push(n.columnKey);
    }
    bucket.push(n.id);
  }
  const columns: MapColumn[] = columnOrder.map((key, index) => ({
    key,
    index,
    nodeIds: columnNodes.get(key) ?? [],
  }));

  return {
    columns,
    lanes: orderLanes(nodes),
    nodes,
    gates: buildGates(columns, byId),
    frontier: promotion.frontier,
  };
}
