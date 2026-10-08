import { describe, expect, it, vi } from "vitest";

import { createHttpFleetApi, HttpError } from "./http-fleet-api";

function okResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("createHttpFleetApi", () => {
  it("requests the fleet route under the injected base path", async () => {
    // /api/fleet is a map keyed by instance name on the wire; the mapper flattens
    // it to a sorted list.
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: {},
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    const plane = await api.getFleet();

    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/fleet",
      expect.objectContaining({ headers: { Accept: "application/json" } }),
    );
    expect(plane.data.instances).toEqual([]);
  });

  it("strips a trailing slash from the base path", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: { order: [], environments: {}, releases: [] },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api/", fetchImpl });

    await api.getPromotion();

    expect(fetchImpl).toHaveBeenCalledWith("/api/promotion", expect.anything());
  });

  it("forwards the abort signal", async () => {
    const controller = new AbortController();
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: {},
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    await api.getTopology(controller.signal);

    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/topology",
      expect.objectContaining({ signal: controller.signal }),
    );
  });

  it("throws HttpError on a non-2xx response", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(new Response("nope", { status: 503 }));
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    await expect(api.getInstances()).rejects.toBeInstanceOf(HttpError);
  });
});
