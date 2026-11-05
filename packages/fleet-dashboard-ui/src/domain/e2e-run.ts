import type {
  PromotionData,
  ReleaseBundle,
  VerificationTarget,
} from "./promotion";
// E2E run plane: the release end-to-end test runs made observable in the glass
// dashboard (e2e-run-log-visibility.spec, HYPERSHELL-402). The BFF reads the
// opt-in structured run reporting (schema hypershell.e2e.v1) from a central Loki
// receiver and serves recent runs, a run's reduced detail, and a live SSE stream;
// this module is the framework-free domain the wire adapter decodes /api/e2e/* into,
// plus the pure event reduction the view renders from.
//
// Event reduction - ordering, deduplication by sequence, and step-state derivation -
// lives HERE, not in a component or adapter (E2E-LOG-11). It mirrors the BFF's own
// reduceRun (components/fleet-dashboard/pkg/sources/e2e.go) so a live-streamed view
// and a reloaded snapshot agree.
//
// FIREWALL: nothing about the fleet is baked in here. Cluster, tenant, revision, run
// ids, and step ids all arrive at runtime inside the payload, never a compiled-in
// table.

/** A run's overall verdict. A run with no terminal record is unknown, never passed. */
export type E2eVerdict = "passed" | "failed" | "unknown";

export const VERDICT_PASSED = "passed";
export const VERDICT_FAILED = "failed";
export const VERDICT_UNKNOWN = "unknown";

/**
 * A reduced step's state. `not-run` (a fail-fast step never reached) is distinct
 * from `skipped` (a mode-skipped step); neither reads as `passed` (E2E-LOG-12).
 */
export type E2eStepState =
  "passed" | "failed" | "skipped" | "running" | "not-run";

export const STEP_PASSED = "passed";
export const STEP_FAILED = "failed";
export const STEP_SKIPPED = "skipped";
export const STEP_RUNNING = "running";
export const STEP_NOT_RUN = "not-run";

/** The live stream connection state. A disconnected feed is never shown as live. */
export type E2eStreamState = "live" | "disconnected" | "gap";

export const STREAM_LIVE = "live";
export const STREAM_DISCONNECTED = "disconnected";
export const STREAM_GAP = "gap";

/** The structured event names the suite emits (mirrors the BFF reducer). */
export type E2eEventName =
  "run_started" | "step_started" | "step_finished" | "log" | "run_finished";

/** A step outcome carried on step_finished / run_finished records. */
export type E2eOutcome = "passed" | "failed" | "skipped";

/** maxDetailLogLines bounds how many log lines a reduced run carries, so a chatty
 *  run cannot produce an unbounded render payload (mirrors the BFF bound). */
export const MAX_DETAIL_LOG_LINES = 2000;

/** One step named in a run's planned set. */
export interface E2ePlannedStep {
  readonly stepId: string;
  readonly name: string;
}

/** One recent run in the list (E2E-LOG-12). */
export interface E2eRunSummary {
  readonly bundle?: ReleaseBundle | null;
  /** Collision-free address "tenant/namespace/runId" (E2E-LOG-01). */
  readonly id: string;
  readonly runId: string;
  readonly tenant: string;
  /** TRUSTED cluster identity (the Loki tenant); never the body (E2E-LOG-08). */
  readonly cluster: string;
  /** The record body's cluster field, display metadata only (E2E-LOG-08). */
  readonly bodyCluster: string | null;
  readonly namespace: string;
  /** Hydrated/deployed revision; sourceRevision is shown separately (E2E-LOG-09). */
  readonly revision: string;
  readonly sourceRevision: string | null;
  readonly startedAt: string;
  readonly endedAt: string | null;
  readonly durationMs: number | null;
  readonly verdict: E2eVerdict;
  /** False when no terminal run_finished was seen: incomplete, not a silent pass. */
  readonly reportingComplete: boolean;
  /** False for a run with no step events (a pre-reporting run), not a stepless pass. */
  readonly hasStructuredSteps: boolean;
  readonly stepCount: number;
}

/** One reduced step with its derived state and timing. */
export interface E2eStep {
  readonly stepId: string;
  readonly name: string;
  readonly state: E2eStepState;
  readonly startedAt: string | null;
  readonly endedAt: string | null;
  readonly durationMs: number | null;
}

/** One reduced log record. */
export interface E2eLogLine {
  readonly sequence: number;
  readonly timestamp: string;
  readonly stepId: string | null;
  readonly message: string;
  readonly truncated: boolean;
}

/** Concise failure summary carried on a failed run's detail (E2E-LOG-28). */
export interface E2eFailureSummary {
  readonly stepId: string;
  readonly message: string;
}

/** Severity level for a structured failure diagnostic section. */
export type E2eDiagnosticSeverity = "info" | "warn" | "error";

/** One failure-diagnostics section attached to a run detail (DIAGNOSTICS_CONTRACT). */
export interface E2eDiagnosticSection {
  readonly title: string;
  readonly body: string;
  readonly severity?: E2eDiagnosticSeverity;
}

