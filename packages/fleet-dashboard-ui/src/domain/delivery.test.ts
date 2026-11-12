import { describe, expect, it } from "vitest";

import {
  type DeliveryChange,
  type DeliveryData,
  type DeliveryStage,
  type DeliveryStageState,
  changeListUnavailable,
  currentStageIndex,
  degradedSources,
  findChange,
  isSlow,
  nowAndNext,
} from "./delivery";

const PIPELINE = ["merged", "build", "policy", "release", "gitops"] as const;

function stage(
  key: string,
  state: DeliveryStageState,
  extra: Partial<DeliveryStage> = {},
): DeliveryStage {
  return {
    key,
    state,
    startedAt: null,
    finishedAt: null,
    reason: null,
    components: [],
    groups: [],
    gate: null,
    done: 0,
    total: 0,
    pr: null,
    checks: null,
    failedAttempts: 0,
    ...extra,
  };
}

/** A change whose stages are Passed up to `current`, then Not started. */
function change(
  sha: string,
  current: string,
  state: DeliveryStageState,
  extra: Partial<DeliveryChange> = {},
  currentExtra: Partial<DeliveryStage> = {},
): DeliveryChange {
  const keys = [...PIPELINE, "env:int"];
  const at = keys.indexOf(current);
  return {
    sha,
    shortSha: sha.slice(0, 7),
    mergedAt: "2026-11-01T15:00:00Z",
    title: `change ${sha.slice(0, 7)}`,
    author: null,
    pr: null,
    prUrl: null,
    commitUrl: null,
    delivered: false,
    deliveredAt: null,
    stages: keys.map((k, i) =>
      i < at
        ? stage(k, "passed")
        : i === at
          ? stage(k, state, currentExtra)
          : stage(k, "not-started"),
    ),
    builds: [],
    policies: [],
    bundle: null,
    gitopsPr: null,
    environments: [],
    ...extra,
  };
}

function data(
  changes: readonly DeliveryChange[],
  typicalSeconds: Record<string, number> = {},
): DeliveryData {
  return {
    enabled: true,
    stages: [
      ...PIPELINE.map((key) => ({
        key,
        kind: "pipeline" as const,
        group: null,
      })),
      { key: "env:int", kind: "group", group: "int" },
    ],
    changes,
    typicalSeconds,
    omitted: 0,
    sources: {},
  };
}

const FIXED_NOW = Date.parse("2026-11-01T18:00:00Z");

const SHA_A = "a15cc91".padEnd(40, "0");
const SHA_B = "b4ccd51".padEnd(40, "0");
const SHA_C = "c53fb9b".padEnd(40, "0");
const SHA_D = "d0000000".padEnd(40, "0");

