import type {
  BillingOwner,
  CostGateway,
  GatewayAccessRole,
} from "./cost-attribution.js";
import {
  queryCostDaily,
  queryCostVector,
  type CostDailySeries,
  type CostSample,
} from "./cost-prometheus.js";
import type { RateCard } from "./cost-rate-card.js";
import type { MetricsSource } from "./metrics-source.js";
import {
  formatUtcCalendarDate,
  utcCalendarDatesInclusive,
  utcCalendarRange,
} from "./prometheus-range-query.js";

/**
 * Cost dashboard calculations (WEB-COST-04, WEB-COST-05, WEB-COST-09).
 *
 * Inputs are hub recording rules that hold reserved *quantities* (cores and
 * bytes), never money. Prometheus sums them into core-hours and byte-hours over
 * the range with a 1-hour subquery; the BFF applies the rate card, so a rate
 * card change re-prices history without rewriting series.
 */

export type CostRange = 7 | 30;

export const costRecordingRules = {
  clusterCpu: "hypershell:cluster_cpu_allocatable:cores",
  clusterMemory: "hypershell:cluster_memory_allocatable:bytes",
  namespaceCpu: "hypershell:namespace_cpu_requests:cores",
  namespaceMemory: "hypershell:namespace_memory_requests:bytes",
} as const;

/** Hours in the 730-hour month used for every monthly figure. */
export const hoursPerMonth = 730;
const bytesPerGib = 1024 ** 3;
const gatewayNamespacePattern = "openshell-.+";

export const includedCostCategories = [
  "cpu-requests",
  "memory-requests",
  "idle-node-capacity",
  "fixed-cluster-fees",
] as const;
export const excludedCostCategories = [
  "storage",
  "network-egress",
  "load-balancers",
  "managed-services",
  "llm-spend",
] as const;

export interface CostEnvelope {
  currency: string;
  estimate: true;
  excluded_categories: readonly string[];
  included_categories: readonly string[];
  range_days: CostRange;
  rate_card_version: string;
  status: "ok";
  coverage_end?: string;
  coverage_start?: string;
  /** Clusters with no CPU or memory rate; excluded from every total. */
  unpriced_clusters?: string[];
}

export interface CostQueryOptions {
  /** Restrict to these `cluster` label values (COST_CLUSTERS). */
  clusters?: readonly string[];
  /** Usage Prometheus (PROMETHEUS_URL) for unused gateways and sandboxes. */
  usageSource: MetricsSource;
  usageNamespace?: string;
}

export interface CostContext {
  costSource: MetricsSource;
  options: CostQueryOptions;
  rateCard: RateCard;
  timeoutMs: number;
}

interface Quantity {
  cpuCoreHours: number;
  memoryGibHours: number;
}

interface ClusterKey {
  cluster: string;
  environment: string;
  provider: string;
}

// ---------------------------------------------------------------------------
// Query building

const labelValue = /^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$/u;

function clusterMatcher(clusters: readonly string[] | undefined): string {
  if (clusters === undefined) return "";
  for (const cluster of clusters) {
    if (!labelValue.test(cluster))
      throw new Error("Invalid cost cluster label");
  }
  return `cluster=~${JSON.stringify(clusters.join("|") || "^$")}`;
}

function selector(...matchers: string[]): string {
  const parts = matchers.filter(Boolean);
  return parts.length > 0 ? `{${parts.join(",")}}` : "";
}

/** Window from the start of the calendar range to now, in whole hours. */
export function rangeWindow(
  rangeDays: CostRange,
  now = Date.now(),
): {
  hours: number;
  start: number;
} {
  const { start } = utcCalendarRange(rangeDays);
  const hours = Math.max(1, Math.ceil((now / 1000 - start) / 3600));
  return { hours, start };
}

const clusterLabels = "cluster, environment, provider";

/** Core-hours or byte-hours per cluster over the window. */
export function clusterQuantityQuery(
  metric: string,
  hours: number,
  extraMatchers: readonly string[] = [],
): string {
  return `sum by (${clusterLabels}) (sum_over_time(${metric}${selector(...extraMatchers)}[${String(hours)}h:1h]))`;
}

/** Core-hours or byte-hours per gateway-related namespace over the window. */
export function namespaceQuantityQuery(
  metric: string,
  hours: number,
  namespaceRegex: string,
  extraMatchers: readonly string[] = [],
): string {
  return `sum by (${clusterLabels}, namespace) (sum_over_time(${metric}${selector(`namespace=~${JSON.stringify(namespaceRegex)}`, ...extraMatchers)}[${String(hours)}h:1h]))`;
}

/** Earliest hourly sample of node capacity in the window, as Unix seconds. */
export function coverageStartQuery(
  hours: number,
  extraMatchers: readonly string[],
): string {
  return `min(min_over_time(timestamp(${costRecordingRules.clusterCpu}${selector(...extraMatchers)})[${String(hours)}h:1h]))`;
}

const namespaceName = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/u;

/**
 * PromQL regex for a gateway set: `openshell-<id>` for every gateway plus
 * `openshell-<id>-<workspace>` for managed-mode gateways (WEB-COST-04
 * "Gateway namespaces"). Operator-mode workspace namespaces are excluded.
 */
