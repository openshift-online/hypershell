// Delivery pipeline domain (fleet-dashboard-delivery-pipeline.spec). Framework
// free: the Now and next summary, severity, the Overdue rule, arrival times,
// and the progress bar are computed here from the decoded /api/delivery payload
// and a reference instant.

export type DeliveryStageState =
  | "not-started"
  | "waiting"
  | "running"
  | "passed"
  | "failed"
  | "not-needed"
  | "unknown";

/** One column of the change table: a fixed stage or an environment group. */
export interface DeliveryStageDef {
  readonly key: string;
  readonly kind: "pipeline" | "group";
  /** The environment group's label value (group columns only). */
  readonly group: string | null;
}

export interface DeliveryStage {
  readonly key: string;
  readonly state: DeliveryStageState;
  readonly startedAt: string | null;
  readonly finishedAt: string | null;
  /** Condition code; the presentation layer owns the wording. */
  readonly reason: string | null;
  readonly components: readonly string[];
  readonly groups: readonly string[];
  readonly gate: string | null;
  readonly done: number;
  readonly total: number;
  readonly pr: number | null;
  readonly checks: string | null;
  /** Failed promotions in a Passed group (FLT-DLV-18). */
  readonly failedAttempts: number;
  /**
   * This change's estimate for the stage: the median of the phase's last 5
   * runs, per component for Build and Policy check. Null below 3 runs
   * (FLT-DLV-09).
   */
  readonly estimateMs: number | null;
  /** The slowest of the same last 5 runs; null with no estimate. */
  readonly slowestMs: number | null;
}

export interface DeliveryCheck {
  readonly component: string;
  readonly state: DeliveryStageState;
  readonly startedAt: string | null;
  readonly completedAt: string | null;
  readonly url: string | null;
  /** No known bundle lists the component: it ships by its own route. */
  readonly outsideBundle: boolean;
}

export interface DeliveryBundle {
  readonly tag: string;
  readonly version: string;
  readonly digest: string;
  readonly publishedAt: string | null;
  readonly locked: boolean;
  /** Published GitHub release; null means downloads are not published yet. */
  readonly releaseUrl: string | null;
}

export type PromotionOutcome =
  "passed" | "failed" | "replaced" | "running" | "unknown";

/** One promotion of a bundle containing the change (FLT-DLV-18). */
export interface DeliveryPromotion {
  readonly version: string;
  readonly digest: string;
  /** The GitOps update pull request that locked this bundle. */
  readonly gitopsPr: number | null;
  readonly promotedAt: string;
  /** The promotion's own pull request on the environment branch. */
  readonly url: string | null;
  readonly outcome: PromotionOutcome;
  /** The gate that failed or was still running. */
  readonly gate: string | null;
  /** When the gates of a passed promotion passed (FLT-DLV-08). */
  readonly gatesPassedAt: string | null;
}

export interface DeliveryEnvState {
  readonly env: string;
  readonly group: string;
  readonly state: DeliveryStageState;
  readonly contains: boolean;
  /** Oldest first (FLT-DLV-18). */
  readonly history: readonly DeliveryPromotion[];
  /** Promotion-payload links for Where to look (FLT-DLV-20). */
  readonly analysisUrl: string | null;
  readonly argoUrl: string | null;
}

/**
 * Whether an environment's history is worth showing: more than one attempt, or
 * one that did not pass (FLT-DLV-18).
 */
export function eventfulHistory(env: DeliveryEnvState): boolean {
  return (
    env.history.length > 1 ||
    env.history.some((a) => a.outcome !== "passed" && a.outcome !== "running")
  );
}

export interface DeliveryChange {
  readonly sha: string;
  readonly shortSha: string;
  readonly mergedAt: string;
  readonly title: string;
  readonly author: string | null;
  readonly pr: number | null;
  readonly prUrl: string | null;
  readonly commitUrl: string | null;
  readonly delivered: boolean;
  readonly deliveredAt: string | null;
  readonly stages: readonly DeliveryStage[];
  readonly builds: readonly DeliveryCheck[];
  readonly policies: readonly DeliveryCheck[];
  readonly bundle: DeliveryBundle | null;
  readonly gitopsPr: {
    readonly number: number;
    readonly url: string | null;
  } | null;
  readonly environments: readonly DeliveryEnvState[];
}

