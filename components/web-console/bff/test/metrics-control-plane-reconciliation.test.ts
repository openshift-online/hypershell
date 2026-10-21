import { createServer } from "node:http";

import { afterEach, describe, expect, it } from "vitest";

import {
  queryControlPlaneReconciliation,
  reconciliationFailureHourlyCountPromql,
  reconciliationFailureCountPromql,
  reconciliationLagP50SecondsPromql,
  reconciliationRetryCountPromql,
  reconciliationRetryHourlyCountPromql,
  reconciliationSuccessHourlyCountPromql,
  reconciliationSuccessCountPromql,
  staleResourceStatusCountPromql,
} from "../src/metrics-control-plane-reconciliation.js";
import {
  hourlyRangeStepSeconds,
  rollingHourlyRange,
} from "../src/prometheus-range-query.js";

describe("queryControlPlaneReconciliation", () => {
  let server: ReturnType<typeof createServer> | undefined;

  afterEach(() => server?.close());

  // Verifies: OBS-CRM-04
  it("keeps valid zero counts when an idle lag histogram returns NaN or empty", async () => {
    server = createServer((request, response) => {
      const query = new URL(
        request.url ?? "",
        "http://127.0.0.1",
      ).searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (request.url?.startsWith("/api/v1/query_range")) {
        response.end(
          JSON.stringify({ status: "success", data: { result: [] } }),
        );
        return;
      }
      const value = query === reconciliationLagP50SecondsPromql ? "NaN" : "0";
      response.end(
        JSON.stringify({
          status: "success",
          data: {
            result: query === "not-used" ? [] : [{ value: ["1", value] }],
          },
        }),
      );
    });
    await new Promise<void>((resolve) =>
      server?.listen(0, "127.0.0.1", resolve),
    );
    const address = server.address();
    if (address === null || typeof address === "string")
      throw new Error("expected listener");

    const metrics = await queryControlPlaneReconciliation(
      `http://127.0.0.1:${String(address.port)}`,
      5_000,
    );
    expect(metrics).toMatchObject({
      reconciliation_failures_count: 0,
      reconciliation_retries_count: 0,
      reconciliation_successes_count: 0,
      stale_resource_status_count: 0,
    });
    expect(metrics.reconciliation_lag_p50_seconds).toBeUndefined();
    expect(reconciliationFailureCountPromql).toContain("increase");
    expect(reconciliationRetryCountPromql).toContain("increase");
    expect(reconciliationSuccessCountPromql).toContain("increase");
    expect(staleResourceStatusCountPromql).toContain("stale");
  });

  // Verifies: OBS-CRM-03
  it("queries reconciliation trends as rolling 24-hour hourly series", async () => {
    const rangeRequests: {
      end: number;
      query: string;
      start: number;
      step: string;
    }[] = [];
    const rangeBefore = rollingHourlyRange();

    server = createServer((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      response.setHeader("content-type", "application/json");

      if (request.url?.startsWith("/api/v1/query_range")) {
        rangeRequests.push({
          end: Number(url.searchParams.get("end")),
          query,
          start: Number(url.searchParams.get("start")),
          step: url.searchParams.get("step") ?? "",
        });
        const timestamp = Math.floor(Date.now() / 1000);
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  values: [
                    [String(timestamp), "2"],
                    [String(timestamp + hourlyRangeStepSeconds), "3"],
                  ],
                },
              ],
            },
          }),
        );
        return;
      }

      const value = query === reconciliationLagP50SecondsPromql ? "NaN" : "0";
      response.end(
        JSON.stringify({
          status: "success",
          data: { result: [{ value: ["1", value] }] },
        }),
      );
    });
    await new Promise<void>((resolve) =>
      server?.listen(0, "127.0.0.1", resolve),
    );
    const address = server.address();
    if (address === null || typeof address === "string")
      throw new Error("expected listener");
    const rangeAfter = rollingHourlyRange();

    const metrics = await queryControlPlaneReconciliation(
      `http://127.0.0.1:${String(address.port)}`,
      5_000,
    );
    const trendQueries = [
      reconciliationFailureHourlyCountPromql,
      reconciliationRetryHourlyCountPromql,
      reconciliationSuccessHourlyCountPromql,
      reconciliationLagP50SecondsPromql,
      staleResourceStatusCountPromql,
    ];
    const trendRequests = rangeRequests.filter((rangeRequest) =>
      trendQueries.includes(rangeRequest.query),
    );
    const countTrendQueries = [
      reconciliationFailureHourlyCountPromql,
      reconciliationRetryHourlyCountPromql,
      reconciliationSuccessHourlyCountPromql,
    ];
    const countTrendRequests = trendRequests.filter((rangeRequest) =>
      countTrendQueries.includes(rangeRequest.query),
    );

    expect(trendRequests).toHaveLength(trendQueries.length);
    for (const rangeRequest of trendRequests) {
      expect(rangeRequest.step).toBe(String(hourlyRangeStepSeconds));
      expect(rangeRequest.start).toBeGreaterThanOrEqual(rangeBefore.start);
      expect(rangeRequest.start).toBeLessThanOrEqual(rangeAfter.start);
      expect(rangeRequest.end).toBeGreaterThanOrEqual(rangeBefore.end);
      expect(rangeRequest.end).toBeLessThanOrEqual(rangeAfter.end);
      expect(rangeRequest.end - rangeRequest.start).toBe(86_400);
    }
    for (const rangeRequest of countTrendRequests) {
      expect(rangeRequest.query).toContain("[1h]");
    }
    expect(metrics.hourly_reconciliation_failures_count).toHaveLength(2);
    expect(metrics.hourly_reconciliation_retries_count).toHaveLength(2);
    expect(metrics.hourly_reconciliation_successes_count).toHaveLength(2);
    expect(metrics.hourly_reconciliation_lag_p50_seconds).toHaveLength(2);
    expect(metrics.hourly_stale_resource_status_count).toHaveLength(2);
  });
});
