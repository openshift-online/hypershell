import { describe, expect, it, vi } from "vitest";

import {
  bulkDeleteConcurrencyLimit,
  deleteGatewaysBounded,
  isGatewayDeletable,
} from "./gateway-bulk-delete";

describe("isGatewayDeletable", () => {
  it("treats an explicit false capability as ineligible", () => {
    expect(isGatewayDeletable({ canDelete: false })).toBe(false);
  });

  it("treats a true or absent capability as eligible", () => {
    // An absent capability stays eligible because the API 403 remains the
    // authorization boundary (SEC-RBAC-10).
    expect(isGatewayDeletable({ canDelete: true })).toBe(true);
    expect(isGatewayDeletable({})).toBe(true);
  });
});

describe("deleteGatewaysBounded", () => {
  it("returns one settled outcome per target in input order", async () => {
    const targets = [
      { id: "a", name: "gw-a" },
      { id: "b", name: "gw-b" },
      { id: "c", name: "gw-c" },
    ];
    const remove = vi.fn((id: string) =>
      id === "b" ? Promise.reject(new Error("nope")) : Promise.resolve(),
    );

    const outcomes = await deleteGatewaysBounded(targets, remove, 2);

    expect(outcomes.map((outcome) => outcome.status)).toEqual([
      "accepted",
      "failed",
      "accepted",
    ]);
    expect(outcomes.map((outcome) => outcome.id)).toEqual(["a", "b", "c"]);
    expect(remove).toHaveBeenCalledTimes(3);
  });

  it("never exceeds the concurrency limit", async () => {
    const targets = Array.from({ length: 10 }, (_unused, index) => ({
      id: String(index),
      name: `gw-${String(index)}`,
    }));
    let inFlight = 0;
    let peak = 0;
    const remove = vi.fn(async () => {
      inFlight += 1;
      peak = Math.max(peak, inFlight);
      await Promise.resolve();
      await Promise.resolve();
      inFlight -= 1;
    });

    await deleteGatewaysBounded(targets, remove, 3);

    expect(peak).toBeLessThanOrEqual(3);
    expect(remove).toHaveBeenCalledTimes(10);
  });

  it("defaults to the shared concurrency limit and handles an empty set", async () => {
    expect(bulkDeleteConcurrencyLimit).toBeGreaterThan(0);
    const remove = vi.fn(() => Promise.resolve());
    await expect(deleteGatewaysBounded([], remove)).resolves.toEqual([]);
    expect(remove).not.toHaveBeenCalled();
  });
});
