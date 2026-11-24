import type {
  PolicyFinding,
  PolicyFindingCounts,
  PolicyFindingOutcome,
  PolicyFleetDefaults,
  PolicyMode,
  PolicyRenderedRule,
  PolicyRolloutCondition,
} from "@openshift-online/hypershell-gateway-management-ui";

/*
 * Application-owned types and ports for the platform policy admin panel
 * (WEB-PADM-01..12). React, TanStack Query, the BFF, and the generated SDK stay
 * outside this boundary (UIS-HEX); the API adapter maps transport records and
 * errors into these types.
 */

/** The gateways a policy applies to (PLAT-PGP-01). */
export interface PlatformPolicyTarget {
  all: boolean;
  exclude: readonly string[];
  gatewayIds: readonly string[];
}

/** Rollout of one policy on one targeted gateway (PLAT-PGP-13). */
export interface PlatformPolicyGatewayStatus {
  appliedRevision?: string;
  condition: PolicyRolloutCondition;
  effectiveMode: PolicyMode;
  gatewayId: string;
  /** Display name when the caller may read the gateway; otherwise absent. */
  gatewayName?: string;
  message?: string;
  revision?: string;
  wouldReject24h: number;
}

export interface PlatformPolicyRolloutCounts {
  applied: number;
  pending: number;
  unsupported: number;
}

export interface PlatformPolicyDiagnostic {
  /** 1-based line of the offending statement; 0 when no line applies. */
  line: number;
  message: string;
  statement: number;
  suggestion?: string;
}

export interface PlatformPolicy {
  builtIn: boolean;
  cedar: string;
  createdAt?: string;
  description?: string;
  documentationUrl?: string;
  effectiveMode: PolicyMode;
  enforcementMode: PolicyMode;
  findingsLast24h: PolicyFindingCounts;
  gateways: readonly PlatformPolicyGatewayStatus[];
  id: string;
  name: string;
  rendered: readonly PolicyRenderedRule[];
  rollout: PlatformPolicyRolloutCounts;
  target: PlatformPolicyTarget;
  updatedAt?: string;
  userMessage: string;
}

/** The global enforcement toggle and fleet defaults (PLAT-DATA-29). */
export interface PlatformPolicySettings {
  defaults: PolicyFleetDefaults;
  mode: PolicyMode;
  schemaVersion: string;
  updatedAt?: string;
  updatedBy?: string;
}

/** Fields an author edits; a new policy is always stored in audit mode. */
export interface PlatformPolicyDraft {
  cedar: string;
  description: string;
  documentationUrl: string;
  name: string;
  target: PlatformPolicyTarget;
  userMessage: string;
}

export interface PlatformPolicyUpdate extends Partial<PlatformPolicyDraft> {
  enforcementMode?: PolicyMode;
}

/** A test request; missing values take the fleet defaults (WEB-PADM-08). */
export interface PlatformPolicyTestRequest {
  cpuLimit?: string;
  cpuRequest?: string;
  gpuCount?: number;
  image?: string;
  memoryLimit?: string;
  memoryRequest?: string;
  roles?: readonly string[];
  sandboxName?: string;
}

export interface PlatformPolicyEvaluationInput {
  cedar: string;
  documentationUrl?: string;
  name?: string;
  request?: PlatformPolicyTestRequest;
  userMessage?: string;
}

/** Validation, rendering, and test-request result of the editor dry run. */
export interface PlatformPolicyEvaluation {
  defaults: PolicyFleetDefaults;
  denialText: string;
  diagnostics: readonly PlatformPolicyDiagnostic[];
  forbidsDefaults: boolean;
  rejectingStatements: readonly number[];
  rendered: readonly PolicyRenderedRule[];
  requestAllowed: boolean;
  valid: boolean;
}

export interface PlatformPolicyFindingsRequest {
  gatewayId?: string;
  outcome?: PolicyFindingOutcome;
  page: number;
  size: number;
}

export interface PlatformPolicyFindingsPage {
  items: readonly PolicyFinding[];
  page: number;
  size: number;
  total: number;
}