/** Lazy failure-diagnostics payload returned by GET /api/e2e/diagnostics?run=<id>. */
export interface E2eDiagnosticsData {
  readonly diagnostics: readonly E2eDiagnosticSection[];
}

/** One run attempt within a grouped (bundle + target + revision) series. */
export interface E2eRunAttemptItem {
  readonly run: E2eRunSummary;
  /** 1-based chronological attempt index (oldest = 1, latest = N). */
  readonly attemptNumber: number;
}

/** A group of run attempts for the same (bundle + target + revision), latest-first. */
export interface E2eRunAttemptGroup {
  readonly key: string;
  readonly latest: E2eRunAttemptItem;
  readonly attempts: readonly E2eRunAttemptItem[];
}

/** A single run's detail: summary plus ordered steps and bounded log history. */
export interface E2eRunDetail extends E2eRunSummary {
  readonly failure?: E2eFailureSummary | null;
  readonly diagnostics?: readonly E2eDiagnosticSection[];
  readonly plannedSteps: readonly E2ePlannedStep[];
  readonly steps: readonly E2eStep[];
  readonly failure: E2eFailureSummary | null;
  readonly logs: readonly E2eLogLine[];
  readonly logsTruncated: boolean;
}

/**
 * One bounded page of a run's raw-log history (E2E-LOG-19). Entries are
 * chronological-ascending (oldest first) for display. `nextCursor` is the oldest
 * timestamp in the page (nanoseconds, decimal string) used to request the
 * immediately older page; `hasMore` reports the page was full so an older page may
 * exist.
 *
 * `boundaryTruncated` reports that a full page's oldest entries all shared the
 * cursor's nanosecond, so the cursor could not advance and the remaining entries at
 * that nanosecond are locally unreachable. It is distinct from end-of-history: the
 * pane SHALL render a locally-limited state rather than "all history was read"
 * (E2E-LOG-26). It is optional so a response from a BFF that predates the signal
 * (where it is absent) is read as not truncated.
 */
export interface E2eLogPage {
  readonly entries: readonly E2eLogLine[];
  readonly nextCursor: string | null;
  readonly hasMore: boolean;
  readonly boundaryTruncated?: boolean;
}

/**
 * The upper bound on how many log lines the in-memory buffer retains, so a very
 * long run does not grow the heap without bound (E2E-LOG-22). The rendered DOM is
 * bounded separately by the view's render window.
 */
export const MAX_LOG_BUFFER = 3000;

/**
 * Merge incoming log lines into an ordered, de-duplicated buffer, keyed by the
 * stable per-entry `sequence` and kept chronological-ascending. Both the paginated
 * history (older pages prepended) and the live stream (newer lines appended) flow
 * through here, so the one-nanosecond page-boundary overlap and the history-to-live
 * seam neither drop nor duplicate a line (E2E-LOG-19/21). Returns the previous
 * reference unchanged when nothing new arrived, so React can skip a re-render.
 */
export function mergeLogLines(
  buffer: readonly E2eLogLine[],
  incoming: readonly E2eLogLine[],
): readonly E2eLogLine[] {
  if (incoming.length === 0) {
    return buffer;
  }
  const bySeq = new Map<number, E2eLogLine>();
  for (const line of buffer) {
    bySeq.set(line.sequence, line);
  }
  let changed = false;
  for (const line of incoming) {
    if (!bySeq.has(line.sequence)) {
      bySeq.set(line.sequence, line);
      changed = true;
    }
  }
  if (!changed) {
    return buffer;
  }
  return [...bySeq.values()].sort((a, b) => a.sequence - b.sequence);
}

/**
 * Bound a buffer to the most recent `max` lines by dropping the oldest, for live
 * tailing of a long in-progress run (E2E-LOG-22). Returns the previous reference
 * unchanged when already within bound.
 */
export function boundLogBuffer(
  buffer: readonly E2eLogLine[],
  max: number,
): readonly E2eLogLine[] {
  if (buffer.length <= max) {
    return buffer;
  }
  return buffer.slice(buffer.length - max);
}

/** The /api/e2e/runs plane. When disabled the BFF returns only `enabled: false`. */
export interface E2eRunsData {
  readonly enabled: boolean;
  readonly runs: readonly E2eRunSummary[];
  readonly truncated: boolean;
  readonly windowSeconds: number;
  readonly limit: number;
}

/** One forwarded structured suite event from the live stream. */
export interface E2eRunEvent {
  readonly event: E2eEventName;
  readonly runId: string;
  readonly sequence: number;
  readonly timestamp: string;
  readonly stepId: string | null;
  readonly name: string | null;
  readonly outcome: E2eOutcome | null;
  readonly durationMs: number | null;
  readonly message: string | null;
  readonly truncated: boolean;
  readonly plannedSteps: readonly E2ePlannedStep[];
}

