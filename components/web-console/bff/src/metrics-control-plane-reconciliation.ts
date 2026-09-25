import { fetchMetrics, type MetricsSource } from "./metrics-source.js";
import {
  formatHourLabel,
  hourlyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  rollingHourlyRange,
} from "./prometheus-range-query.js";

export const reconciliationFailureCountPromql =
  "sum(increase(hypershell_reconciliation_failures_total[24h]))";
export const reconciliationFailureHourlyCountPromql =
  "sum(increase(hypershell_reconciliation_failures_total[1h]))";
export const reconciliationRetryCountPromql =
  "sum(increase(hypershell_reconciliation_retries_total[24h]))";
export const reconciliationRetryHourlyCountPromql =
  "sum(increase(hypershell_reconciliation_retries_total[1h]))";
export const reconciliationLagP50SecondsPromql =
  "histogram_quantile(0.50, sum(rate(hypershell_reconciliation_lag_seconds_bucket[5m])) by (le))";
export const staleResourceStatusCountPromql =
  "sum(max by (hypershell_cluster_id) (hypershell_stale_resource_status_count))";

interface PrometheusQueryResponse {
  status: string;
  data?: { result: { value: [string, string] }[] };
}

export interface ReconciliationHourlyPoint {
  hour: string;
  value: number;
}

export interface ControlPlaneReconciliationMetrics {
  reconciliation_failures_count: number;
  reconciliation_retries_count: number;
  reconciliation_lag_p50_seconds?: number;
  stale_resource_status_count: number;
  hourly_reconciliation_failures_count?: ReconciliationHourlyPoint[];
  hourly_reconciliation_retries_count?: ReconciliationHourlyPoint[];
  hourly_reconciliation_lag_p50_seconds?: ReconciliationHourlyPoint[];
  hourly_stale_resource_status_count?: ReconciliationHourlyPoint[];
}

async function queryInstant(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
  optional = false,
): Promise<number | undefined> {
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort();
  }, timeoutMs);
  try {
    const response = await fetchMetrics(source, query, controller.signal);
    if (!response.ok) throw new Error("Prometheus query failed");
    const body = (await response.json()) as PrometheusQueryResponse;
    const raw = body.data?.result[0]?.value[1];
    const value = Number(raw);
    if (body.status !== "success" || !Number.isFinite(value) || value < 0) {
      if (optional) return undefined;
      throw new Error("Prometheus query returned an invalid sample");
    }
    return value;
  } finally {
    clearTimeout(timeout);
  }
}

function mapHourly(samples: Map<number, number>): ReconciliationHourlyPoint[] {
  return [...samples.entries()]
    .sort(([left], [right]) => left - right)
    .filter(([, value]) => Number.isFinite(value) && value >= 0)
    .map(([timestamp, value]) => ({ hour: formatHourLabel(timestamp), value }));
}

async function optionalHourly(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<ReconciliationHourlyPoint[] | undefined> {
  try {
    const { start, end } = rollingHourlyRange();
    const samples = await queryPrometheusRangeScalarSamples(
      source,
      query,
      start,
      end,
      String(hourlyRangeStepSeconds),
      timeoutMs,
    );
    const result = mapHourly(samples);
    return result.length > 0 ? result : undefined;
  } catch {
    return undefined;
  }
}

export async function queryControlPlaneReconciliation(
  source: MetricsSource,
  timeoutMs: number,
): Promise<ControlPlaneReconciliationMetrics> {
  const [failures, retries, lag, stale] = await Promise.all([
    queryInstant(source, reconciliationFailureCountPromql, timeoutMs),
    queryInstant(source, reconciliationRetryCountPromql, timeoutMs),
    queryInstant(source, reconciliationLagP50SecondsPromql, timeoutMs, true),
    queryInstant(source, staleResourceStatusCountPromql, timeoutMs),
  ]);
  if (failures === undefined || retries === undefined || stale === undefined) {
    throw new Error("Required reconciliation metric is unavailable");
  }

  const [hourlyFailures, hourlyRetries, hourlyLag, hourlyStale] =
    await Promise.all([
      optionalHourly(source, reconciliationFailureHourlyCountPromql, timeoutMs),
      optionalHourly(source, reconciliationRetryHourlyCountPromql, timeoutMs),
      optionalHourly(source, reconciliationLagP50SecondsPromql, timeoutMs),
      optionalHourly(source, staleResourceStatusCountPromql, timeoutMs),
    ]);

  return {
    reconciliation_failures_count: failures,
    reconciliation_retries_count: retries,
    ...(lag === undefined ? {} : { reconciliation_lag_p50_seconds: lag }),
    stale_resource_status_count: stale,
    ...(hourlyFailures
      ? { hourly_reconciliation_failures_count: hourlyFailures }
      : {}),
    ...(hourlyRetries
      ? { hourly_reconciliation_retries_count: hourlyRetries }
      : {}),
    ...(hourlyLag ? { hourly_reconciliation_lag_p50_seconds: hourlyLag } : {}),
    ...(hourlyStale ? { hourly_stale_resource_status_count: hourlyStale } : {}),
  };
}
