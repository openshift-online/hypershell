/**
 * Application-owned domain types for platform gateway policies
 * (PLAT-PGP-01..20). The API adapter maps transport records into these types so
 * presentation code never depends on the generated SDK or snake_case fields.
 */

/** Effective or configured enforcement mode (PLAT-PGP-06). */
export type PolicyMode = "audit" | "enforce";

/** Why a policy is enforced or audit-only on one gateway. */
export type PolicyModeReason =
  "audit" | "platform-enforced" | "policy-enforced";

/** The rollout condition of a policy on one gateway (PLAT-PGP-13). */
export type PolicyRolloutCondition = "applied" | "pending" | "unsupported";

export type PolicyFindingOutcome = "would-add-defaults" | "would-reject";

/** Context attributes the plain-words renderer can attribute to a resource. */
export type PolicyResourceKey =
  "cpuLimit" | "cpuRequest" | "gpuCount" | "memoryLimit" | "memoryRequest";

/** One recognized comparison rendered as the allowed bound (PLAT-PGP-17). */
export interface PolicyBound {
  /** Human display of the bound as a quantity, for example `4` or `16Gi`. */
  display: string;
  /** The Cedar context attribute, for example `cpuRequestMillis`. */
  field: string;
  label: string;
  /** The forbid comparison operator over `field`. */
  op: string;
  /** Plain-words bound, for example `CPU request: at most 4`. */
  text: string;
  /** Bound in schema units (millicores, bytes, count). */
  value: number;
}

/** The best-effort display of one forbid statement (PLAT-PGP-17). */
export interface PolicyRenderedRule {
  bounds: readonly PolicyBound[];
  cedar: string;
  recognized: boolean;
  summary: readonly string[];
}

/** Fleet-wide sandbox resource defaults (PLAT-PGP-08). */
export interface PolicyFleetDefaults {
  cpuLimit?: string;
  cpuRequest?: string;
  memoryLimit?: string;
  memoryRequest?: string;
}

export interface PolicyFindingCounts {
  wouldAddDefaults: number;
  wouldReject: number;
}

/** One audit finding recorded by a gateway interceptor (PLAT-PGP-15). */
export interface PolicyFinding {
  bundleRevision?: string;
  gatewayId: string;
  id: string;
  occurredAt: string;
  operation: string;
  outcome: PolicyFindingOutcome;
  policyNames: readonly string[];
  principalSubject: string;
  /** Request values in schema units keyed by context attribute name. */
  requestSummary: Readonly<Record<string, number>>;
}

/** One platform policy as it applies to a single gateway (PLAT-PGP-19). */
export interface EffectivePolicy {
  builtIn: boolean;
  cedar: string;
  condition: PolicyRolloutCondition;
  documentationUrl?: string;
  effectiveMode: PolicyMode;
  effectiveModeReason: PolicyModeReason;
  enforcementMode: PolicyMode;
  id: string;
  name: string;
  rendered: readonly PolicyRenderedRule[];
  revision?: string;
  updatedAt?: string;
  userMessage: string;
}

/**
 * The read model behind the read-only gateway Policies tab (WEB-GLIS-26). The
 * API masks policy targets from gateway users (SEC-RBAC-31), so the domain type
 * carries no target or exclusion data at all.
 */
export interface GatewayEffectivePolicies {
  defaults: PolicyFleetDefaults;
  findings: readonly PolicyFinding[];
  findingsLast24h: PolicyFindingCounts;
  gatewayId: string;
  globalMode: PolicyMode;
  policies: readonly EffectivePolicy[];
  schemaVersion: string;
}
