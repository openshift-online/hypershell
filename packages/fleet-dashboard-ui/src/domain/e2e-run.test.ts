import { describe, expect, it } from "vitest";

import {
  applyEvents,
  elapsedSeconds,
  isRunning,
  logsForStep,
  makeRunId,
  MAX_DETAIL_LOG_LINES,
  reduceRun,
  runDurationMs,
  runsNewestFirst,
  type E2eRunEvent,
  type E2eRunSummary,
} from "./e2e-run";

// Neutral placeholders only (hub-a/tenant-1/run-1); no fleet identifiers appear in
// these fixtures (firewall, scripts/check_fleet_dashboard_firewall.sh).

function ev(
  partial: Partial<E2eRunEvent> & Pick<E2eRunEvent, "event" | "sequence">,
): E2eRunEvent {
  return {
    runId: "run-1",
    timestamp: "",
    stepId: null,
    name: null,
    outcome: null,
    durationMs: null,
    message: null,
    truncated: false,
    plannedSteps: [],
    ...partial,
  };
}

describe("reduceRun", () => {
  it("orders out-of-order events by sequence and derives step state", () => {
    const detail = reduceRun([
      ev({
        event: "step_finished",
        sequence: 3,
        stepId: "a",
        outcome: "passed",
      }),
      ev({
        event: "run_started",
        sequence: 1,
        plannedSteps: [
          { stepId: "a", name: "Build" },
          { stepId: "b", name: "Test" },
        ],
        timestamp: "2026-01-01T00:00:00Z",
      }),
      ev({ event: "step_started", sequence: 2, stepId: "a", name: "Build" }),
    ]);
    expect(detail.steps.map((s) => s.stepId)).toEqual(["a", "b"]);
    expect(detail.steps[0]?.state).toBe("passed");
    // b was planned but never started -> not-run, never passed.
    expect(detail.steps[1]?.state).toBe("not-run");
    expect(detail.hasStructuredSteps).toBe(true);
    expect(detail.startedAt).toBe("2026-01-01T00:00:00Z");
  });

  it("dedupes duplicate sequences (reconnect overlap) idempotently", () => {
    const log = ev({
      event: "log",
      sequence: 5,
      message: "hello",
      timestamp: "t",
    });
    const once = reduceRun([log]);
    const twice = reduceRun([log, log, { ...log }]);
    expect(once.logs).toHaveLength(1);
    expect(twice.logs).toHaveLength(1);
    expect(twice.logs[0]?.message).toBe("hello");
  });

  it("keeps parallel steps distinct, each with its own state", () => {
    const detail = reduceRun([
      ev({
        event: "run_started",
        sequence: 1,
        plannedSteps: [
          { stepId: "a", name: "A" },
          { stepId: "b", name: "B" },
        ],
      }),
      ev({ event: "step_started", sequence: 2, stepId: "a" }),
      ev({ event: "step_started", sequence: 3, stepId: "b" }),
      ev({
        event: "step_finished",
        sequence: 4,
        stepId: "b",
        outcome: "failed",
      }),
    ]);
    expect(detail.steps[0]?.state).toBe("running");
    expect(detail.steps[1]?.state).toBe("failed");
  });

  it("distinguishes skipped from not-run; neither reads as passed", () => {
    const detail = reduceRun([
      ev({
        event: "run_started",
        sequence: 1,
        plannedSteps: [
          { stepId: "a", name: "A" },
          { stepId: "b", name: "B" },
          { stepId: "c", name: "C" },
        ],
      }),
      ev({
        event: "step_finished",
        sequence: 2,
        stepId: "a",
        outcome: "skipped",
      }),
      // b was reached and failed; c was never reached (fail-fast) -> not-run.
      ev({
        event: "step_finished",
        sequence: 3,
        stepId: "b",
        outcome: "failed",
      }),
    ]);
    const byId = new Map(detail.steps.map((s) => [s.stepId, s.state]));
    expect(byId.get("a")).toBe("skipped");
    expect(byId.get("b")).toBe("failed");
    expect(byId.get("c")).toBe("not-run");
    expect([...byId.values()]).not.toContain("passed");
  });

  it("treats a missing terminal record as unknown, not passed", () => {
    const detail = reduceRun([
      ev({ event: "run_started", sequence: 1 }),
      ev({ event: "step_started", sequence: 2, stepId: "a" }),
    ]);
    expect(detail.verdict).toBe("unknown");
    expect(detail.reportingComplete).toBe(false);
    expect(isRunning(detail)).toBe(true);
  });

  it("sets passed/failed verdict from the terminal record", () => {
    const passed = reduceRun([
      ev({ event: "run_finished", sequence: 1, outcome: "passed" }),
    ]);
    const failed = reduceRun([
      ev({ event: "run_finished", sequence: 1, outcome: "failed" }),
    ]);
    expect(passed.verdict).toBe("passed");
    expect(passed.reportingComplete).toBe(true);
    expect(failed.verdict).toBe("failed");
  });

  it("reduces an empty event list to an unknown, stepless run", () => {
    const detail = reduceRun([]);
    expect(detail.steps).toEqual([]);
    expect(detail.verdict).toBe("unknown");
    expect(detail.hasStructuredSteps).toBe(false);
  });

  it("orders steps not in the planned set after the planned ones, first-seen", () => {
    const detail = reduceRun([
      ev({
        event: "run_started",
        sequence: 1,
        plannedSteps: [{ stepId: "a", name: "A" }],
      }),
      ev({ event: "step_started", sequence: 2, stepId: "setup" }),
      ev({ event: "step_started", sequence: 3, stepId: "a" }),
    ]);
    expect(detail.steps.map((s) => s.stepId)).toEqual(["a", "setup"]);
  });
});

