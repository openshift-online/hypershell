import { describe, expect, it } from "vitest";

import {
  agingRatio,
  agingScaleSeconds,
  boardPhases,
  bundlePRCount,
  bundleRowExpandKey,
  bundleRowSeed,
  cardsInPhase,
  PHASE_SHIPPED,
  reworkRatio,
  reworkSeverity,
  UNRELEASED_ROW_KEY,
  type AgentSdlcCard,
  type AgentSdlcPhase,
  type AgentSdlcPullRequest,
} from "./agent-sdlc";

function card(partial: Partial<AgentSdlcCard>): AgentSdlcCard {
  return {
    number: 1,
    title: "t",
    url: "u",
    phase: "spec",
    timeInPhaseSeconds: 0,
    scope: null,
    escalation: null,
    rework: null,
    history: [],
    ...partial,
  };
}

describe("boardPhases", () => {
  it("drops the terminal Shipped column", () => {
    const phases: AgentSdlcPhase[] = [
      { key: "spec", sleSeconds: 10 },
      { key: PHASE_SHIPPED, sleSeconds: 0 },
    ];
    expect(boardPhases(phases).map((p) => p.key)).toEqual(["spec"]);
  });
});

describe("cardsInPhase", () => {
  it("keeps only the column's cards, freshest first", () => {
    const cards = [
      card({ number: 1, phase: "spec", timeInPhaseSeconds: 100 }),
      card({ number: 2, phase: "code", timeInPhaseSeconds: 5 }),
      card({ number: 3, phase: "spec", timeInPhaseSeconds: 10 }),
    ];
    expect(cardsInPhase(cards, "spec").map((c) => c.number)).toEqual([3, 1]);
  });
});

describe("agingScaleSeconds / agingRatio", () => {
  it("spans a headroom above the SLE so a card past it still lands below the line", () => {
    const phase: AgentSdlcPhase = { key: "spec", sleSeconds: 100 };
    const scale = agingScaleSeconds(phase, [card({ timeInPhaseSeconds: 50 })]);
    // 1.5x headroom over the SLE dominates the stalest (50) card.
    expect(scale).toBe(150);
    expect(agingRatio(phase.sleSeconds, scale)).toBeCloseTo(100 / 150);
    expect(agingRatio(300, scale)).toBe(1); // clamped
  });

  it("stretches to fit a card staler than the SLE headroom", () => {
    const phase: AgentSdlcPhase = { key: "spec", sleSeconds: 100 };
    const scale = agingScaleSeconds(phase, [card({ timeInPhaseSeconds: 400 })]);
    expect(scale).toBe(400);
  });
});

describe("reworkRatio / reworkSeverity", () => {
  it("fills proportionally to its own threshold", () => {
    expect(reworkRatio({ count: 2, threshold: 3 })).toBeCloseTo(2 / 3);
    expect(reworkRatio({ count: 7, threshold: 10 })).toBeCloseTo(0.7);
  });

  it("escalates to critical when one more strike blocks", () => {
    expect(reworkSeverity({ count: 2, threshold: 3 })).toBe("critical");
    expect(reworkSeverity({ count: 9, threshold: 10 })).toBe("critical");
    expect(reworkSeverity({ count: 7, threshold: 10 })).toBe("warning");
    expect(reworkSeverity({ count: 1, threshold: 10 })).toBe("ok");
  });
});

describe("bundleRowSeed", () => {
  it("prefers the digest, falls back to the version, else null", () => {
    expect(
      bundleRowSeed({
        version: "v1",
        digest: "d1",
        date: null,
        unreleased: false,
        hubClusters: [],
        issues: [],
        otherPRs: [],
      }),
    ).toBe("d1");
    expect(
      bundleRowSeed({
        version: "v1",
        digest: null,
        date: null,
        unreleased: false,
        hubClusters: [],
        issues: [],
        otherPRs: [],
      }),
    ).toBe("v1");
    expect(
      bundleRowSeed({
        version: null,
        digest: null,
        date: null,
        unreleased: true,
        hubClusters: [],
        issues: [],
        otherPRs: [],
      }),
    ).toBeNull();
  });
});

describe("bundlePRCount", () => {
  it("sums grouped tracked-issue PRs and one-off PRs (the bundle's whole PR set)", () => {
    const pr = (number: number): AgentSdlcPullRequest => ({
      number,
      title: `pr ${String(number)}`,
      url: `u-${String(number)}`,
      author: null,
      mergedAt: null,
    });
    expect(
      bundlePRCount({
        version: "v1",
        digest: "d1",
        date: null,
        unreleased: false,
        hubClusters: [],
        issues: [
          {
            number: 525,
            title: "tracked",
            url: "i",
            mergedAt: null,
            mergedBy: null,
            prs: [pr(1), pr(2)],
          },
        ],
        otherPRs: [pr(3), pr(4)],
      }),
    ).toBe(4);
  });
});

describe("bundleRowExpandKey", () => {
  it("uses the bundle seed for a released row", () => {
    expect(
      bundleRowExpandKey({
        version: "v1",
        digest: "d1",
        date: null,
        unreleased: false,
        hubClusters: [],
        issues: [],
        otherPRs: [],
      }),
    ).toBe("d1");
  });

  it("falls back to a reserved key for the seedless Unreleased row so it stays expandable", () => {
    // Both digest and version are null for the Unreleased row; without a synthetic
    // key its expand toggle would be inert.
    expect(
      bundleRowExpandKey({
        version: null,
        digest: null,
        date: null,
        unreleased: true,
        hubClusters: [],
        issues: [],
        otherPRs: [],
      }),
    ).toBe(UNRELEASED_ROW_KEY);
  });
});
