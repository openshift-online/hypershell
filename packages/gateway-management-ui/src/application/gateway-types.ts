export type ProvisioningConditionStatus =
  "Complete" | "Failed" | "InProgress" | "Pending";

export interface ProvisioningCondition {
  conditionStatus: ProvisioningConditionStatus;
  message: string;
  type: string;
}

export interface GatewayRecord {
  activeSandboxCount?: number;
  canDelete?: boolean;
  canEdit?: boolean;
  clusterId: string;
  consoleUrl?: string;
  createdAt?: string;
  createdBy?: string;
  externalDns?: string;
  gatewayVersion?: string;
  id: string;
  /**
   * True when the gateway pins an explicit container image and therefore need
   * not track its cluster's advertised provisioning version. A pinned gateway
   * never shows the newer-version-available indicator (see WEB-VER-03).
   */
  imagePinned?: boolean;
  name: string;
  namespace: string;
  oidcAudience?: string;
  oidcClientId?: string;
  oidcIssuer?: string;
  phase?: string;
  provisioningConditions?: readonly ProvisioningCondition[];
  /**
   * The gateway version the gateway's cluster advertises for provisioning onto
   * new gateways (its single-valued `gateway-version/<tag>` capability). Absent
   * when the cluster advertises none.
   */
  provisioningVersion?: string;
  status?: string;
}

/**
 * Derived verdict for whether a gateway's running version is behind the
 * provisioning version its cluster advertises (WEB-VER-03/WEB-VER-04). `"behind"` fires
 * the neutral newer-version-available indicator; `"none"` suppresses it. The
 * verdict is computed once in the application layer and consumed, not
 * recomputed, by React components.
 */
export type GatewayVersionDrift = "behind" | "none";

export interface GatewayPlacement {
  id: string;
  name: string;
  provider: string;
  region?: string;
  status?: string;
}

export interface GatewayPlacementOptions {
  hasMore: boolean;
  items: readonly GatewayPlacement[];
}

export type GatewaySortDirection = "asc" | "desc";
export type GatewaySortField =
  | "activeSandboxes"
  | "cluster"
  | "created"
  | "endpoint"
  | "name"
  | "owner"
  | "status"
  | "version";

export interface GatewayListRequest {
  page: number;
  search: string;
  size: number;
  sortDirection: GatewaySortDirection;
  sortField: GatewaySortField;
}

export const gatewayListPageSizes = [10, 20, 50, 100] as const;

export const defaultGatewayListRequest: Readonly<GatewayListRequest> =
  Object.freeze({
    page: 1,
    search: "",
    size: gatewayListPageSizes[1],
    sortDirection: "asc",
    sortField: "name",
  });

export interface GatewayPage<T> {
  items: readonly T[];
  page: number;
  size: number;
  total: number;
}

export interface GatewayInvocationContext {
  correlationId: string;
  signal?: AbortSignal;
}

export interface GatewayProvisionInput {
  name: string;
  placement?: GatewayPlacementIntent;
  /** @deprecated Kept only for source compatibility with older adapters. */
  clusterId?: string;
}

export type GatewayProvider = "aws" | "ibm";

export type GatewayPlacementIntent =
  { mode: "local-kind" } | { selector: string[] };

export type GatewayPlacementUnavailableReason = "no-eligible-cluster";

export interface GatewayPlacementAvailabilityItem {
  available: boolean;
  reason?: GatewayPlacementUnavailableReason;
  selector: readonly string[];
}

export interface GatewayPlacementAvailability {
  localKind: boolean;
  placements: readonly GatewayPlacementAvailabilityItem[];
}

export type OpenShellGatewayServiceAccountRole =
  "openshell-admin" | "openshell-user";

export type OpenShellGatewayServiceAccountStatus =
  | "degraded"
  | "deleting"
  | "error"
  | "expired"
  | "provisioning"
  | "ready"
  | "revoked"
  | "revoking";