describe("applyEvents", () => {
  const base = reduceRun([
    ev({
      event: "run_started",
      sequence: 1,
      runId: "run-1",
      plannedSteps: [{ stepId: "a", name: "A" }],
      timestamp: "2026-01-01T00:00:00Z",
    }),
    ev({ event: "step_started", sequence: 2, stepId: "a" }),
    ev({ event: "log", sequence: 3, message: "first", timestamp: "t1" }),
  ]);

  it("applies live events onto a snapshot without losing or duplicating overlap", () => {
    const merged = applyEvents(base, [
      // sequence 3 overlaps the snapshot's log -> must not duplicate.
      ev({ event: "log", sequence: 3, message: "first", timestamp: "t1" }),
      ev({ event: "log", sequence: 4, message: "second", timestamp: "t2" }),
      ev({
        event: "step_finished",
        sequence: 5,
        stepId: "a",
        outcome: "passed",
      }),
      ev({
        event: "run_finished",
        sequence: 6,
        outcome: "passed",
        timestamp: "t3",
      }),
    ]);
    expect(merged.logs.map((l) => l.message)).toEqual(["first", "second"]);
    expect(merged.steps[0]?.state).toBe("passed");
    expect(merged.verdict).toBe("passed");
    expect(merged.reportingComplete).toBe(true);
  });

  it("keeps a sliding window of the NEWEST log lines so a live tail never freezes", () => {
    // Stream more log lines than the window. The pane must keep showing the newest
    // lines (dropping the oldest and flagging truncation), not freeze on the first N.
    const overflow = MAX_DETAIL_LOG_LINES + 50;
    const events: E2eRunEvent[] = [];
    for (let i = 0; i < overflow; i++) {
      events.push(
        ev({
          event: "log",
          sequence: 100 + i,
          message: `line-${String(i)}`,
          timestamp: "t",
        }),
      );
    }
    const merged = applyEvents(reduceRun([]), events);
    expect(merged.logs).toHaveLength(MAX_DETAIL_LOG_LINES);
    expect(merged.logsTruncated).toBe(true);
    // The newest line is retained; the oldest has slid out of the window.
    expect(merged.logs[merged.logs.length - 1]?.message).toBe(
      `line-${String(overflow - 1)}`,
    );
    expect(merged.logs[0]?.message).toBe(
      `line-${String(overflow - MAX_DETAIL_LOG_LINES)}`,
    );
    expect(merged.logs.some((l) => l.message === "line-0")).toBe(false);
  });
});

