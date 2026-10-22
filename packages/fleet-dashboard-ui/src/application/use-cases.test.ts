import { describe, expect, it, vi } from "vitest";

import type { AgentSdlcData } from "../domain/agent-sdlc";
import type { E2eRunDetail, E2eRunsData } from "../domain/e2e-run";
import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import type { SdlcAgentsData } from "../domain/sdlc-agents";
import type { FleetApi, InstancesData, TopologyData } from "./ports";
import type {
  E2eWorkflowProbe,
  ObservabilityPort,
  SdlcWorkflowOutcome,
} from "./observability";
import {
  getE2eRun,
  getE2eRuns,
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
  const e2eRuns: E2eRunsData = {
    enabled: true,
    runs: [],
    truncated: false,
    windowSeconds: 0,
    limit: 0,
  };
  const e2eRun: E2eRunDetail = {
    id: "t/n/r",
    runId: "r",
    tenant: "t",
    cluster: "t",
    bodyCluster: null,
    namespace: "n",
    revision: "",
    sourceRevision: null,
    startedAt: "",
    endedAt: null,
    durationMs: null,
    verdict: "unknown",
    reportingComplete: false,
    hasStructuredSteps: false,
    stepCount: 0,
    plannedSteps: [],
    steps: [],
    logs: [],
    logsTruncated: false,
  };
  const sdlcAgents: SdlcAgentsData = { envs: [], agents: [] };
  return {
    getFleet: vi.fn().mockResolvedValue(plane(fleet)),
    getPromotion: vi.fn().mockResolvedValue(plane(promotion)),
    getTopology: vi.fn().mockResolvedValue(plane(topology)),
    getInstances: vi.fn().mockResolvedValue(plane(instances)),
    getSdlc: vi.fn().mockResolvedValue(plane(sdlc)),
    getSdlcAgents: vi.fn().mockResolvedValue(plane(sdlcAgents)),
    getE2eRuns: vi.fn().mockResolvedValue(plane(e2eRuns)),
    getE2eRun: vi.fn().mockResolvedValue(plane(e2eRun)),
    getE2eRunLogs: vi
      .fn()
      .mockResolvedValue(
        plane({ entries: [], nextCursor: null, hasMore: false }),
      ),
    streamE2eRun: vi.fn().mockReturnValue(() => {
      /* unsubscribe */
    }),
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
  e2e: E2eWorkflowProbe[];
} {
  const outcomes: SdlcWorkflowOutcome[] = [];
  const e2e: E2eWorkflowProbe[] = [];
  return {
    outcomes,
    e2e,
    port: {
      publishSdlcWorkflow: (probe) => {
        outcomes.push(probe.outcome);
      },
      publishE2eWorkflow: (probe) => {
        e2e.push(probe);
      },
      publishE2eStream: () => {
        /* not asserted here */
      },
    },
  };
}

describe("getSdlc observability", () => {
  // Verifies: SDLC-VIS-15.01
  it("publishes started then succeeded on a successful load", async () => {
    const { port, outcomes } = recordingObservability();
    await getSdlc(fakeApi(), undefined, port);
    expect(outcomes).toEqual(["started", "succeeded"]);
  });

  // Verifies: SDLC-VIS-15.01
  it("publishes started then failed when the port rejects", async () => {
    const { port, outcomes } = recordingObservability();
    const api = fakeApi({
      getSdlc: vi.fn().mockRejectedValue(new Error("boom")),
    });
    await expect(getSdlc(api, undefined, port)).rejects.toThrow("boom");
    expect(outcomes).toEqual(["started", "failed"]);
  });

  // Verifies: SDLC-VIS-15.01
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

describe("getE2eRuns / getE2eRun observability", () => {
  it("delegates to the port and passes the abort signal through", async () => {
    const api = fakeApi();
    const controller = new AbortController();
    await getE2eRuns(api, controller.signal);
    await getE2eRun(api, "t/n/r", controller.signal);
    /* eslint-disable @typescript-eslint/unbound-method -- asserting on vi.fn() mocks */
    expect(api.getE2eRuns).toHaveBeenCalledWith(controller.signal);
    expect(api.getE2eRun).toHaveBeenCalledWith("t/n/r", controller.signal);
    /* eslint-enable @typescript-eslint/unbound-method */
  });

  it("publishes started then succeeded for both loads", async () => {
    const { port, e2e } = recordingObservability();
    await getE2eRuns(fakeApi(), undefined, port);
    await getE2eRun(fakeApi(), "t/n/r", undefined, port);
    expect(e2e).toEqual([
      { name: "e2e.runs.load", outcome: "started" },
      { name: "e2e.runs.load", outcome: "succeeded" },
      { name: "e2e.run.load", outcome: "started" },
      { name: "e2e.run.load", outcome: "succeeded" },
    ]);
  });

  it("reports failed and cancelled outcomes", async () => {
    const { port, e2e } = recordingObservability();
    const failing = fakeApi({
      getE2eRuns: vi.fn().mockRejectedValue(new Error("boom")),
    });
    await expect(getE2eRuns(failing, undefined, port)).rejects.toThrow("boom");

    const controller = new AbortController();
    controller.abort();
    const aborting = fakeApi({
      getE2eRun: vi.fn().mockRejectedValue(new Error("aborted")),
    });
    await expect(
      getE2eRun(aborting, "t/n/r", controller.signal, port),
    ).rejects.toThrow();

    expect(e2e).toEqual([
      { name: "e2e.runs.load", outcome: "started" },
      { name: "e2e.runs.load", outcome: "failed" },
      { name: "e2e.run.load", outcome: "started" },
      { name: "e2e.run.load", outcome: "cancelled" },
    ]);
  });
});
