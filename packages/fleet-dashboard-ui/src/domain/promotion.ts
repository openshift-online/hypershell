// Promotion plane. THE FLEET-IDENTITY FIREWALL LIVES HERE.
//
// This module hard-codes NOTHING about the fleet: no environment names, no hub
// mapping, no per-gate governing instance, no ordering. Every one of those is
// DERIVED at runtime from the /api/promotion payload. A static
//   const ENV_HUB = { <env>: "<hub-instance>", ... }   // NEVER do this
// would bake the env→hub topology into public source; that is exactly the leak
// this file exists to prevent - see data-architecture.spec §3.5 in the gitops repo.

import type { StatusBadge } from "./status";
import { gatePhaseBadge } from "./status";

/** A gate observed on the promotion path (e.g. a required commit status). */
export interface PromotionGate {
  readonly name: string;
  readonly phase: string | null;
  /**
   * The instance that governs this gate, as reported by the server. NEVER
   * inferred from the environment name - it arrives in the payload or is null.
   */
  readonly governingInstance: string | null;
}

/**
 * A release bundle as reported by the server. `digest` is the image/commit sha
 * the server attributes to the release; it is the stable seed the map uses for
 * the instance identicon + identiname so a bundle always draws the same way. All
 * fields are opaque data - none are parsed for fleet structure.
 */
export interface ReleaseBundle {
  readonly version: string;
  readonly digest: string | null;
  readonly tag: string | null;
  readonly date: string | null;
}

/**
 * One environment entry on the promotion path. In the live payload each entry is
 * a managed instance carrying its delivery labels (role/provider/env/cluster) and
 * Argo status - all DATA from the server, never inferred from the key. The map
 * derives columns from `envLabel` and lanes from `provider`; both degrade to the
 * key when the server omits them.
 */
export interface PromotionEnvironment {
  readonly name: string;
  /** Release currently active in this environment (opaque version string). */
  readonly activeRelease: string | null;
  /** Release proposed/in-flight toward this environment, if any. */
  readonly proposedRelease: string | null;
  readonly gates: readonly PromotionGate[];
  /** Image/commit digest of the active release (identicon/identiname seed). */
  readonly activeDigest: string | null;
  /** Image/commit digest of the proposed release, if any. */
  readonly proposedDigest: string | null;
  /** hub | spoke, from delivery labels at runtime. Null when unlabeled. */
  readonly role: string | null;
  /** Cloud provider, from delivery labels. Lanes group by this. Null when absent. */
  readonly provider: string | null;
  /** Promotion environment (e.g. the column this node sits in). Null => own column. */
  readonly envLabel: string | null;
  /** Cluster the instance runs on, from delivery labels. */
  readonly cluster: string | null;
  readonly argoHealth: string | null;
  readonly argoSync: string | null;
  /** Deep link to the instance's console, when the server supplies one. */
  readonly consoleUrl: string | null;
  /** Deep link to the Argo CD application, when the server supplies one. */
  readonly argoUrl: string | null;
  /** Open/closed/merged state of the promotion PR into this env, if any. */
  readonly prState: string | null;
  /** Deep link to the promotion PR, when present. */
  readonly prUrl: string | null;
  /** Deep link to the analysis check-run for the active hydrated commit. */
  readonly analysisUrl: string | null;
  /** True when active == proposed (nothing in flight toward this env). */
  readonly upToDate: boolean;
}

export interface PromotionData {
  /**
   * Environment keys in promotion order, exactly as the server orders them.
   * The UI renders columns in THIS order - it never sorts or relabels.
   */
  readonly order: readonly string[];
  readonly environments: Readonly<Record<string, PromotionEnvironment>>;
  /** Known release version strings, newest first as ordered by the server. */
  readonly releases: readonly string[];
  /** Full release bundles keyed by digest, for the freight-bar cards. */
  readonly releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  /** Newest release across all envs, as the server computes it. Null when unknown. */
  readonly frontier: ReleaseBundle | null;
}

/** Coarse promotion state of an environment (mirrors the prototype's tri-state). */
export type PromotionState = "up-to-date" | "promoting" | "behind";

/**
 * Promotion state of an environment: up-to-date when active == proposed, else
 * promoting when a PR is open toward it, else behind. Derived only from
 * server-reported fields - no env/instance table.
 */
export function promotionState(env: PromotionEnvironment): PromotionState {
  if (env.upToDate) {
    return "up-to-date";
  }
  if ((env.prState ?? "").toLowerCase() === "open") {
    return "promoting";
  }
  return "behind";
}

/**
 * Resolve the promotion columns in server-defined order. Environments named in
 * `order` but absent from `environments` are skipped (fail-soft on partial
 * payloads); environments present but not ordered are appended in payload order
 * so nothing silently disappears.
 */
export function orderedEnvironments(
  promotion: PromotionData,
): readonly PromotionEnvironment[] {
  const seen = new Set<string>();
  const columns: PromotionEnvironment[] = [];
  for (const key of promotion.order) {
    const env = promotion.environments[key];
    if (env && !seen.has(key)) {
      columns.push(env);
      seen.add(key);
    }
  }
  for (const [key, env] of Object.entries(promotion.environments)) {
    if (!seen.has(key)) {
      columns.push(env);
      seen.add(key);
    }
  }
  return columns;
}

/**
 * The governing instance for an environment, DERIVED from its gates. Returns the
 * first non-null governing instance the server reported, or null. There is no
 * environment→instance table anywhere in this codebase.
 */
export function governingInstance(env: PromotionEnvironment): string | null {
  for (const gate of env.gates) {
    if (gate.governingInstance) {
      return gate.governingInstance;
    }
  }
  return null;
}

/** Worst-case badge across an environment's gates (danger > warning > unknown > success). */
export function environmentGateBadge(env: PromotionEnvironment): StatusBadge {
  const rank: Record<StatusBadge["tone"], number> = {
    danger: 4,
    warning: 3,
    unknown: 2,
    info: 1,
    success: 0,
  };
  let worst: StatusBadge = { tone: "success", labelKey: "passed" };
  let worstRank = -1;
  let anyGate = false;
  for (const gate of env.gates) {
    anyGate = true;
    const badge = gatePhaseBadge(gate.phase);
    if (rank[badge.tone] > worstRank) {
      worst = badge;
      worstRank = rank[badge.tone];
    }
  }
  return anyGate ? worst : { tone: "unknown", labelKey: "unknown" };
}

/** True when a proposed release differs from what is active (promotion pending). */
export function hasPendingPromotion(env: PromotionEnvironment): boolean {
  return (
    env.proposedRelease !== null && env.proposedRelease !== env.activeRelease
  );
}
