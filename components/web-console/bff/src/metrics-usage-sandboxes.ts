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

export interface UsageSandboxMetricsResponse {
  active_sandboxes?: number;
  coverage_end?: string;
  coverage_start?: string;
  daily_utilization_percent?: UsageDailyPoint[];
  sandbox_utilization_percent?: number;
  total_sandboxes?: number;
}

export interface UsageSandboxMetricsOptions {
  activeSandboxes?: number;
  namespaces?: readonly string[];
}

const activeSandboxesMetric = "hypershell_gateways_active_sandboxes_total";
const totalSandboxesMetric = "gateway_sandbox_total";

export function activeSandboxesUsageQuery(
  namespaces?: readonly string[],
): string {
  return `sum(max by (managed_cluster, gateway_namespace) (${activeSandboxesMetric}${namespaceSetSelector(namespaces, "gateway_namespace")}))`;
}

export function totalSandboxesUsageQuery(
  namespaces?: readonly string[],
): string {
  return `sum(max by (hypershell_cluster_id, namespace) (${totalSandboxesMetric}${namespaceSetSelector(namespaces)}))`;
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

async function queryDailySamples(
  source: MetricsSource,
  query: string,
  rangeDays: UsageDashboardRange,
  timeoutMs: number,
): Promise<Map<string, number | null> | undefined> {
  const { end, start } = utcCalendarRange(rangeDays);
  try {
    const samples = await queryPrometheusRangeScalarSamples(
      source,
      query,
      start,
      end,
      `${String(dailyRangeStepSeconds)}s`,
      timeoutMs,
    );
    return new Map(
      alignDailyNumericSeriesPreservingGaps(samples, rangeDays).map(
        ({ date, value }) => [date, value],
      ),
    );
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

export async function queryUsageSandboxMetrics(
  source: MetricsSource,
  timeoutMs: number,
  rangeDays: UsageDashboardRange = 7,
  options: UsageSandboxMetricsOptions = {},
): Promise<UsageSandboxMetricsResponse> {
  const queryNamespaces = options.namespaces;

  const activePromise =
    options.activeSandboxes !== undefined
      ? Promise.resolve(options.activeSandboxes)
      : optionalInstantNumber(
          source,
          activeSandboxesUsageQuery(queryNamespaces),
          timeoutMs,
        );
  const [active, total, dailyActive, dailyTotal] = await Promise.all([
    activePromise,
    optionalInstantNumber(
      source,
      totalSandboxesUsageQuery(queryNamespaces),
      timeoutMs,
    ),
    queryDailySamples(
      source,
      activeSandboxesUsageQuery(queryNamespaces),
      rangeDays,
      timeoutMs,
    ),
    queryDailySamples(
      source,
      totalSandboxesUsageQuery(queryNamespaces),
      rangeDays,
      timeoutMs,
    ),
  ]);

  const response: UsageSandboxMetricsResponse = {};
  if (active !== undefined) {
    response.active_sandboxes = active;
  }
  if (total !== undefined) {
    response.total_sandboxes = total;
  }
  if (active !== undefined && total !== undefined) {
    response.sandbox_utilization_percent =
      total === 0 ? 0 : (active / total) * 100;
  }

  if (dailyActive !== undefined && dailyTotal !== undefined) {
    const points = [...dailyTotal.keys()]
      .sort()
      .map<UsageDailyPoint>((date) => {
        const activeValue = dailyActive.get(date);
        const totalValue = dailyTotal.get(date);
        if (
          activeValue === null ||
          activeValue === undefined ||
          totalValue === null ||
          totalValue === undefined
        ) {
          return { date, value: null };
        }
        return {
          date,
          value: totalValue === 0 ? 0 : (activeValue / totalValue) * 100,
        };
      });
    response.daily_utilization_percent = points;
    Object.assign(response, coverage(points));
  }

  return response;
}
