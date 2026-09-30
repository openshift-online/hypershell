import { describe, expect, it } from "vitest";

import {
  orderedInstances,
  phaseCount,
  totalGateways,
  type InstanceFleet,
} from "./fleet";

function inst(
  overrides: Partial<InstanceFleet> & { instance: string },
): InstanceFleet {
  return {
    role: null,
    provider: null,
    gateways: {},
    managedClusters: null,
    users: null,
    ...overrides,
  };
}

describe("totalGateways", () => {
  it("sums all phase counts", () => {
    expect(totalGateways({ Ready: 3, Provisioning: 2, Failed: 1 })).toBe(6);
  });

  it("is zero for an empty map", () => {
    expect(totalGateways({})).toBe(0);
  });
});

describe("phaseCount", () => {
  it("looks up a phase case-insensitively", () => {
    expect(phaseCount({ Ready: 5 }, "ready")).toBe(5);
  });

  it("returns zero for an absent phase", () => {
    expect(phaseCount({ Ready: 5 }, "failed")).toBe(0);
  });
});

describe("orderedInstances", () => {
  it("puts hubs last and sorts the rest by name", () => {
    const result = orderedInstances([
      inst({ instance: "z-spoke" }),
      inst({ instance: "the-hub", role: "hub" }),
      inst({ instance: "a-spoke" }),
    ]);
    expect(result.map((i) => i.instance)).toEqual([
      "a-spoke",
      "z-spoke",
      "the-hub",
    ]);
  });

  it("does not mutate its input", () => {
    const input = [inst({ instance: "b" }), inst({ instance: "a" })];
    orderedInstances(input);
    expect(input.map((i) => i.instance)).toEqual(["b", "a"]);
  });
});