export function gatewayNamespaceRegex(
  gateways: readonly CostGateway[],
): string {
  const exact: string[] = [];
  const managed: string[] = [];
  for (const gateway of gateways) {
    if (!namespaceName.test(gateway.namespace)) {
      throw new Error("Invalid metrics namespace");
    }
    if (gateway.workspaceMode === "managed") managed.push(gateway.namespace);
    else exact.push(gateway.namespace);
  }
  const parts = [...exact];
  if (managed.length > 0) parts.push(`(${managed.join("|")})(-[a-z0-9-]+)?`);
  // An empty set must match nothing, not everything.
  return parts.length > 0 ? parts.join("|") : "^$";
}

// ---------------------------------------------------------------------------
// Pricing

function clusterKey(labels: Record<string, string>): ClusterKey {
  return {
    cluster: labels.cluster ?? "",
    environment: labels.environment ?? "",
    provider: labels.provider ?? "",
  };
}

function keyString(key: ClusterKey): string {
  return `${key.cluster}\u0000${key.environment}\u0000${key.provider}`;
}

function mergeQuantities(
  cpu: readonly CostSample[],
  memory: readonly CostSample[],
  keyOf: (labels: Record<string, string>) => string,
): Map<string, { labels: Record<string, string>; quantity: Quantity }> {
  const rows = new Map<
    string,
    { labels: Record<string, string>; quantity: Quantity }
  >();
  const row = (labels: Record<string, string>) => {
    const key = keyOf(labels);
    let existing = rows.get(key);
    if (existing === undefined) {
      existing = { labels, quantity: { cpuCoreHours: 0, memoryGibHours: 0 } };
      rows.set(key, existing);
    }
    return existing;
  };
  for (const sample of cpu)
    row(sample.labels).quantity.cpuCoreHours += sample.value;
  for (const sample of memory) {
    row(sample.labels).quantity.memoryGibHours += sample.value / bytesPerGib;
  }
  return rows;
}

/** Price a quantity on a cluster; undefined when the cluster is unpriced. */
export function priceQuantity(
  rateCard: RateCard,
  key: ClusterKey,
  quantity: Quantity,
): number | undefined {
  const rates = rateCard.resolve(key.cluster, key.provider || undefined);
  if (rates === undefined) return undefined;
  return (
    quantity.cpuCoreHours * rates.cpuCoreHour +
    quantity.memoryGibHours * rates.memoryGibHour
  );
}

/** Scale a cost over `coveredHours` to the 730-hour month. */
export function toMonthly(cost: number, coveredHours: number): number {
  return coveredHours > 0 ? (cost * hoursPerMonth) / coveredHours : 0;
}

export function roundMoney(value: number): number {
  return Math.round(value * 100) / 100;
}

// ---------------------------------------------------------------------------
// Gateway namespace mapping

export interface NamespaceOwner {
  gateway?: CostGateway;
  /** Set for `openshell-<id>-<workspace>` namespaces of a managed gateway. */
  workspace?: string;
}

/** Map a namespace to its current gateway, longest gateway namespace first. */
export function namespaceOwnerResolver(
  gateways: readonly CostGateway[],
): (namespace: string) => NamespaceOwner {
  const byNamespace = [...gateways].sort(
    (left, right) => right.namespace.length - left.namespace.length,
  );
  return (namespace) => {
    for (const gateway of byNamespace) {
      if (namespace === gateway.namespace) return { gateway };
      if (
        gateway.workspaceMode === "managed" &&
        namespace.startsWith(`${gateway.namespace}-`)
      ) {
        return {
          gateway,
          workspace: namespace.slice(gateway.namespace.length + 1),
        };
      }
    }
    return {};
  };
}

// ---------------------------------------------------------------------------
// Platform snapshot (admin routes)

export interface ClusterCost extends ClusterKey {
  capacityCost: number;
  fixedCost: number;
  idleCost: number;
  requestedCost: number;
}

export interface GatewayCost {
  cluster: string;
  cost: number;
  gateway: CostGateway;
  serverCost: number;
  workspaces: Map<string, number>;
}

export interface PlatformCostSnapshot {
  clusters: ClusterCost[];
  coveredHours: number;
  coverageStart?: string;
  /** Cost of gateway namespaces whose gateway no longer exists. */
  deletedGatewayCost: number;
  gateways: Map<string, GatewayCost>;
  rangeHours: number;
  unpricedClusters: string[];
}

export interface PlatformSnapshotInput {
  clusterCpu: readonly CostSample[];
  clusterMemory: readonly CostSample[];
  coverageStartSeconds?: number;
  gateways: readonly CostGateway[];
  namespaceCpu: readonly CostSample[];
  namespaceMemory: readonly CostSample[];
  nowSeconds: number;
  rangeHours: number;
  rangeStartSeconds: number;
  requestedCpu: readonly CostSample[];
  requestedMemory: readonly CostSample[];
}

