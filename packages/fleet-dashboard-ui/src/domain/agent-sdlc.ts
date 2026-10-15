// Agent SDLC plane: the label-driven agent funnel made observable
// (agent-sdlc-visibility-dashboard.spec). The BFF reads funnel state from the
// GitHub timeline behind a provider port and reuses the cached promotion bundle
// truth for the release-by-hub-cluster matrix; this module is the framework-free
// domain the wire adapter decodes /api/sdlc into, plus the pure projections the
// board, rework meter, escalation lane, and matrix render from.
//
// FIREWALL: nothing about the fleet is baked in here. Phase keys are the funnel's
// own vocabulary; every issue title, bundle version, hub-cluster name, and digest
// arrives at runtime inside the payload, never a compiled-in table.

/** A funnel phase column. SLESeconds is 0 for the terminal Shipped column. */
export interface AgentSdlcPhase {
  readonly key: string;
  readonly sleSeconds: number;
}

/** A card's progress toward its phase block threshold (spec 3, code 10). */
export interface AgentSdlcRework {
  readonly count: number;
  readonly threshold: number;
}

/** One label transition in a record's reconstructed history (state over time). */
export interface AgentSdlcLabelEvent {
  readonly label: string;
  readonly action: string; // "labeled" | "unlabeled"
  readonly at: string; // RFC3339
}

/** One execution record on the board or in the escalation lane. */
export interface AgentSdlcCard {
  readonly number: number;
  readonly title: string;
  readonly url: string;
  readonly phase: string;
  readonly timeInPhaseSeconds: number;
  /** Escalation bucket when in the lane: blocked/needs-input/duplicate/past-sle. */
  readonly escalation: string | null;
  readonly rework: AgentSdlcRework | null;
  readonly history: readonly AgentSdlcLabelEvent[];
}

/** One matrix column: a hub cluster and the bundle it currently runs. */
export interface AgentSdlcHubCluster {
  readonly key: string;
  readonly cluster: string | null;
  readonly env: string | null;
  readonly currentVersion: string | null;
  readonly currentDigest: string | null;
}

/** One execution record that shipped (or awaits a bundle, in Unreleased). */
export interface AgentSdlcShippedIssue {
  readonly number: number;
  readonly title: string;
  readonly url: string;
  readonly mergedAt: string | null;
}

/** One matrix row: a release bundle (newest first) or the single Unreleased row. */
export interface AgentSdlcBundleRow {
  readonly version: string | null;
  readonly digest: string | null;
  readonly date: string | null;
  readonly unreleased: boolean;
  /** Instance keys currently running this bundle. */
  readonly hubClusters: readonly string[];
  readonly issues: readonly AgentSdlcShippedIssue[];
}

/** The release-by-hub-cluster matrix (the Shipped column). */
export interface AgentSdlcMatrix {
  readonly hubClusters: readonly AgentSdlcHubCluster[];
  readonly rows: readonly AgentSdlcBundleRow[];
}

/** The /api/sdlc plane. */
export interface AgentSdlcData {
  readonly phases: readonly AgentSdlcPhase[];
  readonly cards: readonly AgentSdlcCard[];
  readonly lane: readonly AgentSdlcCard[];
  readonly matrix: AgentSdlcMatrix;
}

/** Escalation bucket constants, shared by the lane projection and the view. */
export const ESCALATION_BLOCKED = "blocked";
export const ESCALATION_NEEDS_INPUT = "needs-input";
export const ESCALATION_DUPLICATE = "duplicate";
export const ESCALATION_PAST_SLE = "past-sle";

/** Terminal phase key: the Shipped column is the matrix, not a timed column. */
export const PHASE_SHIPPED = "shipped";

/** The board columns (timed phases) in funnel order, excluding Shipped. */
export function boardPhases(
  phases: readonly AgentSdlcPhase[],
): readonly AgentSdlcPhase[] {
  return phases.filter((p) => p.key !== PHASE_SHIPPED);
}

/** Cards currently sitting in a given phase column. */
export function cardsInPhase(
  cards: readonly AgentSdlcCard[],
  phase: string,
): readonly AgentSdlcCard[] {
  return cards
    .filter((c) => c.phase === phase)
    .slice()
    .sort((a, b) => a.timeInPhaseSeconds - b.timeInPhaseSeconds);
}

/**
 * The vertical scale (seconds) a column's aging track spans, so a card's
 * time-in-phase maps to a position and the SLE line has somewhere to sit. It is the
 * larger of a comfortable multiple of the SLE and the stalest card, so a card past
 * the SLE still lands below the line rather than pinned to the floor.
 */
export function agingScaleSeconds(
  phase: AgentSdlcPhase,
  cards: readonly AgentSdlcCard[],
): number {
  const sleHeadroom = phase.sleSeconds > 0 ? phase.sleSeconds * 1.5 : 0;
  const stalest = cards.reduce((m, c) => Math.max(m, c.timeInPhaseSeconds), 0);
  return Math.max(sleHeadroom, stalest, 1);
}

/** Fraction (0..1) of a column's track a value sits at: fresh at top, stale low. */
export function agingRatio(seconds: number, scaleSeconds: number): number {
  if (scaleSeconds <= 0) {
    return 0;
  }
  return Math.min(Math.max(seconds / scaleSeconds, 0), 1);
}

/** Fraction (0..1) a rework meter is filled toward its threshold. */
export function reworkRatio(rework: AgentSdlcRework): number {
  if (rework.threshold <= 0) {
    return 0;
  }
  return Math.min(Math.max(rework.count / rework.threshold, 0), 1);
}

/**
 * Rework severity as a non-color cue (SDLC-VIS-09/16): "ok" below two-thirds,
 * "warning" below the last strike, "critical" when one more strike blocks.
 */
export type ReworkSeverity = "ok" | "warning" | "critical";

export function reworkSeverity(rework: AgentSdlcRework): ReworkSeverity {
  if (rework.count >= rework.threshold - 1) {
    return "critical";
  }
  if (reworkRatio(rework) >= 2 / 3) {
    return "warning";
  }
  return "ok";
}

/** The cross-link seed for a bundle row, matching the map's seedForBundle. */
export function bundleRowSeed(row: AgentSdlcBundleRow): string | null {
  return row.digest ?? row.version;
}
