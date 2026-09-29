import { createServer } from "node:http";

import { afterEach, describe, expect, it } from "vitest";

import {
  queryControlPlaneReconciliation,
  reconciliationFailureCountPromql,
  reconciliationLagP50SecondsPromql,
  reconciliationRetryCountPromql,
  staleResourceStatusCountPromql,
} from "../src/metrics-control-plane-reconciliation.js";

describe("queryControlPlaneReconciliation", () => {
  let server: ReturnType<typeof createServer> | undefined;

  afterEach(() => server?.close());

  it("keeps valid zero counts when an idle lag histogram returns NaN or empty", async () => {
    server = createServer((request, response) => {
      const query = new URL(
        request.url ?? "",
        "http://127.0.0.1",
      ).searchParams.get("query");
      response.setHeader("content-type", "application/json");
      if (request.url?.startsWith("/api/v1/query_range")) {
        response.end(
          JSON.stringify({ status: "success", data: { result: [] } }),
        );
        return;
      }
      const value = query === reconciliationLagP50SecondsPromql ? "NaN" : "0";
      response.end(
        JSON.stringify({
          status: "success",
          data: {
            result: query === "not-used" ? [] : [{ value: ["1", value] }],
          },
        }),
      );
    });
    await new Promise<void>((resolve) =>
      server?.listen(0, "127.0.0.1", resolve),
    );
    const address = server.address();
    if (address === null || typeof address === "string")
      throw new Error("expected listener");

    const metrics = await queryControlPlaneReconciliation(
      `http://127.0.0.1:${String(address.port)}`,
      5_000,
    );
    expect(metrics).toMatchObject({
      reconciliation_failures_count: 0,
      reconciliation_retries_count: 0,
      stale_resource_status_count: 0,
    });
    expect(metrics.reconciliation_lag_p50_seconds).toBeUndefined();
    expect(reconciliationFailureCountPromql).toContain("increase");
    expect(reconciliationRetryCountPromql).toContain("increase");
    expect(staleResourceStatusCountPromql).toContain("stale");
  });
});