export type DeliverySourceKey =
  "upstream" | "registry" | "gitops" | "promotion" | "history";

export const DELIVERY_SOURCE_KEYS: readonly DeliverySourceKey[] = [
  "upstream",
  "registry",
  "gitops",
  "promotion",
  "history",
];

export interface DeliverySourceStatus {
  readonly stale: boolean;
  /** The source has never succeeded. */
  readonly never: boolean;
  readonly error: string | null;
  readonly lastSuccess: string | null;
}

/** One application commit linked from the release diagnostics. */
export interface DeliveryCommitLink {
  readonly sha: string;
  readonly shortSha: string;
  readonly title: string;
  readonly pr: number | null;
  readonly url: string;
  readonly mergedAt: string | null;
}

/** What the BFF read while a Release phase is Overdue (FLT-DLV-20). */
export interface ReleaseDiagnostics {
  readonly lastGood: DeliveryBundle | null;
  readonly logUrl: string | null;
  readonly appUrl: string | null;
  readonly commits: readonly DeliveryCommitLink[];
  readonly commitsError: string | null;
  readonly registryTag: string | null;
  readonly githubReleaseTag: string | null;
  readonly releaseError: string | null;
}

export interface DeliveryData {
  readonly enabled: boolean;
  readonly stages: readonly DeliveryStageDef[];
  readonly changes: readonly DeliveryChange[];
  /** Present only while a listed change's Release phase is Overdue. */
  readonly releaseDiagnostics: ReleaseDiagnostics | null;
  readonly omitted: number;
  readonly sources: Readonly<
    Partial<Record<DeliverySourceKey, DeliverySourceStatus>>
  >;
}

/** The stages each source feeds (FLT-DLV-13). Group stages belong to promotion. */
export const SOURCE_STAGES: Readonly<
  Record<DeliverySourceKey, readonly string[]>
> = {
  upstream: ["merged", "build", "policy"],
  registry: ["release"],
  gitops: ["release", "gitops"],
  promotion: [],
  history: [],
};

/** Severity of a Now and next row and of the summary heading (FLT-DLV-10). */
export type DeliverySeverity = "danger" | "warning" | "info" | "good";

const SEVERITY_RANK: Readonly<Record<DeliverySeverity, number>> = {
  good: 0,
  info: 1,
  warning: 2,
  danger: 3,
};

function parseMs(iso: string | null): number | null {
  if (iso === null || iso === "") {
    return null;
  }
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? null : ms;
}

/** Added to the slowest of the last 5 runs before a stage is Overdue. */
export const OVERDUE_MARGIN_MS = 5 * 60_000;

/** Early end of an arrival range before publication (FLT-DLV-10). */
export const EARLY_FACTOR = 0.8;

/** Elapsed time of an in-flight stage, or null when it has no start. */
export function elapsedMs(stage: DeliveryStage, nowMs: number): number | null {
  if (stage.state !== "running" && stage.state !== "waiting") {
    return null;
  }
  const start = parseMs(stage.startedAt);
  return start === null ? null : Math.max(0, nowMs - start);
}

/**
 * A Running or Waiting stage is Overdue once it has run longer than the
 * slowest of its phase's last 5 runs plus 5 minutes (FLT-DLV-04). No estimate,
 * never Overdue.
 */
export function isOverdue(stage: DeliveryStage, nowMs: number): boolean {
  const elapsed = elapsedMs(stage, nowMs);
  return (
    stage.slowestMs !== null &&
    elapsed !== null &&
    elapsed > stage.slowestMs + OVERDUE_MARGIN_MS
  );
}

/** Whether any stage of the change is Overdue. */
export function changeOverdue(change: DeliveryChange, nowMs: number): boolean {
  return change.stages.some((s) => isOverdue(s, nowMs));
}

function done(stage: DeliveryStage): boolean {
  return stage.state === "passed" || stage.state === "not-needed";
}

