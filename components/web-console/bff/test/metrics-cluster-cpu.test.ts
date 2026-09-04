import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterCpuCapacityPromql,
  clusterCpuUsedPromql,
  queryClusterCpu,
} from "../src/metrics-cluster-cpu.js";

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

describe("queryClusterCpu", () => {
  it("maps Prometheus capacity and used samples into fractional cores", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusSample("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusSample("48"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const cpu = await queryClusterCpu(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(cpu).toEqual({
        available_cores: 12,
        capacity_cores: 60,
        used_cores: 48,
      });
    } finally {
      prometheus.close();
    }
  });

  it("preserves fractional used cores from Prometheus", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusSample("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusSample("48.2"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const cpu = await queryClusterCpu(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(cpu.used_cores).toBe(48.2);
      expect(cpu.capacity_cores).toBe(60);
      expect(cpu.available_cores).toBeCloseTo(11.8);
    } finally {
      prometheus.close();
    }
  });

  it("fails when Prometheus returns no capacity samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(
          JSON.stringify({
            status: "success",
            data: { result: [] },
          }),
        );
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusSample("48"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterCpu(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("No cluster CPU capacity data");
    } finally {
      prometheus.close();
    }
  });

  it("fails when used exceeds capacity beyond tolerance", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusSample("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusSample("60.02"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterCpu(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Inconsistent cluster CPU samples");
    } finally {
      prometheus.close();
    }
  });
});
