import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import {
  clusterNodesReadyPromql,
  clusterNodesTotalPromql,
  queryClusterNodes,
} from "../src/metrics-cluster-nodes.js";

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

describe("queryClusterNodes", () => {
  it("maps Prometheus total and ready samples into node counts", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterNodesTotalPromql) {
        response.end(prometheusSample("8"));
        return;
      }
      if (query === clusterNodesReadyPromql) {
        response.end(prometheusSample("7"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      const nodes = await queryClusterNodes(
        `http://127.0.0.1:${String(prometheus.port)}`,
        5_000,
      );
      expect(nodes).toEqual({
        not_ready_nodes: 1,
        ready_nodes: 7,
        total_nodes: 8,
      });
    } finally {
      prometheus.close();
    }
  });

  it("fails when Prometheus returns no total node samples", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterNodesTotalPromql) {
        response.end(
          JSON.stringify({
            status: "success",
            data: { result: [] },
          }),
        );
        return;
      }
      if (query === clusterNodesReadyPromql) {
        response.end(prometheusSample("0"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterNodes(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("No cluster node data");
    } finally {
      prometheus.close();
    }
  });

  it("fails when ready exceeds total", async () => {
    const prometheus = await startPrometheusStub((request, response) => {
      const url = new URL(request.url ?? "", "http://127.0.0.1");
      const query = url.searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (query === clusterNodesTotalPromql) {
        response.end(prometheusSample("3"));
        return;
      }
      if (query === clusterNodesReadyPromql) {
        response.end(prometheusSample("4"));
        return;
      }
      response.statusCode = 400;
      response.end();
    });

    try {
      await expect(
        queryClusterNodes(`http://127.0.0.1:${String(prometheus.port)}`, 5_000),
      ).rejects.toThrow("Inconsistent cluster node samples");
    } finally {
      prometheus.close();
    }
  });
});