/** Pure calculation of the platform view from query results (WEB-COST-04). */
export function computePlatformSnapshot(
  rateCard: RateCard,
  input: PlatformSnapshotInput,
): PlatformCostSnapshot {
  const partial =
    input.coverageStartSeconds !== undefined &&
    input.coverageStartSeconds > input.rangeStartSeconds + 3600;
  const coveredHours = partial
    ? Math.max(
        1,
        Math.ceil(
          (input.nowSeconds - (input.coverageStartSeconds ?? 0)) / 3600,
        ),
      )
    : input.rangeHours;
  const byCluster = (labels: Record<string, string>) =>
    keyString(clusterKey(labels));
  const capacity = mergeQuantities(
    input.clusterCpu,
    input.clusterMemory,
    byCluster,
  );
  const requested = mergeQuantities(
    input.requestedCpu,
    input.requestedMemory,
    byCluster,
  );
  const unpriced = new Set<string>();
  const clusters: ClusterCost[] = [];
  const priced = new Set<string>();

  for (const [key, { labels, quantity }] of capacity) {
    const cluster = clusterKey(labels);
    const capacityCost = priceQuantity(rateCard, cluster, quantity);
    if (capacityCost === undefined) {
      unpriced.add(cluster.cluster);
      continue;
    }
    priced.add(key);
    const requestedQuantity = requested.get(key)?.quantity ?? {
      cpuCoreHours: 0,
      memoryGibHours: 0,
    };
    const requestedCost =
      priceQuantity(rateCard, cluster, requestedQuantity) ?? 0;
    clusters.push({
      ...cluster,
      capacityCost,
      fixedCost:
        (rateCard.fixedMonthly(cluster.cluster) * coveredHours) / hoursPerMonth,
      // WEB-COST-04.02: an over-committed cluster has no idle cost.
      idleCost: Math.max(0, capacityCost - requestedCost),
      requestedCost,
    });
  }

  const owner = namespaceOwnerResolver(input.gateways);
  const gateways = new Map<string, GatewayCost>();
  let deletedGatewayCost = 0;
  const namespaces = mergeQuantities(
    input.namespaceCpu,
    input.namespaceMemory,
    (labels) => `${byCluster(labels)}\u0000${labels.namespace ?? ""}`,
  );
  for (const { labels, quantity } of namespaces.values()) {
    const cluster = clusterKey(labels);
    // Namespaces on an unpriced cluster are excluded with that cluster.
    if (capacity.has(keyString(cluster)) && !priced.has(keyString(cluster))) {
      continue;
    }
    const cost = priceQuantity(rateCard, cluster, quantity);
    if (cost === undefined) {
      unpriced.add(cluster.cluster);
      continue;
    }
    const namespace = labels.namespace ?? "";
    const { gateway, workspace } = owner(namespace);
    if (gateway === undefined) {
      deletedGatewayCost += cost;
      continue;
    }
    let row = gateways.get(gateway.id);
    if (row === undefined) {
      row = {
        cluster: cluster.cluster,
        cost: 0,
        gateway,
        serverCost: 0,
        workspaces: new Map(),
      };
      gateways.set(gateway.id, row);
    }
    row.cost += cost;
    if (workspace === undefined) row.serverCost += cost;
    else
      row.workspaces.set(
        workspace,
        (row.workspaces.get(workspace) ?? 0) + cost,
      );
  }

  return {
    clusters,
    coveredHours,
    coverageStart: partial
      ? formatUtcCalendarDate(input.coverageStartSeconds ?? 0)
      : undefined,
    deletedGatewayCost,
    gateways,
    rangeHours: input.rangeHours,
    unpricedClusters: [...unpriced].sort(),
  };
}

async function optionalNumber(
  run: () => Promise<CostSample[]>,
): Promise<number | undefined> {
  try {
    const samples = await run();
    return samples[0]?.value;
  } catch {
    return undefined;
  }
}

/** Run the hub queries and compute the platform snapshot. */
export async function queryPlatformSnapshot(
  context: CostContext,
  rangeDays: CostRange,
  gateways: readonly CostGateway[],
  now = Date.now(),
): Promise<PlatformCostSnapshot> {
  const { costSource, options, rateCard, timeoutMs } = context;
  const { hours, start } = rangeWindow(rangeDays, now);
  const clusters = clusterMatcher(options.clusters);
  const matchers = clusters ? [clusters] : [];
  const [
    clusterCpu,
    clusterMemory,
    requestedCpu,
    requestedMemory,
    namespaceCpu,
    namespaceMemory,
    coverageStartSeconds,
  ] = await Promise.all([
    queryCostVector(
      costSource,
      clusterQuantityQuery(costRecordingRules.clusterCpu, hours, matchers),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      clusterQuantityQuery(costRecordingRules.clusterMemory, hours, matchers),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      clusterQuantityQuery(costRecordingRules.namespaceCpu, hours, matchers),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      clusterQuantityQuery(costRecordingRules.namespaceMemory, hours, matchers),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      namespaceQuantityQuery(
        costRecordingRules.namespaceCpu,
        hours,
        gatewayNamespacePattern,
        matchers,
      ),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      namespaceQuantityQuery(
        costRecordingRules.namespaceMemory,
        hours,
        gatewayNamespacePattern,
        matchers,
      ),
      timeoutMs,
    ),
    optionalNumber(() =>
      queryCostVector(
        costSource,
        coverageStartQuery(hours, matchers),
        timeoutMs,
      ),
    ),
  ]);
  if (clusterCpu.length === 0 || clusterMemory.length === 0) {
    // WEB-COST-06: missing capacity series make the metrics unavailable.
    throw new CostSeriesMissingError();
  }
  return computePlatformSnapshot(rateCard, {
    clusterCpu,
    clusterMemory,
    coverageStartSeconds,
    gateways,
    namespaceCpu,
    namespaceMemory,
    nowSeconds: Math.floor(now / 1000),
    rangeHours: hours,
    rangeStartSeconds: start,
    requestedCpu,
    requestedMemory,
  });
}

