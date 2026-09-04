import {
  gatewayCanonicalPhaseStrings,
  type GatewayCanonicalPhase,
} from "../gateways/gateway-data";

export type GatewayPhaseCounts = Record<GatewayCanonicalPhase, number>;

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
): Promise<GatewayPhaseCounts> {
  const response = await fetch("/api/hypershell/v1/metrics/gateways", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch gateway metrics: ${String(response.status)}`,
    );
  }
  const body = (await response.json()) as { counts: Record<string, number> };
  const counts = emptyGatewayPhaseCounts();
  for (const phase of gatewayPhases) {
    counts[phase] = body.counts[phase] ?? 0;
  }
  return counts;
}
