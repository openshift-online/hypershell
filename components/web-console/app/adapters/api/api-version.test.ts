import { describe, expect, it, vi } from "vitest";

import { createApiVersionAdapter } from "./api-version";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

describe("API version adapter", () => {
  it("reads the API image version and preserves cancellation", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        jsonResponse({
          build_time: "2026-09-02T15:00:00Z",
          href: "/api/hypershell/v1/metadata",
          id: "hypershell",
          kind: "API",
          version: "v1.6.0-7654321",
        }),
      );
    const abortController = new AbortController();

    await expect(
      createApiVersionAdapter(fetchImplementation).readVersion(
        abortController.signal,
      ),
    ).resolves.toBe("v1.6.0-7654321");
    expect(fetchImplementation).toHaveBeenCalledWith(
      "/api/hypershell/v1/metadata",
      {
        credentials: "same-origin",
        headers: { accept: "application/json" },
        signal: abortController.signal,
      },
    );
  });

  it("rejects a failed metadata response", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(jsonResponse({ error: "unavailable" }, 503));

    await expect(
      createApiVersionAdapter(fetchImplementation).readVersion(),
    ).rejects.toThrow("API metadata request failed with 503");
  });

  it.each([null, {}, { version: "" }, { version: 42 }])(
    "rejects metadata without a version: %j",
    async (body) => {
      const fetchImplementation = vi
        .fn<typeof globalThis.fetch>()
        .mockResolvedValue(jsonResponse(body));

      await expect(
        createApiVersionAdapter(fetchImplementation).readVersion(),
      ).rejects.toThrow(/API metadata response/u);
    },
  );
});
