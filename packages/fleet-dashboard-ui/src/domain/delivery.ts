// Delivery pipeline domain (fleet-dashboard-delivery-pipeline.spec). Framework
// free: the Now and next summary, severity, and the slow-stage rule are computed
// here from the decoded /api/delivery payload and a reference instant.

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
}

export interface DeliveryCheck {
  readonly component: string;
  readonly state: DeliveryStageState;
  readonly startedAt: string | null;
  readonly completedAt: string | null;
  readonly url: string | null;
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
}

export interface DeliveryEnvState {
  readonly env: string;
  readonly group: string;
  readonly state: DeliveryStageState;
  readonly contains: boolean;
  /** Oldest first (FLT-DLV-18). */
  readonly history: readonly DeliveryPromotion[];
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

export interface DeliveryData {
  readonly enabled: boolean;
  readonly stages: readonly DeliveryStageDef[];
  readonly changes: readonly DeliveryChange[];
  /** Median duration per stage key, in seconds; absent when too little history. */
  readonly typicalSeconds: Readonly<Record<string, number>>;
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

/** Typical duration of a stage in ms, or null below three samples (FLT-DLV-09). */
export function typicalMs(data: DeliveryData, key: string): number | null {
  const s = data.typicalSeconds[key];
  return s === undefined ? null : s * 1000;
}

/** Elapsed time of an in-flight stage, or null when it has no start. */
export function elapsedMs(stage: DeliveryStage, nowMs: number): number | null {
  if (stage.state !== "running" && stage.state !== "waiting") {
    return null;
  }
  const start = parseMs(stage.startedAt);
  return start === null ? null : Math.max(0, nowMs - start);
}

/**
 * A Running or Waiting stage is slower than usual when it has lasted more than
 * twice its typical duration (FLT-DLV-04). No typical duration, no warning.
 */
export function isSlow(
  data: DeliveryData,
  stage: DeliveryStage,
  nowMs: number,
): boolean {
  const typical = typicalMs(data, stage.key);
  const elapsed = elapsedMs(stage, nowMs);
  return typical !== null && elapsed !== null && elapsed > 2 * typical;
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
  readonly typicalMs: number | null;
  readonly slow: boolean;
}

export type NextPart =
  | {
      readonly kind: "stage";
      readonly stage: DeliveryStage;
      /** When the next stage is expected to finish; null when unknown. */
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
    typicalMs: typicalMs(data, stage.key),
    slow: isSlow(data, stage, nowMs),
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
      // Remaining typical time of the current stage (never below zero) plus the
      // typical duration of the next one (FLT-DLV-10).
      const nextTypical = typicalMs(data, following.key);
      let expectedAtMs: number | null = null;
      if (now.typicalMs !== null && nextTypical !== null) {
        const remaining = Math.max(0, now.typicalMs - (now.elapsedMs ?? 0));
        expectedAtMs = nowMs + remaining + nextTypical;
      }
      next = { kind: "stage", stage: following, expectedAtMs };
    }
  }

  const severity: DeliverySeverity =
    stage.state === "failed" ? "danger" : now.slow ? "warning" : "info";
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
