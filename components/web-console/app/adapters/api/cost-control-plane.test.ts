import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchCostDashboardMetrics } from "./cost-control-plane";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

const envelope = {
  status: "ok",
  estimate: true,
  currency: "USD",
  rate_card_version: "v1",
  range_days: 7,
};

describe("fetchCostDashboardMetrics", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads the three platform sources with the selected range", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) => {
        urls.push(url);
        if (url.startsWith("/api/metrics/cost/summary")) {
          return Promise.resolve(
            jsonResponse({
              ...envelope,
              monthly_total: 100,
              range_total: 23,
              idle: { monthly: 10, percent_of_total: 10 },
            }),
          );
        }
        if (url.startsWith("/api/metrics/cost/breakdown")) {
          return Promise.resolve(jsonResponse({}, 502));
        }
        return Promise.resolve(jsonResponse({ ...envelope, gateways: [] }));
      }),
    );

    const metrics = await fetchCostDashboardMetrics({
      correlationId: "c",
      costView: "platform",
      timeRange: 30,
    });

    expect(urls.sort()).toEqual([
      "/api/metrics/cost/breakdown?range=30",
      "/api/metrics/cost/gateways?range=30",
      "/api/metrics/cost/summary?range=30",
    ]);
    expect(metrics).toMatchObject({
      currency: "USD",
      failedSources: ["cost-breakdown"],
      platform: { summary: { monthlyTotal: 100, idleMonthly: 10 } },
      rateCardVersion: "v1",
      status: "ok",
      view: "platform",
    });
  });

  it("maps the not-configured state without amounts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({ status: "not-configured", estimate: true }),
        ),
      ),
    );
    const metrics = await fetchCostDashboardMetrics({
      correlationId: "c",
      costView: "mine",
    });
    expect(metrics.status).toBe("not-configured");
    expect(metrics.mine?.summary).toBeUndefined();
  });

  it("maps My cost and rejects negative amounts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) =>
        Promise.resolve(
          url.startsWith("/api/metrics/cost/me/summary")
            ? jsonResponse({
                ...envelope,
                empty: false,
                charged: { gateway_count: 1, monthly: 12.5 },
                shared: { gateway_count: 0, monthly: -1 },
                trend: [],
              })
            : jsonResponse({ ...envelope, gateways: [], workspaces: [] }),
        ),
      ),
    );
    await expect(
      fetchCostDashboardMetrics({ correlationId: "c", costView: "mine" }),
    ).rejects.toThrow(/shared\.monthly/u);
  });
});
