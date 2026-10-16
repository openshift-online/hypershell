import { describe, expect, it, vi } from "vitest";

import type { AgentSdlcData } from "../domain/agent-sdlc";
import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import type { FleetApi, InstancesData, TopologyData } from "./ports";
import type { ObservabilityPort, SdlcWorkflowOutcome } from "./observability";
import {
  getFleet,
  getInstances,
  getPromotion,
  getSdlc,
  getTopology,
} from "./use-cases";

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
  const topology: TopologyData = {};
  const instances: InstancesData = { instances: [] };
  const sdlc: AgentSdlcData = {
    phases: [],
    cards: [],
    lane: [],
    matrix: { hubClusters: [], rows: [] },
  };
  return {
    getFleet: vi.fn().mockResolvedValue(plane(fleet)),
    getPromotion: vi.fn().mockResolvedValue(plane(promotion)),
    getTopology: vi.fn().mockResolvedValue(plane(topology)),
    getInstances: vi.fn().mockResolvedValue(plane(instances)),
    getSdlc: vi.fn().mockResolvedValue(plane(sdlc)),
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
          gatewaysByCluster: [],
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
          historyByCluster: [],
          logins: null,
          userHistory: [],
          loginsHistory: [],
          historyTimes: [],
        },
      ],
    });
    const api = fakeApi({ getFleet: vi.fn().mockResolvedValue(wanted) });
    await expect(getFleet(api)).resolves.toBe(wanted);
  });
});

function recordingObservability(): {
  port: ObservabilityPort;
  outcomes: SdlcWorkflowOutcome[];
} {
  const outcomes: SdlcWorkflowOutcome[] = [];
  return {
    outcomes,
    port: {
      publishSdlcWorkflow: (probe) => {
        outcomes.push(probe.outcome);
      },
    },
  };
}

describe("getSdlc observability", () => {
  it("publishes started then succeeded on a successful load", async () => {
    const { port, outcomes } = recordingObservability();
    await getSdlc(fakeApi(), undefined, port);
    expect(outcomes).toEqual(["started", "succeeded"]);
  });

  it("publishes started then failed when the port rejects", async () => {
    const { port, outcomes } = recordingObservability();
    const api = fakeApi({
      getSdlc: vi.fn().mockRejectedValue(new Error("boom")),
    });
    await expect(getSdlc(api, undefined, port)).rejects.toThrow("boom");
    expect(outcomes).toEqual(["started", "failed"]);
  });

  it("reports cancelled when the request was aborted", async () => {
    const { port, outcomes } = recordingObservability();
    const controller = new AbortController();
    controller.abort();
    const api = fakeApi({
      getSdlc: vi.fn().mockRejectedValue(new Error("aborted")),
    });
    await expect(getSdlc(api, controller.signal, port)).rejects.toThrow();
    expect(outcomes).toEqual(["started", "cancelled"]);
  });
});