describe("nowAndNext (FLT-DLV-10)", () => {
  // Verifies: FLT-DLV-10.01
  it("states Now with elapsed and typical time, and Next with its expected time", () => {
    const nowMs = Date.parse("2026-11-01T15:17:00Z");
    const d = data(
      [
        change(
          SHA_A,
          "build",
          "running",
          { pr: 997 },
          {
            startedAt: "2026-11-01T15:11:00Z",
            components: ["control-plane"],
          },
        ),
      ],
      { build: 600, policy: 180 },
    );
    const summary = nowAndNext(d, nowMs);
    expect(summary.rows).toHaveLength(1);
    const row = summary.rows[0];
    expect(row?.now.stage.key).toBe("build");
    expect(row?.now.elapsedMs).toBe(6 * 60_000);
    expect(row?.now.typicalMs).toBe(10 * 60_000);
    expect(row?.now.slow).toBe(false);
    // 15:17 + 4 min left of Build + 3 min of Policy check = 15:24.
    expect(row?.next).toEqual({
      kind: "stage",
      stage: expect.objectContaining({ key: "policy" }) as DeliveryStage,
      expectedAtMs: Date.parse("2026-11-01T15:24:00Z"),
    });
    expect(summary.severity).toBe("info");
  });

  it("never counts the remaining time of an overdue stage below zero", () => {
    const nowMs = Date.parse("2026-11-01T15:30:00Z");
    const d = data(
      [
        change(
          SHA_A,
          "build",
          "running",
          {},
          { startedAt: "2026-11-01T15:11:00Z" },
        ),
      ],
      { build: 600, policy: 180 },
    );
    const next = nowAndNext(d, nowMs).rows[0]?.next;
    expect(next).toMatchObject({
      kind: "stage",
      expectedAtMs: nowMs + 180_000,
    });
  });

  // Verifies: FLT-DLV-09.01
  it("omits the expected time when a typical duration is unknown", () => {
    const nowMs = Date.parse("2026-11-01T15:17:00Z");
    const d = data(
      [
        change(
          SHA_A,
          "build",
          "running",
          {},
          { startedAt: "2026-11-01T15:11:00Z" },
        ),
      ],
      { build: 600 },
    );
    const row = nowAndNext(d, nowMs).rows[0];
    expect(row?.next).toMatchObject({ kind: "stage", expectedAtMs: null });
  });

  // Verifies: FLT-DLV-10.02
  it("reports everything delivered with the newest change's delivery time", () => {
    const delivered = change(SHA_A, "env:int", "passed", {
      delivered: true,
      deliveredAt: "2026-11-01T16:30:00Z",
    });
    const older = change(SHA_B, "env:int", "passed", {
      delivered: true,
      deliveredAt: "2026-11-01T14:00:00Z",
    });
    const summary = nowAndNext(data([delivered, older]), FIXED_NOW);
    expect(summary.rows).toHaveLength(0);
    expect(summary.severity).toBe("good");
    expect(summary.delivered?.change.sha).toBe(SHA_A);
    expect(summary.delivered?.deliveredAtMs).toBe(
      Date.parse("2026-11-01T16:30:00Z"),
    );
    // Merged 15:00, delivered 16:30.
    expect(summary.delivered?.totalMs).toBe(90 * 60_000);
  });

  // Verifies: FLT-DLV-10.03
  it("uses the highest severity across all undelivered changes", () => {
    const nowMs = Date.parse("2026-11-01T18:00:00Z");
    const slowGitops = change(
      SHA_B,
      "gitops",
      "waiting",
      {},
      { startedAt: "2026-11-01T15:00:00Z" },
    );
    const failedBuild = change(SHA_A, "build", "failed");
    const summary = nowAndNext(
      data([slowGitops, failedBuild], { gitops: 600 }),
      nowMs,
    );
    expect(summary.rows.map((r) => r.severity)).toEqual(["warning", "danger"]);
    expect(summary.severity).toBe("danger");
    expect(summary.rows[1]?.next).toEqual({
      kind: "restart",
      restart: "new-change",
    });
  });

  it("follows severity beyond the three listed rows and counts the rest", () => {
    const changes = [
      change(SHA_A, "build", "running"),
      change(SHA_B, "build", "running"),
      change(SHA_C, "build", "running"),
      change(SHA_D, "build", "failed"),
    ];
    const summary = nowAndNext(data(changes), FIXED_NOW);
    expect(summary.rows).toHaveLength(3);
    expect(summary.more).toBe(1);
    expect(summary.severity).toBe("danger");
  });

  it("restarts a failed environment group from its gate, not a new change", () => {
    const summary = nowAndNext(
      data([change(SHA_A, "env:int", "failed")]),
      FIXED_NOW,
    );
    expect(summary.rows[0]?.next).toEqual({
      kind: "restart",
      restart: "gate-retry",
    });
  });
});

describe("stage helpers", () => {
  it("finds the earliest stage that is not Passed or Not needed", () => {
    const c = change(SHA_A, "policy", "running");
    const skipped: DeliveryChange = {
      ...c,
      stages: c.stages.map((s) =>
        s.key === "build" ? { ...s, state: "not-needed" } : s,
      ),
    };
    expect(currentStageIndex(skipped)).toBe(2);
  });

  it("warns only past twice the typical duration", () => {
    const d = data([], { build: 600 });
    const s = stage("build", "running", { startedAt: "2026-11-01T15:00:00Z" });
    expect(isSlow(d, s, Date.parse("2026-11-01T15:20:00Z"))).toBe(false);
    expect(isSlow(d, s, Date.parse("2026-11-01T15:20:01Z"))).toBe(true);
    expect(isSlow(data([]), s, Date.parse("2026-11-01T18:00:00Z"))).toBe(false);
  });

  it("finds a change by full SHA only", () => {
    const d = data([change(SHA_A, "build", "running")]);
    expect(findChange(d, SHA_A)?.sha).toBe(SHA_A);
    expect(findChange(d, "a15cc91")).toBeNull();
    expect(findChange(d, null)).toBeNull();
  });
});

describe("source degradation (FLT-DLV-13)", () => {
  it("lists stale or never-succeeded sources in a fixed order", () => {
    const d: DeliveryData = {
      ...data([]),
      sources: {
        promotion: { stale: true, never: true, error: null, lastSuccess: null },
        upstream: { stale: false, never: false, error: null, lastSuccess: "x" },
        gitops: { stale: true, never: false, error: "e", lastSuccess: "x" },
      },
    };
    expect(degradedSources(d).map((s) => s.key)).toEqual([
      "gitops",
      "promotion",
    ]);
  });

  it("reports the change list unavailable only when upstream never succeeded", () => {
    const never: DeliveryData = {
      ...data([]),
      sources: {
        upstream: { stale: true, never: true, error: "e", lastSuccess: null },
      },
    };
    expect(changeListUnavailable(never)).toBe(true);
    expect(changeListUnavailable(data([]))).toBe(false);
  });
});
