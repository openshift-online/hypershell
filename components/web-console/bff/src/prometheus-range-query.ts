import { fetchMetricsRange, type MetricsSource } from "./metrics-source.js";

export const dailyRangeStepSeconds = 86_400;
export const hourlyRangeStepSeconds = 3_600;
export const hourlyRangeLookbackSeconds = 86_400;
export const dailyRangeLookbackDays = 7;

interface PrometheusRangeQueryResponse {
  status: string;
  data?: {
    result: {
      values: [string, string][];
    }[];
  };
}

export function utcCalendarDatesInclusive(days: number): string[] {
  const dates: string[] = [];
  const today = new Date();
  const utcToday = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  for (let offset = days - 1; offset >= 0; offset -= 1) {
    const day = new Date(utcToday - offset * 24 * 60 * 60 * 1000);
    dates.push(day.toISOString().slice(0, 10));
  }

  return dates;
}

export function formatUtcCalendarDate(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");

  return `${String(year)}-${month}-${day}`;
}

export function formatHourLabel(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  const hour = String(date.getUTCHours()).padStart(2, "0");

  return `${String(year)}-${month}-${day}T${hour}:00`;
}

export function sevenDayUtcCalendarRange(): { end: number; start: number } {
  const now = Date.now();
  const today = new Date(now);
  const utcTodayStart = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  return {
    end: Math.floor(now / 1000),
    start: Math.floor(
      (utcTodayStart - (dailyRangeLookbackDays - 1) * 24 * 60 * 60 * 1000) /
        1000,
    ),
  };
}

export function rollingHourlyRange(): { end: number; start: number } {
  const end = Math.floor(Date.now() / 1000);

  return {
    end,
    start: end - hourlyRangeLookbackSeconds,
  };
}

export async function queryPrometheusRangeScalarSamples(
  prometheusUrl: MetricsSource,
  query: string,
  start: number,
  end: number,
  step: string,
  timeoutMs: number,
): Promise<Map<number, number>> {
  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetchMetricsRange(
      prometheusUrl,
      query,
      start,
      end,
      step,
      controller.signal,
    );
    if (!response.ok) {
      throw new Error("Prometheus range query request failed");
    }

    const body = (await response.json()) as PrometheusRangeQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus range query returned non-success status");
    }

    const values = body.data?.result[0]?.values ?? [];
    const samples = new Map<number, number>();

    for (const [timestamp, rawValue] of values) {
      const value = Number(rawValue);
      if (!Number.isFinite(value) || value < 0) {
        throw new Error("Prometheus range query returned invalid sample");
      }

      samples.set(Number(timestamp), value);
    }

    return samples;
  } finally {
    clearTimeout(timeout);
  }
}

export function alignDailyIntegerSeries(
  samples: Map<number, number>,
  days = dailyRangeLookbackDays,
): { date: string; value: number }[] {
  const valuesByDate = new Map<string, number>();

  for (const [timestamp, rawValue] of samples) {
    valuesByDate.set(
      formatUtcCalendarDate(timestamp),
      Math.max(0, Math.round(rawValue)),
    );
  }

  return utcCalendarDatesInclusive(days).map((date) => ({
    date,
    value: valuesByDate.get(date) ?? 0,
  }));
}

export function mapHourlyIntegerSeries(
  samples: Map<number, number>,
): { count: number; hour: string }[] {
  const hourly: { count: number; hour: string }[] = [];

  for (const [timestamp, rawValue] of [...samples.entries()].sort(
    (left, right) => left[0] - right[0],
  )) {
    hourly.push({
      count: Math.max(0, Math.round(rawValue)),
      hour: formatHourLabel(timestamp),
    });
  }

  return hourly;
}
