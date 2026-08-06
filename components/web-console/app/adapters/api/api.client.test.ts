import { describe, expect, it, vi } from "vitest";

import { createCorrelatedFetch, gatewayCorrelationHeader } from "./api.client";

describe("correlated API fetch", () => {
  it("adds the application invocation correlation identifier", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response(null, { status: 204 }));
    const correlatedFetch = createCorrelatedFetch(
      "11111111-1111-4111-8111-111111111111",
      fetchImplementation,
    );

    await correlatedFetch("/api/hypershell/v1/gateways", {
      headers: { accept: "application/json" },
    });

    expect(fetchImplementation).toHaveBeenCalledOnce();
    const init = fetchImplementation.mock.calls[0]?.[1];
    const headers = new Headers(init?.headers);
    expect(headers.get("accept")).toBe("application/json");
    expect(headers.get(gatewayCorrelationHeader)).toBe(
      "11111111-1111-4111-8111-111111111111",
    );
  });
});
