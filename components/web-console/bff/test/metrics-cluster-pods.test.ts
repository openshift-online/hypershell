import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterPodPhasePromql,
  clusterPodsCapacityPromql,
  clusterPodsDailyUsedPromql,
  clusterPodsDailyUsedStepSeconds,
  clusterPodsUsedPromql,
  type ClusterPodPhase,
  queryClusterPods,
} from "../src/metrics-cluster-pods.js";
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

const defaultPhaseCounts: Record<ClusterPodPhase, string> = {
  Failed: "16",
  Pending: "12",
  Running: "500",
  Succeeded: "20",
  Unknown: "0",
};

function handleClusterPodsQuery(
  query: string | null,
  response: ServerResponse,
  options: {
    capacity?: string;
    phases?: Partial<Record<ClusterPodPhase, string>>;
    used?: string;
  } = {},
  request?: IncomingMessage,
): boolean {
  if (request !== undefined) {
    const url = parsePrometheusUrl(request);
    if (isPrometheusRangeRequest(url)) {
      rejectPrometheusRange(response);
      return true;
    }
  }
  response.setHeader("content-type", "application/json");
  if (query === clusterPodsCapacityPromql) {
    response.end(prometheusSample(options.capacity ?? "2000"));
    return true;
  }
  if (query === clusterPodsUsedPromql) {
    response.end(prometheusSample(options.used ?? "548"));
    return true;
  }
  for (const phase of [
    "Pending",
    "Running",
    "Succeeded",
    "Failed",
    "Unknown",
  ] as const) {
    if (query === clusterPodPhasePromql(phase)) {
      response.end(
        prometheusSample(options.phases?.[phase] ?? defaultPhaseCounts[phase]),
      );
      return true;
    }
  }
  return false;
}

describe("queryClusterPods", () => {
  it("exports the documented daily pods range query contract", () => {
    expect(clusterPodsDailyUsedPromql).toBe("count(kube_pod_info)");
    expect(clusterPodsDailyUsedStepSeconds).toBe(86_400);
  });

  it("maps Prometheus capacity, used, and phase samples into pod counts", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      const query = url.searchParams.get("query");
      if (handleClusterPodsQuery(query, response, {}, request)) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const pods = await queryClusterPods(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(pods).toEqual({
        available_pods: 1452,
        capacity_pods: 2000,
        phase_failed_pods: 16,
        phase_pending_pods: 12,
        phase_running_pods: 500,
        phase_succeeded_pods: 20,
        phase_unknown_pods: 0,
        used_pods: 548,
      });
    } finally {
      prometheus.close();
    }
  });

  it("maps Prometheus range samples into daily_used", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      response.setHeader("content-type", "application/json");
      if (isPrometheusRangeRequest(url)) {
        expect(url.searchParams.get("query")).toBe(clusterPodsDailyUsedPromql);
        expect(url.searchParams.get("step")).toBe("86400s");
        response.end(
          prometheusRangeSampleBody([
            [String(utcDayStartUnixSeconds(6)), "540"],
            [String(utcDayStartUnixSeconds(5)), "548"],
          ]),
        );
        return;
      }
      const query = url.searchParams.get("query");
      if (handleClusterPodsQuery(query, response)) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const pods = await queryClusterPods(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(pods.used_pods).toBe(548);
      expect(pods.daily_used).toHaveLength(7);
      const mappedValues = pods.daily_used?.map((entry) => entry.value) ?? [];
      expect(mappedValues).toContain(540);
      expect(mappedValues).toContain(548);
    } finally {
      prometheus.close();
    }
  });

  it("returns instant fields without daily_used when the range query fails", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      const query = url.searchParams.get("query");
      if (handleClusterPodsQuery(query, response, {}, request)) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const pods = await queryClusterPods(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(pods).toEqual({
        available_pods: 1452,
        capacity_pods: 2000,
        phase_failed_pods: 16,
        phase_pending_pods: 12,
        phase_running_pods: 500,
        phase_succeeded_pods: 20,
        phase_unknown_pods: 0,
        used_pods: 548,
      });
    } finally {
      prometheus.close();
    }
  });

  it("fails when Prometheus returns no capacity samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (isPrometheusRangeRequest(url)) {
        rejectPrometheusRange(response);
        return;
      }
      if (query === clusterPodsCapacityPromql) {
        response.end(
          JSON.stringify({
            status: "success",
            data: { result: [] },
          }),
        );
        return;
      }
      if (handleClusterPodsQuery(query, response)) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterPods(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Prometheus query returned no samples");
    } finally {
      prometheus.close();
    }
  });

  it("fails when used exceeds capacity", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      const query = url.searchParams.get("query");
      if (
        handleClusterPodsQuery(
          query,
          response,
          {
            capacity: "100",
            phases: {
              Failed: "0",
              Pending: "0",
              Running: "101",
              Succeeded: "0",
              Unknown: "0",
            },
            used: "101",
          },
          request,
        )
      ) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterPods(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Inconsistent cluster pod samples");
    } finally {
      prometheus.close();
    }
  });

  it("fails when phase counts do not sum to used pods", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = parsePrometheusUrl(request);
      const query = url.searchParams.get("query");
      if (
        handleClusterPodsQuery(
          query,
          response,
          {
            phases: {
              Failed: "16",
              Pending: "12",
              Running: "499",
              Succeeded: "20",
              Unknown: "0",
            },
          },
          request,
        )
      ) {
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterPods(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Inconsistent cluster pod phase samples");
    } finally {
      prometheus.close();
    }
  });
});
