import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchGatewayMetrics } from "./gateway-metrics-data";

describe("fetchGatewayMetrics", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns phase counts from a successful response", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          counts: {
            Running: 5,
            Provisioning: 2,
            Degraded: 1,
            Failed: 0,
          },
        }),
        { status: 200 },
      ),
    );

    await expect(fetchGatewayMetrics()).resolves.toEqual({
      Pending: 0,
      Running: 5,
      Provisioning: 2,
      Degraded: 1,
      Failed: 0,
    });
    expect(fetch).toHaveBeenCalledWith("/api/metrics/gateways", {
      credentials: "same-origin",
      signal: undefined,
    });
  });

  it("defaults absent phases to zero", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ counts: { Running: 3 } }), {
        status: 200,
      }),
    );

    await expect(fetchGatewayMetrics()).resolves.toEqual({
      Pending: 0,
      Running: 3,
      Provisioning: 0,
      Degraded: 0,
      Failed: 0,
    });
  });

  it("returns optional fleet totals and forwards the selected time range", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          counts: { Running: 4 },
          daily_fleet_totals: [
            { date: "2026-09-01", total: 3 },
            { date: "2026-09-02", total: 4 },
          ],
        }),
        { status: 200 },
      ),
    );

    await expect(fetchGatewayMetrics(undefined, 30)).resolves.toEqual({
      Pending: 0,
      Running: 4,
      Provisioning: 0,
      Degraded: 0,
      Failed: 0,
      dailyFleetTotals: [
        { date: "2026-09-01", total: 3 },
        { date: "2026-09-02", total: 4 },
      ],
    });
    expect(fetch).toHaveBeenCalledWith("/api/metrics/gateways?range=30", {
      credentials: "same-origin",
      signal: undefined,
    });
  });

  it("throws when the response is not ok", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 502 }),
    );

    await expect(fetchGatewayMetrics()).rejects.toThrow(
      "Failed to fetch gateway metrics: 502",
    );
  });

  it("forwards an abort signal to fetch", async () => {
    const controller = new AbortController();
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          counts: {
            Pending: 0,
            Running: 1,
            Provisioning: 0,
            Degraded: 0,
            Failed: 0,
          },
        }),
        { status: 200 },
      ),
    );

    await fetchGatewayMetrics(controller.signal);

    expect(fetch).toHaveBeenCalledWith("/api/metrics/gateways", {
      credentials: "same-origin",
      signal: controller.signal,
    });
  });
});
