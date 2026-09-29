import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  emptyGatewayPhaseCounts,
  gatewayFleetTotalDailyPromql,
  gatewayFleetTotalDailyStepSeconds,
  queryGatewayMetrics,
  queryGatewayPhaseCounts,
} from "../src/metrics-gateways.js";

async function startPrometheusStub(
  handler: (request: IncomingMessage, response: ServerResponse) => void,
): Promise<{ close: () => void; port: number }> {
  const server = createServer(handler);
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("expected tcp listener address");
  }
  return {
    close: () => server.close(),
    port: address.port,
  };
}

describe("queryGatewayPhaseCounts", () => {
  it("maps Prometheus samples into phase counts", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      expect(request.url).toBe("/api/v1/query?query=hypershell_gateways_total");
      response.setHeader("content-type", "application/json");
      response.end(
        JSON.stringify({
          status: "success",
          data: {
            result: [
              {
                metric: { phase: "Running" },
                value: ["1704067200", "5"],
              },
              {
                metric: { phase: "Failed" },
                value: ["1704067200", "2"],
              },
            ],
          },
        }),
      );
    });

    try {
      const counts = await queryGatewayPhaseCounts(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(counts).toEqual({
        Pending: 0,
        Running: 5,
        Provisioning: 0,
        Degraded: 0,
        Failed: 2,
      });
    } finally {
      prometheus.close();
    }
  });

  it("exports the documented daily fleet-total range query contract", () => {
    expect(gatewayFleetTotalDailyPromql).toBe("sum(hypershell_gateways_total)");
    expect(gatewayFleetTotalDailyStepSeconds).toBe(86_400);
  });

  it("returns zeroed counts when Prometheus has no samples", async () => {
    const prometheus = await startPrometheusStub((_request, response) => {
      response.end(
        JSON.stringify({
          status: "success",
          data: { result: [] },
        }),
      );
    });

    try {
      const counts = await queryGatewayPhaseCounts(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(counts).toEqual(emptyGatewayPhaseCounts());
    } finally {
      prometheus.close();
    }
  });
});

describe("queryGatewayMetrics", () => {
  it("maps Prometheus range samples into daily_fleet_totals", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      if (url.pathname === "/api/v1/query") {
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  metric: { phase: "Running" },
                  value: ["1704067200", "5"],
                },
              ],
            },
          }),
        );
        return;
      }
      if (url.pathname === "/api/v1/query_range") {
        expect(url.searchParams.get("query")).toBe(
          gatewayFleetTotalDailyPromql,
        );
        expect(url.searchParams.get("step")).toBe("86400s");
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  values: [
                    ["1694217600", "18"],
                    ["1694304000", "20"],
                  ],
                },
              ],
            },
          }),
        );
        return;
      }
      response.statusCode = 404;
      response.end();
    });

    try {
      const metrics = await queryGatewayMetrics(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics.counts.Running).toBe(5);
      expect(metrics.daily_fleet_totals).toHaveLength(7);
      const firstFleetTotal = metrics.daily_fleet_totals?.[0];
      expect(firstFleetTotal).toBeDefined();
      expect(firstFleetTotal?.date).toMatch(/^\d{4}-\d{2}-\d{2}$/u);
      expect(typeof firstFleetTotal?.total).toBe("number");
    } finally {
      prometheus.close();
    }
  });

  it("returns counts without daily_fleet_totals when the range query fails", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      if (url.pathname === "/api/v1/query") {
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  metric: { phase: "Running" },
                  value: ["1704067200", "5"],
                },
              ],
            },
          }),
        );
        return;
      }
      response.statusCode = 500;
      response.end("range failed");
    });

    try {
      const metrics = await queryGatewayMetrics(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics.counts.Running).toBe(5);
      expect(metrics.daily_fleet_totals).toBeUndefined();
    } finally {
      prometheus.close();
    }
  });
});
