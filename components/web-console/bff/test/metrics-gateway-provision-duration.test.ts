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

describe("queryGatewayProvisionDuration", () => {
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
});