/** The live connection state the stream reports (E2E-LOG-06/10). */
export interface E2eStreamStatus {
  readonly state: E2eStreamState;
  readonly droppedEntries: number;
}

/** An empty run detail seed, used as the base of a from-scratch reduction. */
function emptyDetail(): E2eRunDetail {
  return {
    id: "",
    runId: "",
    tenant: "",
    cluster: "",
    bodyCluster: null,
    namespace: "",
    revision: "",
    sourceRevision: null,
    startedAt: "",
    endedAt: null,
    durationMs: null,
    verdict: VERDICT_UNKNOWN,
    reportingComplete: false,
    hasStructuredSteps: false,
    stepCount: 0,
    failure: null,
    diagnostics: [],
    plannedSteps: [],
    steps: [],
    failure: null,
    logs: [],
    logsTruncated: false,
  };
}

/** A mutable working copy of a detail, used only inside the fold. */
interface Mutable {
  bundle?: ReleaseBundle | null;
  id: string;
  runId: string;
  tenant: string;
  cluster: string;
  bodyCluster: string | null;
  namespace: string;
  revision: string;
  sourceRevision: string | null;
  startedAt: string;
  endedAt: string | null;
  durationMs: number | null;
  verdict: E2eVerdict;
  reportingComplete: boolean;
  hasStructuredSteps: boolean;
  stepCount: number;
  failure: E2eFailureSummary | null;
  diagnostics: readonly E2eDiagnosticSection[];
  plannedSteps: readonly E2ePlannedStep[];
  steps: readonly E2eStep[];
  failure: E2eFailureSummary | null;
  logs: readonly E2eLogLine[];
  logsTruncated: boolean;
}

interface WorkStep {
  stepId: string;
  name: string;
  state: E2eStepState;
  startedAt: string | null;
  endedAt: string | null;
  durationMs: number | null;
}

function firstNonEmpty(a: string, b: string): string {
  return a !== "" ? a : b;
}

function stepStateFromOutcome(outcome: E2eOutcome | null): E2eStepState {
  switch (outcome) {
    case VERDICT_PASSED:
      return STEP_PASSED;
    case STEP_SKIPPED:
      return STEP_SKIPPED;
    default:
      // A finished step with no/failed outcome reads as failed, never passed.
      return STEP_FAILED;
  }
}

function verdictFromOutcome(outcome: E2eOutcome | null): E2eVerdict {
  return outcome === VERDICT_PASSED ? VERDICT_PASSED : VERDICT_FAILED;
}

/**
 * Extract the human-readable `Messages:` summary from a testify assertion failure
 * (multiline or pipe-collapsed), falling back to the trimmed raw text when no
 * `Messages:` section is present.
 */
export function extractFailureMessage(raw: string): string {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return "";
  }

  // 1. Explicit `Messages:` label (multiline testify output or pipe-separated).
  const messagesMatches = [
    ...trimmed.matchAll(/(?:^|[\r\n|])\s*Messages\s*:\s*([\s\S]*)$/gi),
  ];
  if (messagesMatches.length > 0) {
    const tail = messagesMatches[messagesMatches.length - 1]?.[1] ?? "";
    const cleanedLines: string[] = [];
    for (const part of tail.split(/\r?\n|\|/)) {
      const line = part.trim();
      if (line === "") {
        continue;
      }
      if (/^(?:Error Trace|Error|Test)\s*:/i.test(line)) {
        break;
      }
      cleanedLines.push(line);
    }
    if (cleanedLines.length > 0) {
      return cleanedLines.join(" ");
    }
  }

  // 2. Pipe-collapsed testify diff where the `Messages:` label was already stripped
  // by the BFF's conciseFailure() helper, leaving the message as the trailing
  // segment(s) after the diff/assertion lines.
  if (trimmed.includes("|")) {
    const segments = trimmed
      .split("|")
      .map((s) => s.trim())
      .filter((s) => s !== "");
    const hasTestifyMarker = segments.some((s) =>
      /^(?:(?:Failed step:\s*\S+\s+)?(?:Not equal|equal|elements differ)\s*:|expected\s*:|actual\s*:|Diff\s*:|---\s*Expected|\+\+\+\s*Actual)/i.test(
        s,
      ),
    );
    if (hasTestifyMarker) {
      const isDiffOrAssertionSegment = (s: string): boolean =>
        /^(?:(?:Failed step:\s*\S+\s+)?(?:Not equal|equal|elements differ|expected|actual|Diff|Error|Error Trace|Test)\s*:|---\s*Expected|\+\+\+\s*Actual|@@\s*[-+]|[-+](?:[^\s]|$)|\.{3}$|\([A-Za-z0-9_.*[\]]+\))/i.test(
          s,
        );
      let lastMarkerIndex = -1;
      for (let i = 0; i < segments.length; i++) {
        if (isDiffOrAssertionSegment(segments[i] ?? "")) {
          lastMarkerIndex = i;
        }
      }
      if (lastMarkerIndex >= 0 && lastMarkerIndex < segments.length - 1) {
        const trailing = segments
          .slice(lastMarkerIndex + 1)
          .join(" ")
          .trim();
        if (trailing !== "") {
          return trailing;
        }
      }
    }
  }

  return trimmed;
}

