import { describe, expect, it, vi } from "vitest";

import {
  checkExternalLinkRegistryUsage,
  checkExternalLinks,
  validateExternalLinkRegistry,
} from "./check-external-links.mjs";

describe("external link checks", () => {
  it("validates registry integrity", () => {
    expect(() =>
      validateExternalLinkRegistry({
        example: "https://example.com",
        duplicate: "https://example.com",
      }),
    ).toThrow("duplicate URL");
    expect(() =>
      validateExternalLinkRegistry({ example: "http://example.com" }),
    ).toThrow("HTTPS URL");
  });

  it("rejects a production literal HTTPS URL outside the registry", async () => {
    await expect(checkExternalLinkRegistryUsage()).resolves.toBeUndefined();
  });

  it("fails a 404 response", async () => {
    const results = await checkExternalLinks({
      fetchImplementation: async () => ({ status: 404 }),
      registry: { example: "https://example.com/missing" },
    });

    expect(results).toEqual([
      {
        error: "HTTP 404",
        name: "example",
        ok: false,
        status: 404,
        url: "https://example.com/missing",
      },
    ]);
  });

  it("falls back to GET when HEAD is rejected", async () => {
    const calls = [];
    const results = await checkExternalLinks({
      fetchImplementation: async (_url, init) => {
        calls.push(init.method);
        return { status: init.method === "HEAD" ? 405 : 200 };
      },
      registry: { example: "https://example.com" },
    });

    expect(calls).toEqual(["HEAD", "GET"]);
    expect(results[0]).toMatchObject({ ok: true, status: 200 });
  });

  it("skips local command endpoints without a network request", async () => {
    const fetchImplementation = vi.fn();
    const results = await checkExternalLinks({
      fetchImplementation,
      registry: { localEndpoint: "https://inference.local" },
    });

    expect(fetchImplementation).not.toHaveBeenCalled();
    expect(results).toEqual([
      {
        name: "localEndpoint",
        ok: true,
        status: undefined,
        url: "https://inference.local",
        skipped: true,
      },
    ]);
  });
});
