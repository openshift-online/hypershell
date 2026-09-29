import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterCpuCapacityPromql,
  clusterCpuDailyUsedPromql,
  clusterCpuDailyUsedStepSeconds,
  clusterCpuUsedPromql,
  queryClusterCpu,
} from "../src/metrics-cluster-cpu.js";
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

describe("queryClusterCpu", () => {
  it("exports the documented daily CPU range query contract", () => {
    expect(clusterCpuDailyUsedPromql).toBe(
      'sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))',
    );
    expect(clusterCpuDailyUsedStepSeconds).toBe(86_400);
  });

  it("maps Prometheus capacity and used samples into fractional cores", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusInstantSampleBody("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusInstantSampleBody("48"));
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
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusInstantSampleBody("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusInstantSampleBody("48.2"));
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

  it("maps Prometheus range samples into daily_used", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      response.setHeader("content-type", "application/json");
      if (isPrometheusRangeRequest(url)) {
        expect(url.searchParams.get("query")).toBe(clusterCpuDailyUsedPromql);
        expect(url.searchParams.get("step")).toBe("86400s");
        response.end(
          prometheusRangeSampleBody([
            [String(utcDayStartUnixSeconds(6)), "48.6"],
            [String(utcDayStartUnixSeconds(5)), "50.2"],
          ]),
        );
        return;
      }
      const query = url.searchParams.get("query");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusInstantSampleBody("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusInstantSampleBody("48"));
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
      expect(cpu.used_cores).toBe(48);
      expect(cpu.daily_used).toHaveLength(7);
      const firstDailyUsed = cpu.daily_used?.[0];
      expect(firstDailyUsed).toBeDefined();
      expect(firstDailyUsed?.date).toMatch(/^\d{4}-\d{2}-\d{2}$/u);
      expect(typeof firstDailyUsed?.value).toBe("number");
      const mappedValues = cpu.daily_used?.map((entry) => entry.value) ?? [];
      expect(mappedValues).toContain(49);
      expect(mappedValues).toContain(50);
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
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusInstantSampleBody("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusInstantSampleBody("48"));
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

  it("fails when Prometheus returns no capacity samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
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
        response.end(prometheusInstantSampleBody("48"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterCpu(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Prometheus query returned no samples");
    } finally {
      prometheus.close();
    }
  });

  it("fails when used exceeds capacity beyond tolerance", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterCpuCapacityPromql) {
        response.end(prometheusInstantSampleBody("60"));
        return;
      }
      if (query === clusterCpuUsedPromql) {
        response.end(prometheusInstantSampleBody("60.02"));
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
