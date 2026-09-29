import { fetchMetrics, type MetricsSource } from "./metrics-source.js";
import {
  formatHourLabel,
  hourlyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  rollingHourlyRange,
} from "./prometheus-range-query.js";

/**
 * Framework Prometheus histogram scraped from the API server `/metrics`
 * endpoint (ServiceMonitor job `hypershell-api-server`). Kind does not export
 * OTel `http.server.request.duration` into Prometheus today (no API-server
 * OTEL_EXPORTER_OTLP_ENDPOINT), so the reliability dashboard uses this series.
 */
export const apiReliabilityMetricPrefix = "api_inbound_request_duration";

/** Prometheus job label for the API server scrape target. */
export const apiReliabilityJob = "hypershell-api-server";

/** HTTP status code label on `api_inbound_request_duration_*`. */
export const apiReliabilityStatusCodeLabel = "code";

export const apiReliabilityRateWindow = "5m";

const seriesSelector = `job="${apiReliabilityJob}"`;

export const apiRequestRatePromql = `sum(rate(${apiReliabilityMetricPrefix}_count{${seriesSelector}}[${apiReliabilityRateWindow}]))`;

/**
 * 5xx share of requests. Missing 5xx series become 0 via `or vector(0)`.
 * `clamp_min` on the denominator avoids NaN when recent request rate is 0
 * (low-traffic Kind / idle fleets) while still returning 0% errors.
 */
export const apiErrorRatePercentPromql = `100 * (sum(rate(${apiReliabilityMetricPrefix}_count{${seriesSelector},${apiReliabilityStatusCodeLabel}=~"5.."}[${apiReliabilityRateWindow}])) or vector(0)) / clamp_min(sum(rate(${apiReliabilityMetricPrefix}_count{${seriesSelector}}[${apiReliabilityRateWindow}])), 1e-12)`;

/** Rate-based P50; may be NaN when the rate window has no observations. */
export const apiLatencyP50SecondsPromql = `histogram_quantile(0.50, sum(rate(${apiReliabilityMetricPrefix}_bucket{${seriesSelector}}[${apiReliabilityRateWindow}])) by (le))`;

/**
 * Cumulative P50 fallback when the rate-based quantile is non-finite
 * (common right after rollout or with sparse traffic).
 */
export const apiLatencyP50SecondsCumulativePromql = `histogram_quantile(0.50, sum(${apiReliabilityMetricPrefix}_bucket{${seriesSelector}}) by (le))`;

export interface ApiReliabilityHourlyPoint {
  hour: string;
  value: number;
}

export interface ApiReliabilityMetrics {
  error_rate_percent: number;
  hourly_error_rate_percent?: ApiReliabilityHourlyPoint[];
  hourly_latency_p50_seconds?: ApiReliabilityHourlyPoint[];
  hourly_request_rate?: ApiReliabilityHourlyPoint[];
  latency_p50_seconds: number;
  request_rate: number;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

async function queryPrometheusInstantFloat(
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

    const rawValue = samples[0]?.value[1];
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

async function queryLatencyP50Seconds(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
): Promise<number> {
  try {
    return await queryPrometheusInstantFloat(
      prometheusUrl,
      apiLatencyP50SecondsPromql,
      timeoutMs,
    );
  } catch {
    return queryPrometheusInstantFloat(
      prometheusUrl,
      apiLatencyP50SecondsCumulativePromql,
      timeoutMs,
    );
  }
}

function mapHourlyFloatSeries(
  samples: Map<number, number>,
): ApiReliabilityHourlyPoint[] {
  const hourly: ApiReliabilityHourlyPoint[] = [];

  for (const [timestamp, value] of [...samples.entries()].sort(
    (left, right) => left[0] - right[0],
  )) {
    if (!Number.isFinite(value) || value < 0) {
      continue;
    }

    hourly.push({
      hour: formatHourLabel(timestamp),
      value,
    });
  }

  return hourly;
}

async function queryOptionalHourlySeries(
  prometheusUrl: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<ApiReliabilityHourlyPoint[] | undefined> {
  try {
    const { end, start } = rollingHourlyRange();
    const samples = await queryPrometheusRangeScalarSamples(
      prometheusUrl,
      query,
      start,
      end,
      String(hourlyRangeStepSeconds),
      timeoutMs,
    );
    const hourly = mapHourlyFloatSeries(samples);
    return hourly.length > 0 ? hourly : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Hourly P50 uses the rate-based quantile first (same shape as ARM-01). When
 * that range query fails or yields no finite samples (sparse traffic), fall
 * back to the cumulative histogram series - matching instant latency.
 */
async function queryOptionalHourlyLatencySeries(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
): Promise<ApiReliabilityHourlyPoint[] | undefined> {
  const rateBased = await queryOptionalHourlySeries(
    prometheusUrl,
    apiLatencyP50SecondsPromql,
    timeoutMs,
  );
  if (rateBased !== undefined) {
    return rateBased;
  }

  return queryOptionalHourlySeries(
    prometheusUrl,
    apiLatencyP50SecondsCumulativePromql,
    timeoutMs,
  );
}

export async function queryApiReliability(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
): Promise<ApiReliabilityMetrics> {
  const [requestRate, errorRatePercent, latencyP50Seconds] = await Promise.all([
    queryPrometheusInstantFloat(prometheusUrl, apiRequestRatePromql, timeoutMs),
    queryPrometheusInstantFloat(
      prometheusUrl,
      apiErrorRatePercentPromql,
      timeoutMs,
    ),
    queryLatencyP50Seconds(prometheusUrl, timeoutMs),
  ]);

  const [hourlyRequestRate, hourlyErrorRate, hourlyLatency] = await Promise.all(
    [
      queryOptionalHourlySeries(prometheusUrl, apiRequestRatePromql, timeoutMs),
      queryOptionalHourlySeries(
        prometheusUrl,
        apiErrorRatePercentPromql,
        timeoutMs,
      ),
      queryOptionalHourlyLatencySeries(prometheusUrl, timeoutMs),
    ],
  );

  return {
    error_rate_percent: errorRatePercent,
    latency_p50_seconds: latencyP50Seconds,
    request_rate: requestRate,
    ...(hourlyRequestRate === undefined
      ? {}
      : { hourly_request_rate: hourlyRequestRate }),
    ...(hourlyErrorRate === undefined
      ? {}
      : { hourly_error_rate_percent: hourlyErrorRate }),
    ...(hourlyLatency === undefined
      ? {}
      : { hourly_latency_p50_seconds: hourlyLatency }),
  };
}
