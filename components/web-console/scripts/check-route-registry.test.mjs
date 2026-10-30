import { describe, expect, it } from "vitest";

import {
  checkRouteContract,
  checkRouteRegistryUsage,
} from "./check-route-registry.mjs";

describe("route registry checks", () => {
  it("keeps the browser route contract synchronized", async () => {
    await expect(checkRouteContract()).resolves.toBeUndefined();
  });

  it("rejects hard-coded production routes outside the registry", async () => {
    await expect(checkRouteRegistryUsage()).resolves.toBeUndefined();
  });
});
