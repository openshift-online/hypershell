import { describe, expect, it } from "vitest";

import {
  agingRatio,
  agingScaleSeconds,
  boardPhases,
  bundlePRCount,
  bundleRowExpandKey,
  bundleRowSeed,
  cardsInPhase,
  computeCardFlights,
  computeReplayCardFlights,
  PHASE_SHIPPED,
  reworkRatio,
  reworkSeverity,
  UNRELEASED_ROW_KEY,
  type AgentSdlcBundleRow,
  type AgentSdlcCard,
  type AgentSdlcData,
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
  // Verifies: SDLC-VIS-09.01
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

describe("computeCardFlights", () => {
  function data(
    cards: AgentSdlcCard[],
    rows: AgentSdlcBundleRow[] = [],
  ): AgentSdlcData {
    return {
      phases: [],
      cards,
      lane: [],
      matrix: { hubClusters: [], rows },
    };
  }
  const shippedRow = (numbers: number[]): AgentSdlcBundleRow => ({
    version: "v1",
    digest: "d1",
    date: null,
    unreleased: false,
    hubClusters: [],
    issues: numbers.map((number) => ({
      number,
      title: `#${String(number)}`,
      url: "u",
      mergedAt: null,
      mergedBy: null,
      prs: [],
    })),
    otherPRs: [],
  });

  it("returns nothing on first render (no prior snapshot)", () => {
    expect(computeCardFlights(null, data([card({ number: 1 })]))).toEqual([]);
  });

  it("emits a forward flight when a card advances a phase", () => {
    const prev = data([card({ number: 1, phase: "spec" })]);
    const next = data([card({ number: 1, phase: "spec-approved" })]);
    expect(computeCardFlights(prev, next)).toEqual([
      {
        number: 1,
        fromPhase: "spec",
        toPhase: "spec-approved",
        toShipped: false,
      },
    ]);
  });

  it("emits a backward flight on a reject loop", () => {
    const prev = data([card({ number: 1, phase: "spec-approved" })]);
    const next = data([card({ number: 1, phase: "spec" })]);
    expect(computeCardFlights(prev, next)).toEqual([
      {
        number: 1,
        fromPhase: "spec-approved",
        toPhase: "spec",
        toShipped: false,
      },
    ]);
  });

  // Verifies: SDLC-VIS-21.01
  it("does NOT emit a flight when only the aged time-in-phase changes (same phase)", () => {
    // The continuous-aging re-render advances timeInPhaseSeconds but never the server
    // phase, so diffing on phase alone emits nothing (SDLC-VIS-21).
    const prev = data([
      card({ number: 1, phase: "spec", timeInPhaseSeconds: 10 }),
    ]);
    const next = data([
      card({ number: 1, phase: "spec", timeInPhaseSeconds: 70 }),
    ]);
    expect(computeCardFlights(prev, next)).toEqual([]);
  });

  it("flies a brand-new card in at intake", () => {
    const prev = data([card({ number: 1, phase: "spec" })]);
    const next = data([
      card({ number: 1, phase: "spec" }),
      card({ number: 2, phase: "intake" }),
    ]);
    expect(computeCardFlights(prev, next)).toEqual([
      { number: 2, fromPhase: null, toPhase: "intake", toShipped: false },
    ]);
  });

  it("flies a card into the Shipped matrix when it leaves the board", () => {
    const prev = data([card({ number: 1, phase: "code-approved" })]);
    const next = data([], [shippedRow([1])]);
    expect(computeCardFlights(prev, next)).toEqual([
      {
        number: 1,
        fromPhase: "code-approved",
        toPhase: PHASE_SHIPPED,
        toShipped: true,
      },
    ]);
  });

  it("does not fly a card that merely left the board without shipping", () => {
    // Gone from the board but not present in the matrix (e.g. closed/filtered out
    // upstream): no flight, nothing to land.
    const prev = data([card({ number: 1, phase: "code" })]);
    const next = data([]);
    expect(computeCardFlights(prev, next)).toEqual([]);
  });

  it("emits a separate flight for each simultaneous transition", () => {
    const prev = data([
      card({ number: 1, phase: "spec" }),
      card({ number: 2, phase: "code" }),
    ]);
    const next = data([
      card({ number: 1, phase: "spec-approved" }),
      card({ number: 2, phase: "code-approved" }),
    ]);
    const flights = computeCardFlights(prev, next);
    expect(flights).toHaveLength(2);
    expect(flights.map((f) => f.number).sort()).toEqual([1, 2]);
  });
});

describe("computeReplayCardFlights", () => {
  const phases: AgentSdlcPhase[] = [
    { key: "intake", sleSeconds: 100 },
    { key: "workable", sleSeconds: 100 },
    { key: "spec", sleSeconds: 100 },
    { key: "spec-approved", sleSeconds: 100 },
    { key: PHASE_SHIPPED, sleSeconds: 0 },
  ];
  function data(cards: AgentSdlcCard[]): AgentSdlcData {
    return { phases, cards, lane: [], matrix: { hubClusters: [], rows: [] } };
  }

  // Verifies: SDLC-VIS-24.01, SDLC-VIS-24.03
  it("replays a card that arrived within the window, from its funnel predecessor", () => {
    const flights = computeReplayCardFlights(
      data([card({ number: 1, phase: "spec", timeInPhaseSeconds: 60 })]),
      600,
    );
    expect(flights).toEqual([
      {
        number: 1,
        fromPhase: "workable",
        toPhase: "spec",
        toShipped: false,
        replay: true,
      },
    ]);
  });

  // Verifies: SDLC-VIS-24.02
  it("does not replay a card older than the window", () => {
    const flights = computeReplayCardFlights(
      data([card({ number: 1, phase: "spec", timeInPhaseSeconds: 5000 })]),
      600,
    );
    expect(flights).toEqual([]);
  });

  it("flies an intake arrival in from nowhere (fromPhase null)", () => {
    const flights = computeReplayCardFlights(
      data([card({ number: 7, phase: "intake", timeInPhaseSeconds: 10 })]),
      600,
    );
    expect(flights).toEqual([
      {
        number: 7,
        fromPhase: null,
        toPhase: "intake",
        toShipped: false,
        replay: true,
      },
    ]);
  });

  it("orders most-recent-first and caps the number of replays", () => {
    const cards = Array.from({ length: 20 }, (_, i) =>
      card({ number: i + 1, phase: "spec", timeInPhaseSeconds: i + 1 }),
    );
    const flights = computeReplayCardFlights(data(cards), 600, 3);
    expect(flights).toHaveLength(3);
    // Freshest (smallest time-in-phase) first.
    expect(flights.map((f) => f.number)).toEqual([1, 2, 3]);
  });
});
