import {
  gatewayCanonicalPhaseStrings,
  type GatewayCanonicalPhase,
} from "../gateways/gateway-data";

export type GatewayPhaseCounts = Record<GatewayCanonicalPhase, number>;

export interface GatewayMetricsDailyTotal {
  date: string;
  total: number;
}

export interface GatewayMetrics extends GatewayPhaseCounts {
  dailyFleetTotals?: GatewayMetricsDailyTotal[];
}

export const gatewayPhases = gatewayCanonicalPhaseStrings;

export const gatewayMetricsQueryKey = ["gateways", "metrics"] as const;

export function emptyGatewayPhaseCounts(): GatewayPhaseCounts {
  return {
    Pending: 0,
    Provisioning: 0,
    Running: 0,
    Degraded: 0,
    Failed: 0,
  };
}

export async function fetchGatewayMetrics(
  signal?: AbortSignal,
  timeRange: 7 | 30 = 7,
): Promise<GatewayMetrics> {
  const response = await fetch(
    timeRange === 30
      ? "/api/metrics/gateways?range=30"
      : "/api/metrics/gateways",
    {
      credentials: "same-origin",
      signal,
    },
  );
  if (!response.ok) {
    throw new Error(
      `Failed to fetch gateway metrics: ${String(response.status)}`,
    );
  }
  const body = (await response.json()) as {
    counts: Record<string, number>;
    daily_fleet_totals?: GatewayMetricsDailyTotal[];
  };
  const metrics = emptyGatewayPhaseCounts();
  for (const phase of gatewayPhases) {
    metrics[phase] = body.counts[phase] ?? 0;
  }
  return {
    ...metrics,
    ...(body.daily_fleet_totals === undefined
      ? {}
      : { dailyFleetTotals: body.daily_fleet_totals }),
  };
}
