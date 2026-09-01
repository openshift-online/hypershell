export interface GatewayRecord {
  activeSandboxCount?: number;
  clusterId: string;
  consoleUrl?: string;
  createdAt?: string;
  createdBy?: string;
  databaseId: string;
  externalDns?: string;
  id: string;
  name: string;
  namespace: string;
  oidcAudience?: string;
  oidcClientId?: string;
  oidcIssuer?: string;
  phase?: string;
  profileId?: string;
  releaseId: string;
  status?: string;
}

export interface GatewayProfileSummary {
  description?: string;
  id: string;
  name: string;
}

export interface GatewayProfileSummaryOptions {
  hasMore: boolean;
  items: readonly GatewayProfileSummary[];
}

export interface GatewayPlacement {
  id: string;
  name: string;
  profileId?: string;
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
  "cluster" | "created" | "endpoint" | "name" | "owner" | "status";

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
  clusterId: string;
  name: string;
  profileId?: string;
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
  createOpenShellGatewayServiceAccount(
    gatewayId: string,
    input: OpenShellGatewayServiceAccountCreateInput,
    context: GatewayInvocationContext,
  ): Promise<OpenShellGatewayServiceAccountCreateResult>;
  deleteOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    context: GatewayInvocationContext,
  ): Promise<void>;
  findGatewayPlacements(
    search: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayPlacementOptions>;
  findGatewayProfiles(
    search: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayProfileSummaryOptions>;
  getGatewayPlacement(
    clusterId: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayPlacement>;
  getGatewayPlacements(
    clusterIds: readonly string[],
    context: GatewayInvocationContext,
  ): Promise<readonly GatewayPlacement[]>;
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
  createOpenShellGatewayServiceAccount(
    gatewayId: string,
    input: OpenShellGatewayServiceAccountCreateInput,
    signal?: AbortSignal,
  ): Promise<OpenShellGatewayServiceAccountCreateResult>;
  deleteOpenShellGatewayServiceAccount(
    gatewayId: string,
    serviceAccountId: string,
    signal?: AbortSignal,
  ): Promise<void>;
  findGatewayPlacements(
    search: string,
    signal?: AbortSignal,
  ): Promise<GatewayPlacementOptions>;
  findGatewayProfiles(
    search: string,
    signal?: AbortSignal,
  ): Promise<GatewayProfileSummaryOptions>;
  getGatewayPlacement(
    clusterId: string,
    signal?: AbortSignal,
  ): Promise<GatewayPlacement>;
  getGatewayPlacements(
    clusterIds: readonly string[],
    signal?: AbortSignal,
  ): Promise<readonly GatewayPlacement[]>;
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
