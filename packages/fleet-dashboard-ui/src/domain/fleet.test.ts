import { describe, expect, it } from "vitest";

import {
  findInstance,
  gatewayTone,
  orderedInstances,
  otherGateways,
  phaseCount,
  totalGateways,
  ZERO_RATE,
  type InstanceFleet,
} from "./fleet";

function inst(
  overrides: Partial<InstanceFleet> & { instance: string },
): InstanceFleet {
  return {
    role: null,
    provider: null,
    gateways: {},
    gatewaysTotal: 0,
    gatewaysByCluster: [],
    managedClusters: null,
    users: null,
    rpc: ZERO_RATE,
    reconcile: ZERO_RATE,
    bff: ZERO_RATE,
    provisionP95Ms: null,
    gatewayHistory: [],
    sandboxes: 0,
    sandboxesByCluster: [],
    sandboxHistory: [],
    historyByCluster: [],
    logins: null,
    userHistory: [],
    loginsHistory: [],
    historyTimes: [],
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

describe("otherGateways", () => {
  it("counts gateways in phases beyond running/provisioning/failed", () => {
    // total 2, only 1 running -> 1 gateway in some other phase (e.g. deleting).
    expect(otherGateways({ running: 1, deleting: 1 })).toBe(1);
  });

  it("is zero when every gateway is in a named phase", () => {
    expect(otherGateways({ running: 2, provisioning: 1, failed: 1 })).toBe(0);
  });

  it("never goes negative", () => {
    expect(otherGateways({})).toBe(0);
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

describe("gatewayTone", () => {
  it("is unknown when there are no gateways", () => {
    expect(gatewayTone({})).toBe("unknown");
  });

  it("is danger when any gateway has failed", () => {
    expect(gatewayTone({ running: 4, provisioning: 1, failed: 2 })).toBe(
      "danger",
    );
  });

  it("is warning when provisioning but none failed", () => {
    expect(gatewayTone({ running: 4, provisioning: 1 })).toBe("warning");
  });

  it("is success when all gateways are settled", () => {
    expect(gatewayTone({ running: 4 })).toBe("success");
  });
});

describe("findInstance", () => {
  it("returns the matching instance record", () => {
    const list = [inst({ instance: "a" }), inst({ instance: "b" })];
    expect(findInstance(list, "b")?.instance).toBe("b");
  });

  it("returns null when no instance matches", () => {
    expect(findInstance([inst({ instance: "a" })], "missing")).toBeNull();
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
