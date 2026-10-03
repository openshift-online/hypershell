import { describe, expect, it, vi } from "vitest";

import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import type { FleetApi, InstancesData, TopologyData } from "./ports";
import { getFleet, getInstances, getPromotion, getTopology } from "./use-cases";

function plane<T>(data: T): Plane<T> {
  return {
    data,
    generatedAt: "2026-01-01T00:00:00Z",
    stale: false,
    error: null,
  };
}

function fakeApi(overrides: Partial<FleetApi> = {}): FleetApi {
  const fleet: FleetData = { instances: [] };
  const promotion: PromotionData = {
    order: [],
    environments: {},
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
  const topology: TopologyData = { nodes: [], edges: [] };
  const instances: InstancesData = { instances: [] };
  return {
    getFleet: vi.fn().mockResolvedValue(plane(fleet)),
    getPromotion: vi.fn().mockResolvedValue(plane(promotion)),
    getTopology: vi.fn().mockResolvedValue(plane(topology)),
    getInstances: vi.fn().mockResolvedValue(plane(instances)),
    ...overrides,
  };
}

describe("use cases", () => {
  it("delegate to the port and pass the abort signal through", async () => {
    const api = fakeApi();
    const controller = new AbortController();

    await getFleet(api, controller.signal);
    await getPromotion(api, controller.signal);
    await getTopology(api, controller.signal);
    await getInstances(api, controller.signal);

    /* eslint-disable @typescript-eslint/unbound-method -- asserting on vi.fn() mocks, not calling them */
    expect(api.getFleet).toHaveBeenCalledWith(controller.signal);
    expect(api.getPromotion).toHaveBeenCalledWith(controller.signal);
    expect(api.getTopology).toHaveBeenCalledWith(controller.signal);
    expect(api.getInstances).toHaveBeenCalledWith(controller.signal);
    /* eslint-enable @typescript-eslint/unbound-method */
  });

  it("returns the plane produced by the port unchanged", async () => {
    const wanted = plane<FleetData>({
      instances: [
        {
          instance: "some-inst",
          role: null,
          provider: null,
          gateways: { Ready: 1 },
          gatewaysTotal: 1,
          managedClusters: null,
          users: null,
          rpc: { rate: 0, errorPct: 0, p95Ms: 0 },
          reconcile: { rate: 0, errorPct: 0, p95Ms: 0 },
          bff: { rate: 0, errorPct: 0, p95Ms: 0 },
          provisionP95Ms: null,
          gatewayHistory: [],
          sandboxes: 0,
          sandboxesByCluster: [],
          sandboxHistory: [],
        },
      ],
    });
    const api = fakeApi({ getFleet: vi.fn().mockResolvedValue(wanted) });
    await expect(getFleet(api)).resolves.toBe(wanted);
  });
});
