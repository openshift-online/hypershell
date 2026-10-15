import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import { describe, expect, it } from "vitest";

import { queryAuthorizedGatewayMetrics } from "../src/authorized-gateway-metrics.js";

async function startApiStub(
  handler: (request: IncomingMessage, response: ServerResponse) => void,
): Promise<{ close: () => void; origin: string }> {
  const server = createServer(handler);
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("expected tcp address for API stub");
  }

  return {
    close: () => server.close(),
    origin: `http://127.0.0.1:${String(address.port)}`,
  };
}

describe("queryAuthorizedGatewayMetrics", () => {
  it("aggregates only the authorized gateway list", async () => {
    const api = await startApiStub((request, response) => {
      const url = new URL(request.url ?? "/", "http://api.invalid");
      expect(url.pathname).toBe("/api/hypershell/v1/gateways");
      expect(request.headers.authorization).toBe("Bearer user-token");
      expect(url.searchParams.get("page")).toBe("1");
      expect(url.searchParams.get("size")).toBe("100");

      response.setHeader("content-type", "application/json");
      response.end(
        JSON.stringify({
          items: [
            {
              active_sandbox_count: 2,
              namespace: "openshell-gateway-1",
              phase: "Running",
            },
            {
              active_sandbox_count: 1,
              namespace: "openshell-gateway-1",
              phase: "Running",
            },
            {
              active_sandbox_count: 0,
              namespace: "openshell-gateway-2",
              phase: "Failed",
            },
          ],
          page: 1,
          total: 3,
        }),
      );
    });

    try {
      await expect(
        queryAuthorizedGatewayMetrics(api.origin, "user-token", 5_000),
      ).resolves.toEqual({
        activeSandboxes: 3,
        counts: {
          Degraded: 0,
          Failed: 1,
          Pending: 0,
          Provisioning: 0,
          Running: 2,
        },
        total: 3,
      });
    } finally {
      api.close();
    }
  });

  it("rejects an inconsistent gateway list page", async () => {
    const api = await startApiStub((_request, response) => {
      response.setHeader("content-type", "application/json");
      response.end(
        JSON.stringify({
          items: [
            { active_sandbox_count: 1, phase: "Running" },
            { active_sandbox_count: 1, phase: "Running" },
          ],
          page: 2,
          total: 2,
        }),
      );
    });

    try {
      await expect(
        queryAuthorizedGatewayMetrics(api.origin, undefined, 5_000),
      ).rejects.toThrow("Authorized gateway list response was inconsistent");
    } finally {
      api.close();
    }
  });
});