export class CostSeriesMissingError extends Error {
  constructor() {
    super("Cost recording rules returned no series");
    this.name = "CostSeriesMissingError";
  }
}

// ---------------------------------------------------------------------------
// Daily series

export interface CostDailyPoint {
  date: string;
  value: number | null;
}

function priceDaily(
  rateCard: RateCard,
  cpu: readonly CostDailySeries[],
  memory: readonly CostDailySeries[],
  groupOf: (key: ClusterKey) => string,
): Map<string, Map<string, number>> {
  const result = new Map<string, Map<string, number>>();
  const add = (
    labels: Record<string, string>,
    date: string,
    quantity: Quantity,
  ) => {
    const key = clusterKey(labels);
    const cost = priceQuantity(rateCard, key, quantity);
    if (cost === undefined) return;
    const group = groupOf(key);
    const byDate = result.get(group) ?? new Map<string, number>();
    byDate.set(date, (byDate.get(date) ?? 0) + cost);
    result.set(group, byDate);
  };
  for (const series of cpu) {
    for (const [date, value] of series.values) {
      add(series.labels, date, { cpuCoreHours: value, memoryGibHours: 0 });
    }
  }
  for (const series of memory) {
    for (const [date, value] of series.values) {
      add(series.labels, date, {
        cpuCoreHours: 0,
        memoryGibHours: value / bytesPerGib,
      });
    }
  }
  return result;
}

function alignDaily(
  values: Map<string, number> | undefined,
  rangeDays: CostRange,
): CostDailyPoint[] {
  return utcCalendarDatesInclusive(rangeDays).map((date) => {
    const value = values?.get(date);
    return { date, value: value === undefined ? null : roundMoney(value) };
  });
}

function dailyQuery(metric: string, matchers: readonly string[]): string {
  return `sum by (${clusterLabels}) (sum_over_time(${metric}${selector(...matchers)}[1d:1h]))`;
}

/** Daily total and idle cost per cluster, for the platform trend sparklines. */
export async function queryPlatformDaily(
  context: CostContext,
  rangeDays: CostRange,
  now = Date.now(),
): Promise<{
  idle: CostDailyPoint[];
  total: CostDailyPoint[];
  byCluster: Map<string, CostDailyPoint[]>;
  byEnvironment: Map<string, CostDailyPoint[]>;
}> {
  const { costSource, options, rateCard, timeoutMs } = context;
  const { start } = rangeWindow(rangeDays, now);
  const clusters = clusterMatcher(options.clusters);
  const matchers = clusters ? [clusters] : [];
  const [capCpu, capMemory, reqCpu, reqMemory] = await Promise.all([
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.clusterCpu, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.clusterMemory, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.namespaceCpu, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.namespaceMemory, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
  ]);
  const perCluster = (key: ClusterKey) => keyString(key);
  const capacity = priceDaily(rateCard, capCpu, capMemory, perCluster);
  const requested = priceDaily(rateCard, reqCpu, reqMemory, perCluster);

  const total = new Map<string, number>();
  const idle = new Map<string, number>();
  const byCluster = new Map<string, Map<string, number>>();
  const byEnvironment = new Map<string, Map<string, number>>();
  const addTo = (
    map: Map<string, Map<string, number>>,
    group: string,
    date: string,
    value: number,
  ) => {
    const byDate = map.get(group) ?? new Map<string, number>();
    byDate.set(date, (byDate.get(date) ?? 0) + value);
    map.set(group, byDate);
  };
  for (const [key, byDate] of capacity) {
    const [cluster = "", environment = ""] = key.split("\u0000");
    for (const [date, capacityCost] of byDate) {
      const requestedCost = requested.get(key)?.get(date) ?? 0;
      const dayTotal = Math.max(capacityCost, requestedCost);
      total.set(date, (total.get(date) ?? 0) + dayTotal);
      idle.set(
        date,
        (idle.get(date) ?? 0) + Math.max(0, capacityCost - requestedCost),
      );
      addTo(byCluster, cluster, date, dayTotal);
      addTo(byEnvironment, environment, date, dayTotal);
    }
  }
  const align = (map: Map<string, Map<string, number>>) =>
    new Map(
      [...map].map(([group, values]) => [group, alignDaily(values, rangeDays)]),
    );
  return {
    idle: alignDaily(idle, rangeDays),
    total: alignDaily(total, rangeDays),
    byCluster: align(byCluster),
    byEnvironment: align(byEnvironment),
  };
}

