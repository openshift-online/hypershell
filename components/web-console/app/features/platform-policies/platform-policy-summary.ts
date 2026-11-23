import type { PolicyMode } from "@openshift-online/hypershell-gateway-management-ui";

import type {
  PlatformPolicy,
  PlatformPolicyFailureKind,
  PlatformPolicyGatewayStatus,
  PlatformPolicyTarget,
} from "../../application/platform-policies/platform-policy-types";

/*
 * Pure summaries for the admin list, toggle confirmation, and detail views.
 * Kept free of React so the impact numbers a confirmation shows are testable
 * on their own (WEB-PADM-03).
 */

export type PolicyTargetKind = "all" | "except" | "selected";

export function policyTargetSummary(target: PlatformPolicyTarget): {
  count: number;
  kind: PolicyTargetKind;
} {
  if (target.all) {
    return target.exclude.length
      ? { count: target.exclude.length, kind: "except" }
      : { count: 0, kind: "all" };
  }
  return { count: target.gatewayIds.length, kind: "selected" };
}

export type RolloutAppearance = "danger" | "good" | "warning";

/** Maps a rollout appearance to the canonical gateway status token. */
export const rolloutStatusToken: Readonly<Record<RolloutAppearance, string>> = {
  danger: "failed",
  good: "healthy",
  warning: "warning",
};

export function rolloutSummary(policy: PlatformPolicy) {
  const counted = policy.gateways.length
    ? {
        applied: policy.gateways.filter(
          ({ condition }) => condition === "applied",
        ).length,
        pending: policy.gateways.filter(
          ({ condition }) => condition === "pending",
        ).length,
        unsupported: policy.gateways.filter(
          ({ condition }) => condition === "unsupported",
        ).length,
      }
    : policy.rollout;
  const total = counted.applied + counted.pending + counted.unsupported;
  const appearance: RolloutAppearance = counted.unsupported
    ? "danger"
    : counted.pending
      ? "warning"
      : "good";
  return { ...counted, appearance, total };
}

export function gatewayLabel(gateway: PlatformPolicyGatewayStatus): string {
  return gateway.gatewayName ?? gateway.gatewayId;
}

/** One row of the per-gateway impact table in an enforce confirmation. */
export interface GatewayImpact {
  gatewayId: string;
  label: string;
  message?: string;
  note?: "pending" | "unsupported";
  wouldReject: number;
}

/**
 * The gateways an enforce or audit change affects and their last-24-hour
 * would-reject counts. Rows with nothing to report are dropped from the table
 * but still counted in `gatewayCount`.
 */
export function enforcementImpact(policies: readonly PlatformPolicy[]): {
  gatewayCount: number;
  rows: readonly GatewayImpact[];
  wouldAddDefaults: number;
  wouldReject: number;
} {
  const byGateway = new Map<string, GatewayImpact>();
  for (const policy of policies) {
    for (const gateway of policy.gateways) {
      const current = byGateway.get(gateway.gatewayId) ?? {
        gatewayId: gateway.gatewayId,
        label: gatewayLabel(gateway),
        wouldReject: 0,
      };
      const note =
        gateway.condition === "unsupported"
          ? "unsupported"
          : gateway.condition === "pending" && current.note !== "unsupported"
            ? "pending"
            : current.note;
      byGateway.set(gateway.gatewayId, {
        ...current,
        ...(note ? { note } : {}),
        ...(gateway.condition === "unsupported" && gateway.message
          ? { message: gateway.message }
          : {}),
        wouldReject: current.wouldReject + gateway.wouldReject24h,
      });
    }
  }
  const audited = policies.filter((policy) => policy.effectiveMode === "audit");
  return {
    gatewayCount: byGateway.size,
    rows: [...byGateway.values()]
      .filter((row) => row.wouldReject > 0 || row.note !== undefined)
      .sort(
        (left, right) =>
          right.wouldReject - left.wouldReject ||
          left.label.localeCompare(right.label),
      ),
    wouldAddDefaults: audited.reduce(
      (total, policy) => total + policy.findingsLast24h.wouldAddDefaults,
      0,
    ),
    wouldReject: audited.reduce(
      (total, policy) => total + policy.findingsLast24h.wouldReject,
      0,
    ),
  };
}

/** Distinct targeted gateways, and those with would-reject findings. */
export function affectedGateways(policies: readonly PlatformPolicy[]): {
  affected: number;
  total: number;
} {
  const all = new Set<string>();
  const affected = new Set<string>();
  for (const policy of policies) {
    for (const gateway of policy.gateways) {
      all.add(gateway.gatewayId);
      if (policy.effectiveMode === "audit" && gateway.wouldReject24h > 0) {
        affected.add(gateway.gatewayId);
      }
    }
  }
  return { affected: affected.size, total: all.size };
}

/** A dialog the admin opened from the list or detail view. */
export type PlatformPolicyDialog =
  | { kind: "delete"; policyId: string }
  | { kind: "global-mode"; to: PolicyMode }
  | { kind: "policy-mode"; policyId: string; to: PolicyMode };

/** A write the API refused or that failed; never retried automatically. */
export type PlatformPolicyWriteFailure = "conflicted" | "denied" | "failed";

export function writeFailureFor(
  kind: PlatformPolicyFailureKind,
): PlatformPolicyWriteFailure {
  return kind === "denied" || kind === "conflicted" ? kind : "failed";
}

export type PolicySortDirection = "asc" | "desc";

export function filterAndSortPolicies(
  policies: readonly PlatformPolicy[],
  filterText: string,
  direction: PolicySortDirection,
): readonly PlatformPolicy[] {
  const needle = filterText.trim().toLocaleLowerCase();
  return policies
    .filter((policy) => policy.name.toLocaleLowerCase().includes(needle))
    .sort((left, right) =>
      direction === "asc"
        ? left.name.localeCompare(right.name)
        : right.name.localeCompare(left.name),
    );
}
