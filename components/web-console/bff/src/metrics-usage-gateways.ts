import {
  queryPrometheusInstantLabels,
  queryPrometheusInstantNumber,
  queryPrometheusInstantNumberAllowAbsent,
} from "./prometheus-instant-query.js";
import {
  alignDailyNumericSeriesPreservingGaps,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  utcCalendarRange,
} from "./prometheus-range-query.js";
import { namespaceSetSelector, type MetricsSource } from "./metrics-source.js";

export type UsageDashboardRange = 7 | 30;

export interface UsageDailyPoint {
  date: string;
  value: number | null;
}

export interface UsageGatewayMetricsResponse {
  active_gateways?: number;
  coverage_end?: string;
  coverage_start?: string;
  daily_request_error_rate_percent?: UsageDailyPoint[];
  daily_request_volume?: UsageDailyPoint[];
  request_error_rate_percent?: number;
  request_volume?: number;
  total_gateways?: number;
  unused_gateways?: number;
}

export interface UsageGatewayMetricsOptions {
  currentGatewayCount?: number;
  currentGatewayNamespaces?: readonly string[];
  namespaces?: readonly string[];
}

const grpcMetric = "openshell_server_grpc_requests_total";
const httpMetric = "openshell_server_http_requests_total";

function rangeSelector(rangeDays: UsageDashboardRange): string {
  return `[${String(rangeDays)}d]`;
}

function requestSelector(
  metric: string,
  namespaces: readonly string[] | undefined,
  extraFilters: readonly string[] = [],
): string {
  const namespaceFilter = namespaceSetSelector(namespaces);
  const filters = [...extraFilters, namespaceFilter].filter(Boolean).join(",");
  return `${metric}${filters ? `{${filters}}` : ""}`;
}

export function grpcRequestVolumeQuery(
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `sum(increase(${requestSelector(grpcMetric, namespaces, ['method!~"Peer.*"'])}${rangeSelector(rangeDays)}))`;
}

export function httpRequestVolumeQuery(
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `sum(increase(${requestSelector(httpMetric, namespaces)}${rangeSelector(rangeDays)}))`;
}

export function grpcErrorVolumeQuery(
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `sum(increase(${requestSelector(grpcMetric, namespaces, ['method!~"Peer.*"', 'code!="OK"'])}${rangeSelector(rangeDays)}))`;
}

export function httpErrorVolumeQuery(
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  return `sum(increase(${requestSelector(httpMetric, namespaces, ['status=~"5.."'])}${rangeSelector(rangeDays)}))`;
}

export function activeGatewayNamespacesQuery(
  metric: "grpc" | "http",
  rangeDays: UsageDashboardRange,
  namespaces?: readonly string[],
): string {
  const selectedMetric =
    metric === "grpc"
      ? requestSelector(grpcMetric, namespaces, ['method!~"Peer.*"'])
      : requestSelector(httpMetric, namespaces);
  return `count by (namespace) (increase(${selectedMetric}${rangeSelector(rangeDays)}) > 0)`;
}