export interface OpenShellGatewayServiceAccountRecord {
  clientId: string;
  createdAt: string;
  createdByUserId: string;
  description?: string;
  expiresAt: string;
  gatewayId: string;
  id: string;
  lastError?: string;
  name: string;
  revokedAt?: string;
  role: OpenShellGatewayServiceAccountRole;
  status: OpenShellGatewayServiceAccountStatus;
  subject: string;
  updatedAt: string;
}

export interface OpenShellGatewayServiceAccountConnection {
  accessTokenLifetimeSeconds: number;
  audience: string;
  clientId: string;
  gatewayEndpoint?: string;
  gatewayName: string;
  issuer: string;
  tokenEndpoint: string;
}

export interface OpenShellGatewayServiceAccountCredential extends OpenShellGatewayServiceAccountConnection {
  clientSecret: string;
}

export interface OpenShellGatewayServiceAccountExpirationPolicy {
  defaultSeconds: number;
  maximumSeconds: number;
  minimumSeconds: number;
}

export interface OpenShellGatewayServiceAccountCapabilities {
  allowedRoles: readonly OpenShellGatewayServiceAccountRole[];
  canCreate: boolean;
  canManageAll: boolean;
  expirationPolicy: OpenShellGatewayServiceAccountExpirationPolicy;
}

export interface OpenShellGatewayServiceAccountPage {
  capabilities: OpenShellGatewayServiceAccountCapabilities;
  items: readonly OpenShellGatewayServiceAccountRecord[];
  page: number;
  size: number;
  total: number;
}

export interface OpenShellGatewayServiceAccountDetail {
  connection: OpenShellGatewayServiceAccountConnection;
  serviceAccount: OpenShellGatewayServiceAccountRecord;
}

export interface OpenShellGatewayServiceAccountCreateResult {
  credential: OpenShellGatewayServiceAccountCredential;
  serviceAccount: OpenShellGatewayServiceAccountRecord;
}

export interface OpenShellGatewayServiceAccountCreateInput {
  description?: string;
  expiresAt: string;
  name: string;
  role: OpenShellGatewayServiceAccountRole;
}

export type OpenShellGatewayServiceAccountSortField =
  "created_at" | "expires_at" | "name" | "role" | "status";

export interface OpenShellGatewayServiceAccountListRequest {
  order: GatewaySortDirection;
  page: number;
  search: string;
  size: number;
  sort: OpenShellGatewayServiceAccountSortField;
  status?: OpenShellGatewayServiceAccountStatus;
}

export const openShellGatewayServiceAccountPageSizes = [
  10, 20, 50, 100,
] as const;

export const defaultOpenShellGatewayServiceAccountListRequest: Readonly<OpenShellGatewayServiceAccountListRequest> =
  Object.freeze({
    order: "desc",
    page: 1,
    search: "",
    size: openShellGatewayServiceAccountPageSizes[1],
    sort: "created_at",
  });

export type GatewayAccessRole = "admin" | "owner" | "user";

export interface GatewayAccessGrantRecord {
  email?: string;
  grantedAt: string;
  name?: string;
  role: GatewayAccessRole;
  roleBindingId: string;
  userId: string;
  username: string;
}

export interface GatewayAccessCapabilities {
  callerRole?: GatewayAccessRole;
  canManageAccess: boolean;
  canManageOwners: boolean;
}

export interface GatewayAccessPage {
  capabilities: GatewayAccessCapabilities;
  items: readonly GatewayAccessGrantRecord[];
  page: number;
  size: number;
  total: number;
}

export interface GatewayDirectoryUser {
  email?: string;
  name?: string;
  subject?: string;
  username: string;
}

export interface GatewayAccessGrantInput {
  role: GatewayAccessRole;
  subject?: string;
  username: string;
}

export type GatewayAccessSortField =
  "granted_at" | "name" | "role" | "username";

export interface GatewayAccessListRequest {
  order: GatewaySortDirection;
  page: number;
  role?: GatewayAccessRole;
  search: string;
  size: number;
  sort: GatewayAccessSortField;
}

