import {
  queryPrometheusInstantLabels,
  queryPrometheusInstantNumberAllowAbsent,
} from "./prometheus-instant-query.js";
import {
  alignDailyNumericSeriesPreservingGaps,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  utcCalendarRange,
} from "./prometheus-range-query.js";
import { namespaceSetMatcher, type MetricsSource } from "./metrics-source.js";

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
  const namespaceFilter = namespaceSetMatcher(namespaces);
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
  return `sum(increase(${requestSelector(grpcMetric, namespaces, ['method!~"Peer.*"', 'code!="0"'])}${rangeSelector(rangeDays)}))`;
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

async function optionalInstantCounterNumber(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<OptionalInstantCounterResult> {
  try {
    const value = await queryPrometheusInstantNumberAllowAbsent(
      source,
      query,
      timeoutMs,
    );
    return value === undefined
      ? { status: "absent" }
      : { status: "success", value };
  } catch {
    return { status: "failed" };
  }
}

type OptionalInstantCounterResult =
  | { status: "absent" }
  | { status: "failed" }
  | { status: "success"; value: number };

function optionalCounterValue(
  result: OptionalInstantCounterResult,
): number | undefined {
  if (result.status === "success") return result.value;
  if (result.status === "absent") return 0;
  return undefined;
}

type DailySamplesResult =
  | { status: "failed" }
  | { status: "success"; samples: Map<string, number | null> };

async function queryOptionalDailySamples(
  source: MetricsSource,
  query: string,
  rangeDays: UsageDashboardRange,
  timeoutMs: number,
): Promise<DailySamplesResult> {
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
    return {
      status: "success",
      samples: new Map(
        alignDailyNumericSeriesPreservingGaps(samples, rangeDays).map(
          ({ date, value }) => [date, value],
        ),
      ),
    };
  } catch {
    return { status: "failed" };
  }
}

function combineDailySeries(
  left: DailySamplesResult,
  right: DailySamplesResult,
): UsageDailyPoint[] | undefined {
  if (left.status === "failed" || right.status === "failed") {
    return undefined;
  }

  const dates = new Set([...left.samples.keys(), ...right.samples.keys()]);
  return [...dates].sort().map((date) => {
    const leftValue = left.samples.get(date);
    const rightValue = right.samples.get(date);
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
  left: DailySamplesResult,
  right: DailySamplesResult,
): UsageDailyPoint[] | undefined {
  if (left.status === "failed" || right.status === "failed") {
    return undefined;
  }

  const dates = new Set([...left.samples.keys(), ...right.samples.keys()]);
  return [...dates].sort().map((date) => ({
    date,
    value: (left.samples.get(date) ?? 0) + (right.samples.get(date) ?? 0),
  }));
}

function hasDailySamples(points: readonly UsageDailyPoint[]): boolean {
  return points.some((point) => point.value !== null);
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
    optionalInstantCounterNumber(
      source,
      grpcRequestVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantCounterNumber(
      source,
      httpRequestVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantCounterNumber(
      source,
      grpcErrorVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
    optionalInstantCounterNumber(
      source,
      httpErrorVolumeQuery(rangeDays, options.namespaces),
      timeoutMs,
    ),
  ]);

  const response: UsageGatewayMetricsResponse = {};
  const volumeQueriesSucceeded =
    grpcVolume.status !== "failed" && httpVolume.status !== "failed";
  const hasRequestTelemetry =
    grpcVolume.status === "success" || httpVolume.status === "success";

  if (volumeQueriesSucceeded && hasRequestTelemetry) {
    const grpcRequestVolume = optionalCounterValue(grpcVolume) ?? 0;
    const httpRequestVolume = optionalCounterValue(httpVolume) ?? 0;
    response.request_volume = grpcRequestVolume + httpRequestVolume;
  }
  const totalRequests = response.request_volume;
  if (
    totalRequests !== undefined &&
    totalRequests > 0 &&
    grpcErrors.status !== "failed" &&
    httpErrors.status !== "failed"
  ) {
    const errors =
      (optionalCounterValue(grpcErrors) ?? 0) +
      (optionalCounterValue(httpErrors) ?? 0);
    response.request_error_rate_percent = (errors / totalRequests) * 100;
  }

  if (currentGatewayCount !== undefined) {
    response.total_gateways = currentGatewayCount;
  }

  const currentNamespaces = options.currentGatewayNamespaces;

  if (
    currentNamespaces !== undefined &&
    volumeQueriesSucceeded &&
    hasRequestTelemetry
  ) {
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
    if (successful.length === activeResults.length) {
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
  const hasDailyRequestSamples =
    dailyVolume !== undefined && hasDailySamples(dailyVolume);
  if (hasDailyRequestSamples) {
    response.daily_request_volume = dailyVolume;
    Object.assign(response, coverage(dailyVolume));
  }

  const dailyErrors = combineDailyErrorSeries(dailyGrpcErrors, dailyHttpErrors);
  if (hasDailyRequestSamples && dailyErrors !== undefined) {
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
