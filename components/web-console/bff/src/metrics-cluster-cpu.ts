import { fetchMetrics, type MetricsSource } from "./metrics-source.js";
import {
  alignDailyIntegerSeries,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  sevenDayUtcCalendarRange,
} from "./prometheus-range-query.js";

export const clusterCpuCapacityPromql =
  'sum(count by (instance) (node_cpu_seconds_total{mode="idle"}))';
export const clusterCpuUsedPromql =
  'sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))';
export const clusterCpuDailyUsedPromql = clusterCpuUsedPromql;
export const clusterCpuDailyUsedStepSeconds = dailyRangeStepSeconds;

const usedExceedsCapacityToleranceCores = 0.01;

export interface ClusterCpuDailyUsed {
  date: string;
  value: number;
}

export interface ClusterCpuCores {
  available_cores: number;
  capacity_cores: number;
  daily_used?: ClusterCpuDailyUsed[];
  used_cores: number;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

async function queryPrometheusInstant(
  prometheusUrl: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number> {
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

    const samples = body.data?.result ?? [];
    if (samples.length === 0) {
      throw new Error("Prometheus query returned no samples");
    }

    const sample = samples[0];
    const rawValue = sample?.value[1];
    if (rawValue === undefined) {
      throw new Error("Prometheus query returned invalid sample");
    }

    const value = Number(rawValue);
    if (!Number.isFinite(value) || value < 0) {
      throw new Error("Prometheus query returned invalid sample");
    }

    return value;
  } finally {
    clearTimeout(timeout);
  }
}

export async function queryClusterCpu(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
): Promise<ClusterCpuCores> {
  const [capacity_cores, used_cores] = await Promise.all([
    queryPrometheusInstant(prometheusUrl, clusterCpuCapacityPromql, timeoutMs),
    queryPrometheusInstant(prometheusUrl, clusterCpuUsedPromql, timeoutMs),
  ]);

  if (capacity_cores === 0) {
    throw new Error("No cluster CPU capacity data");
  }

  if (used_cores > capacity_cores + usedExceedsCapacityToleranceCores) {
    throw new Error("Inconsistent cluster CPU samples");
  }

  const available_cores = capacity_cores - used_cores;

  const response: ClusterCpuCores = {
    available_cores,
    capacity_cores,
    used_cores,
  };

  try {
    const { end, start } = sevenDayUtcCalendarRange();
    const samples = await queryPrometheusRangeScalarSamples(
      prometheusUrl,
      clusterCpuDailyUsedPromql,
      start,
      end,
      `${String(clusterCpuDailyUsedStepSeconds)}s`,
      timeoutMs,
    );
    response.daily_used = alignDailyIntegerSeries(samples).map(
      ({ date, value }) => ({
        date,
        value: Math.round(value),
      }),
    );
  } catch {
    // Omit trend only.
  }

  return response;
}
