import { queryPrometheusInstantNumber } from "./prometheus-instant-query.js";
import {
  alignDailyNumericSeriesPreservingGaps,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  utcCalendarRange,
} from "./prometheus-range-query.js";
import { namespaceSetSelector, type MetricsSource } from "./metrics-source.js";

import type {
  UsageDailyPoint,
  UsageDashboardRange,
} from "./metrics-usage-gateways.js";

export interface UsageSandboxLifetimeResponse {
  coverage_end?: string;
  coverage_start?: string;
  daily_mean_seconds?: UsageDailyPoint[];
  mean_seconds?: number;
  p50_seconds?: number;
  p95_seconds?: number;
}

const lifetimeMetric = "gateway_sandbox_lifetime_seconds";

function rangeSelector(rangeDays: UsageDashboardRange): string {
  return `[${String(rangeDays)}d]`;
}

function lifetimeSelector(
  suffix: string,
  namespaces?: readonly string[],
): string {
  return `${lifetimeMetric}${suffix}${namespaceSetSelector(namespaces)}`;
}

export function sandboxLifetimeMeanQuery(
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `sum(increase(${lifetimeSelector("_sum", namespaces)}${rangeSelector(rangeDays)})) / sum(increase(${lifetimeSelector("_count", namespaces)}${rangeSelector(rangeDays)}))`;
}

export function sandboxLifetimeQuantileQuery(
  quantile: 0.5 | 0.95,
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `histogram_quantile(${String(quantile)}, sum by (le) (increase(${lifetimeSelector("_bucket", namespaces)}${rangeSelector(rangeDays)})))`;
}

function dailyQuery(query: string): string {
  return query.replace(/\[(?:7|30)d\]/gu, "[1d]");
}

async function optionalInstantNumber(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number | undefined> {
  try {
    return await queryPrometheusInstantNumber(source, query, timeoutMs);
  } catch {
    return undefined;
  }
}

function coverage(points: readonly UsageDailyPoint[]): {
  coverage_end?: string;
  coverage_start?: string;
} {
  const available = points.filter((point) => point.value !== null);
  if (available.length === 0) {
    return {};
  }

  return {
    coverage_end: available.at(-1)?.date,
    coverage_start: available[0]?.date,
  };
}

export async function queryUsageSandboxLifetime(
  source: MetricsSource,
  timeoutMs: number,
  rangeDays: UsageDashboardRange = 7,
  namespaces?: readonly string[],
): Promise<UsageSandboxLifetimeResponse> {
  const [mean, p50, p95, dailyMean] = await Promise.all([
    optionalInstantNumber(
      source,
      sandboxLifetimeMeanQuery(rangeDays, namespaces),
      timeoutMs,
    ),
    optionalInstantNumber(
      source,
      sandboxLifetimeQuantileQuery(0.5, rangeDays, namespaces),
      timeoutMs,
    ),
    optionalInstantNumber(
      source,
      sandboxLifetimeQuantileQuery(0.95, rangeDays, namespaces),
      timeoutMs,
    ),
    (async () => {
      const { end, start } = utcCalendarRange(rangeDays);
      try {
        const samples = await queryPrometheusRangeScalarSamples(
          source,
          dailyQuery(sandboxLifetimeMeanQuery(rangeDays, namespaces)),
          start,
          end,
          `${String(dailyRangeStepSeconds)}s`,
          timeoutMs,
        );
        return alignDailyNumericSeriesPreservingGaps(samples, rangeDays);
      } catch {
        return undefined;
      }
    })(),
  ]);

  const response: UsageSandboxLifetimeResponse = {};
  if (mean !== undefined) {
    response.mean_seconds = mean;
  }
  if (p50 !== undefined) {
    response.p50_seconds = p50;
  }
  if (p95 !== undefined) {
    response.p95_seconds = p95;
  }
  if (dailyMean !== undefined) {
    response.daily_mean_seconds = dailyMean;
    Object.assign(response, coverage(dailyMean));
  }

  return response;
}
