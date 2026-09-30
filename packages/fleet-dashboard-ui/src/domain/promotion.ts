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

/** One environment column on the promotion path. */
export interface PromotionEnvironment {
  readonly name: string;
  /** Release currently active in this environment (opaque version string). */
  readonly activeRelease: string | null;
  /** Release proposed/in-flight toward this environment, if any. */
  readonly proposedRelease: string | null;
  readonly gates: readonly PromotionGate[];
}

export interface PromotionData {
  /**
   * Environment keys in promotion order, exactly as the server orders them.
   * The UI renders columns in THIS order - it never sorts or relabels.
   */
  readonly order: readonly string[];
  readonly environments: Readonly<Record<string, PromotionEnvironment>>;
  /** Known releases, newest first as ordered by the server. */
  readonly releases: readonly string[];
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
