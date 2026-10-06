// Promotion-topology MAP model. Pure projection of the promotion plane (+ fleet
// metrics) into the nodes / columns / lanes / gates the interactive map draws.
//
// FIREWALL: this module hard-codes NOTHING about the fleet - no environment
// names, no hub table, no provider list, no ordering. Columns are the promotion
// STAGES - the hubs in `promotion.order`, left -> right - and a spoke (a hub's
// managed cluster) stacks into its hub's column rather than forming its own stage,
// so a hub and its managed clusters share one column with no gate between them.
// Lanes come from `role`/`provider`, the env-type (`envLabel`) only groups column
// headers, and each column gets a gate to its right reporting that env's own
// analysis (the condition to promote out of it). A static ENV->HUB or provider
// table here would bake the topology into public source; deriving it at runtime is
// the whole point (data-architecture.spec §3.5 in the gitops repo).

import {
  findInstance,
  gatewayTone,
  totalGateways,
  ZERO_RATE,
  type FleetData,
  type GatewayHistorySample,
  type GatewayPhaseCounts,
  type RateStats,
  type SandboxClusterCount,
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
import { gatePhaseBadge, type StatusBadge } from "../status";

/** External deep-links the server attaches to a node (all optional). */
export interface MapNodeLinks {
  readonly console: string | null;
  readonly grafana: string | null;
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
  /**
   * Column (promotion stage) this node belongs to. A hub is its own column, laid
   * left -> right in promotion order. A spoke (a hub's managed cluster) carries its
   * hub's key, so it stacks into the hub's column instead of forming a separate
   * stage - a hub and its managed clusters move together, with no gate between them.
   */
  readonly columnKey: string;
  /** Env-type grouping (int/stage/prod, server data) used for the header bands. */
  readonly envLabel: string | null;
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
  /**
   * True when this node runs a DIFFERENT active release digest than its column's
   * governing (hub) node - i.e. a spoke that has drifted off the bundle its hub is
   * running, so the environment is internally inconsistent. Derived per column from
   * the active digests alone; false for the governing node itself and whenever a
   * digest is missing on either side (drift is then unknowable, not asserted).
   */
  readonly driftsFromColumn: boolean;
  readonly argoHealth: string | null;
  readonly argoSync: string | null;
  readonly prState: string | null;
  /** Worst-case gate badge for this node's active gates. */
  readonly gateBadge: StatusBadge;
  /** Names of this node's promotion gates, in server order (opaque data). */
  readonly gateNames: readonly string[];
  /**
   * Every promotion gate on this node, in server order: its opaque name paired
   * with its own phase badge. The full per-gate breakdown (e.g. argocd-health +
   * hypershell-analysis), so the detail panel can list each gate individually
   * rather than collapsing to the worst-case {@link gateBadge}. Gate names are
   * data, never inferred; empty-named gates are dropped.
   */
  readonly gateChecks: readonly GateCheck[];
  readonly gateways: GatewayPhaseCounts;
  readonly gatewaysTotal: number;
  readonly gatewayTone: StatusBadge["tone"];
  /** Per-phase samples oldest -> newest for the stacked sand spark (may be empty). */
  readonly gatewayHistory: readonly GatewayHistorySample[];
  /** Total active agent sandboxes across this instance's gateways. */
  readonly sandboxes: number;
  /** Active sandboxes per managed cluster, busiest-first (may be empty). */
  readonly sandboxesByCluster: readonly SandboxClusterCount[];
  /** Total active-sandbox count over the last day, oldest-first, on the gateway
   *  sparkline's grid - drives the lower sandbox sparkline (may be empty). */
  readonly sandboxHistory: readonly number[];
  readonly managedClusters: number | null;
  readonly users: number | null;
  /** Rolling 7-day unique-login count, or null when unknown. */
  readonly logins: number | null;
  /** Registered-user total over the last day, oldest-first, on the sandbox grid -
   *  drives the Users tile's mini sparkline (may be empty). */
  readonly userHistory: readonly number[];
  /** Unique-login count over the last day, oldest-first, same grid - drives the
   *  Logins tile's mini sparkline (may be empty). */
  readonly loginsHistory: readonly number[];
  /** Shared time axis (unix seconds, oldest-first) that gateway/sandbox/user/login
   *  histories are index-aligned to - drives the detail panel's shared temporal
   *  cursor and the hovered sample's date/time (may be empty). */
  readonly historyTimes: readonly number[];
  readonly metrics: MapNodeMetrics;
  readonly links: MapNodeLinks;
}

/** A column of the map (a promotion environment), in server order. */
export interface MapColumn {
  readonly key: string;
  readonly index: number;
  readonly nodeIds: readonly string[];
  /** Env-type (int/stage/prod) of this column's nodes, for the grouping bands. */
  readonly envLabel: string | null;
}

/** A horizontal lane: one per cloud provider (rows are clouds, columns are envs). */
export interface MapLane {
  readonly key: string;
  readonly provider: string | null;
  /** True when any hub-role node lives in this provider's lane. */
  readonly hostsHub: boolean;
}

/**
 * A promotion gate on the hub spine. It rides to the RIGHT of its SOURCE column
 * (`fromColumnKey`) because a gate reports the source environment's own health
 * (its analysis commit-status, name from runtime data), which is what must pass to
 * promote OUT of it into the next stage - a GitOps-Promoter env only advances once
 * its upstream dependency's checks are green. The last stage's gate is `terminal`
 * (no downstream column): it sits past the final column and reports that stage's
 * own analysis.
 */
/** One promotion gate's opaque name paired with its own phase badge. */
export interface GateCheck {
  readonly name: string;
  readonly badge: StatusBadge;
}

export interface MapGate {
  readonly id: string;
  /** The SOURCE column: the env whose analysis this gate reports. */
  readonly fromColumnKey: string;
  /** The destination column fed when this gate passes; "" for the terminal gate. */
  readonly toColumnKey: string;
  /** True for the final stage's gate: it has no downstream column. */
  readonly terminal: boolean;
  readonly badge: StatusBadge;
  /** Gate display name, from the SOURCE env's governing gates (opaque data). */
  readonly name: string | null;
  /**
   * The SOURCE env's full set of gates (each with its own badge), so the detail
   * panel lists every gate - e.g. argocd-health AND hypershell-analysis - instead
   * of only the worst-case summary {@link badge}. Derived from runtime gate data;
   * no gate name is hard-coded.
   */
  readonly checks: readonly GateCheck[];
  /** Deep-link to the source env's analysis run, when the server provides one. */
  readonly analysisUrl: string | null;
  /**
   * Deep-link to the source env's Argo CD Application tree, where its analysis
   * AnalysisRun and the Jobs/Pods it spawns surface - so the analysis logs are
   * viewable there without piping. Null when the server provides no Argo URL.
   */
  readonly argoUrl: string | null;
  /** True when a release is actively promoting into the destination column. */
  readonly promoting: boolean;
  /**
   * Identicon/identiname seed of the release promoting into the destination, when
   * one is (the destination's proposed digest, else its key). Null when nothing is
   * promoting. Lets the gate sidebar name WHICH bundle it is carrying.
   */
  readonly promotingSeed: string | null;
  /** Version string of the promoting release, when one is. Null otherwise. */
  readonly promotingVersion: string | null;
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

function buildNode(
  env: PromotionEnvironment,
  fleet: FleetData,
  columnKey: string,
): MapNode {
  const provider = nonEmpty(env.provider);
  const fl = findInstance(fleet.instances, env.name);
  const gateways = fl?.gateways ?? {};
  return {
    id: env.name,
    columnKey,
    envLabel: nonEmpty(env.envLabel),
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
    // Filled in by a post-pass in buildMapModel, once columns + their governing
    // nodes are known (a node cannot see its column-mates at build time).
    driftsFromColumn: false,
    argoHealth: env.argoHealth,
    argoSync: env.argoSync,
    prState: env.prState,
    gateBadge: environmentGateBadge(env),
    gateNames: env.gates.map((g) => g.name).filter((n) => n !== ""),
    gateChecks: env.gates
      .filter((g) => g.name !== "")
      .map((g) => ({ name: g.name, badge: gatePhaseBadge(g.phase) })),
    gateways,
    gatewaysTotal: fl?.gatewaysTotal ?? totalGateways(gateways),
    gatewayTone: gatewayTone(gateways),
    gatewayHistory: fl?.gatewayHistory ?? [],
    sandboxes: fl?.sandboxes ?? 0,
    sandboxesByCluster: fl?.sandboxesByCluster ?? [],
    sandboxHistory: fl?.sandboxHistory ?? [],
    managedClusters: fl?.managedClusters ?? null,
    users: fl?.users ?? null,
    logins: fl?.logins ?? null,
    userHistory: fl?.userHistory ?? [],
    loginsHistory: fl?.loginsHistory ?? [],
    historyTimes: fl?.historyTimes ?? [],
    metrics: {
      rpc: fl?.rpc ?? ZERO_RATE,
      reconcile: fl?.reconcile ?? ZERO_RATE,
      bff: fl?.bff ?? ZERO_RATE,
      provisionP95Ms: fl?.provisionP95Ms ?? null,
    },
    links: {
      console: env.consoleUrl,
      grafana: env.grafanaUrl,
      argo: env.argoUrl,
      pr: env.prUrl,
      analysis: env.analysisUrl,
    },
  };
}

/**
 * Assign each environment to a promotion COLUMN (stage). Hubs are the stages, one
 * column each. A spoke (non-hub) joins the column of the hub it belongs to: the hub
 * whose name is the longest prefix of the spoke's name (a managed cluster's name
 * extends its hub's name), so a hub and its managed clusters share one stage and get
 * no gate between them. A spoke that matches no hub keeps its own column. FIREWALL:
 * the pairing is derived from the runtime role + name data alone - no fleet names,
 * hub table or ordering are baked in.
 */
function assignColumns(
  envs: readonly PromotionEnvironment[],
): Map<string, string> {
  const hubNames = envs.filter((e) => isHubRole(e.role)).map((e) => e.name);
  const columnKey = new Map<string, string>();
  for (const e of envs) {
    if (isHubRole(e.role)) {
      columnKey.set(e.name, e.name);
      continue;
    }
    let hub: string | null = null;
    for (const h of hubNames) {
      if (
        e.name !== h &&
        e.name.startsWith(h) &&
        (hub === null || h.length > hub.length)
      ) {
        hub = h;
      }
    }
    columnKey.set(e.name, hub ?? e.name);
  }
  return columnKey;
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
  // One gate per column, riding to its RIGHT and reporting THAT column's (source)
  // analysis - the condition to promote out of it. The last column's gate is
  // terminal (no downstream env), reporting the final stage's own analysis.
  for (let i = 0; i < columns.length; i++) {
    const from = columns[i];
    if (!from) {
      continue;
    }
    const source = governingNode(from, byId);
    if (!source) {
      continue;
    }
    const to = columns[i + 1] ?? null;
    const dest = to ? governingNode(to, byId) : null;
    // A release is crossing this gate only when the DESTINATION is receiving one;
    // the terminal gate (no destination) never animates.
    const promoting = dest?.state === "promoting";
    gates.push({
      id: to ? `${from.key}->${to.key}` : `${from.key}->end`,
      fromColumnKey: from.key,
      toColumnKey: to?.key ?? "",
      terminal: to === null,
      badge: source.gateBadge,
      name: source.gateNames[0] ?? null,
      checks: source.gateChecks,
      analysisUrl: source.links.analysis,
      argoUrl: source.links.argo,
      promoting,
      // The bundle in flight is the destination's PROPOSED release (identicon seed
      // = its proposed digest, else the instance key). Null when nothing is moving.
      promotingSeed: promoting ? (dest.proposedDigest ?? dest.id) : null,
      promotingVersion: promoting ? dest.proposedVersion : null,
    });
  }
  return gates;
}

/**
 * Project the promotion plane (+ fleet metrics) into the map model. Columns are the
 * hub stages in the server's promotion order (spokes stack into their hub's column);
 * nodes carry their merged promotion + fleet state; each column gets a gate to its
 * right reporting that (source) column's governing node's analysis.
 */
export function buildMapModel(
  promotion: PromotionData,
  fleet: FleetData,
): MapModel {
  const envs = orderedEnvironments(promotion);
  const columnKeyByName = assignColumns(envs);
  const built = envs.map((env) =>
    buildNode(env, fleet, columnKeyByName.get(env.name) ?? env.name),
  );

  // Version-drift post-pass: a node drifts when it runs a different active digest
  // than its column's governing (hub) node. Needs the whole column, so it runs here
  // rather than in buildNode. Group by columnKey, pick the governing digest (hub's,
  // else the first node's), and flag every node whose digest differs. Missing
  // digests are never asserted as drift (unknowable, not divergent).
  const columnGoverningDigest = new Map<string, string | null>();
  for (const n of built) {
    const cur = columnGoverningDigest.get(n.columnKey);
    // Hub wins; otherwise the first node seen seeds the column's reference digest.
    if (cur === undefined || n.isHub) {
      columnGoverningDigest.set(n.columnKey, nonEmpty(n.digest));
    }
  }
  const nodes = built.map((n) => {
    const ref = columnGoverningDigest.get(n.columnKey) ?? null;
    const own = nonEmpty(n.digest);
    const driftsFromColumn =
      own !== null && ref !== null && !n.isHub && own !== ref;
    return driftsFromColumn ? { ...n, driftsFromColumn } : n;
  });
  const byId = new Map(nodes.map((n) => [n.id, n]));

  // Columns in server order: first appearance of each columnKey wins.
  const columnOrder: string[] = [];
  const columnNodes = new Map<string, string[]>();
  const columnEnvLabel = new Map<string, string | null>();
  for (const n of nodes) {
    let bucket = columnNodes.get(n.columnKey);
    if (!bucket) {
      bucket = [];
      columnNodes.set(n.columnKey, bucket);
      columnEnvLabel.set(n.columnKey, n.envLabel);
      columnOrder.push(n.columnKey);
    }
    bucket.push(n.id);
  }
  const columns: MapColumn[] = columnOrder.map((key, index) => ({
    key,
    index,
    nodeIds: columnNodes.get(key) ?? [],
    envLabel: columnEnvLabel.get(key) ?? null,
  }));

  return {
    columns,
    lanes: orderLanes(nodes),
    nodes,
    gates: buildGates(columns, byId),
    frontier: promotion.frontier,
  };
}
