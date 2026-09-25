import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  apiErrorRatePercentPromql,
  apiLatencyP50SecondsCumulativePromql,
  apiLatencyP50SecondsPromql,
  apiRequestRatePromql,
  queryApiReliability,
} from "../src/metrics-api-reliability.js";

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

function emptyPrometheusResult() {
  return JSON.stringify({
    status: "success",
    data: { result: [] },
  });
}

describe("queryApiReliability", () => {
  it("maps Prometheus instant and hourly series into API reliability metrics", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query") {
        if (query === apiRequestRatePromql) {
          response.end(prometheusSample("12.5"));
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(prometheusSample("1.25"));
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(prometheusSample("0.084"));
          return;
        }
      }

      if (url.pathname === "/api/v1/query_range") {
        if (query === apiRequestRatePromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "10.2"],
              ["1704070800", "12.5"],
            ]),
          );
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "0.5"],
              ["1704070800", "1.25"],
            ]),
          );
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "0.09"],
              ["1704070800", "0.084"],
            ]),
          );
          return;
        }
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const metrics = await queryApiReliability(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics).toEqual({
        error_rate_percent: 1.25,
        hourly_error_rate_percent: [
          { hour: "2024-01-01T00:00", value: 0.5 },
          { hour: "2024-01-01T01:00", value: 1.25 },
        ],
        hourly_latency_p50_seconds: [
          { hour: "2024-01-01T00:00", value: 0.09 },
          { hour: "2024-01-01T01:00", value: 0.084 },
        ],
        hourly_request_rate: [
          { hour: "2024-01-01T00:00", value: 10.2 },
          { hour: "2024-01-01T01:00", value: 12.5 },
        ],
        latency_p50_seconds: 0.084,
        request_rate: 12.5,
      });
    } finally {
      prometheus.close();
    }
  });

  it("falls back to cumulative latency when rate-based P50 is non-finite", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query") {
        if (query === apiRequestRatePromql) {
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(prometheusSample("NaN"));
          return;
        }
        if (query === apiLatencyP50SecondsCumulativePromql) {
          response.end(prometheusSample("0.05"));
          return;
        }
      }

      if (url.pathname === "/api/v1/query_range") {
        response.end(prometheusRangeSample([]));
        return;
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const metrics = await queryApiReliability(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics.request_rate).toBe(0);
      expect(metrics.error_rate_percent).toBe(0);
      expect(metrics.latency_p50_seconds).toBe(0.05);
    } finally {
      prometheus.close();
    }
  });

  it("falls back to cumulative hourly latency when rate-based range fails", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query") {
        if (query === apiRequestRatePromql) {
          response.end(prometheusSample("3"));
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(prometheusSample("0.05"));
          return;
        }
      }

      if (url.pathname === "/api/v1/query_range") {
        if (query === apiRequestRatePromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "2"],
              ["1704070800", "3"],
            ]),
          );
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "0"],
              ["1704070800", "0"],
            ]),
          );
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.statusCode = 500;
          response.end();
          return;
        }
        if (query === apiLatencyP50SecondsCumulativePromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "0.06"],
              ["1704070800", "0.05"],
            ]),
          );
          return;
        }
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const metrics = await queryApiReliability(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics.hourly_latency_p50_seconds).toEqual([
        { hour: "2024-01-01T00:00", value: 0.06 },
        { hour: "2024-01-01T01:00", value: 0.05 },
      ]);
    } finally {
      prometheus.close();
    }
  });

  it("omits only failed hourly series when instant queries succeed", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query") {
        if (query === apiRequestRatePromql) {
          response.end(prometheusSample("3"));
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(prometheusSample("0.05"));
          return;
        }
      }

      if (url.pathname === "/api/v1/query_range") {
        if (query === apiRequestRatePromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "2"],
              ["1704070800", "3"],
            ]),
          );
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          response.end(
            prometheusRangeSample([
              ["1704067200", "0"],
              ["1704070800", "0"],
            ]),
          );
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.statusCode = 500;
          response.end();
          return;
        }
        if (query === apiLatencyP50SecondsCumulativePromql) {
          response.statusCode = 500;
          response.end();
          return;
        }
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const metrics = await queryApiReliability(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics.request_rate).toBe(3);
      expect(metrics.error_rate_percent).toBe(0);
      expect(metrics.latency_p50_seconds).toBe(0.05);
      expect(metrics.hourly_request_rate).toHaveLength(2);
      expect(metrics.hourly_error_rate_percent).toHaveLength(2);
      expect(metrics.hourly_latency_p50_seconds).toBeUndefined();
    } finally {
      prometheus.close();
    }
  });

  it("fails when instant request-rate samples are empty", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query") {
        response.end(emptyPrometheusResult());
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryApiReliability(
          `http://127.0.0.1:${String(prometheus.port)}`,
          5_000,
        ),
      ).rejects.toThrow(/no samples/u);
    } finally {
      prometheus.close();
    }
  });

  it("exports PromQL against api_inbound_request_duration with 5xx-only code filter", () => {
    expect(apiErrorRatePercentPromql).toContain('code=~"5.."');
    expect(apiErrorRatePercentPromql).toContain('job="hypershell-api-server"');
    expect(apiErrorRatePercentPromql).toContain("or vector(0)");
    expect(apiErrorRatePercentPromql).not.toMatch(/code=~"[^"]*4/u);
    expect(apiErrorRatePercentPromql).not.toContain("4..");
    expect(apiRequestRatePromql).toContain(
      "api_inbound_request_duration_count",
    );
    expect(apiLatencyP50SecondsPromql).toContain("histogram_quantile(0.50");
    expect(apiLatencyP50SecondsCumulativePromql).toContain(
      "api_inbound_request_duration_bucket",
    );
  });

  it("treats idle request rate as 0% errors without failing the collection", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query") {
        if (query === apiRequestRatePromql) {
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiErrorRatePercentPromql) {
          // PromQL clamp_min path: Prometheus returns 0 when there is no traffic.
          response.end(prometheusSample("0"));
          return;
        }
        if (query === apiLatencyP50SecondsPromql) {
          response.end(prometheusSample("NaN"));
          return;
        }
        if (query === apiLatencyP50SecondsCumulativePromql) {
          response.end(prometheusSample("0.01"));
          return;
        }
      }

      if (url.pathname === "/api/v1/query_range") {
        response.end(prometheusRangeSample([]));
        return;
      }

      response.statusCode = 400;
      response.end();
    });

    try {
      const metrics = await queryApiReliability(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(metrics).toMatchObject({
        error_rate_percent: 0,
        latency_p50_seconds: 0.01,
        request_rate: 0,
      });
    } finally {
      prometheus.close();
    }
  });
});
