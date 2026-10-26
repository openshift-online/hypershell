import { describe, expect, it } from "vitest";
import {
  agentActivity,
  agentRowsByEnv,
  aggregateAgentsByName,
  HEARTBEAT_STALE,
  type SdlcAgentMetrics,
  type SdlcAgentsData,
} from "./sdlc-agents";

const EMPTY_SERIES = {
  times: [],
  queueDepth: [],
  itemsProcessed: [],
  itemsFailed: [],
  provisionSeconds: [],
};

function makeAgent(
  name: string,
  env: string,
  overrides?: Partial<SdlcAgentMetrics>,
): SdlcAgentMetrics {
  return {
    name,
    env,
    replicasOn: 1,
    replicasTotal: 2,
    queueDepth: 0,
    itemsProcessed: 10,
    itemsFailed: 0,
    provisionSeconds: 5,
    series: EMPTY_SERIES,
    ...overrides,
  };
}

describe("agentRowsByEnv", () => {
  it("groups agents by environment key", () => {
    const data: SdlcAgentsData = {
      envs: ["integration", "production"],
      agents: [
        makeAgent("triage", "integration"),
        makeAgent("spec-review", "integration"),
        makeAgent("triage", "production"),
      ],
    };
    const result = agentRowsByEnv(data);
    expect(result.get("integration")).toHaveLength(2);
    expect(result.get("production")).toHaveLength(1);
    expect(result.has("staging")).toBe(false);
  });

  it("returns an empty map for empty agents", () => {
    const data: SdlcAgentsData = { envs: [], agents: [] };
    const result = agentRowsByEnv(data);
    expect(result.size).toBe(0);
  });

  it("preserves agent order within each env group", () => {
    const data: SdlcAgentsData = {
      envs: ["integration"],
      agents: [
        makeAgent("triage", "integration"),
        makeAgent("spec-review", "integration"),
        makeAgent("code-review", "integration"),
      ],
    };
    const result = agentRowsByEnv(data);
    const names = result.get("integration")?.map((a) => a.name);
    expect(names).toEqual(["triage", "spec-review", "code-review"]);
  });
});

describe("aggregateAgentsByName", () => {
  it("passes a single-env agent through unchanged", () => {
    const agent = makeAgent("triage", "integration", { queueDepth: 4 });
    const data: SdlcAgentsData = { envs: ["integration"], agents: [agent] };
    const result = aggregateAgentsByName(data);
    expect(result.get("triage")).toBe(agent);
  });

  it("sums scalar counts across environments instead of dropping one", () => {
    const data: SdlcAgentsData = {
      envs: ["integration", "production"],
      agents: [
        makeAgent("spec-review", "integration", {
          replicasOn: 1,
          replicasTotal: 2,
          queueDepth: 3,
          itemsProcessed: 10,
          itemsFailed: 1,
        }),
        makeAgent("spec-review", "production", {
          replicasOn: 2,
          replicasTotal: 2,
          queueDepth: 5,
          itemsProcessed: 20,
          itemsFailed: 2,
        }),
      ],
    };
    const result = aggregateAgentsByName(data);
    const agent = result.get("spec-review");
    expect(agent?.replicasOn).toBe(3);
    expect(agent?.replicasTotal).toBe(4);
    expect(agent?.queueDepth).toBe(8);
    expect(agent?.itemsProcessed).toBe(30);
    expect(agent?.itemsFailed).toBe(3);
    expect(agent?.env).toBe("integration+production");
  });

  it("carries the most recent lastReportedAt across environments", () => {
    const data: SdlcAgentsData = {
      envs: ["integration", "production"],
      agents: [
        makeAgent("spec-review", "integration", { lastReportedAt: 1000 }),
        makeAgent("spec-review", "production", { lastReportedAt: 2000 }),
      ],
    };
    const result = aggregateAgentsByName(data);
    expect(result.get("spec-review")?.lastReportedAt).toBe(2000);
  });

  it("merges history series onto the union of timestamps", () => {
    const data: SdlcAgentsData = {
      envs: ["integration", "production"],
      agents: [
        makeAgent("spec-review", "integration", {
          series: {
            times: [100, 200],
            queueDepth: [1, 2],
            itemsProcessed: [0, 0],
            itemsFailed: [0, 0],
            provisionSeconds: [0, 0],
          },
        }),
        // production only reports at ts 200 - ts 100 must zero-fill for it.
        makeAgent("spec-review", "production", {
          series: {
            times: [200],
            queueDepth: [5],
            itemsProcessed: [0],
            itemsFailed: [0],
            provisionSeconds: [0],
          },
        }),
      ],
    };
    const result = aggregateAgentsByName(data);
    const series = result.get("spec-review")?.series;
    expect(series?.times).toEqual([100, 200]);
    expect(series?.queueDepth).toEqual([1, 7]);
  });
});

describe("agentActivity", () => {
  // A fixed clock; lastReportedAt values are expressed relative to it in seconds.
  const NOW_MS = 1_700_000_000_000;
  const nowS = NOW_MS / 1000;
  const fresh = nowS - 10; // well within the liveness window
  const decayed = nowS - (HEARTBEAT_STALE + 60); // older than the window

  const cases: {
    label: string;
    agent: Partial<SdlcAgentMetrics>;
    planeStale: boolean;
    want: "active" | "notActive" | "blind";
  }[] = [
    // SDLC-AGT-16.01: a stale plane is blind regardless of the gauge.
    {
      label: "stale plane -> blind",
      agent: { replicasOn: 1, lastReportedAt: fresh },
      planeStale: true,
      want: "blind",
    },
    // SDLC-AGT-16.02: running with a fresh timestamp -> active.
    {
      label: "replicasOn + fresh timestamp -> active",
      agent: { replicasOn: 1, lastReportedAt: fresh },
      planeStale: false,
      want: "active",
    },
    // SDLC-AGT-16.03: running with a stale timestamp (decayed crash) -> notActive.
    {
      label: "replicasOn + stale timestamp -> notActive",
      agent: { replicasOn: 1, lastReportedAt: decayed },
      planeStale: false,
      want: "notActive",
    },
    // SDLC-AGT-16.04: no running replicas -> notActive.
    {
      label: "replicasOn = 0 -> notActive",
      agent: { replicasOn: 0, lastReportedAt: fresh },
      planeStale: false,
      want: "notActive",
    },
    // SDLC-AGT-16.05: no timestamp yet, falls back to the raw gauge.
    {
      label: "replicasOn + no timestamp -> active (fallback)",
      agent: { replicasOn: 1, lastReportedAt: 0 },
      planeStale: false,
      want: "active",
    },
    {
      label: "replicasOn = 0 + no timestamp -> notActive (fallback)",
      agent: { replicasOn: 0, lastReportedAt: 0 },
      planeStale: false,
      want: "notActive",
    },
  ];

  for (const c of cases) {
    it(c.label, () => {
      const agent = makeAgent("triage", "integration", c.agent);
      expect(agentActivity(agent, NOW_MS, c.planeStale)).toBe(c.want);
    });
  }

  it("treats a missing lastReportedAt field as the raw-gauge fallback", () => {
    const agent = makeAgent("triage", "integration", { replicasOn: 1 });
    // makeAgent leaves lastReportedAt undefined (optional field).
    expect(agent.lastReportedAt).toBeUndefined();
    expect(agentActivity(agent, NOW_MS, false)).toBe("active");
  });
});
