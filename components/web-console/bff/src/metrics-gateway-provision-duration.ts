import { fetchMetrics, type MetricsSource } from "./metrics-source.js";
import {
  dailyRangeStepSeconds,
  formatUtcCalendarDate,
  queryPrometheusRangeScalarSamples,
  utcCalendarRange,
} from "./prometheus-range-query.js";

export const gatewayProvisionDurationCountPromql =
  "gateway_provision_duration_seconds_count";
export const gatewayProvisionDurationMeanPromql =
  "gateway_provision_duration_seconds_sum / gateway_provision_duration_seconds_count";
export const gatewayProvisionDurationP50Promql =
  "histogram_quantile(0.50, sum(gateway_provision_duration_seconds_bucket) by (le))";
export const gatewayProvisionDurationP95Promql =
  "histogram_quantile(0.95, sum(gateway_provision_duration_seconds_bucket) by (le))";
export const gatewayProvisionDurationP99SevenDayPromql =
  "histogram_quantile(0.99, sum by (le) (increase(gateway_provision_duration_seconds_bucket[7d])))";
export const gatewayProvisionDurationP99DailyPromql =
  "histogram_quantile(0.99, sum by (le) (increase(gateway_provision_duration_seconds_bucket[1d])))";
export const gatewayProvisionDurationTargetSeconds = 300;

export interface GatewayProvisionDurationDailyP99 {
  date: string;
  p99_seconds: number;
}

export interface GatewayProvisionDurationSeconds {
  mean_seconds: number;
  observation_count: number;
  p50_seconds: number;
  p95_seconds: number;
  p99_seconds: number | null;
  target_seconds: number;
  daily_p99: GatewayProvisionDurationDailyP99[] | null;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

async function queryPrometheusInstantNumber(
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

export async function queryGatewayProvisionDuration(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
  rangeDays?: 7 | 30,
): Promise<GatewayProvisionDurationSeconds> {
  // OBS-SLI-04: the API server emits these series at fleet level with no
  // namespace or gateway identifier label, so queries are never namespace
  // scoped. Any namespace argument is ignored for these provision series.
  const window = rangeDays === undefined ? undefined : `${String(rangeDays)}d`;
  const count = window
    ? `sum(increase(gateway_provision_duration_seconds_count[${window}]))`
    : gatewayProvisionDurationCountPromql;
  const mean = window
    ? `sum(increase(gateway_provision_duration_seconds_sum[${window}])) / ${count}`
    : gatewayProvisionDurationMeanPromql;
  const p50 = window
    ? `histogram_quantile(0.50, sum by (le) (increase(gateway_provision_duration_seconds_bucket[${window}])))`
    : gatewayProvisionDurationP50Promql;
  const p95 = window
    ? `histogram_quantile(0.95, sum by (le) (increase(gateway_provision_duration_seconds_bucket[${window}])))`
    : gatewayProvisionDurationP95Promql;
  const observation_count = Math.round(
    await queryPrometheusInstantNumber(prometheusUrl, count, timeoutMs),
  );

  if (observation_count === 0) {
    throw new Error("No gateway provision duration observations");
  }

  const [mean_seconds, p50_seconds, p95_seconds] = await Promise.all([
    queryPrometheusInstantNumber(prometheusUrl, mean, timeoutMs),
    queryPrometheusInstantNumber(prometheusUrl, p50, timeoutMs),
    queryPrometheusInstantNumber(prometheusUrl, p95, timeoutMs),
  ]);

  let p99Seconds: number | undefined;
  let dailyP99: GatewayProvisionDurationDailyP99[] | undefined;
  if (rangeDays === 7) {
    p99Seconds = await queryPrometheusInstantNumber(
      prometheusUrl,
      gatewayProvisionDurationP99SevenDayPromql,
      timeoutMs,
    ).catch(() => undefined);

    try {
      const { start, end } = utcCalendarRange(7);
      const samples = await queryPrometheusRangeScalarSamples(
        prometheusUrl,
        gatewayProvisionDurationP99DailyPromql,
        start,
        end,
        String(dailyRangeStepSeconds),
        timeoutMs,
      );
      const points = [...samples.entries()]
        .sort(([left], [right]) => left - right)
        .map(([timestamp, value]) => ({
          date: formatUtcCalendarDate(timestamp),
          p99_seconds: value,
        }));
      dailyP99 = points.length > 0 ? points : undefined;
    } catch {
      dailyP99 = undefined;
    }
  }

  return {
    mean_seconds,
    observation_count,
    p50_seconds,
    p95_seconds,
    p99_seconds: p99Seconds ?? null,
    target_seconds: gatewayProvisionDurationTargetSeconds,
    daily_p99: dailyP99 ?? null,
  };
}
