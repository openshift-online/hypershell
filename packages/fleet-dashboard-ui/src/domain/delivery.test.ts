import { describe, expect, it } from "vitest";

import {
  type DeliveryChange,
  type DeliveryData,
  type DeliveryStage,
  type DeliveryStageState,
  changeListUnavailable,
  currentStageIndex,
  degradedSources,
  arrivals,
  findChange,
  isOverdue,
  nowAndNext,
  progress,
  whereToLook,
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
    estimateMs: null,
    slowestMs: null,
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
    imageCheck: null,
    ...extra,
  };
}

/**
 * Estimates per stage key, in seconds: the estimate alone (slowest is then 1.5
 * times it) or [estimate, slowest]. The BFF puts them on every change's stages
 * (FLT-DLV-09).
 */
type Estimates = Record<string, number | readonly [number, number]>;

function withEstimates(c: DeliveryChange, est: Estimates): DeliveryChange {
  return {
    ...c,
    stages: c.stages.map((st) => {
      const e = est[st.key];
      if (e === undefined) {
        return st;
      }
      const [median, slowest] = typeof e === "number" ? [e, e * 1.5] : e;
      return { ...st, estimateMs: median * 1000, slowestMs: slowest * 1000 };
    }),
  };
}

function data(
  changes: readonly DeliveryChange[],
  est: Estimates = {},
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
    changes: changes.map((c) => withEstimates(c, est)),
    releaseDiagnostics: null,
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
    expect(row?.now.estimateMs).toBe(10 * 60_000);
    expect(row?.now.overdue).toBe(false);
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
  it("omits the expected time when an estimate is unknown", () => {
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
    const overdueGitops = change(
      SHA_B,
      "gitops",
      "waiting",
      {},
      { startedAt: "2026-11-01T15:00:00Z" },
    );
    const failedBuild = change(SHA_A, "build", "failed");
    const summary = nowAndNext(
      data([overdueGitops, failedBuild], { gitops: 600 }),
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

  // Verifies: FLT-DLV-04.01, FLT-DLV-04.02
  it("is Overdue only past the slowest of the last 5 runs plus 5 minutes", () => {
    const s = stage("build", "running", {
      startedAt: "2026-11-01T15:00:00Z",
      estimateMs: 10 * 60_000,
      slowestMs: 14 * 60_000,
    });
    expect(isOverdue(s, Date.parse("2026-11-01T15:18:00Z"))).toBe(false);
    expect(isOverdue(s, Date.parse("2026-11-01T15:19:00Z"))).toBe(false);
    expect(isOverdue(s, Date.parse("2026-11-01T15:20:00Z"))).toBe(true);
    const noEstimate = stage("build", "running", {
      startedAt: "2026-11-01T15:00:00Z",
    });
    expect(isOverdue(noEstimate, Date.parse("2026-11-01T18:00:00Z"))).toBe(
      false,
    );
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

/** A change with Merged passed and every later stage Not started. */
function pipeline(
  current: string,
  state: DeliveryStageState,
  currentExtra: Partial<DeliveryStage> = {},
): DeliveryChange {
  return change(SHA_A, current, state, {}, currentExtra);
}

describe("arrivals (FLT-DLV-10)", () => {
  const est: Estimates = {
    build: [600, 900],
    policy: [300, 480],
    release: [1800, 2700],
    gitops: [600, 900],
    "env:int": [1200, 1800],
  };

  // Verifies: FLT-DLV-10.04
  it("is a range before the bundle is published", () => {
    const nowMs = Date.parse("2026-11-01T12:00:00Z");
    const d = data(
      [pipeline("build", "running", { startedAt: "2026-11-01T11:56:00Z" })],
      est,
    );
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    const arrival = arrivals(d, c, nowMs).get("env:int");
    // Early: 0.8 x (6 + 5 + 30 + 10 + 20) min = 56.8 min.
    // Late: (15 - 4) + 8 + 45 + 15 + 30 = 109 min.
    expect(arrival).toEqual({
      kind: "range",
      earlyMs: nowMs + 0.8 * 71 * 60_000,
      lateMs: nowMs + 109 * 60_000,
    });
  });

  // Verifies: FLT-DLV-10.05
  it("is one time once the bundle is published", () => {
    const nowMs = Date.parse("2026-11-01T12:00:00Z");
    const d = data(
      [pipeline("gitops", "waiting", { startedAt: "2026-11-01T11:56:00Z" })],
      est,
    );
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    expect(arrivals(d, c, nowMs).get("env:int")).toEqual({
      kind: "time",
      atMs: Date.parse("2026-11-01T12:26:00Z"),
    });
  });

  // Verifies: FLT-DLV-10.06
  it("is withdrawn while a stage is Overdue, after a failure, or without an estimate", () => {
    const nowMs = Date.parse("2026-11-01T12:00:00Z");
    // Release started 11:00: 60 min against a 45 min slowest run.
    const overdue = data(
      [pipeline("release", "running", { startedAt: "2026-11-01T11:00:00Z" })],
      est,
    );
    const failed = data([pipeline("build", "failed")], est);
    const missing = data(
      [pipeline("build", "running", { startedAt: "2026-11-01T11:56:00Z" })],
      { build: 600, policy: 300, gitops: 600, "env:int": 1200 },
    );
    for (const d of [overdue, failed, missing]) {
      const c = d.changes[0];
      if (c === undefined) throw new Error("no change");
      expect(arrivals(d, c, nowMs).get("env:int")).toEqual({ kind: "none" });
    }
  });

  it("shows the finish time of a group already reached", () => {
    const d = data([
      pipeline("env:int", "passed", { finishedAt: "2026-11-01T11:30:00Z" }),
    ]);
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    expect(arrivals(d, c, FIXED_NOW).get("env:int")).toEqual({
      kind: "reached",
      atMs: Date.parse("2026-11-01T11:30:00Z"),
    });
  });
});

describe("progress (FLT-DLV-19)", () => {
  // Verifies: FLT-DLV-19.01
  it("has one numbered cell per phase after Merged", () => {
    const d = data([pipeline("build", "running")]);
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    const model = progress(d, c, FIXED_NOW);
    expect(model.cells.map((cell) => [cell.number, cell.stage.key])).toEqual([
      [1, "build"],
      [2, "policy"],
      [3, "release"],
      [4, "gitops"],
      [5, "env:int"],
    ]);
  });

  // Verifies: FLT-DLV-19.02
  it("is elapsed over elapsed plus remaining", () => {
    // Merged 15:00, now 15:30, Build started 15:25: 5 of 10 min left, then
    // 85 min of later estimates: 90 min remaining, 30 elapsed = 25%.
    const nowMs = Date.parse("2026-11-01T15:30:00Z");
    const d = data(
      [pipeline("build", "running", { startedAt: "2026-11-01T15:25:00Z" })],
      { build: 600, policy: 300, release: 1800, gitops: 600, "env:int": 2400 },
    );
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    expect(progress(d, c, nowMs).percent).toBe(25);
  });

  it("omits the percent without an estimate or after a failure", () => {
    const d = data([pipeline("build", "failed")], { build: 600 });
    const c = d.changes[0];
    if (c === undefined) throw new Error("no change");
    expect(progress(d, c, FIXED_NOW).percent).toBeNull();
  });
});

describe("whereToLook (FLT-DLV-20)", () => {
  // Verifies: FLT-DLV-20.02
  it("names the failed build checks", () => {
    const c: DeliveryChange = {
      ...pipeline("build", "failed"),
      builds: [
        {
          component: "web-console",
          state: "failed",
          startedAt: null,
          completedAt: null,
          url: "https://example.test/run",
          outsideBundle: false,
        },
        {
          component: "cli",
          state: "passed",
          startedAt: null,
          completedAt: null,
          url: null,
          outsideBundle: false,
        },
      ],
    };
    const d = data([c]);
    const build = c.stages[1];
    if (build === undefined) throw new Error("no stage");
    const look = whereToLook(d, c, build, FIXED_NOW);
    expect(look?.kind).toBe("build-failed");
    expect(look?.kind === "build-failed" ? look.checks : []).toHaveLength(1);
  });

  // Verifies: FLT-DLV-20.03
  it("adds the image check to Build popups only", () => {
    const imageCheck = {
      state: "failed" as const,
      url: "https://example.test/job",
      failures: ["No usable image for: web-console."],
    };
    const failedBuild: DeliveryChange = {
      ...pipeline("build", "failed"),
      imageCheck,
    };
    const failedPolicy: DeliveryChange = {
      ...pipeline("policy", "failed"),
      imageCheck,
    };
    const look = (c: DeliveryChange, i: number) => {
      const st = c.stages[i];
      if (st === undefined) throw new Error("no stage");
      return whereToLook(data([c]), c, st, FIXED_NOW);
    };
    const build = look(failedBuild, 1);
    expect(build?.kind === "build-failed" ? build.imageCheck : null).toEqual(
      imageCheck,
    );
    const policy = look(failedPolicy, 2);
    expect(policy?.kind).toBe("checks-overdue");
    expect(
      policy?.kind === "checks-overdue" ? policy.imageCheck : "missing",
    ).toBeNull();
  });

  // Verifies: FLT-DLV-20.01
  it("offers the release diagnostics only for an Overdue release", () => {
    const nowMs = Date.parse("2026-11-01T12:00:00Z");
    const running = data(
      [pipeline("release", "running", { startedAt: "2026-11-01T11:50:00Z" })],
      { release: [1800, 2700] },
    );
    const overdue = data(
      [pipeline("release", "running", { startedAt: "2026-11-01T11:00:00Z" })],
      { release: [1800, 2700] },
    );
    const at = (d: DeliveryData) => {
      const c = d.changes[0];
      const st = c?.stages[3];
      if (c === undefined || st === undefined) throw new Error("no stage");
      return whereToLook(d, c, st, nowMs);
    };
    expect(at(running)).toBeNull();
    expect(at(overdue)).toEqual({
      kind: "release-overdue",
      diagnostics: null,
    });
  });
});