function derivedFailure(
  verdict: E2eVerdict,
  steps: readonly E2eStep[],
  logs: readonly E2eLogLine[],
): E2eFailureSummary | null {
  if (verdict !== VERDICT_FAILED) {
    return null;
  }
  const failedStep = steps.find((step) => step.state === STEP_FAILED);
  const stepId = failedStep?.stepId ?? "";
  let message = "";
  if (stepId !== "") {
    for (let i = logs.length - 1; i >= 0; i--) {
      const line = logs[i];
      if (line?.stepId === stepId && line.message.trim() !== "") {
        message = line.message.trim();
        break;
      }
    }
  }
  if (message === "") {
    for (let i = logs.length - 1; i >= 0; i--) {
      const line = logs[i];
      if (line && line.message.trim() !== "") {
        message = line.message.trim();
        break;
      }
    }
  }
  if (stepId === "" && message === "") {
    return null;
  }
  return { stepId, message };
}

/**
 * Order the deduped events by sequence. A stable sort keeps same-sequence order
 * deterministic; dedup then drops the later duplicate so reconnect-overlap backfill
 * is idempotent (E2E-LOG-06).
 */
function orderAndDedupe(
  events: readonly E2eRunEvent[],
): readonly E2eRunEvent[] {
  const sorted = events
    .map((ev, i) => ({ ev, i }))
    .sort((a, b) =>
      a.ev.sequence !== b.ev.sequence
        ? a.ev.sequence - b.ev.sequence
        : a.i - b.i,
    )
    .map((x) => x.ev);
  const seen = new Set<number>();
  const out: E2eRunEvent[] = [];
  for (const ev of sorted) {
    if (seen.has(ev.sequence)) {
      continue;
    }
    seen.add(ev.sequence);
    out.push(ev);
  }
  return out;
}

/**
 * List planned steps in planned order first, then any additional seen steps in
 * first-seen order, so a consumer gets a stable ordering (mirrors the BFF).
 */
function orderSteps(
  planned: readonly E2ePlannedStep[],
  seenOrder: readonly string[],
  byId: ReadonlyMap<string, WorkStep>,
): E2eStep[] {
  const out: E2eStep[] = [];
  const emitted = new Set<string>();
  for (const p of planned) {
    const s = byId.get(p.stepId);
    if (s) {
      out.push(freezeStep(s));
    } else {
      out.push({
        stepId: p.stepId,
        name: p.name,
        state: STEP_NOT_RUN,
        startedAt: null,
        endedAt: null,
        durationMs: null,
      });
    }
    emitted.add(p.stepId);
  }
  for (const id of seenOrder) {
    if (emitted.has(id)) {
      continue;
    }
    const s = byId.get(id);
    if (s) {
      out.push(freezeStep(s));
      emitted.add(id);
    }
  }
  return out;
}

function freezeStep(s: WorkStep): E2eStep {
  return {
    stepId: s.stepId,
    name: s.name,
    state: s.state,
    startedAt: s.startedAt,
    endedAt: s.endedAt,
    durationMs: s.durationMs,
  };
}

/**
 * Fold live events onto a base detail. This is the single reduction routine shared
 * by reduceRun (from an empty base) and applyEvents (onto a snapshot base), so the
 * ordering/dedup/step-state rules live in exactly one framework-free place
 * (E2E-LOG-11). Reduction is clock-free.
 */
