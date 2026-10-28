import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  gatewayProvisionDurationCountPromql,
  gatewayProvisionDurationMeanPromql,
  gatewayProvisionDurationP50Promql,
  gatewayProvisionDurationP95Promql,
  gatewayProvisionDurationP99DailyPromql,
  gatewayProvisionDurationP99SevenDayPromql,
  queryGatewayProvisionDuration,
} from "../src/metrics-gateway-provision-duration.js";

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

function prometheusSample(value: string) {
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

function prometheusRangeSample(values: [string, string][]) {
  return JSON.stringify({
    status: "success",
    data: { result: [{ metric: {}, values }] },
  });
}

describe("queryGatewayProvisionDuration", () => {
  // Verifies: OBS-GPT-09.01
  it("maps Prometheus histogram samples into mean, P50, and P95 seconds", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === gatewayProvisionDurationCountPromql) {
        response.end(prometheusSample("2"));
        return;
      }
      if (query === gatewayProvisionDurationMeanPromql) {
        response.end(prometheusSample("315"));
        return;
      }
      if (query === gatewayProvisionDurationP50Promql) {
        response.end(prometheusSample("288"));
        return;
      }
      if (query === gatewayProvisionDurationP95Promql) {
        response.end(prometheusSample("726"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const duration = await queryGatewayProvisionDuration(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(duration).toEqual({
        mean_seconds: 315,
        observation_count: 2,
        p50_seconds: 288,
        p95_seconds: 726,
      });
    } finally {
      prometheus.close();
    }
  });

  it("fails when observation count is zero", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === gatewayProvisionDurationCountPromql) {
        response.end(prometheusSample("0"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryGatewayProvisionDuration(
          `http://127.0.0.1:${String(prometheus.port)}`,
          5_000,
        ),
      ).rejects.toThrow("No gateway provision duration observations");
    } finally {
      prometheus.close();
    }
  });

  it("adds optional seven-day P99, target, and daily trend", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        if (query === gatewayProvisionDurationP99DailyPromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "320"],
              ["1704153600", "280"],
            ]),
          );
          return;
        }
      }
      const values = new Map<string, string>([
        ["sum(increase(gateway_provision_duration_seconds_count[7d]))", "2"],
        [
          "sum(increase(gateway_provision_duration_seconds_sum[7d])) / sum(increase(gateway_provision_duration_seconds_count[7d]))",
          "315",
        ],
        [
          "histogram_quantile(0.50, sum by (le) (increase(gateway_provision_duration_seconds_bucket[7d])))",
          "288",
        ],
        [
          "histogram_quantile(0.95, sum by (le) (increase(gateway_provision_duration_seconds_bucket[7d])))",
          "726",
        ],
        [gatewayProvisionDurationP99SevenDayPromql, "810"],
      ]);
      const value = query === null ? undefined : values.get(query);
      if (value === undefined) {
        response.statusCode = 400;
        response.end();
        return;
      }
      response.end(prometheusSample(value));
    });

    try {
      const duration = await queryGatewayProvisionDuration(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
        undefined,
        7,
      );
      expect(duration).toMatchObject({
        p99_seconds: 810,
        target_seconds: 300,
        daily_p99: [
          { date: "2024-01-01", p99_seconds: 320 },
          { date: "2024-01-02", p99_seconds: 280 },
        ],
      });
    } finally {
      prometheus.close();
    }
  });
});