/** An expected arrival in an environment group (FLT-DLV-10). */
export type Arrival =
  /** The group finished the change at this time (null when unknown). */
  | { readonly kind: "reached"; readonly atMs: number | null }
  /** Before the bundle is published: from early to late. */
  | {
      readonly kind: "range";
      readonly earlyMs: number;
      readonly lateMs: number;
    }
  /** After the bundle is published: one time. */
  | { readonly kind: "time"; readonly atMs: number }
  /** No arrival: an estimate is missing, a stage failed, or one is Overdue. */
  | { readonly kind: "none" };

interface Remaining {
  /** Remaining estimate and slowest time up to and including each stage. */
  readonly estimate: readonly (number | null)[];
  readonly slowest: readonly (number | null)[];
}

/**
 * The remaining time to the end of each stage: the current stage's estimate
 * (and slowest value) minus its elapsed time, never below zero, plus the
 * estimates of every later stage. Null from the first stage without an
 * estimate on.
 */
function remaining(change: DeliveryChange, nowMs: number): Remaining {
  const estimate: (number | null)[] = [];
  const slowest: (number | null)[] = [];
  let est: number | null = 0;
  let slow: number | null = 0;
  for (const stage of change.stages) {
    if (stage.key === "merged" || done(stage)) {
      estimate.push(est);
      slowest.push(slow);
      continue;
    }
    if (stage.estimateMs === null || stage.slowestMs === null) {
      est = null;
      slow = null;
    } else if (est !== null && slow !== null) {
      const elapsed = elapsedMs(stage, nowMs) ?? 0;
      est += Math.max(0, stage.estimateMs - elapsed);
      slow += Math.max(0, stage.slowestMs - elapsed);
    }
    estimate.push(est);
    slowest.push(slow);
  }
  return { estimate, slowest };
}

/** Whether the change's bundle is published: its Release stage passed. */
function published(change: DeliveryChange): boolean {
  return change.stages.some((s) => s.key === "release" && s.state === "passed");
}

/**
 * The expected arrival of a change in each environment group, keyed by stage
 * key (FLT-DLV-10).
 */
export function arrivals(
  data: DeliveryData,
  change: DeliveryChange,
  nowMs: number,
): ReadonlyMap<string, Arrival> {
  const out = new Map<string, Arrival>();
  const blocked =
    change.stages.some((s) => s.state === "failed") ||
    changeOverdue(change, nowMs);
  const rem = remaining(change, nowMs);
  const isPublished = published(change);
  change.stages.forEach((stage, i) => {
    const def = data.stages.find((d) => d.key === stage.key);
    if (def?.kind !== "group") {
      return;
    }
    if (stage.state === "passed") {
      out.set(stage.key, { kind: "reached", atMs: parseMs(stage.finishedAt) });
      return;
    }
    const est = rem.estimate[i] ?? null;
    const slow = rem.slowest[i] ?? null;
    if (blocked || est === null || slow === null) {
      out.set(stage.key, { kind: "none" });
      return;
    }
    out.set(
      stage.key,
      isPublished
        ? { kind: "time", atMs: nowMs + est }
        : {
            kind: "range",
            earlyMs: nowMs + EARLY_FACTOR * est,
            lateMs: nowMs + slow,
          },
    );
  });
  return out;
}

/** One progress-bar cell (FLT-DLV-19). */
export interface ProgressCell {
  /** 1-based phase number. */
  readonly number: number;
  readonly stage: DeliveryStage;
  readonly overdue: boolean;
  /** For an environment group: its arrival. */
  readonly arrival: Arrival | null;
}

export interface ProgressModel {
  readonly cells: readonly ProgressCell[];
  /** Whole percent complete, or null when it cannot be told. */
  readonly percent: number | null;
}

/**
 * The progress bar: one cell per phase after Merged, and percent complete as
 * elapsed / (elapsed + remaining) (FLT-DLV-19).
 */
export function progress(
  data: DeliveryData,
  change: DeliveryChange,
  nowMs: number,
): ProgressModel {
  const arr = arrivals(data, change, nowMs);
  const cells: ProgressCell[] = change.stages
    .filter((s) => s.key !== "merged")
    .map((stage, i) => ({
      number: i + 1,
      stage,
      overdue: isOverdue(stage, nowMs),
      arrival: arr.get(stage.key) ?? null,
    }));
  if (change.delivered) {
    return { cells, percent: 100 };
  }
  const blocked =
    change.stages.some((s) => s.state === "failed") ||
    changeOverdue(change, nowMs);
  const left = remaining(change, nowMs).estimate.at(-1) ?? null;
  const merged = parseMs(change.mergedAt);
  if (blocked || left === null || merged === null) {
    return { cells, percent: null };
  }
  const elapsed = Math.max(0, nowMs - merged);
  const total = elapsed + left;
  return {
    cells,
    percent: total === 0 ? 100 : Math.floor((100 * elapsed) / total),
  };
}