function fold(
  base: E2eRunDetail,
  events: readonly E2eRunEvent[],
): E2eRunDetail {
  const d = toMutable(base);

  const stepById = new Map<string, WorkStep>();
  const stepOrder: string[] = [];
  for (const s of base.steps) {
    const w: WorkStep = {
      stepId: s.stepId,
      name: s.name,
      state: s.state,
      startedAt: s.startedAt,
      endedAt: s.endedAt,
      durationMs: s.durationMs,
    };
    stepById.set(s.stepId, w);
    stepOrder.push(s.stepId);
  }
  const ensureStep = (id: string, name: string): WorkStep => {
    let s = stepById.get(id);
    if (!s) {
      s = {
        stepId: id,
        name,
        state: STEP_NOT_RUN,
        startedAt: null,
        endedAt: null,
        durationMs: null,
      };
      stepById.set(id, s);
      stepOrder.push(id);
    }
    if (name !== "") {
      s.name = name;
    }
    return s;
  };

  let planned: readonly E2ePlannedStep[] = base.plannedSteps;
  let sawStepEvent = base.hasStructuredSteps;
  const logs: E2eLogLine[] = [...base.logs];
  const logSeen = new Set<number>(base.logs.map((l) => l.sequence));
  let logsTruncated = base.logsTruncated;

  for (const ev of orderAndDedupe(events)) {
    d.runId = firstNonEmpty(d.runId, ev.runId);

    switch (ev.event) {
      case "run_started":
        if (ev.plannedSteps.length > 0) {
          planned = ev.plannedSteps;
        }
        d.startedAt = firstNonEmpty(d.startedAt, ev.timestamp);
        for (const p of ev.plannedSteps) {
          ensureStep(p.stepId, p.name);
        }
        break;
      case "step_started": {
        sawStepEvent = true;
        const s = ensureStep(ev.stepId ?? "", ev.name ?? "");
        s.state = STEP_RUNNING;
        s.startedAt = ev.timestamp;
        d.startedAt = firstNonEmpty(d.startedAt, ev.timestamp);
        break;
      }
      case "step_finished": {
        sawStepEvent = true;
        const s = ensureStep(ev.stepId ?? "", ev.name ?? "");
        s.state = stepStateFromOutcome(ev.outcome);
        s.endedAt = ev.timestamp;
        s.durationMs = ev.durationMs;
        break;
      }
      case "run_finished":
        d.verdict = verdictFromOutcome(ev.outcome);
        d.reportingComplete = true;
        d.endedAt = ev.timestamp;
        d.durationMs = ev.durationMs;
        d.failure =
          d.verdict === VERDICT_FAILED && ev.message !== null
            ? { stepId: ev.stepId ?? "", message: ev.message }
            : null;
        break;
      case "log":
        if (logSeen.has(ev.sequence)) {
          break;
        }
        logSeen.add(ev.sequence);
        logs.push({
          sequence: ev.sequence,
          timestamp: ev.timestamp,
          stepId: ev.stepId,
          message: ev.message ?? "",
          truncated: ev.truncated,
        });
        // Keep a sliding window of the most recent lines rather than the first N:
        // a live tail must keep showing the newest lines, so once the bound is hit
        // the oldest line is dropped (and flagged truncated) instead of freezing the
        // pane on the first N lines of a long run (E2E-LOG-12). Dropped sequences
        // leave logSeen so the retained set and the dedup window stay aligned.
        if (logs.length > MAX_DETAIL_LOG_LINES) {
          const dropped = logs.shift();
          if (dropped !== undefined) {
            logSeen.delete(dropped.sequence);
          }
          logsTruncated = true;
        }
        break;
    }
  }

  const steps = orderSteps(planned, stepOrder, stepById);
  return {
    ...d,
    failure: derivedFailure(d.verdict, steps, logs) ?? d.failure,
    plannedSteps: planned,
    steps,
    logs,
    logsTruncated,
    hasStructuredSteps: sawStepEvent,
    stepCount: steps.length,
    id: d.id !== "" ? d.id : makeRunId(d.tenant, d.namespace, d.runId),
  };
}

function toMutable(base: E2eRunDetail): Mutable {
  return {
    bundle: base.bundle,
    id: base.id,
    runId: base.runId,
    tenant: base.tenant,
    cluster: base.cluster,
    bodyCluster: base.bodyCluster,
    namespace: base.namespace,
    revision: base.revision,
    sourceRevision: base.sourceRevision,
    startedAt: base.startedAt,
    endedAt: base.endedAt,
    durationMs: base.durationMs,
    verdict: base.verdict,
    reportingComplete: base.reportingComplete,
    hasStructuredSteps: base.hasStructuredSteps,
    stepCount: base.stepCount,
    failure: base.failure ?? null,
    diagnostics: base.diagnostics ?? [],
    plannedSteps: base.plannedSteps,
    steps: base.steps,
    failure: base.failure,
    logs: base.logs,
    logsTruncated: base.logsTruncated,
  };
}

/** Build the collision-free run address (mirrors the BFF makeRunID). */
export function makeRunId(
  tenant: string,
  namespace: string,
  runId: string,
): string {
  return `${tenant}/${namespace}/${runId}`;
}

/**
 * Reduce a run's structured events into a detail from scratch: order + dedup by
 * sequence, derive each step's state (planned-but-unstarted -> not-run,
 * step_started -> running, step_finished -> its outcome), and set the verdict
 * (run_finished -> passed/failed; absent -> unknown with reportingComplete=false).
 */
export function reduceRun(events: readonly E2eRunEvent[]): E2eRunDetail {
  return fold(emptyDetail(), events);
}

/**
 * Apply live stream events onto an already-reduced snapshot detail. New runs,
 * steps, and log lines arrive live and are deduped by sequence against the snapshot,
 * so the snapshot-to-tail bridge never double-counts or loses overlap (E2E-LOG-06).
 */
export function applyEvents(
  base: E2eRunDetail,
  events: readonly E2eRunEvent[],
): E2eRunDetail {
  return fold(base, events);
}

