import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import { queryUsageGatewayMetrics } from "../src/metrics-usage-gateways.js";
import { queryUsageSandboxLifetime } from "../src/metrics-usage-sandbox-lifetime.js";
import { queryUsageSandboxMetrics } from "../src/metrics-usage-sandboxes.js";

async function startPrometheusStub(
  handler: (request: IncomingMessage, response: ServerResponse) => void,
): Promise<{ close: () => void; origin: string }> {
  const server = createServer(handler);
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("expected tcp listener for Prometheus stub");
  }

  return {
    close: () => server.close(),
    origin: `http://127.0.0.1:${String(address.port)}`,
  };
}

function instantPayload(value: string, namespace?: string): string {
  return JSON.stringify({
    status: "success",
    data: {
      resultType: "vector",
      result: [
        {
          metric: namespace ? { namespace } : {},
          value: ["1704067200", value],
        },
      ],
    },
  });
}

function emptyInstantPayload(): string {
  return JSON.stringify({
    status: "success",
    data: { resultType: "vector", result: [] },
  });
}

function rangePayload(value: string): string {
  const timestamp = String(Math.floor(Date.now() / 1000));
  return JSON.stringify({
    status: "success",
    data: {
      resultType: "matrix",
      result: [{ values: [[timestamp, value] as [string, string]] }],
    },
  });
}

describe("usage metrics queries", () => {
  it("scopes gateway usage and preserves daily gaps", async () => {
    const queries: string[] = [];
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      queries.push(query);
      response.setHeader("content-type", "application/json");

      if (url.pathname === "/api/v1/query_range") {
        response.end(
          rangePayload(
            query.includes('code!="OK"') || query.includes('status=~"5.."')
              ? "1"
              : "99",
          ),
        );
        return;
      }

      if (query.includes("count by (namespace)")) {
        response.end(
          query.includes("openshell_server_grpc_requests_total")
            ? instantPayload("1", "openshell-gateway-1")
            : emptyInstantPayload(),
        );
        return;
      }

      response.end(
        instantPayload(
          query.includes('code!="OK"') || query.includes('status=~"5.."')
            ? "1"
            : "99",
        ),
      );
    });

    try {
      const response = await queryUsageGatewayMetrics(
        prometheus.origin,
        5_000,
        7,
        {
          currentGatewayCount: 2,
          currentGatewayNamespaces: [
            "openshell-gateway-1",
            "openshell-gateway-2",
          ],
          namespaces: ["openshell-gateway-1", "openshell-gateway-2"],
        },
      );

      expect(response).toMatchObject({
        active_gateways: 1,
        request_error_rate_percent: (2 / 198) * 100,
        request_volume: 198,
        total_gateways: 2,
        unused_gateways: 1,
      });
      expect(response.daily_request_volume).toHaveLength(7);
      expect(
        response.daily_request_volume?.filter((point) => point.value === null),
      ).toHaveLength(6);
      expect(response.coverage_start).toBe(response.coverage_end);

      for (const query of queries) {
        if (query.includes("openshell_server_")) {
          expect(query).toContain(
            'namespace=~"openshell-gateway-1|openshell-gateway-2"',
          );
        }
      }
      expect(queries.some((query) => query.includes('method!~"Peer.*"'))).toBe(
        true,
      );
    } finally {
      prometheus.close();
    }
  });

  it("calculates sandbox utilization from scoped active and total series", async () => {
    const queries: string[] = [];
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      queries.push(query);
      response.setHeader("content-type", "application/json");
      response.end(
        url.pathname === "/api/v1/query_range"
          ? rangePayload(query.includes("gateway_sandbox_total") ? "10" : "5")
          : instantPayload(
              query.includes("gateway_sandbox_total") ? "10" : "5",
            ),
      );
    });

    try {
      const response = await queryUsageSandboxMetrics(
        prometheus.origin,
        5_000,
        7,
        { namespaces: ["openshell-gateway-1"] },
      );

      expect(response).toMatchObject({
        active_sandboxes: 5,
        sandbox_utilization_percent: 50,
        total_sandboxes: 10,
      });
      expect(response.daily_utilization_percent).toHaveLength(7);
      expect(
        response.daily_utilization_percent?.filter(
          (point) => point.value === null,
        ),
      ).toHaveLength(6);
      for (const query of queries) {
        if (query.includes("hypershell_gateways_active_sandboxes_total")) {
          expect(query).toContain('gateway_namespace=~"openshell-gateway-1"');
        } else {
          expect(query).toContain('namespace=~"openshell-gateway-1"');
        }
      }
    } finally {
      prometheus.close();
    }
  });

  it("reports a zero error rate when traffic exists and error series are absent", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      response.setHeader("content-type", "application/json");

      if (query.includes('code!="OK"') || query.includes('status=~"5.."')) {
        response.end(
          url.pathname === "/api/v1/query_range"
            ? JSON.stringify({
                status: "success",
                data: { resultType: "matrix", result: [] },
              })
            : emptyInstantPayload(),
        );
        return;
      }

      response.end(
        url.pathname === "/api/v1/query_range"
          ? rangePayload("10")
          : instantPayload("10"),
      );
    });

    try {
      const response = await queryUsageGatewayMetrics(
        prometheus.origin,
        5_000,
        7,
        {
          currentGatewayCount: 1,
          currentGatewayNamespaces: ["openshell-gateway-1"],
        },
      );

      expect(response).toMatchObject({
        request_error_rate_percent: 0,
        request_volume: 20,
      });
      expect(response.daily_request_error_rate_percent).toHaveLength(7);
      expect(
        response.daily_request_error_rate_percent?.filter(
          (point) => point.value === 0,
        ),
      ).toHaveLength(1);
    } finally {
      prometheus.close();
    }
  });

  it("calculates sandbox lifetime mean and quantiles", async () => {
    const queries: string[] = [];
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      queries.push(query);
      response.setHeader("content-type", "application/json");
      const value = query.includes("0.95")
        ? "300"
        : query.includes("0.5")
          ? "100"
          : "120";
      response.end(
        url.pathname === "/api/v1/query_range"
          ? rangePayload(value)
          : instantPayload(value),
      );
    });

    try {
      const response = await queryUsageSandboxLifetime(
        prometheus.origin,
        5_000,
        7,
        ["openshell-gateway-1"],
      );

      expect(response).toMatchObject({
        mean_seconds: 120,
        p50_seconds: 100,
        p95_seconds: 300,
      });
      expect(response.daily_mean_seconds).toHaveLength(7);
      expect(
        response.daily_mean_seconds?.filter((point) => point.value === null),
      ).toHaveLength(6);
      const dailyMeanQuery = queries.find((query) => query.includes("[1d]"));
      expect(dailyMeanQuery).toContain("[1d]");
      expect(dailyMeanQuery?.match(/\[1d\]/gu)).toHaveLength(2);
      expect(dailyMeanQuery).not.toContain("[7d]");
    } finally {
      prometheus.close();
    }
  });
});