function dailyQuery(query: string): string {
  const range = /\[(7|30)d\]/u.exec(query);
  return query.replace(`[${range?.[1] ?? "7"}d]`, "[1d]");
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

async function optionalInstantErrorNumber(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number | undefined> {
  try {
    return (
      (await queryPrometheusInstantNumberAllowAbsent(
        source,
        query,
        timeoutMs,
      )) ?? 0
    );
  } catch {
    return undefined;
  }
}

async function queryOptionalDailySamples(
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

function combineDailySeries(
  left: Map<string, number | null> | undefined,
  right: Map<string, number | null> | undefined,
): UsageDailyPoint[] | undefined {
  if (left === undefined && right === undefined) {
    return undefined;
  }

  const dates = new Set([...(left?.keys() ?? []), ...(right?.keys() ?? [])]);
  return [...dates].sort().map((date) => {
    const leftValue = left?.get(date);
    const rightValue = right?.get(date);
    if (leftValue === null && rightValue === null) {
      return { date, value: null };
    }
    if (leftValue === undefined && rightValue === null) {
      return { date, value: null };
    }
    if (rightValue === undefined && leftValue === null) {
      return { date, value: null };
    }
    return {
      date,
      value: (leftValue ?? 0) + (rightValue ?? 0),
    };
  });
}

function combineDailyErrorSeries(
  left: Map<string, number | null> | undefined,
  right: Map<string, number | null> | undefined,
): UsageDailyPoint[] | undefined {
  if (left === undefined || right === undefined) {
    return undefined;
  }

  const dates = new Set([...left.keys(), ...right.keys()]);
  return [...dates].sort().map((date) => ({
    date,
    value: (left.get(date) ?? 0) + (right.get(date) ?? 0),
  }));
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

export async function queryUsageGatewayMetrics(
  source: MetricsSource,
  timeoutMs: number,
  rangeDays: UsageDashboardRange = 7,
  options: UsageGatewayMetricsOptions = {},
): Promise<UsageGatewayMetricsResponse> {
  const currentGatewayCount =
    options.currentGatewayCount ??
    (options.currentGatewayNamespaces
      ? options.currentGatewayNamespaces.length
      : undefined);

  const [grpcVolume, httpVolume, grpcErrors, httpErrors] = await Promise.all([
    optionalInstantNumber(
      source,
      grpcRequestVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantNumber(
      source,
      httpRequestVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantErrorNumber(
      source,
      grpcErrorVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantErrorNumber(
      source,
      httpErrorVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
  ]);

  const response: UsageGatewayMetricsResponse = {};
  if (
    grpcVolume !== undefined ||
    httpVolume !== undefined ||
    currentGatewayCount === 0
  ) {
    response.request_volume = (grpcVolume ?? 0) + (httpVolume ?? 0);
  }
  const totalRequests = response.request_volume;
  if (
    totalRequests !== undefined &&
    totalRequests > 0 &&
    grpcErrors !== undefined &&
    httpErrors !== undefined
  ) {
    const errors = grpcErrors + httpErrors;
    response.request_error_rate_percent = (errors / totalRequests) * 100;
  }

  if (currentGatewayCount !== undefined) {
    response.total_gateways = currentGatewayCount;
  }

  const currentNamespaces = options.currentGatewayNamespaces;

  if (currentNamespaces !== undefined) {
    const activeResults = await Promise.allSettled([
      queryPrometheusInstantLabels(
        source,
        activeGatewayNamespacesQuery("grpc", rangeDays, options.namespaces),
        timeoutMs,
        "namespace",
      ),
      queryPrometheusInstantLabels(
        source,
        activeGatewayNamespacesQuery("http", rangeDays, options.namespaces),
        timeoutMs,
        "namespace",
      ),
    ]);
    const successful = activeResults.flatMap((result) =>
      result.status === "fulfilled" ? [result.value] : [],
    );
    if (successful.length > 0) {
      const activeNamespaces = new Set<string>();
      for (const labels of successful) {
        for (const namespace of labels) {
          if (currentNamespaces.includes(namespace)) {
            activeNamespaces.add(namespace);
          }
        }
      }
      const activeGateways = activeNamespaces.size;
      response.active_gateways = activeGateways;
      response.unused_gateways =
        (response.total_gateways ?? currentNamespaces.length) - activeGateways;
    }
  }

  const [dailyGrpcVolume, dailyHttpVolume, dailyGrpcErrors, dailyHttpErrors] =
    await Promise.all([
      queryOptionalDailySamples(
        source,
        dailyQuery(grpcRequestVolumeQuery(rangeDays, options.namespaces)),
        rangeDays,
        timeoutMs,
      ),
      queryOptionalDailySamples(
        source,
        dailyQuery(httpRequestVolumeQuery(rangeDays, options.namespaces)),
        rangeDays,
        timeoutMs,
      ),
      queryOptionalDailySamples(
        source,
        dailyQuery(grpcErrorVolumeQuery(rangeDays, options.namespaces)),
        rangeDays,
        timeoutMs,
      ),
      queryOptionalDailySamples(
        source,
        dailyQuery(httpErrorVolumeQuery(rangeDays, options.namespaces)),
        rangeDays,
        timeoutMs,
      ),
    ]);

  const dailyVolume = combineDailySeries(dailyGrpcVolume, dailyHttpVolume);
  if (dailyVolume !== undefined) {
    response.daily_request_volume = dailyVolume;
    Object.assign(response, coverage(dailyVolume));
  }

  const dailyErrors = combineDailyErrorSeries(dailyGrpcErrors, dailyHttpErrors);
  if (dailyVolume !== undefined && dailyErrors !== undefined) {
    const totalByDate = new Map(
      dailyVolume.map(({ date, value }) => [date, value]),
    );
    response.daily_request_error_rate_percent = dailyErrors.map(
      ({ date, value }) => {
        const total = totalByDate.get(date);
        if (
          total === null ||
          total === undefined ||
          total === 0 ||
          value === null
        ) {
          return { date, value: null };
        }
        return { date, value: (value / total) * 100 };
      },
    );
  }

  return response;
}
