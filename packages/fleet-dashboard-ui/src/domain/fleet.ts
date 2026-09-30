// Fleet plane: per-instance observed state derived from metrics. No instance or
// cluster identity is baked in here - the instance list arrives from the server.

export type GatewayPhaseCounts = Readonly<Record<string, number>>;

export interface InstanceFleet {
  readonly instance: string;
  /** hub | spoke - from delivery labels at runtime, never assumed. */
  readonly role: string | null;
  readonly provider: string | null;
  readonly gateways: GatewayPhaseCounts;
  readonly managedClusters: number | null;
  readonly users: number | null;
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
