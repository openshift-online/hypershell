import {
  fetchMetrics,
  namespaceSelector,
  type MetricsSource,
} from "./metrics-source.js";
import {
  alignDailyIntegerSeries,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  sevenDayUtcCalendarRange,
} from "./prometheus-range-query.js";

import {
  emptyGatewayPhaseCounts,
  gatewayCanonicalPhaseStrings,
  type GatewayCanonicalPhase,
} from "../../shared/gateway-phases.js";

export type GatewayPhaseCounts = Record<GatewayCanonicalPhase, number>;

export const gatewayPhases = gatewayCanonicalPhaseStrings;

export { emptyGatewayPhaseCounts };

export const gatewayFleetTotalDailyPromql = "sum(hypershell_gateways_total)";
export const gatewayFleetTotalDailyStepSeconds = dailyRangeStepSeconds;

export interface DailyFleetTotal {
  date: string;
  total: number;
}

export interface GatewayMetricsResponse {
  counts: GatewayPhaseCounts;
  daily_fleet_totals?: DailyFleetTotal[];
}

function gatewayFleetTotalDailyQuery(namespace?: string): string {
  if (!namespace) {
    return gatewayFleetTotalDailyPromql;
  }

  return `sum(hypershell_gateways_total${namespaceSelector(namespace)})`;
}

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
  // API gauges use the scrape target's namespace label. The api-server emits one
  // series per replica AND per managed_cluster (spoke), so dedupe replicas with an
  // inner `max by (phase, managed_cluster)` and then sum across spokes: a plain
  // `sum` would double-count replicas, while the old `max by (phase)` collapsed the
  // spokes into a single (largest) value once the managed_cluster label was added.
  const query = namespace
    ? `sum by (phase) (max by (phase, managed_cluster) (hypershell_gateways_total${namespaceSelector(namespace)}))`
    : "sum by (phase) (max by (phase, managed_cluster, namespace) (hypershell_gateways_total))";

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

async function queryGatewayDailyFleetTotals(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<DailyFleetTotal[]> {
  const { end, start } = sevenDayUtcCalendarRange();
  const samples = await queryPrometheusRangeScalarSamples(
    prometheusUrl,
    gatewayFleetTotalDailyQuery(namespace),
    start,
    end,
    `${String(gatewayFleetTotalDailyStepSeconds)}s`,
    timeoutMs,
  );

  return alignDailyIntegerSeries(samples).map(({ date, value }) => ({
    date,
    total: value,
  }));
}

export async function queryGatewayMetrics(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewayMetricsResponse> {
  const counts = await queryGatewayPhaseCounts(
    prometheusUrl,
    timeoutMs,
    namespace,
  );

  try {
    const daily_fleet_totals = await queryGatewayDailyFleetTotals(
      prometheusUrl,
      timeoutMs,
      namespace,
    );
    return { counts, daily_fleet_totals };
  } catch {
    return { counts };
  }
}