// ---------------------------------------------------------------------------
// Scoped (My cost) queries

export interface ScopedGatewayCost {
  cluster: string;
  cost: number;
  workspaces: Map<string, number>;
}

/**
 * Cost of a gateway set, constrained to the set's namespaces in the PromQL
 * selector before aggregation (WEB-COST-09 step 2).
 */
export async function queryScopedGatewayCost(
  context: CostContext,
  rangeDays: CostRange,
  gateways: readonly CostGateway[],
  now = Date.now(),
): Promise<{
  coveredHours: number;
  coverageStart?: string;
  costs: Map<string, ScopedGatewayCost>;
  unpricedClusters: string[];
}> {
  const { costSource, options, rateCard, timeoutMs } = context;
  const { hours, start } = rangeWindow(rangeDays, now);
  const clusters = clusterMatcher(options.clusters);
  const matchers = clusters ? [clusters] : [];
  const regex = gatewayNamespaceRegex(gateways);
  const [cpu, memory, coverageStartSeconds] = await Promise.all([
    queryCostVector(
      costSource,
      namespaceQuantityQuery(
        costRecordingRules.namespaceCpu,
        hours,
        regex,
        matchers,
      ),
      timeoutMs,
    ),
    queryCostVector(
      costSource,
      namespaceQuantityQuery(
        costRecordingRules.namespaceMemory,
        hours,
        regex,
        matchers,
      ),
      timeoutMs,
    ),
    optionalNumber(() =>
      queryCostVector(
        costSource,
        coverageStartQuery(hours, matchers),
        timeoutMs,
      ),
    ),
  ]);
  const nowSeconds = Math.floor(now / 1000);
  const partial =
    coverageStartSeconds !== undefined && coverageStartSeconds > start + 3600;
  const coveredHours = partial
    ? Math.max(1, Math.ceil((nowSeconds - coverageStartSeconds) / 3600))
    : hours;
  const owner = namespaceOwnerResolver(gateways);
  const costs = new Map<string, ScopedGatewayCost>();
  const unpriced = new Set<string>();
  const rows = mergeQuantities(
    cpu,
    memory,
    (labels) =>
      `${keyString(clusterKey(labels))}\u0000${labels.namespace ?? ""}`,
  );
  for (const { labels, quantity } of rows.values()) {
    const key = clusterKey(labels);
    const cost = priceQuantity(rateCard, key, quantity);
    if (cost === undefined) {
      unpriced.add(key.cluster);
      continue;
    }
    const { gateway, workspace } = owner(labels.namespace ?? "");
    if (gateway === undefined) continue;
    const row = costs.get(gateway.id) ?? {
      cluster: key.cluster,
      cost: 0,
      workspaces: new Map<string, number>(),
    };
    row.cost += cost;
    if (workspace !== undefined) {
      row.workspaces.set(
        workspace,
        (row.workspaces.get(workspace) ?? 0) + cost,
      );
    }
    costs.set(gateway.id, row);
  }
  return {
    costs,
    coveredHours,
    coverageStart: partial
      ? formatUtcCalendarDate(coverageStartSeconds)
      : undefined,
    unpricedClusters: [...unpriced].sort(),
  };
}

/** Daily cost of a gateway set (scoped before aggregation). */
export async function queryScopedDaily(
  context: CostContext,
  rangeDays: CostRange,
  gateways: readonly CostGateway[],
  now = Date.now(),
): Promise<CostDailyPoint[]> {
  const { costSource, options, rateCard, timeoutMs } = context;
  const { start } = rangeWindow(rangeDays, now);
  if (gateways.length === 0) {
    return utcCalendarDatesInclusive(rangeDays).map((date) => ({
      date,
      value: 0,
    }));
  }
  const clusters = clusterMatcher(options.clusters);
  const matchers = [
    `namespace=~${JSON.stringify(gatewayNamespaceRegex(gateways))}`,
    ...(clusters ? [clusters] : []),
  ];
  const [cpu, memory] = await Promise.all([
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.namespaceCpu, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
    queryCostDaily(
      costSource,
      dailyQuery(costRecordingRules.namespaceMemory, matchers),
      start,
      rangeDays,
      timeoutMs,
    ),
  ]);
  const priced = priceDaily(rateCard, cpu, memory, () => "all");
  const values = priced.get("all");
  // Days before any sample are gaps (null), not zero (WEB-COST-06).
  return alignDaily(values ?? new Map(), rangeDays);
}

// ---------------------------------------------------------------------------
// Usage inputs (PROMETHEUS_URL)

function usageNamespaceMatcher(
  namespaces: readonly string[] | undefined,
): string {
  if (namespaces === undefined) return "";
  for (const namespace of namespaces) {
    if (!namespaceName.test(namespace))
      throw new Error("Invalid metrics namespace");
  }
  return `namespace=~${JSON.stringify(namespaces.join("|") || "^$")}`;
}