export type PlatformPolicyFailureKind =
  | "cancelled"
  | "conflicted"
  | "denied"
  | "invalid"
  | "not-found"
  | "unavailable"
  | "unknown";

/**
 * Typed operation error. `403` maps to `denied` (never retried, WEB-PADM-01)
 * and `409` to `conflicted` (WEB-PADM-10). `reason` carries the server's
 * validation message for `invalid` only; it never holds credentials.
 */
export class PlatformPolicyOperationError extends Error {
  readonly kind: PlatformPolicyFailureKind;
  readonly operationId?: string;
  readonly reason?: string;

  constructor(
    kind: PlatformPolicyFailureKind,
    options: ErrorOptions & { operationId?: string; reason?: string } = {},
  ) {
    super(`Platform policy operation failed: ${kind}`, options);
    this.name = "PlatformPolicyOperationError";
    this.kind = kind;
    this.operationId = options.operationId;
    this.reason = options.reason;
  }
}

export interface PlatformPolicyInvocationContext {
  correlationId: string;
  signal?: AbortSignal;
}

/** Driven port for the HyperShell platform policy API. */
export interface PlatformPolicyControlPlane {
  createPolicy(
    draft: PlatformPolicyDraft,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicy>;
  deletePolicy(
    policyId: string,
    context: PlatformPolicyInvocationContext,
  ): Promise<void>;
  evaluatePolicy(
    input: PlatformPolicyEvaluationInput,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicyEvaluation>;
  getPolicy(
    policyId: string,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicy>;
  getSettings(
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicySettings>;
  listFindings(
    policyId: string,
    request: PlatformPolicyFindingsRequest,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicyFindingsPage>;
  listPolicies(
    context: PlatformPolicyInvocationContext,
  ): Promise<readonly PlatformPolicy[]>;
  updatePolicy(
    policyId: string,
    update: PlatformPolicyUpdate,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicy>;
  updateSettings(
    mode: PolicyMode,
    context: PlatformPolicyInvocationContext,
  ): Promise<PlatformPolicySettings>;
}

/** Whether the caller may administer platform policies (WEB-PADM-01). */
export type PlatformPolicyAccess = "authorized" | "denied";

/** Driving port used by the admin panel presentation adapters. */
export interface PlatformPolicyOperations {
  /**
   * Resolves the caller's policy-admin authority. `platform-policy:admin` is a
   * database RoleBinding, not a JWT realm role, so the console asks the API
   * (the enforcement point, SEC-RBAC-30) instead of reading session roles.
   */
  checkAccess(signal?: AbortSignal): Promise<PlatformPolicyAccess>;
  /** Publishes the confirmed global mode change before it is written. */
  confirmModeChange(from: PolicyMode, to: PolicyMode): void;
  createPolicy(
    draft: PlatformPolicyDraft,
    signal?: AbortSignal,
  ): Promise<PlatformPolicy>;
  deletePolicy(policyId: string, signal?: AbortSignal): Promise<void>;
  evaluatePolicy(
    input: PlatformPolicyEvaluationInput,
    signal?: AbortSignal,
  ): Promise<PlatformPolicyEvaluation>;
  getPolicy(policyId: string, signal?: AbortSignal): Promise<PlatformPolicy>;
  getSettings(signal?: AbortSignal): Promise<PlatformPolicySettings>;
  listFindings(
    policyId: string,
    request: PlatformPolicyFindingsRequest,
    signal?: AbortSignal,
  ): Promise<PlatformPolicyFindingsPage>;
  listPolicies(signal?: AbortSignal): Promise<readonly PlatformPolicy[]>;
  setGlobalMode(
    mode: PolicyMode,
    signal?: AbortSignal,
  ): Promise<PlatformPolicySettings>;
  updatePolicy(
    policyId: string,
    update: PlatformPolicyUpdate,
    signal?: AbortSignal,
  ): Promise<PlatformPolicy>;
}

/** Application-owned port for nondeterministic workflow context. */
export interface PlatformPolicyRuntime {
  createCorrelationId(): string;
  createTraceId(): string;
  now(): string;
}
