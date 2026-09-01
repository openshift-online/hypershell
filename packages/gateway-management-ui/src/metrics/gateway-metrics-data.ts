export interface GatewayPhaseCounts {
  Running: number;
  Provisioning: number;
  Degraded: number;
  Failed: number;
}

export const gatewayPhases = [
  "Running",
  "Provisioning",
  "Degraded",
  "Failed",
] as const satisfies readonly (keyof GatewayPhaseCounts)[];

export const gatewayMetricsQueryKey = ["gateways", "metrics"] as const;

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
  return {
    Running: body.counts.Running ?? 0,
    Provisioning: body.counts.Provisioning ?? 0,
    Degraded: body.counts.Degraded ?? 0,
    Failed: body.counts.Failed ?? 0,
  };
}