/** Namespaces with non-peer gateway traffic in the range (WEB-USE-07). */
export function activeNamespacesQuery(
  rangeDays: CostRange,
  namespaces?: readonly string[],
): string {
  const scope = usageNamespaceMatcher(namespaces);
  const grpc = selector('method!~"Peer.*"', scope);
  const http = selector(scope);
  return `count by (namespace) (increase(openshell_server_grpc_requests_total${grpc}[${String(rangeDays)}d]) > 0) or count by (namespace) (increase(openshell_server_http_requests_total${http}[${String(rangeDays)}d]) > 0)`;
}

export async function queryActiveGatewayNamespaces(
  context: CostContext,
  rangeDays: CostRange,
  namespaces?: readonly string[],
): Promise<Set<string> | undefined> {
  try {
    const samples = await queryCostVector(
      context.options.usageSource,
      activeNamespacesQuery(rangeDays, namespaces),
      context.timeoutMs,
    );
    return new Set(
      samples.map((sample) => sample.labels.namespace ?? "").filter(Boolean),
    );
  } catch {
    return undefined;
  }
}

export async function queryMeanActiveSandboxes(
  context: CostContext,
  rangeDays: CostRange,
  now = Date.now(),
): Promise<number | undefined> {
  const { hours } = rangeWindow(rangeDays, now);
  return optionalNumber(() =>
    queryCostVector(
      context.options.usageSource,
      `avg_over_time(sum(max by (managed_cluster, gateway_namespace) (hypershell_gateways_active_sandboxes_total))[${String(hours)}h:1h])`,
      context.timeoutMs,
    ),
  );
}

export async function queryActiveUsers(
  context: CostContext,
  rangeDays: CostRange,
): Promise<number | undefined> {
  const metric =
    rangeDays === 30
      ? "hypershell_users_unique_logins_last_30_days_total"
      : "hypershell_users_unique_logins_last_7_days_total";
  const scope = context.options.usageNamespace
    ? `{namespace=${JSON.stringify(context.options.usageNamespace)}}`
    : "";
  return optionalNumber(() =>
    queryCostVector(
      context.options.usageSource,
      `max(${metric}${scope})`,
      context.timeoutMs,
    ),
  );
}

// ---------------------------------------------------------------------------
// Responses

export function costEnvelope(
  rateCard: RateCard,
  rangeDays: CostRange,
  coverageStart: string | undefined,
  unpricedClusters: readonly string[],
): CostEnvelope {
  return {
    status: "ok",
    estimate: true,
    currency: rateCard.currency,
    rate_card_version: rateCard.version,
    range_days: rangeDays,
    included_categories: includedCostCategories,
    excluded_categories: excludedCostCategories,
    ...(coverageStart
      ? {
          coverage_start: coverageStart,
          coverage_end: formatUtcCalendarDate(Date.now() / 1000),
        }
      : {}),
    ...(unpricedClusters.length > 0
      ? { unpriced_clusters: [...unpricedClusters] }
      : {}),
  };
}

export interface CostSummaryResponse extends CostEnvelope {
  monthly_total: number;
  range_total: number;
  idle: {
    daily?: CostDailyPoint[];
    monthly: number;
    percent_of_total: number;
    unused_gateways_monthly?: number;
  };
  per_active_user?: number;
  per_gateway?: number;
  per_sandbox?: number;
}

export interface CostBreakdownRow {
  daily?: CostDailyPoint[];
  key: string;
  monthly: number;
}

export interface CostBreakdownResponse extends CostEnvelope {
  by_billing_owner?: {
    gateway_count: number;
    monthly: number;
    username: string;
  }[];
  by_cluster: (CostBreakdownRow & { environment: string; provider: string })[];
  by_environment: CostBreakdownRow[];
  by_resource_class: {
    gateways: number;
    idle: number;
    platform_overhead: number;
    sandboxes: number;
  };
}

export interface CostGatewayRow {
  billing_owner?: string;
  cluster: string;
  id: string;
  monthly: number;
  name: string;
  unused?: boolean;
}

export interface CostGatewaysResponse extends CostEnvelope {
  gateways: CostGatewayRow[];
}

function totals(snapshot: PlatformCostSnapshot) {
  let total = 0;
  let idle = 0;
  let fixed = 0;
  let requested = 0;
  for (const cluster of snapshot.clusters) {
    total +=
      Math.max(cluster.capacityCost, cluster.requestedCost) + cluster.fixedCost;
    idle += cluster.idleCost;
    fixed += cluster.fixedCost;
    requested += cluster.requestedCost;
  }
  let gatewayServer = snapshot.deletedGatewayCost;
  let sandboxes = 0;
  for (const gateway of snapshot.gateways.values()) {
    gatewayServer += gateway.serverCost;
    sandboxes += gateway.cost - gateway.serverCost;
  }
  const overhead = Math.max(0, requested - gatewayServer - sandboxes) + fixed;
  return { fixed, gatewayServer, idle, overhead, requested, sandboxes, total };
}

