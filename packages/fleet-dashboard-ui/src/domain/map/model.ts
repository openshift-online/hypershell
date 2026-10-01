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
  /**
   * Lane key: one lane per cloud provider (the provider string, or "none" when
   * the server omits it). Hub-hosting providers sink to the bottom - geometry is
   * layout's job.
   */
  readonly laneKey: string;
  /** True when the server labels this node's role "hub" (case-insensitive). */
  readonly isHub: boolean;
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
  /** Names of this node's promotion gates, in server order (opaque data). */
  readonly gateNames: readonly string[];
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

/** A horizontal lane: one per cloud provider (rows are clouds, columns are envs). */
export interface MapLane {
  readonly key: string;
  readonly provider: string | null;
  /** True when any hub-role node lives in this provider's lane. */
  readonly hostsHub: boolean;
}

/** A promotion gate sitting between two adjacent columns on the hub spine. */
export interface MapGate {
  readonly id: string;
  readonly fromColumnKey: string;
  readonly toColumnKey: string;
  readonly badge: StatusBadge;
  /** Gate display name, from the destination's governing gates (opaque data). */
  readonly name: string | null;
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

/** Whether the server-reported role is "hub" (case-insensitive). */
function isHubRole(role: string | null): boolean {
  return (role ?? "").toLowerCase() === "hub";
}

/** The lane key for a node: its cloud provider, or "none" when unlabeled. */
function laneKeyFor(provider: string | null): string {
  return provider ?? "none";
}

function buildNode(env: PromotionEnvironment, fleet: FleetData): MapNode {
  const provider = nonEmpty(env.provider);
  const fl = findInstance(fleet.instances, env.name);
  const gateways = fl?.gateways ?? {};
  return {
    id: env.name,
    columnKey: nonEmpty(env.envLabel) ?? env.name,
    laneKey: laneKeyFor(provider),
    isHub: isHubRole(env.role),
    role: nonEmpty(env.role),
    provider,
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
    gateNames: env.gates.map((g) => g.name).filter((n) => n !== ""),
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
 * Order lanes top -> bottom, one per cloud provider: spoke-only clouds on top,
 * hub-hosting clouds sink to the bottom (so the promotion spine reads left->right
 * along the hubs), then stable alphabetical within each group. This mirrors the
 * prototype's per-cloud lanes and is fully derived - no provider list is baked in.
 */
function orderLanes(nodes: readonly MapNode[]): MapLane[] {
  const hostsHub = new Map<string, boolean>();
  const providerOf = new Map<string, string | null>();
  for (const n of nodes) {
    providerOf.set(n.laneKey, n.provider);
    hostsHub.set(n.laneKey, (hostsHub.get(n.laneKey) ?? false) || n.isHub);
  }
  const keys = [...hostsHub.keys()].sort((a, b) => {
    const ha = hostsHub.get(a) ?? false;
    const hb = hostsHub.get(b) ?? false;
    if (ha !== hb) {
      return ha ? 1 : -1; // hub clouds later => lower
    }
    return a < b ? -1 : a > b ? 1 : 0; // stable alpha
  });
  return keys.map((key) => ({
    key,
    provider: providerOf.get(key) ?? null,
    hostsHub: hostsHub.get(key) ?? false,
  }));
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
    if (node.isHub) {
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
      name: governing.gateNames[0] ?? null,
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