/**
 * A run's wall-clock duration in milliseconds. Prefers a positive reported durationMs;
 * when durationMs is null or 0 (for example when a failed run's summary omits or
 * zeroes durationMs), derives it from start/end, step timestamps/durations, or
 * from start to `nowMs` while still running. Returns null when the start is unparseable.
 */
export function runDurationMs(
  run: Pick<E2eRunSummary, "startedAt" | "endedAt" | "durationMs"> & {
    readonly steps?: readonly E2eStep[];
  },
  nowMs: number,
): number | null {
  if (run.durationMs !== null && run.durationMs > 0) {
    return run.durationMs;
  }
  const start = Date.parse(run.startedAt);
  const explicitEnd =
    run.endedAt !== null ? Date.parse(run.endedAt) : Number.NaN;
  if (
    !Number.isNaN(start) &&
    !Number.isNaN(explicitEnd) &&
    explicitEnd > start
  ) {
    return explicitEnd - start;
  }
  if (
    run.steps &&
    run.steps.length > 0 &&
    !run.steps.some((s) => s.state === "running")
  ) {
    let earliest = Number.isNaN(start) ? Number.POSITIVE_INFINITY : start;
    let latest = !Number.isNaN(explicitEnd)
      ? explicitEnd
      : Number.NEGATIVE_INFINITY;
    let summed = 0;
    for (const step of run.steps) {
      if (step.startedAt !== null) {
        const s = Date.parse(step.startedAt);
        if (!Number.isNaN(s) && s < earliest) {
          earliest = s;
        }
      }
      if (step.endedAt !== null) {
        const e = Date.parse(step.endedAt);
        if (!Number.isNaN(e) && e > latest) {
          latest = e;
        }
      }
      if (step.durationMs !== null && step.durationMs > 0) {
        summed += step.durationMs;
      }
    }
    if (
      Number.isFinite(earliest) &&
      Number.isFinite(latest) &&
      latest > earliest
    ) {
      return latest - earliest;
    }
    if (summed > 0) {
      return summed;
    }
  }
  const end =
    run.endedAt !== null
      ? explicitEnd
      : !Number.isNaN(start)
        ? nowMs
        : Number.NaN;
  if (!Number.isNaN(start) && !Number.isNaN(end) && end > start) {
    return end - start;
  }
  if (run.durationMs !== null) {
    return run.durationMs;
  }
  if (!Number.isNaN(start) && !Number.isNaN(end)) {
    return Math.max(end - start, 0);
  }
  return null;
}

/**
 * A step's wall-clock duration in milliseconds. Prefers a positive reported
 * `durationMs`; when `durationMs` is null or 0 (for example when a step fails before
 * or immediately upon its gate assertion), derives elapsed time from `startedAt` and
 * `endedAt` (or `nowMs` while running), from the step's log timestamps, or from the
 * preceding step's completion timestamp.
 */
export function stepDurationMs(
  step: E2eStep,
  nowMs: number,
  steps?: readonly E2eStep[],
  logs?: readonly E2eLogLine[],
  runStartedAt?: string | null,
): number | null {
  if (step.durationMs !== null && step.durationMs > 0) {
    return step.durationMs;
  }
  const start =
    step.startedAt !== null ? Date.parse(step.startedAt) : Number.NaN;
  const end =
    step.endedAt !== null
      ? Date.parse(step.endedAt)
      : step.state === STEP_RUNNING
        ? nowMs
        : Number.NaN;
  if (!Number.isNaN(start) && !Number.isNaN(end) && end > start) {
    return end - start;
  }
  if (logs && logs.length > 0) {
    let minLog = Number.NaN;
    let maxLog = Number.NaN;
    for (const line of logs) {
      if (line.stepId !== step.stepId) {
        continue;
      }
      const t = Date.parse(line.timestamp);
      if (Number.isNaN(t)) {
        continue;
      }
      if (Number.isNaN(minLog) || t < minLog) {
        minLog = t;
      }
      if (Number.isNaN(maxLog) || t > maxLog) {
        maxLog = t;
      }
    }
    const effectiveStart = !Number.isNaN(start)
      ? Math.min(start, Number.isNaN(minLog) ? start : minLog)
      : minLog;
    const effectiveEnd = !Number.isNaN(end)
      ? Math.max(end, Number.isNaN(maxLog) ? end : maxLog)
      : maxLog;
    if (
      !Number.isNaN(effectiveStart) &&
      !Number.isNaN(effectiveEnd) &&
      effectiveEnd > effectiveStart
    ) {
      return effectiveEnd - effectiveStart;
    }
  }
  if (step.state === STEP_FAILED && !Number.isNaN(end)) {
    let foundPrev = false;
    if (steps) {
      const idx = steps.findIndex((s) => s.stepId === step.stepId);
      for (let i = idx - 1; i >= 0; i--) {
        const prev = steps[i];
        const prevIso = prev?.endedAt ?? prev?.startedAt ?? null;
        if (prevIso) {
          foundPrev = true;
          const prevEnd = Date.parse(prevIso);
          if (!Number.isNaN(prevEnd) && end > prevEnd) {
            return end - prevEnd;
          }
          break;
        }
      }
    }
    if (!foundPrev && runStartedAt) {
      const runStart = Date.parse(runStartedAt);
      if (!Number.isNaN(runStart) && end > runStart) {
        return end - runStart;
      }
    }
  }
  if (step.durationMs !== null) {
    return step.durationMs;
  }
  if (!Number.isNaN(start) && !Number.isNaN(end)) {
    return Math.max(end - start, 0);
  }
  return null;
}