/** What a Where to look popup shows for one stage (FLT-DLV-20). */
export type WhereToLook =
  | {
      readonly kind: "build-failed";
      readonly checks: readonly DeliveryCheck[];
    }
  | {
      readonly kind: "checks-overdue";
      readonly checks: readonly DeliveryCheck[];
    }
  | {
      readonly kind: "release-overdue";
      readonly diagnostics: ReleaseDiagnostics | null;
    }
  | {
      readonly kind: "gitops";
      readonly pr: DeliveryChange["gitopsPr"];
      readonly checks: string | null;
    }
  | {
      readonly kind: "group";
      readonly environments: readonly DeliveryEnvState[];
    };

/**
 * The Where to look content for an Overdue or Failed stage, or null for a
 * stage that offers none (FLT-DLV-20).
 */
export function whereToLook(
  data: DeliveryData,
  change: DeliveryChange,
  stage: DeliveryStage,
  nowMs: number,
): WhereToLook | null {
  const failed = stage.state === "failed";
  if (!failed && !isOverdue(stage, nowMs)) {
    return null;
  }
  switch (stage.key) {
    case "build":
      return failed
        ? {
            kind: "build-failed",
            checks: change.builds.filter((c) => c.state === "failed"),
          }
        : {
            kind: "checks-overdue",
            checks: change.builds.filter((c) => c.state === "running"),
          };
    case "policy":
      return {
        kind: "checks-overdue",
        checks: change.policies.filter((c) =>
          failed ? c.state === "failed" : c.state === "running",
        ),
      };
    case "release":
      return { kind: "release-overdue", diagnostics: data.releaseDiagnostics };
    case "gitops":
      return { kind: "gitops", pr: change.gitopsPr, checks: stage.checks };
    case "merged":
      return null;
    default: {
      const group = stage.key.replace(/^env:/, "");
      return {
        kind: "group",
        environments: change.environments.filter(
          (e) => e.group === group && e.state !== "passed",
        ),
      };
    }
  }
}

/** The earliest stage that is not Passed or Not needed, or null when none. */
export function currentStageIndex(change: DeliveryChange): number | null {
  const i = change.stages.findIndex(
    (s) => s.state !== "passed" && s.state !== "not-needed",
  );
  return i === -1 ? null : i;
}

export interface NowPart {
  readonly stage: DeliveryStage;
  readonly elapsedMs: number | null;
  readonly estimateMs: number | null;
  /** This stage is Overdue (FLT-DLV-04). */
  readonly stageOverdue: boolean;
  /** Any stage of the change is Overdue: no arrival time (FLT-DLV-10). */
  readonly overdue: boolean;
}

export type NextPart =
  | {
      readonly kind: "stage";
      readonly stage: DeliveryStage;
      /** When the next stage is expected to finish; null when unknown or Overdue. */
      readonly expectedAtMs: number | null;
    }
  /** The current stage failed: what restarts the pipeline. */
  | { readonly kind: "restart"; readonly restart: "new-change" | "gate-retry" }
  /** The current stage is the last one. */
  | { readonly kind: "none" };

export interface NowNextRow {
  readonly change: DeliveryChange;
  readonly now: NowPart;
  readonly next: NextPart;
  readonly severity: DeliverySeverity;
}

export interface NowNextSummary {
  readonly rows: readonly NowNextRow[];
  /** Undelivered changes beyond the three shown. */
  readonly more: number;
  readonly severity: DeliverySeverity;
  /** Set when every listed change is delivered (FLT-DLV-10.02). */
  readonly delivered: {
    readonly change: DeliveryChange;
    readonly deliveredAtMs: number | null;
    readonly totalMs: number | null;
  } | null;
}

/** At most this many rows in the summary (FLT-DLV-10). */
export const NOW_NEXT_LIMIT = 3;

