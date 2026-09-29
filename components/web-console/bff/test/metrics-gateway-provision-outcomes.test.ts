import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  gatewayProvisionDurationCount24hPromql,
  gatewayProvisionDurationCountHourlyPromql,
  gatewayProvisionDurationCountLifetimePromql,
  gatewayProvisionOutcomesFailure24hPromql,
  gatewayProvisionOutcomesFailureHourlyPromql,
  gatewayProvisionOutcomesSuccess24hPromql,
  gatewayProvisionOutcomesSuccessHourlyPromql,
  queryGatewayProvisionOutcomes,
} from "../src/metrics-gateway-provision-outcomes.js";

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
    data: {
      result: [
        {
          metric: {},
          values,
        },
      ],
    },
  });
}

describe("queryGatewayProvisionOutcomes", () => {
  it("maps Prometheus outcome counters into 24-hour totals and hourly success rates", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (query === gatewayProvisionOutcomesSuccess24hPromql) {
        response.end(prometheusSample("9"));
        return;
      }
      if (query === gatewayProvisionOutcomesFailure24hPromql) {
        response.end(prometheusSample("1"));
        return;
      }
      if (query === gatewayProvisionOutcomesSuccessHourlyPromql) {
        response.end(
          prometheusRangeSample([
            ["1704067200", "4"],
            ["1704070800", "5"],
          ]),
        );
        return;
      }
      if (query === gatewayProvisionOutcomesFailureHourlyPromql) {
        response.end(
          prometheusRangeSample([
            ["1704067200", "1"],
            ["1704070800", "0"],
          ]),
        );
        return;
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const outcomes = await queryGatewayProvisionOutcomes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(outcomes).toEqual({
        failure_count_24h: 1,
        hourly_success_rate: [
          {
            failure_count: 1,
            hour: "2024-01-01T00:00",
            success_count: 4,
            success_rate_percent: 80,
          },
          {
            failure_count: 0,
            hour: "2024-01-01T01:00",
            success_count: 5,
            success_rate_percent: 100,
          },
        ],
        success_count_24h: 9,
        success_rate_percent: 90,
      });
    } finally {
      prometheus.close();
    }
  });

  it("returns null success rate when no provisions completed in the window", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (
        query === gatewayProvisionOutcomesSuccess24hPromql ||
        query === gatewayProvisionOutcomesFailure24hPromql ||
        query === gatewayProvisionDurationCount24hPromql ||
        query === gatewayProvisionDurationCountLifetimePromql
      ) {
        response.end(prometheusSample("0"));
        return;
      }
      if (
        query === gatewayProvisionOutcomesSuccessHourlyPromql ||
        query === gatewayProvisionOutcomesFailureHourlyPromql
      ) {
        response.end(prometheusRangeSample([]));
        return;
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const outcomes = await queryGatewayProvisionOutcomes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(outcomes).toEqual({
        failure_count_24h: 0,
        hourly_success_rate: [],
        success_count_24h: 0,
        success_rate_percent: null,
      });
    } finally {
      prometheus.close();
    }
  });

  it("falls back to the duration histogram when outcome counters are empty", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (
        query === gatewayProvisionOutcomesSuccess24hPromql ||
        query === gatewayProvisionOutcomesFailure24hPromql
      ) {
        response.end(prometheusSample("0"));
        return;
      }
      if (query === gatewayProvisionDurationCount24hPromql) {
        response.end(prometheusSample("3"));
        return;
      }
      if (query === gatewayProvisionDurationCountHourlyPromql) {
        response.end(
          prometheusRangeSample([
            ["1704067200", "1"],
            ["1704070800", "2"],
          ]),
        );
        return;
      }
      if (query === gatewayProvisionOutcomesFailureHourlyPromql) {
        response.end(prometheusRangeSample([]));
        return;
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const outcomes = await queryGatewayProvisionOutcomes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(outcomes).toEqual({
        failure_count_24h: 0,
        hourly_success_rate: [
          {
            failure_count: 0,
            hour: "2024-01-01T00:00",
            success_count: 1,
            success_rate_percent: 100,
          },
          {
            failure_count: 0,
            hour: "2024-01-01T01:00",
            success_count: 2,
            success_rate_percent: 100,
          },
        ],
        success_count_24h: 3,
        success_rate_percent: 100,
      });
    } finally {
      prometheus.close();
    }
  });
});
