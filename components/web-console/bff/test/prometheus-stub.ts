import type { IncomingMessage, ServerResponse } from "node:http";

export function parsePrometheusUrl(request: IncomingMessage): URL {
  return new URL(request.url ?? "/", "http://127.0.0.1");
}

export function isPrometheusRangeRequest(url: URL): boolean {
  return url.pathname === "/api/v1/query_range";
}

export function isPrometheusInstantRequest(url: URL): boolean {
  return url.pathname === "/api/v1/query";
}

export function rejectPrometheusRange(response: ServerResponse): void {
  response.statusCode = 500;
  response.end("range failed");
}

export function utcDayStartUnixSeconds(daysBeforeToday: number): number {
  const today = new Date();
  const utcTodayStart = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  return Math.floor(
    (utcTodayStart - daysBeforeToday * 24 * 60 * 60 * 1000) / 1000,
  );
}

export function prometheusInstantSampleBody(value: string): string {
  return JSON.stringify({
    status: "success",
    data: {
      result: [
        {
          metric: {},
          value: ["1704067200", value],
        },
      ],
    },
  });
}

export function prometheusRangeSampleBody(values: [string, string][]): string {
  return JSON.stringify({
    status: "success",
    data: {
      result: [
        {
          values,
        },
      ],
    },
  });
}