/** Whole seconds elapsed from an ISO instant to `nowMs`; null when unparseable. */
export function elapsedSeconds(iso: string, nowMs: number): number | null {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) {
    return null;
  }
  return Math.max(Math.floor((nowMs - t) / 1000), 0);
}

/** True when a run is still in progress (no terminal record observed). */
export function isRunning(
  run: Pick<E2eRunSummary, "reportingComplete">,
): boolean {
  return !run.reportingComplete;
}

/** Recent runs newest-first by start time, ties broken by id for stability. */
export function runsNewestFirst(
  runs: readonly E2eRunSummary[],
): readonly E2eRunSummary[] {
  return runs
    .slice()
    .sort((a, b) =>
      a.startedAt !== b.startedAt
        ? b.startedAt.localeCompare(a.startedAt)
        : a.id.localeCompare(b.id),
    );
}

/** Log lines belonging to one step (by stepId), preserving reduced order. */
export function logsForStep(
  logs: readonly E2eLogLine[],
  stepId: string,
): readonly E2eLogLine[] {
  return logs.filter((l) => l.stepId === stepId);
}

/**
 * Numeric rank for step state so sorting by status/failure orders steps from
 * most actionable to least actionable: failed -> running -> passed -> skipped -> not-run.
 */
export function stepStateRank(state: E2eStepState): number {
  switch (state) {
    case "failed":
      return 0;
    case "running":
      return 1;
    case "passed":
      return 2;
    case "skipped":
      return 3;
    case "not-run":
      return 4;
  }
}

const PHASE_STEP_RE = /^p(\d+)(?:\.(\d+))?(?:$|[-_\s:])/i;

function parseStepPhase(value: string): readonly [number, number] | null {
  const m = PHASE_STEP_RE.exec(value.trim());
  if (!m) {
    return null;
  }
  const major = Number(m[1]);
  const minor = m[2] !== undefined ? Number(m[2]) : 0;
  return [major, minor];
}

/** Identify whether a step is the failure-diagnostics step (DIAGNOSTICS_CONTRACT). */
export function isDiagnosticsStep(
  step: Pick<E2eStep, "stepId" | "name">,
): boolean {
  const id = step.stepId.trim().toLowerCase();
  const name = step.name.trim().toLowerCase();
  return (
    id === "diagnostics" ||
    id === "failure-diagnostics" ||
    name === "diagnostics" ||
    name === "failure-diagnostics" ||
    name === "failure diagnostics"
  );
}

function classifyStepOrder(step: E2eStep): {
  readonly category: number;
  readonly phase: readonly [number, number] | null;
} {
  const id = step.stepId.trim().toLowerCase();
  const name = step.name.trim().toLowerCase();
  if (
    id === "setup" ||
    id === "suite-setup" ||
    name === "setup" ||
    name === "suite-setup"
  ) {
    return { category: 0, phase: null };
  }
  if (isDiagnosticsStep(step)) {
    return { category: 3, phase: null };
  }
  if (
    id === "cleanup" ||
    id === "suite-teardown" ||
    id === "teardown" ||
    name === "cleanup" ||
    name === "suite-teardown" ||
    name === "teardown"
  ) {
    return { category: 4, phase: null };
  }
  const phase = parseStepPhase(step.stepId) ?? parseStepPhase(step.name);
  if (phase) {
    return { category: 1, phase };
  }
  return { category: 2, phase: null };
}

/**
 * Order E2E steps in their canonical test-execution sequence:
 * 1. Suite setup (`setup` / `suite-setup`)
 * 2. Numbered phase steps (`P0.1`, `P0.2`, `P1.1`..`P1.5`, `P2.1`..`P2.11`, `P3.1`)
 *    sorted numerically by (phase, subphase) so `P2.2` precedes `P2.10` and `P2.7`
 *    precedes `P3.1` even when parallel subtests finish out of order or plannedSteps
 *    lists phases non-monotonically.
 * 3. Other steps in `plannedSteps` order (or `startedAt` order when `plannedSteps`
 *    is absent, then original index).
 * 4. Failure diagnostics (`diagnostics` / `failure-diagnostics`).
 * 5. Suite teardown (`cleanup` / `suite-teardown`).
 */
