import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterMemoryAvailablePromql,
  clusterMemoryCapacityPromql,
  queryClusterMemory,
} from "../src/metrics-cluster-memory.js";

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

describe("queryClusterMemory", () => {
  it("maps Prometheus capacity and available samples into bytes", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterMemoryCapacityPromql) {
        response.end(prometheusSample("17179869184"));
        return;
      }
      if (query === clusterMemoryAvailablePromql) {
        response.end(prometheusSample("4294967296"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const memory = await queryClusterMemory(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(memory).toEqual({
        available_bytes: 4294967296,
        capacity_bytes: 17179869184,
        used_bytes: 12884901888,
      });
    } finally {
      prometheus.close();
    }
  });

  it("fails when Prometheus returns no capacity samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterMemoryCapacityPromql) {
        response.end(
          JSON.stringify({
            status: "success",
            data: { result: [] },
          }),
        );
        return;
      }
      if (query === clusterMemoryAvailablePromql) {
        response.end(prometheusSample("4294967296"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterMemory(
          `http://127.0.0.1:${String(prometheus.port)}`,
          5_000,
        ),
      ).rejects.toThrow("No cluster memory capacity data");
    } finally {
      prometheus.close();
    }
  });

  it("fails when available exceeds capacity", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterMemoryCapacityPromql) {
        response.end(prometheusSample("1000"));
        return;
      }
      if (query === clusterMemoryAvailablePromql) {
        response.end(prometheusSample("2000"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterMemory(
          `http://127.0.0.1:${String(prometheus.port)}`,
          5_000,
        ),
      ).rejects.toThrow("Inconsistent cluster memory samples");
    } finally {
      prometheus.close();
    }
  });
});
