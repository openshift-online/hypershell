import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  emptyGatewayPhaseCounts,
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