export const gatewayAccessPageSizes = [10, 20, 50, 100] as const;

export const defaultGatewayAccessListRequest: Readonly<GatewayAccessListRequest> =
  Object.freeze({
    order: "desc",
    page: 1,
    search: "",
    size: gatewayAccessPageSizes[1],
    sort: "granted_at",
  });

export type GatewayFailureKind =
  "cancelled" | "conflict" | "denied" | "not-found" | "unavailable" | "unknown";

export type GatewayFailureCode = "service-account-name-exists";

export class GatewayOperationError extends Error {
  readonly code?: GatewayFailureCode;
  readonly kind: GatewayFailureKind;
  readonly operationId?: string;

  constructor(
    kind: GatewayFailureKind,
    options: ErrorOptions & {
      code?: GatewayFailureCode;
      operationId?: string;
    } = {},
  ) {
    super(`Gateway operation failed: ${kind}`, options);
    this.name = "GatewayOperationError";
    this.code = options.code;
    this.kind = kind;
    this.operationId = options.operationId;
  }
}

/** Application-owned driven port for the HyperShell gateway control plane. */
export interface GatewayControlPlane {
  changeGatewayAccessRole(
    gatewayId: string,
    userId: string,
    role: GatewayAccessRole,
    context: GatewayInvocationContext,
  ): Promise<GatewayAccessGrantRecord>;
  createOpenShellGatewayServiceAccount(
    gatewayId: string,
    input: OpenShellGatewayServiceAccountCreateInput,
    context: GatewayInvocationContext,
  ): Promise<OpenShellGatewayServiceAccountCreateResult>;
  grantGatewayAccess(
    gatewayId: string,
    input: GatewayAccessGrantInput,
    context: GatewayInvocationContext,
  ): Promise<GatewayAccessGrantRecord>;
  listGatewayAccess(
    gatewayId: string,
    request: GatewayAccessListRequest,
    context: GatewayInvocationContext,
  ): Promise<GatewayAccessPage>;
  revokeGatewayAccess(
    gatewayId: string,
    userId: string,
    context: GatewayInvocationContext,
  ): Promise<void>;
  searchGatewayDirectory(
    gatewayId: string,
    search: string,
    context: GatewayInvocationContext,
  ): Promise<readonly GatewayDirectoryUser[]>;
  deleteOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    context: GatewayInvocationContext,
  ): Promise<void>;
  findGatewayPlacements(
    search: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayPlacementOptions>;
  getGatewayPlacementAvailability?(
    selectors: readonly (readonly string[])[],
    context: GatewayInvocationContext,
  ): Promise<GatewayPlacementAvailability>;
  getGatewayPlacement(
    clusterId: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayPlacement>;
  getGatewayPlacements(
    clusterIds: readonly string[],
    context: GatewayInvocationContext,
  ): Promise<readonly GatewayPlacement[]>;
  /**
   * Distinct `gateway-version/<tag>` tag values advertised by registered
   * clusters, used to seed the provisioning-version choice (WEB-PLC-11). Availability
   * for a specific network/provider/version is confirmed separately through
   * getGatewayPlacementAvailability.
   */
  getGatewayProvisioningVersions?(
    context: GatewayInvocationContext,
  ): Promise<readonly string[]>;
  getGateway(
    gatewayId: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayRecord>;
  getOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    context: GatewayInvocationContext,
  ): Promise<OpenShellGatewayServiceAccountDetail>;
  listGateways(
    request: GatewayListRequest,
    context: GatewayInvocationContext,
  ): Promise<GatewayPage<GatewayRecord>>;
  listOpenShellGatewayServiceAccounts(
    gatewayId: string,
    request: OpenShellGatewayServiceAccountListRequest,
    context: GatewayInvocationContext,
  ): Promise<OpenShellGatewayServiceAccountPage>;
  provisionGateway(
    input: GatewayProvisionInput,
    context: GatewayInvocationContext,
  ): Promise<GatewayRecord>;
  removeGateway(
    gatewayId: string,
    context: GatewayInvocationContext,
  ): Promise<void>;
  renameGateway(
    gatewayId: string,
    name: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayRecord>;
  revokeOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    context: GatewayInvocationContext,
  ): Promise<OpenShellGatewayServiceAccountRecord>;
}

/** Driving entry port used by the Gateway UI presentation adapters. */
export interface GatewayOperations {
  changeGatewayAccessRole(
    gatewayId: string,
    userId: string,
    role: GatewayAccessRole,
    signal?: AbortSignal,
  ): Promise<GatewayAccessGrantRecord>;
  createOpenShellGatewayServiceAccount(
    gatewayId: string,
    input: OpenShellGatewayServiceAccountCreateInput,
    signal?: AbortSignal,
  ): Promise<OpenShellGatewayServiceAccountCreateResult>;
  grantGatewayAccess(
    gatewayId: string,
    input: GatewayAccessGrantInput,
    signal?: AbortSignal,
  ): Promise<GatewayAccessGrantRecord>;
  listGatewayAccess(
    gatewayId: string,
    request: GatewayAccessListRequest,
    signal?: AbortSignal,
  ): Promise<GatewayAccessPage>;
  revokeGatewayAccess(
    gatewayId: string,
    userId: string,
    signal?: AbortSignal,
  ): Promise<void>;
  searchGatewayDirectory(
    gatewayId: string,
    search: string,
    signal?: AbortSignal,
  ): Promise<readonly GatewayDirectoryUser[]>;
  deleteOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    signal?: AbortSignal,
  ): Promise<void>;
  findGatewayPlacements(
    search: string,
    signal?: AbortSignal,
  ): Promise<GatewayPlacementOptions>;
  getGatewayPlacementAvailability?(
    selectors: readonly (readonly string[])[],
    signal?: AbortSignal,
  ): Promise<GatewayPlacementAvailability>;
  getGatewayPlacement(
    clusterId: string,
    signal?: AbortSignal,
  ): Promise<GatewayPlacement>;
  getGatewayPlacements(
    clusterIds: readonly string[],
    signal?: AbortSignal,
  ): Promise<readonly GatewayPlacement[]>;
  getGatewayProvisioningVersions?(
    signal?: AbortSignal,
  ): Promise<readonly string[]>;
  getGateway(gatewayId: string, signal?: AbortSignal): Promise<GatewayRecord>;
  getOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    signal?: AbortSignal,
  ): Promise<OpenShellGatewayServiceAccountDetail>;
  listGateways(
    request: GatewayListRequest,
    signal?: AbortSignal,
  ): Promise<GatewayPage<GatewayRecord>>;
  listOpenShellGatewayServiceAccounts(
    gatewayId: string,
    request: OpenShellGatewayServiceAccountListRequest,
    signal?: AbortSignal,
  ): Promise<OpenShellGatewayServiceAccountPage>;
  provisionGateway(
    input: GatewayProvisionInput,
    signal?: AbortSignal,
  ): Promise<GatewayRecord>;
  removeGateway(gatewayId: string, signal?: AbortSignal): Promise<void>;
  renameGateway(
    gatewayId: string,
    name: string,
    signal?: AbortSignal,
  ): Promise<GatewayRecord>;
  revokeOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    signal?: AbortSignal,
  ): Promise<OpenShellGatewayServiceAccountRecord>;
}

/** Application-owned port for nondeterministic workflow context. */
export interface GatewayWorkflowRuntime {
  createCorrelationId(): string;
  /**
   * Creates the W3C trace identifier (16-byte value as 32 lowercase hex
   * digits) that identifies one workflow invocation across the browser, the
   * BFF, and the API. A trace sink adopts this value as the span trace id, so
   * probe consumers can join a workflow to its trace.
   */
  createTraceId(): string;
  now(): string;
}