export function orderStepsCanonical(
  steps: readonly E2eStep[],
  plannedSteps: readonly E2ePlannedStep[] = [],
): readonly E2eStep[] {
  const plannedIndex = new Map<string, number>();
  plannedSteps.forEach((p, idx) => {
    if (!plannedIndex.has(p.stepId)) {
      plannedIndex.set(p.stepId, idx);
    }
  });

  return steps
    .map((step, index) => ({
      step,
      index,
      meta: classifyStepOrder(step),
      plannedIdx: plannedIndex.get(step.stepId),
      startMs: step.startedAt ? Date.parse(step.startedAt) : Number.NaN,
    }))
    .sort((a, b) => {
      if (a.meta.category !== b.meta.category) {
        return a.meta.category - b.meta.category;
      }
      if (a.meta.phase && b.meta.phase) {
        if (a.meta.phase[0] !== b.meta.phase[0]) {
          return a.meta.phase[0] - b.meta.phase[0];
        }
        if (a.meta.phase[1] !== b.meta.phase[1]) {
          return a.meta.phase[1] - b.meta.phase[1];
        }
      }
      if (a.plannedIdx !== undefined && b.plannedIdx !== undefined) {
        if (a.plannedIdx !== b.plannedIdx) {
          return a.plannedIdx - b.plannedIdx;
        }
      } else if (a.plannedIdx !== undefined || b.plannedIdx !== undefined) {
        return a.plannedIdx !== undefined ? -1 : 1;
      } else if (
        Number.isFinite(a.startMs) &&
        Number.isFinite(b.startMs) &&
        a.startMs !== b.startMs
      ) {
        return a.startMs - b.startMs;
      }
      return a.index - b.index;
    })
    .map((entry) => entry.step);
}

/** Never substitute a different revision or cluster's "latest" run for a gate.
 * Attempts are newest first, and selection stays pinned while live data updates. */
export function runsForVerification(
  runs: readonly E2eRunSummary[],
  target: VerificationTarget | null | undefined,
): readonly E2eRunSummary[] {
  if (!target?.revision || !target.cluster || !target.namespace) return [];
  return runsNewestFirst(
    runs.filter(
      (run) =>
        run.revision === target.revision &&
        run.cluster === target.cluster &&
        run.namespace === target.namespace,
    ),
  );
}

/** Exact target identity only; do not infer instance names from namespace strings. */
export function runTargetInstance(
  run: E2eRunSummary,
  promotion?: PromotionData,
): string | null {
  const matches = Object.values(promotion?.environments ?? {}).filter(
    (env) =>
      env.verification?.cluster === run.cluster &&
      env.verification.namespace === run.namespace,
  );
  return matches.length === 1 ? (matches[0]?.name ?? null) : null;
}

/** A historical server resolution wins. Current promotion is an exact-revision fallback. */
export function runBundle(
  run: E2eRunSummary,
  promotion?: PromotionData,
): ReleaseBundle | null {
  if (run.bundle?.digest) return run.bundle;
  const matches = Object.values(promotion?.environments ?? {}).filter(
    (env) =>
      env.verification?.cluster === run.cluster &&
      env.verification.namespace === run.namespace &&
      env.verification.revision === run.revision,
  );
  if (matches.length !== 1) return null;
  const digest = matches[0]?.activeDigest;
  return digest ? (promotion?.releaseByDigest[digest] ?? null) : null;
}

/**
 * Group runs sharing the same (bundle, target, revision) so multiple attempts of
 * the same revision collapse under a single row with a 1-based attempt counter
 * (latest attempt first, e.g. Attempt 3, Attempt 2, Attempt 1). Outer group order
 * preserves the caller's sorted run order.
 */
export function groupRunAttempts(
  runs: readonly E2eRunSummary[],
  promotion?: PromotionData,
): readonly E2eRunAttemptGroup[] {
  const buckets = new Map<string, E2eRunSummary[]>();
  const groupOrder: string[] = [];

  for (const run of runs) {
    const bundle = runBundle(run, promotion);
    const bundleKey =
      bundle?.digest ??
      bundle?.version ??
      run.bundle?.version ??
      run.sourceRevision ??
      "";
    const targetKey = `${run.cluster}/${run.namespace}`;
    const key = `${bundleKey}::${targetKey}::${run.revision}`;
    let bucket = buckets.get(key);
    if (!bucket) {
      bucket = [];
      buckets.set(key, bucket);
      groupOrder.push(key);
    }
    bucket.push(run);
  }

  const out: E2eRunAttemptGroup[] = [];
  for (const key of groupOrder) {
    const bucket = buckets.get(key);
    if (!bucket || bucket.length === 0) {
      continue;
    }
    const orderedNewestFirst = runsNewestFirst(bucket);
    const total = orderedNewestFirst.length;
    const attempts: E2eRunAttemptItem[] = orderedNewestFirst.map(
      (run, idx) => ({
        run,
        attemptNumber: total - idx,
      }),
    );
    const latest = attempts[0];
    if (latest) {
      out.push({
        key,
        latest,
        attempts,
      });
    }
  }
  return out;
}
