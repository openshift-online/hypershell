import {
  fetchMetrics,
  namespaceSelector,
  type MetricsSource,
} from "./metrics-source.js";

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
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewayPhaseCounts> {
  // API gauges use the scrape target's namespace label.
  const query = namespace
    ? `max by (phase) (hypershell_gateways_total${namespaceSelector(namespace)})`
    : "hypershell_gateways_total";

  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetchMetrics(
      prometheusUrl,
      query,
      controller.signal,
    );
    if (!response.ok) {
      throw new Error("Prometheus query request failed");
    }

    const body = (await response.json()) as PrometheusQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }

    const counts = emptyGatewayPhaseCounts();
    const samples = body.data?.result ?? [];
    if (namespace && samples.length === 0) {
      throw new Error("No gateway metrics for the configured namespace");
    }
    for (const sample of samples) {
      const phase = sample.metric.phase;
      if (phase === undefined || !isGatewayMetricPhase(phase)) {
        continue;
      }
      const value = Number(sample.value[1]);
      if (!Number.isFinite(value) || value < 0) {
        throw new Error("Prometheus query returned invalid sample");
      }
      counts[phase] = Math.max(counts[phase], Math.round(value));
    }
    return counts;
  } finally {
    clearTimeout(timeout);
  }
}
