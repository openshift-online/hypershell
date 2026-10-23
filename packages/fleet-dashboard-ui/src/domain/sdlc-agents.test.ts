import { describe, expect, it } from "vitest";
import {
  agentRowsByEnv,
  latestSample,
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

describe("latestSample", () => {
  it("returns the last element of a non-empty array", () => {
    expect(latestSample([1, 2, 3, 4])).toBe(4);
  });

  it("returns 0 for an empty array", () => {
    expect(latestSample([])).toBe(0);
  });

  it("returns the single element of a one-element array", () => {
    expect(latestSample([42])).toBe(42);
  });
});
