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

/** A single run's detail: summary plus ordered steps and bounded log history. */
export interface E2eRunDetail extends E2eRunSummary {
  readonly plannedSteps: readonly E2ePlannedStep[];
  readonly steps: readonly E2eStep[];
  readonly logs: readonly E2eLogLine[];
  readonly logsTruncated: boolean;
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
    plannedSteps: [],
    steps: [],
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
  plannedSteps: readonly E2ePlannedStep[];
  steps: readonly E2eStep[];
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
    plannedSteps: base.plannedSteps,
    steps: base.steps,
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
 * A run's wall-clock duration in milliseconds. Prefers the reported durationMs;
 * otherwise derives it from start/end, or from start to `nowMs` while still running.
 * Returns null when the start is unparseable.
 */
export function runDurationMs(
  run: Pick<E2eRunSummary, "startedAt" | "endedAt" | "durationMs">,
  nowMs: number,
): number | null {
  if (run.durationMs !== null) {
    return run.durationMs;
  }
  const start = Date.parse(run.startedAt);
  if (Number.isNaN(start)) {
    return null;
  }
  const end = run.endedAt !== null ? Date.parse(run.endedAt) : nowMs;
  if (Number.isNaN(end)) {
    return null;
  }
  return Math.max(end - start, 0);
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