export function buildSummaryResponse(
  rateCard: RateCard,
  rangeDays: CostRange,
  snapshot: PlatformCostSnapshot,
  extras: {
    activeNamespaces?: Set<string>;
    activeUsers?: number;
    currentGatewayCount: number;
    daily?: CostDailyPoint[];
    meanActiveSandboxes?: number;
  },
): CostSummaryResponse {
  const sums = totals(snapshot);
  const monthly = (value: number) =>
    roundMoney(toMonthly(value, snapshot.coveredHours));
  let currentGatewayCost = 0;
  let unusedGatewayCost = 0;
  for (const gateway of snapshot.gateways.values()) {
    currentGatewayCost += gateway.cost;
    if (
      extras.activeNamespaces &&
      !extras.activeNamespaces.has(gateway.gateway.namespace)
    ) {
      unusedGatewayCost += gateway.cost;
    }
  }
  // Gateways with no cost series at all are unused too, but add no cost.
  const monthlyTotal = monthly(sums.total);
  return {
    ...costEnvelope(
      rateCard,
      rangeDays,
      snapshot.coverageStart,
      snapshot.unpricedClusters,
    ),
    monthly_total: monthlyTotal,
    range_total: roundMoney(sums.total),
    idle: {
      monthly: monthly(sums.idle),
      percent_of_total:
        sums.total > 0 ? Math.round((sums.idle / sums.total) * 1000) / 10 : 0,
      ...(extras.activeNamespaces
        ? { unused_gateways_monthly: monthly(unusedGatewayCost) }
        : {}),
      ...(extras.daily ? { daily: extras.daily } : {}),
    },
    ...(extras.activeUsers !== undefined && extras.activeUsers > 0
      ? { per_active_user: roundMoney(monthlyTotal / extras.activeUsers) }
      : {}),
    ...(extras.currentGatewayCount > 0
      ? {
          per_gateway: monthly(currentGatewayCost / extras.currentGatewayCount),
        }
      : {}),
    ...(extras.meanActiveSandboxes !== undefined &&
    extras.meanActiveSandboxes > 0
      ? { per_sandbox: monthly(sums.sandboxes / extras.meanActiveSandboxes) }
      : {}),
  };
}

export function buildBreakdownResponse(
  rateCard: RateCard,
  rangeDays: CostRange,
  snapshot: PlatformCostSnapshot,
  extras: {
    billingOwners?: Map<string, BillingOwner | undefined>;
    daily?: Awaited<ReturnType<typeof queryPlatformDaily>>;
  },
): CostBreakdownResponse {
  const sums = totals(snapshot);
  const monthly = (value: number) =>
    roundMoney(toMonthly(value, snapshot.coveredHours));
  const environments = new Map<string, number>();
  const clusterRows: CostBreakdownResponse["by_cluster"] = [];
  for (const cluster of snapshot.clusters) {
    const value =
      Math.max(cluster.capacityCost, cluster.requestedCost) + cluster.fixedCost;
    environments.set(
      cluster.environment,
      (environments.get(cluster.environment) ?? 0) + value,
    );
    clusterRows.push({
      key: cluster.cluster,
      environment: cluster.environment,
      provider: cluster.provider,
      monthly: monthly(value),
      ...(extras.daily?.byCluster.get(cluster.cluster)
        ? { daily: extras.daily.byCluster.get(cluster.cluster) }
        : {}),
    });
  }
  clusterRows.sort((left, right) => right.monthly - left.monthly);
  const byEnvironment = [...environments]
    .map(([key, value]) => ({
      key,
      monthly: monthly(value),
      ...(extras.daily?.byEnvironment.get(key)
        ? { daily: extras.daily.byEnvironment.get(key) }
        : {}),
    }))
    .sort((left, right) => right.monthly - left.monthly);

  let byBillingOwner: CostBreakdownResponse["by_billing_owner"];
  if (extras.billingOwners) {
    const owners = new Map<string, { gateway_count: number; cost: number }>();
    for (const gateway of snapshot.gateways.values()) {
      const username =
        extras.billingOwners.get(gateway.gateway.id)?.username ?? "";
      const row = owners.get(username) ?? { gateway_count: 0, cost: 0 };
      row.gateway_count += 1;
      row.cost += gateway.cost;
      owners.set(username, row);
    }
    byBillingOwner = [...owners]
      .map(([username, row]) => ({
        username,
        gateway_count: row.gateway_count,
        monthly: monthly(row.cost),
      }))
      .sort((left, right) => right.monthly - left.monthly);
  }

  return {
    ...costEnvelope(
      rateCard,
      rangeDays,
      snapshot.coverageStart,
      snapshot.unpricedClusters,
    ),
    by_environment: byEnvironment,
    by_cluster: clusterRows,
    by_resource_class: {
      gateways: monthly(sums.gatewayServer),
      sandboxes: monthly(sums.sandboxes),
      platform_overhead: monthly(sums.overhead),
      idle: monthly(sums.idle),
    },
    ...(byBillingOwner ? { by_billing_owner: byBillingOwner } : {}),
  };
}