function nowNextRow(
  data: DeliveryData,
  change: DeliveryChange,
  index: number,
  nowMs: number,
): NowNextRow {
  const stage = change.stages[index];
  if (stage === undefined) {
    throw new Error("stage index out of range");
  }
  const now: NowPart = {
    stage,
    elapsedMs: elapsedMs(stage, nowMs),
    estimateMs: stage.estimateMs,
    stageOverdue: isOverdue(stage, nowMs),
    overdue: changeOverdue(change, nowMs),
  };

  let next: NextPart;
  if (stage.state === "failed") {
    const def = data.stages.find((d) => d.key === stage.key);
    next = {
      kind: "restart",
      restart: def?.kind === "group" ? "gate-retry" : "new-change",
    };
  } else {
    const following = change.stages[index + 1];
    if (following === undefined) {
      next = { kind: "none" };
    } else {
      // Remaining time of the current stage (never below zero) plus the
      // estimate of the next one; none while a stage is Overdue (FLT-DLV-10).
      let expectedAtMs: number | null = null;
      if (
        !now.overdue &&
        now.estimateMs !== null &&
        following.estimateMs !== null
      ) {
        const left = Math.max(0, now.estimateMs - (now.elapsedMs ?? 0));
        expectedAtMs = nowMs + left + following.estimateMs;
      }
      next = { kind: "stage", stage: following, expectedAtMs };
    }
  }

  const severity: DeliverySeverity =
    stage.state === "failed" ? "danger" : now.overdue ? "warning" : "info";
  return { change, now, next, severity };
}

/** The Now and next summary (FLT-DLV-10). */
export function nowAndNext(data: DeliveryData, nowMs: number): NowNextSummary {
  const inFlight: NowNextRow[] = [];
  for (const change of data.changes) {
    if (change.delivered) {
      continue;
    }
    const index = currentStageIndex(change);
    if (index === null) {
      continue;
    }
    inFlight.push(nowNextRow(data, change, index, nowMs));
  }

  if (inFlight.length === 0) {
    const newest = data.changes[0];
    if (!newest?.delivered) {
      return { rows: [], more: 0, severity: "info", delivered: null };
    }
    const deliveredAtMs = parseMs(newest.deliveredAt);
    const mergedMs = parseMs(newest.mergedAt);
    return {
      rows: [],
      more: 0,
      severity: "good",
      delivered: {
        change: newest,
        deliveredAtMs,
        totalMs:
          deliveredAtMs !== null && mergedMs !== null
            ? Math.max(0, deliveredAtMs - mergedMs)
            : null,
      },
    };
  }

  // Severity follows every undelivered change, not only the three shown.
  const severity = inFlight.reduce<DeliverySeverity>(
    (worst, row) =>
      SEVERITY_RANK[row.severity] > SEVERITY_RANK[worst] ? row.severity : worst,
    "info",
  );
  return {
    rows: inFlight.slice(0, NOW_NEXT_LIMIT),
    more: Math.max(0, inFlight.length - NOW_NEXT_LIMIT),
    severity,
    delivered: null,
  };
}

/** Sources to warn about: stale or never succeeded (FLT-DLV-13). */
export function degradedSources(
  data: DeliveryData,
): readonly { key: DeliverySourceKey; status: DeliverySourceStatus }[] {
  const out: { key: DeliverySourceKey; status: DeliverySourceStatus }[] = [];
  for (const key of DELIVERY_SOURCE_KEYS) {
    const status = data.sources[key];
    if (status !== undefined && (status.stale || status.never)) {
      out.push({ key, status });
    }
  }
  return out;
}

/** The application repository never succeeded: no change list can be built. */
export function changeListUnavailable(data: DeliveryData): boolean {
  return data.sources.upstream?.never === true && data.changes.length === 0;
}

/** Title-case an environment group's label value for its column header. */
export function groupLabel(group: string): string {
  return `${(group[0] ?? "").toUpperCase()}${group.slice(1)}`;
}

/** Find a listed change by full SHA. */
export function findChange(
  data: DeliveryData,
  sha: string | null,
): DeliveryChange | null {
  if (sha === null) {
    return null;
  }
  return data.changes.find((c) => c.sha === sha) ?? null;
}
