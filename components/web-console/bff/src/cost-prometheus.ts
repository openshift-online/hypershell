import {
  fetchMetrics,
  fetchMetricsRange,
  type MetricsSource,
} from "./metrics-source.js";
import { formatUtcCalendarDate } from "./prometheus-range-query.js";

/**
 * Small Prometheus helpers for the cost dashboard. Unlike the usage helpers,
 * cost needs labeled, unrounded values (core-hours and GiB-hours are priced in
 * fractions of a cent per hour).
 */

export interface CostSample {
  labels: Record<string, string>;
  value: number;
}

export interface CostDailySeries {
  labels: Record<string, string>;
  /** Value per UTC calendar date. Days without a sample are absent. */
  values: Map<string, number>;
}

interface InstantBody {
  status: string;
  data?: {
    result: { metric: Record<string, string>; value: [number, string] }[];
  };
}

interface RangeBody {
  status: string;
  data?: {
    result: { metric: Record<string, string>; values: [number, string][] }[];
  };
}

function parseValue(raw: string | undefined): number {
  const value = Number(raw);
  if (raw === undefined || !Number.isFinite(value) || value < 0) {
    throw new Error("Prometheus query returned invalid sample");
  }
  return value;
}

async function withTimeout<T>(
  timeoutMs: number,
  run: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort(new Error("Prometheus query timed out"));
  }, timeoutMs);
  try {
    return await run(controller.signal);
  } finally {
    clearTimeout(timeout);
  }
}

export async function queryCostVector(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<CostSample[]> {
  return withTimeout(timeoutMs, async (signal) => {
    const response = await fetchMetrics(source, query, signal);
    if (!response.ok) throw new Error("Prometheus query request failed");
    const body = (await response.json()) as InstantBody;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }
    return (body.data?.result ?? []).map((sample) => ({
      labels: sample.metric,
      value: parseValue(sample.value[1]),
    }));
  });
}

/**
 * Evaluate a per-day expression (one that already aggregates the previous 24
 * hours, such as `sum_over_time(x[1d:1h])`) once per UTC day. The sample taken
 * at the end of a day is reported under that day's date.
 */
export async function queryCostDaily(
  source: MetricsSource,
  query: string,
  rangeStart: number,
  days: number,
  timeoutMs: number,
): Promise<CostDailySeries[]> {
  const step = 86_400;
  return withTimeout(timeoutMs, async (signal) => {
    const response = await fetchMetricsRange(
      source,
      query,
      rangeStart + step,
      rangeStart + days * step,
      `${String(step)}s`,
      signal,
    );
    if (!response.ok) throw new Error("Prometheus range query request failed");
    const body = (await response.json()) as RangeBody;
    if (body.status !== "success") {
      throw new Error("Prometheus range query returned non-success status");
    }
    return (body.data?.result ?? []).map((series) => {
      const values = new Map<string, number>();
      for (const [timestamp, raw] of series.values) {
        values.set(formatUtcCalendarDate(timestamp - step), parseValue(raw));
      }
      return { labels: series.metric, values };
    });
  });
}
