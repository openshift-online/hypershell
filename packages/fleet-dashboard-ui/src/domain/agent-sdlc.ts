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
  /** The agent/scope/* selector label this record carries, if any (for filtering). */
  readonly scope: string | null;
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

/** One pull request shipped in a bundle. Bundle contents are PR-driven
 *  (SDLC-VIS-03): these mirror the shared bundle PRs the Fleet Map freight bar
 *  shows, so a pull request is never hidden because its issue link is unrecoverable. */
export interface AgentSdlcPullRequest {
  readonly number: number;
  readonly title: string;
  readonly url: string;
  readonly author: string | null;
  readonly mergedAt: string | null;
}

/** One tracked execution record that shipped (or awaits a bundle, in Unreleased).
 *  In a bundle it carries the bundle's pull requests that resolve to it (prs), so a
 *  single issue delivered by several PRs appears once with those PRs grouped beneath
 *  it; in the Unreleased row prs is empty. */
export interface AgentSdlcShippedIssue {
  readonly number: number;
  readonly title: string;
  readonly url: string;
  readonly mergedAt: string | null;
  /** GitHub login of whoever merged the closing PR, if known. */
  readonly mergedBy: string | null;
  /** The bundle's pull requests grouped under this tracked issue (empty in Unreleased). */
  readonly prs: readonly AgentSdlcPullRequest[];
}

/** One matrix row: a release bundle (newest first) or the single Unreleased row.
 *  A bundle row's contents are PR-driven and split into two groups: issues (the
 *  bundle's PRs grouped under the tracked execution record they resolve to) and
 *  otherPRs (one-off PRs with no tracked issue). The Unreleased row carries only
 *  tracked issues and no otherPRs. */
export interface AgentSdlcBundleRow {
  readonly version: string | null;
  readonly digest: string | null;
  readonly date: string | null;
  readonly unreleased: boolean;
  /** Instance keys currently running this bundle. */
  readonly hubClusters: readonly string[];
  readonly issues: readonly AgentSdlcShippedIssue[];
  readonly otherPRs: readonly AgentSdlcPullRequest[];
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

/**
 * A single scale (seconds) shared by every board column so one vertical time axis
 * reads across all of them: the larger of a comfortable multiple of the biggest
 * phase SLE and the stalest board card, floored at an hour. Per-column SLE lines
 * then sit at their own heights against this common axis.
 */
export function globalAgingScaleSeconds(
  phases: readonly AgentSdlcPhase[],
  cards: readonly AgentSdlcCard[],
): number {
  const sleMax = phases.reduce((m, p) => Math.max(m, p.sleSeconds), 0);
  const ageMax = cards.reduce((m, c) => Math.max(m, c.timeInPhaseSeconds), 0);
  return Math.max(sleMax * 1.3, ageMax * 1.1, 3600);
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

/**
 * A reserved stable expand key for the single Unreleased matrix row, which has no
 * digest or version to address it by. The sentinel wrapping keeps it from colliding
 * with a real bundle seed (a sha256 digest or a release version).
 */
export const UNRELEASED_ROW_KEY = "__unreleased__";

/**
 * The stable expand/collapse key for a matrix row. Released rows use their bundle
 * seed (digest, version fallback); the Unreleased row has neither, so it falls back
 * to a reserved synthetic token and stays expandable across refresh reorders.
 */
export function bundleRowExpandKey(row: AgentSdlcBundleRow): string {
  return bundleRowSeed(row) ?? UNRELEASED_ROW_KEY;
}

/**
 * Total pull requests in a bundle row: every grouped pull request under a tracked
 * issue plus every one-off pull request. This is the "M PRs" count in the row summary
 * (SDLC-VIS-11), equal to the bundle's whole PR set since each PR is either grouped
 * under a tracked issue or listed as a one-off.
 */
export function bundlePRCount(row: AgentSdlcBundleRow): number {
  const grouped = row.issues.reduce((sum, issue) => sum + issue.prs.length, 0);
  return grouped + row.otherPRs.length;
}

/**
 * One card phase-transition flight (SDLC-VIS-12a/12b): the board analogue of the map's
 * Flight. The whole card flies from its old column slot to its new one.
 */
export interface CardFlight {
  readonly number: number;
  /** The server phase the card left. null when the card is newly on the board (an
   *  intake entry that flies in, the board analogue of the map's release-tray special). */
  readonly fromPhase: string | null;
  /** The server phase the card arrived in; PHASE_SHIPPED for the final hop into the
   *  release matrix. */
  readonly toPhase: string;
  /** True when the card left the board into the Shipped release matrix. */
  readonly toShipped: boolean;
}

/** Issue numbers that appear as shipped (tracked) in the matrix for a snapshot. */
function shippedNumbers(data: AgentSdlcData): Set<number> {
  const nums = new Set<number>();
  for (const row of data.matrix.rows) {
    for (const issue of row.issues) {
      nums.add(issue.number);
    }
  }
  return nums;
}

/**
 * Phase-transition flights for the transition prev -> next, derived PURELY by diffing
 * the two already-fetched SDLC snapshots (the board analogue of the map's computeFlights,
 * SDLC-VIS-12b). A flight is emitted when a card's server `phase` changes between the
 * snapshots, when a card newly appears on the board (intake entry), or when a board card
 * leaves into the Shipped matrix. Detection keys off the server `phase` ONLY (never the
 * continuously aged copy), so the one-second continuous-aging re-render does not spuriously
 * emit flights. Returns [] on first render (no prior snapshot) so the board never flies on
 * initial load.
 */
export function computeCardFlights(
  prev: AgentSdlcData | null,
  next: AgentSdlcData,
): CardFlight[] {
  if (!prev) return [];

  const prevPhaseByNumber = new Map(prev.cards.map((c) => [c.number, c.phase]));
  const prevNumbers = new Set(prev.cards.map((c) => c.number));
  const nextNumbers = new Set(next.cards.map((c) => c.number));
  const nextShipped = shippedNumbers(next);

  const flights: CardFlight[] = [];

  // Cards present on the board in the new snapshot: a phase change flies between columns;
  // a brand-new card flies in at intake.
  for (const card of next.cards) {
    const before = prevPhaseByNumber.get(card.number);
    if (before === undefined) {
      // Only count it as an intake entry if it was not simply shipped-and-returned; a
      // card the previous snapshot had nowhere is a genuine new arrival.
      if (!prevNumbers.has(card.number)) {
        flights.push({
          number: card.number,
          fromPhase: null,
          toPhase: card.phase,
          toShipped: false,
        });
      }
      continue;
    }
    if (before !== card.phase) {
      flights.push({
        number: card.number,
        fromPhase: before,
        toPhase: card.phase,
        toShipped: false,
      });
    }
  }

  // Cards that left the board into the Shipped matrix: fly the departing card into the
  // release matrix (subsumes the old separate ship-glyph animation).
  for (const card of prev.cards) {
    if (nextNumbers.has(card.number)) continue;
    if (!nextShipped.has(card.number)) continue;
    flights.push({
      number: card.number,
      fromPhase: card.phase,
      toPhase: PHASE_SHIPPED,
      toShipped: true,
    });
  }

  return flights;
}
