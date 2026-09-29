import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  attentionFleetQuery,
  gatewayActiveSandboxesPromql,
  gatewayExpiringSandboxesPromql,
  gatewayIdleSandboxesPromql,
  gatewayOrphanedSandboxesPromql,
  queryGatewaySandboxes,
} from "../src/metrics-gateway-sandboxes.js";

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

function instantPayload(value: string) {
  return JSON.stringify({
    status: "success",
    data: {
      resultType: "vector",
      result: [{ metric: {}, value: ["1704067200", value] }],
    },
  });
}

describe("queryGatewaySandboxes", () => {
  it("exports the documented attention PromQL contracts", () => {
    expect(gatewayActiveSandboxesPromql).toBe(
      "hypershell_gateways_active_sandboxes_total",
    );
    expect(gatewayOrphanedSandboxesPromql).toBe("gateway_sandbox_orphaned");
    expect(gatewayExpiringSandboxesPromql).toBe("gateway_sandbox_expiring");
    expect(gatewayIdleSandboxesPromql).toBe("gateway_sandbox_idle");
    expect(attentionFleetQuery(gatewayOrphanedSandboxesPromql)).toBe(
      "sum(max by (hypershell_cluster_id) (gateway_sandbox_orphaned))",
    );
    expect(attentionFleetQuery(gatewayOrphanedSandboxesPromql, "hyp1")).toBe(
      'sum(max by (hypershell_cluster_id) (gateway_sandbox_orphaned{k8s_namespace_name="hyp1"}))',
    );
  });

  it("scopes attention queries with k8s_namespace_name when namespace is set", async () => {
    const queries: string[] = [];
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        response.end(
          JSON.stringify({
            status: "success",
            data: { resultType: "matrix", result: [] },
          }),
        );
        return;
      }
      queries.push(url.searchParams.get("query") ?? "");
      response.end(instantPayload("1"));
    });

    try {
      await queryGatewaySandboxes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
        "hyp1",
      );
      const attentionQueries = queries.filter((query) =>
        query.includes("gateway_sandbox_"),
      );
      expect(attentionQueries).toHaveLength(3);
      for (const query of attentionQueries) {
        expect(query).toContain('{k8s_namespace_name="hyp1"}');
        expect(query).toContain("max by (hypershell_cluster_id)");
        expect(query).not.toContain("{namespace=");
      }
    } finally {
      prometheus.close();
    }
  });

  it("includes Active and all attention fields on success", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        response.end(
          JSON.stringify({
            status: "success",
            data: { resultType: "matrix", result: [] },
          }),
        );
        return;
      }
      const query = url.searchParams.get("query") ?? "";
      if (query.includes("orphaned")) {
        response.end(instantPayload("3"));
        return;
      }
      if (query.includes("expiring")) {
        response.end(instantPayload("12"));
        return;
      }
      if (query.includes("idle")) {
        response.end(instantPayload("7"));
        return;
      }
      response.end(instantPayload("214"));
    });

    try {
      const counts = await queryGatewaySandboxes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(counts.active_sandboxes).toBe(214);
      expect(counts.orphaned_sandboxes).toBe(3);
      expect(counts.expiring_sandboxes).toBe(12);
      expect(counts.idle_sandboxes).toBe(7);
    } finally {
      prometheus.close();
    }
  });

  it("omits only failed attention fields when Active succeeds", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        response.end(
          JSON.stringify({
            status: "success",
            data: { resultType: "matrix", result: [] },
          }),
        );
        return;
      }
      const query = url.searchParams.get("query") ?? "";
      if (
        query.includes("orphaned") ||
        query.includes("expiring") ||
        query.includes("idle")
      ) {
        response.statusCode = 500;
        response.end("boom");
        return;
      }
      response.end(instantPayload("8"));
    });

    try {
      const counts = await queryGatewaySandboxes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(counts.active_sandboxes).toBe(8);
      expect(counts.orphaned_sandboxes).toBeUndefined();
      expect(counts.expiring_sandboxes).toBeUndefined();
      expect(counts.idle_sandboxes).toBeUndefined();
    } finally {
      prometheus.close();
    }
  });

  it("keeps successful attention fields when peers fail in parallel", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      response.setHeader("content-type", "application/json");
      if (url.pathname === "/api/v1/query_range") {
        response.end(
          JSON.stringify({
            status: "success",
            data: { resultType: "matrix", result: [] },
          }),
        );
        return;
      }
      const query = url.searchParams.get("query") ?? "";
      if (query.includes("expiring") || query.includes("idle")) {
        response.statusCode = 500;
        response.end("boom");
        return;
      }
      if (query.includes("orphaned")) {
        response.end(instantPayload("5"));
        return;
      }
      response.end(instantPayload("9"));
    });

    try {
      const counts = await queryGatewaySandboxes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(counts.active_sandboxes).toBe(9);
      expect(counts.orphaned_sandboxes).toBe(5);
      expect(counts.expiring_sandboxes).toBeUndefined();
      expect(counts.idle_sandboxes).toBeUndefined();
    } finally {
      prometheus.close();
    }
  });
});
