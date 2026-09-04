import {
  emptyGatewayPhaseCounts,
  gatewayCanonicalPhaseStrings,
  type GatewayCanonicalPhase,
} from "../../shared/gateway-phases.js";

export type GatewayPhaseCounts = Record<GatewayCanonicalPhase, number>;

export const gatewayPhases = gatewayCanonicalPhaseStrings;

export { emptyGatewayPhaseCounts };

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      metric: { phase?: string };
      value: [string, string];
    }[];
  };
}

function isGatewayMetricPhase(
  value: string,
): value is keyof GatewayPhaseCounts {
  return (gatewayPhases as readonly string[]).includes(value);
}

export async function queryGatewayPhaseCounts(
  prometheusUrl: string,
  timeoutMs: number,
): Promise<GatewayPhaseCounts> {
  const queryUrl = new URL("/api/v1/query", prometheusUrl);
  queryUrl.searchParams.set("query", "hypershell_gateways_total");

  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetch(queryUrl, { signal: controller.signal });
    if (!response.ok) {
      throw new Error("Prometheus query request failed");
    }

    const body = (await response.json()) as PrometheusQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }

    const counts = emptyGatewayPhaseCounts();
    for (const sample of body.data?.result ?? []) {
      const phase = sample.metric.phase;
      if (phase === undefined || !isGatewayMetricPhase(phase)) {
        continue;
      }
      counts[phase] = Math.round(Number(sample.value[1]));
    }
    return counts;
  } finally {
    clearTimeout(timeout);
  }
}
