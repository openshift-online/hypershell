import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterMemoryAvailablePromql,
  clusterMemoryCapacityPromql,
  clusterMemoryDailyUsedPromql,
  clusterMemoryDailyUsedStepSeconds,
  queryClusterMemory,
} from "../src/metrics-cluster-memory.js";
import {
  isPrometheusRangeRequest,
  parsePrometheusUrl,
  prometheusInstantSampleBody,
  prometheusRangeSampleBody,
  rejectPrometheusRange,
  utcDayStartUnixSeconds,
} from "./prometheus-stub.js";

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
  return prometheusInstantSampleBody(value);
}

describe("queryClusterMemory", () => {
  it("exports the documented daily memory range query contract", () => {
    expect(clusterMemoryDailyUsedPromql).toBe(
      "sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)",
    );
    expect(clusterMemoryDailyUsedStepSeconds).toBe(86_400);
  });

  it("maps Prometheus capacity and available samples into bytes", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
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

  it("maps Prometheus range samples into daily_used GiB", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      response.setHeader("content-type", "application/json");
      if (isPrometheusRangeRequest(url)) {
        expect(url.searchParams.get("query")).toBe(
          clusterMemoryDailyUsedPromql,
        );
        expect(url.searchParams.get("step")).toBe("86400s");
        response.end(
          prometheusRangeSampleBody([
            [String(utcDayStartUnixSeconds(6)), String(220 * 1024 ** 3)],
            [String(utcDayStartUnixSeconds(5)), String(218 * 1024 ** 3)],
          ]),
        );
        return;
      }
      const query = url.searchParams.get("query");
      if (query === clusterMemoryCapacityPromql) {
        response.end(prometheusSample(String(237 * 1024 ** 3)));
        return;
      }
      if (query === clusterMemoryAvailablePromql) {
        response.end(prometheusSample(String(17 * 1024 ** 3)));
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
      expect(memory.used_bytes).toBe(220 * 1024 ** 3);
      expect(memory.daily_used).toHaveLength(7);
      const mappedValues = memory.daily_used?.map((entry) => entry.value) ?? [];
      expect(mappedValues).toContain(220);
      expect(mappedValues).toContain(218);
    } finally {
      prometheus.close();
    }
  });

  it("returns instant fields without daily_used when the range query fails", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
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
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
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
      ).rejects.toThrow("Prometheus query returned no samples");
    } finally {
      prometheus.close();
    }
  });

  it("fails when available exceeds capacity", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
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
