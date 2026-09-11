import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import { queryPlatformInventory } from "../src/metrics-platform-inventory.js";

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

function prometheusScalarSample(value: string) {
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

describe("queryPlatformInventory", () => {
  it("aggregates labeled cluster and database inventory samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      const query = url.searchParams.get("query") ?? "";
      response.setHeader("content-type", "application/json");

      if (query === "hypershell_managed_clusters_total") {
        response.end(prometheusScalarSample("150"));
        return;
      }
      if (query === "hypershell_managed_clusters_created_last_30_days_total") {
        response.end(prometheusScalarSample("2"));
        return;
      }
      if (query === "hypershell_managed_databases_total") {
        response.end(prometheusScalarSample("2"));
        return;
      }
      if (query === "hypershell_managed_clusters_inventory_total") {
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  metric: {
                    provider: "aws",
                    region: "us-east-1",
                    status: "Ready",
                  },
                  value: ["1704067200", "4"],
                },
                {
                  metric: {
                    provider: "openshift",
                    region: "unknown",
                    status: "Failed",
                  },
                  value: ["1704067200", "50"],
                },
              ],
            },
          }),
        );
        return;
      }
      if (query === "hypershell_managed_databases_inventory_total") {
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [
                {
                  metric: { status: "Ready" },
                  value: ["1704067200", "1"],
                },
                {
                  metric: { status: "unknown" },
                  value: ["1704067200", "1"],
                },
              ],
            },
          }),
        );
        return;
      }

      response.statusCode = 404;
      response.end();
    });

    const result = await queryPlatformInventory(
      `http://127.0.0.1:${String(prometheus.port)}`,
      5_000,
    );

    expect(result.managed_clusters.total).toBe(150);
    expect(result.managed_clusters.created_last_30_days).toBe(2);
    expect(result.managed_clusters.by_status).toEqual({
      Failed: 50,
      Ready: 4,
    });
    expect(result.managed_databases.total).toBe(2);
    expect(result.managed_databases.by_status).toEqual({
      Ready: 1,
      unknown: 1,
    });

    prometheus.close();
  });

  it("scopes application inventory queries to the configured namespace", async () => {
    const queries: string[] = [];
    const prometheus = await startPrometheusStub((request, response) => {
      const query =
        new URL(request.url ?? "/", "http://127.0.0.1").searchParams.get(
          "query",
        ) ?? "";
      queries.push(query);
      response.setHeader("content-type", "application/json");
      if (query.includes("inventory_total")) {
        response.end(
          JSON.stringify({ status: "success", data: { result: [] } }),
        );
        return;
      }
      response.end(prometheusScalarSample("0"));
    });

    await queryPlatformInventory(
      `http://127.0.0.1:${String(prometheus.port)}`,
      5_000,
      "hyp1",
    );

    expect(queries).toEqual([
      'max(hypershell_managed_clusters_total{namespace="hyp1"})',
      'max(hypershell_managed_clusters_created_last_30_days_total{namespace="hyp1"})',
      'max by (status, provider, region) (hypershell_managed_clusters_inventory_total{namespace="hyp1"})',
      'max(hypershell_managed_databases_total{namespace="hyp1"})',
      'max by (status) (hypershell_managed_databases_inventory_total{namespace="hyp1"})',
    ]);

    prometheus.close();
  });

  it("does not report zero totals when scalar samples are missing", async () => {
    const prometheus = await startPrometheusStub((_request, response) => {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ status: "success", data: { result: [] } }));
    });

    await expect(
      queryPlatformInventory(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      ),
    ).rejects.toThrow("Prometheus query returned no samples");

    prometheus.close();
  });

  it("keeps repeated gauge samples from adding or overwriting totals", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const query =
        new URL(request.url ?? "/", "http://127.0.0.1").searchParams.get(
          "query",
        ) ?? "";
      response.setHeader("content-type", "application/json");
      if (query === "hypershell_managed_clusters_total") {
        response.end(
          JSON.stringify({
            status: "success",
            data: {
              result: [3, 3, 2].map((value) => ({
                metric: {},
                value: ["1704067200", String(value)],
              })),
            },
          }),
        );
        return;
      }
      if (query.includes("inventory_total")) {
        response.end(
          JSON.stringify({ status: "success", data: { result: [] } }),
        );
        return;
      }
      response.end(prometheusScalarSample("0"));
    });

    const result = await queryPlatformInventory(
      `http://127.0.0.1:${String(prometheus.port)}`,
      5_000,
    );

    expect(result.managed_clusters.total).toBe(3);

    prometheus.close();
  });
});