export function buildGatewaysResponse(
  rateCard: RateCard,
  rangeDays: CostRange,
  snapshot: PlatformCostSnapshot,
  extras: {
    activeNamespaces?: Set<string>;
    billingOwners?: Map<string, BillingOwner | undefined>;
  },
  limit = 10,
): CostGatewaysResponse {
  const rows = [...snapshot.gateways.values()]
    .sort((left, right) => right.cost - left.cost)
    .slice(0, limit)
    .map((row): CostGatewayRow => {
      const owner = extras.billingOwners?.get(row.gateway.id)?.username;
      return {
        id: row.gateway.id,
        name: row.gateway.name,
        cluster: row.cluster,
        monthly: roundMoney(toMonthly(row.cost, snapshot.coveredHours)),
        ...(owner ? { billing_owner: owner } : {}),
        ...(extras.activeNamespaces
          ? { unused: !extras.activeNamespaces.has(row.gateway.namespace) }
          : {}),
      };
    });
  return {
    ...costEnvelope(
      rateCard,
      rangeDays,
      snapshot.coverageStart,
      snapshot.unpricedClusters,
    ),
    gateways: rows,
  };
}

// ---------------------------------------------------------------------------
// My cost responses

export interface MyCostGatewayInput {
  billingOwner?: BillingOwner;
  callerRole: GatewayAccessRole;
  gateway: CostGateway;
  isBillingOwner: boolean;
}

export interface MyCostSummaryResponse extends CostEnvelope {
  charged: { gateway_count: number; monthly: number; range_total: number };
  shared: { gateway_count: number; monthly: number };
  trend: CostDailyPoint[];
  /** True when the caller holds no gateway binding (WEB-COST-09.03). */
  empty: boolean;
}

export interface MyCostGatewaysResponse extends CostEnvelope {
  gateways: {
    billing_owner?: string;
    charged: boolean;
    cluster: string;
    id: string;
    monthly: number;
    name: string;
    role: GatewayAccessRole;
    unused?: boolean;
  }[];
  workspaces: {
    gateway_id: string;
    gateway_name: string;
    monthly: number;
    workspace: string;
  }[];
  empty: boolean;
}

export function buildMyCostSummary(
  rateCard: RateCard,
  rangeDays: CostRange,
  inputs: readonly MyCostGatewayInput[],
  scoped: Awaited<ReturnType<typeof queryScopedGatewayCost>>,
  trend: CostDailyPoint[],
): MyCostSummaryResponse {
  let charged = 0;
  let shared = 0;
  let chargedCount = 0;
  let sharedCount = 0;
  for (const input of inputs) {
    const cost = scoped.costs.get(input.gateway.id)?.cost ?? 0;
    if (input.isBillingOwner) {
      charged += cost;
      chargedCount += 1;
    } else {
      shared += cost;
      sharedCount += 1;
    }
  }
  const monthly = (value: number) =>
    roundMoney(toMonthly(value, scoped.coveredHours));
  return {
    ...costEnvelope(
      rateCard,
      rangeDays,
      scoped.coverageStart,
      scoped.unpricedClusters,
    ),
    empty: inputs.length === 0,
    charged: {
      gateway_count: chargedCount,
      monthly: monthly(charged),
      range_total: roundMoney(charged),
    },
    shared: { gateway_count: sharedCount, monthly: monthly(shared) },
    trend,
  };
}

export function buildMyCostGateways(
  rateCard: RateCard,
  rangeDays: CostRange,
  inputs: readonly MyCostGatewayInput[],
  scoped: Awaited<ReturnType<typeof queryScopedGatewayCost>>,
  visibleWorkspaces: (
    input: MyCostGatewayInput,
  ) => (workspace: string) => boolean,
  activeNamespaces: Set<string> | undefined,
): MyCostGatewaysResponse {
  const monthly = (value: number) =>
    roundMoney(toMonthly(value, scoped.coveredHours));
  const gateways: MyCostGatewaysResponse["gateways"] = [];
  const workspaces: MyCostGatewaysResponse["workspaces"] = [];
  for (const input of inputs) {
    const cost = scoped.costs.get(input.gateway.id);
    gateways.push({
      id: input.gateway.id,
      name: input.gateway.name,
      role: input.callerRole,
      charged: input.isBillingOwner,
      cluster: cost?.cluster ?? input.gateway.clusterId,
      monthly: monthly(cost?.cost ?? 0),
      ...(input.billingOwner
        ? { billing_owner: input.billingOwner.username }
        : {}),
      ...(activeNamespaces
        ? { unused: !activeNamespaces.has(input.gateway.namespace) }
        : {}),
    });
    const visible = visibleWorkspaces(input);
    for (const [workspace, value] of cost?.workspaces ?? []) {
      if (!visible(workspace)) continue;
      workspaces.push({
        gateway_id: input.gateway.id,
        gateway_name: input.gateway.name,
        workspace,
        monthly: monthly(value),
      });
    }
  }
  gateways.sort((left, right) => right.monthly - left.monthly);
  workspaces.sort((left, right) => right.monthly - left.monthly);
  return {
    ...costEnvelope(
      rateCard,
      rangeDays,
      scoped.coverageStart,
      scoped.unpricedClusters,
    ),
    empty: inputs.length === 0,
    gateways,
    workspaces,
  };
}