describe("output escaping fixtures", () => {
  it("carries markup-like characters through as plain data (escaped by the view)", () => {
    const detail = reduceRun([
      ev({
        event: "log",
        sequence: 1,
        message: "<script>alert(1)</script>",
        timestamp: "t",
      }),
    ]);
    // The domain stores the raw text verbatim; the view renders it as text so React
    // escapes it (E2E-LOG-14). No markup stripping happens here.
    expect(detail.logs[0]?.message).toBe("<script>alert(1)</script>");
  });
});

describe("derivations", () => {
  const summary: E2eRunSummary = {
    id: "tenant-1/ns/run-1",
    runId: "run-1",
    tenant: "tenant-1",
    cluster: "tenant-1",
    bodyCluster: "hub-a",
    namespace: "ns",
    revision: "abc",
    sourceRevision: "def",
    startedAt: "2026-01-01T00:00:00Z",
    endedAt: "2026-01-01T00:01:00Z",
    durationMs: null,
    verdict: "passed",
    reportingComplete: true,
    hasStructuredSteps: true,
    stepCount: 1,
  };

  it("prefers reported durationMs, else derives from start/end, else from now", () => {
    expect(runDurationMs({ ...summary, durationMs: 1234 }, 0)).toBe(1234);
    expect(runDurationMs(summary, 0)).toBe(60_000);
    const running = {
      startedAt: "2026-01-01T00:00:00Z",
      endedAt: null,
      durationMs: null,
    };
    const now = Date.parse("2026-01-01T00:00:30Z");
    expect(runDurationMs(running, now)).toBe(30_000);
  });

  it("returns null duration for an unparseable start", () => {
    expect(
      runDurationMs({ startedAt: "nope", endedAt: null, durationMs: null }, 0),
    ).toBeNull();
  });

  it("computes whole elapsed seconds, floored at zero", () => {
    const now = Date.parse("2026-01-01T00:02:00Z");
    expect(elapsedSeconds("2026-01-01T00:00:00Z", now)).toBe(120);
    expect(elapsedSeconds("2026-01-01T00:03:00Z", now)).toBe(0);
    expect(elapsedSeconds("bad", now)).toBeNull();
  });

  it("sorts runs newest-first, breaking ties by id", () => {
    const a: E2eRunSummary = {
      ...summary,
      id: "a",
      startedAt: "2026-01-01T00:00:00Z",
    };
    const b: E2eRunSummary = {
      ...summary,
      id: "b",
      startedAt: "2026-02-01T00:00:00Z",
    };
    const c: E2eRunSummary = {
      ...summary,
      id: "c",
      startedAt: "2026-01-01T00:00:00Z",
    };
    expect(runsNewestFirst([a, b, c]).map((r) => r.id)).toEqual([
      "b",
      "a",
      "c",
    ]);
  });

  it("filters logs for one step", () => {
    const detail = reduceRun([
      ev({ event: "log", sequence: 1, stepId: "a", message: "a1" }),
      ev({ event: "log", sequence: 2, stepId: "b", message: "b1" }),
      ev({ event: "log", sequence: 3, stepId: "a", message: "a2" }),
    ]);
    expect(logsForStep(detail.logs, "a").map((l) => l.message)).toEqual([
      "a1",
      "a2",
    ]);
  });

  it("builds a collision-free run id", () => {
    expect(makeRunId("tenant-1", "ns", "run-1")).toBe("tenant-1/ns/run-1");
  });
});
