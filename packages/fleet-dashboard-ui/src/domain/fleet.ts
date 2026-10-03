// Fleet plane: per-instance observed state derived from metrics. No instance or
// cluster identity is baked in here - the instance list arrives from the server.
// The map view merges this (gateway counts, control-plane RED metrics, history)
// with the promotion plane (role/provider/env labels, digests, links); role and
// provider therefore stay nullable here because the fleet metrics source does not
// report them - see domain/map/model.ts for the merge.

export type GatewayPhaseCounts = Readonly<Record<string, number>>;

/**
 * One time-step of the stacked "sand" sparkline: gateway counts split into the
 * three phases the chart layers. The server reports oldest-first samples already
 * bucketed into these three; any other phase it tracks is not plotted.
 */
export interface GatewayHistorySample {
  readonly running: number;
  readonly provisioning: number;
  readonly failed: number;
}

/**
 * One managed cluster's active-sandbox count within an instance. `cluster` is the
 * opaque server-reported cluster label; no cluster identity is baked in here.
 */
export interface SandboxClusterCount {
  readonly cluster: string;
  readonly count: number;
}

/** A rate + error% + p95-latency triple, as the BFF reports per control-plane. */
export interface RateStats {
  readonly rate: number;
  readonly errorPct: number;
  readonly p95Ms: number;
}

/** A zeroed RateStats - the honest default when a metric sub-query yields nothing. */
export const ZERO_RATE: RateStats = { rate: 0, errorPct: 0, p95Ms: 0 };

export interface InstanceFleet {
  readonly instance: string;
  /** hub | spoke - from delivery labels at runtime, never assumed. Null here. */
  readonly role: string | null;
  readonly provider: string | null;
  readonly gateways: GatewayPhaseCounts;
  /** Total gateways across phases, as the server sums them. */
  readonly gatewaysTotal: number;
  readonly managedClusters: number | null;
  readonly users: number | null;
  /** api-server (gRPC) RED metrics. */
  readonly rpc: RateStats;
  /** controller reconcile-loop RED metrics. */
  readonly reconcile: RateStats;
  /** web-console BFF (OTLP span) RED metrics. */
  readonly bff: RateStats;
  /** p95 gateway-provision latency in ms, or null when unknown. */
  readonly provisionP95Ms: number | null;
  /**
   * Per-phase gateway samples oldest -> newest, for the stacked "sand" sparkline.
   * Empty when the server reports no history (the UI degrades to an absent spark).
   */
  readonly gatewayHistory: readonly GatewayHistorySample[];
  /** Total active agent sandboxes across the instance's gateways. */
  readonly sandboxes: number;
  /**
   * Active sandboxes broken down per managed cluster, busiest-first as the server
   * orders them. Empty when the server reports none (the UI degrades gracefully).
   */
  readonly sandboxesByCluster: readonly SandboxClusterCount[];
  /**
   * Total active-sandbox count over the last day, oldest-first, on the SAME grid as
   * {@link gatewayHistory} so the two node-card sparklines share an x-axis. Drives the
   * lower sandbox "sand" sparkline. May be empty when no history is available.
   */
  readonly sandboxHistory: readonly number[];
}

export interface FleetData {
  readonly instances: readonly InstanceFleet[];
}

export function totalGateways(counts: GatewayPhaseCounts): number {
  return Object.values(counts).reduce((sum, n) => sum + n, 0);
}

/** Case-insensitive lookup of a phase count (phases vary by controller version). */
export function phaseCount(counts: GatewayPhaseCounts, phase: string): number {
  const wanted = phase.toLowerCase();
  for (const [key, value] of Object.entries(counts)) {
    if (key.toLowerCase() === wanted) {
      return value;
    }
  }
  return 0;
}

/**
 * Gateways in a phase outside the three the UI buckets by name
 * (running/provisioning/failed): the server's total minus those three, floored at
 * zero. Controllers report other phases (e.g. deleting/pending/unknown); folding
 * them into one "other" bucket keeps the donut ring + legend summing to the total
 * instead of silently dropping them (which made the centre count exceed the rows).
 */
export function otherGateways(counts: GatewayPhaseCounts): number {
  const known =
    phaseCount(counts, "running") +
    phaseCount(counts, "provisioning") +
    phaseCount(counts, "failed");
  return Math.max(0, totalGateways(counts) - known);
}

/**
 * Coarse health of a gateway population, from phase counts alone: any failed is
 * danger, any provisioning is warning, otherwise success; no gateways at all is
 * unknown. Mirrors the prototype's gwSummary kind.
 */
export function gatewayTone(
  counts: GatewayPhaseCounts,
): "success" | "warning" | "danger" | "unknown" {
  if (totalGateways(counts) === 0) {
    return "unknown";
  }
  if (phaseCount(counts, "failed") > 0) {
    return "danger";
  }
  if (phaseCount(counts, "provisioning") > 0) {
    return "warning";
  }
  return "success";
}

/** Look up one instance's fleet record by name, or null when absent. */
export function findInstance(
  instances: readonly InstanceFleet[],
  name: string,
): InstanceFleet | null {
  for (const i of instances) {
    if (i.instance === name) {
      return i;
    }
  }
  return null;
}

/** Instances sorted by role (hubs last, so spokes read first) then name. */
export function orderedInstances(
  instances: readonly InstanceFleet[],
): readonly InstanceFleet[] {
  return [...instances].sort((a, b) => {
    const aHub = a.role === "hub";
    const bHub = b.role === "hub";
    if (aHub !== bHub) {
      return aHub ? 1 : -1;
    }
    return a.instance.localeCompare(b.instance);
  });
}
