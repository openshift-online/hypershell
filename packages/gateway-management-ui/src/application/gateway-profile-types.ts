import type { GatewayWorkflowRuntime } from "./gateway-types";

export type { GatewayWorkflowRuntime };

export interface GatewayProfileRecord {
  containerCpuLimitMax?: string;
  containerCpuRequestDefault?: string;
  containerMemoryLimitMax?: string;
  containerMemoryRequestDefault?: string;
  cpuLimitTotal?: string;
  cpuRequestTotal?: string;
  createdAt?: string;
  description?: string;
  ephemeralStorageTotal?: string;
  id: string;
  memoryLimitTotal?: string;
  memoryRequestTotal?: string;
  name: string;
  podCount?: number;
  pvcCount?: number;
}

export type GatewayProfileSortDirection = "asc" | "desc";
export type GatewayProfileSortField = "created" | "name";

export interface GatewayProfileListRequest {
  page: number;
  search: string;
  size: number;
  sortDirection: GatewayProfileSortDirection;
  sortField: GatewayProfileSortField;
}

export const gatewayProfileListPageSizes = [10, 20, 50, 100] as const;

export const defaultGatewayProfileListRequest: Readonly<GatewayProfileListRequest> =
  Object.freeze({
    page: 1,
    search: "",
    size: gatewayProfileListPageSizes[1],
    sortDirection: "asc",
    sortField: "name",
  });

export interface GatewayProfilePage<T> {
  items: readonly T[];
  page: number;
  size: number;
  total: number;
}

export interface GatewayProfileInvocationContext {
  correlationId: string;
  signal?: AbortSignal;
}

export interface GatewayProfileCreateInput {
  containerCpuLimitMax?: string;
  containerCpuRequestDefault?: string;
  containerMemoryLimitMax?: string;
  containerMemoryRequestDefault?: string;
  cpuLimitTotal?: string;
  cpuRequestTotal?: string;
  description?: string;
  ephemeralStorageTotal?: string;
  memoryLimitTotal?: string;
  memoryRequestTotal?: string;
  name: string;
  podCount?: number;
  pvcCount?: number;
}

export type GatewayProfileFailureKind =
  "cancelled" | "conflict" | "denied" | "not-found" | "unavailable" | "unknown";

export class GatewayProfileOperationError extends Error {
  readonly kind: GatewayProfileFailureKind;
  readonly operationId?: string;

  constructor(
    kind: GatewayProfileFailureKind,
    options: ErrorOptions & {
      operationId?: string;
    } = {},
  ) {
    super(`Gateway profile operation failed: ${kind}`, options);
    this.name = "GatewayProfileOperationError";
    this.kind = kind;
    this.operationId = options.operationId;
  }
}

/** Application-owned driven port for HyperShell gateway profiles. */
export interface GatewayProfileControlPlane {
  createGatewayProfile(
    input: GatewayProfileCreateInput,
    context: GatewayProfileInvocationContext,
  ): Promise<GatewayProfileRecord>;
  getGatewayProfile(
    gatewayProfileId: string,
    context: GatewayProfileInvocationContext,
  ): Promise<GatewayProfileRecord>;
  listGatewayProfiles(
    request: GatewayProfileListRequest,
    context: GatewayProfileInvocationContext,
  ): Promise<GatewayProfilePage<GatewayProfileRecord>>;
  removeGatewayProfile(
    gatewayProfileId: string,
    context: GatewayProfileInvocationContext,
  ): Promise<void>;
}

/** Driving entry port used by the GatewayProfile UI presentation adapters. */
export interface GatewayProfileOperations {
  createGatewayProfile(
    input: GatewayProfileCreateInput,
    signal?: AbortSignal,
  ): Promise<GatewayProfileRecord>;
  getGatewayProfile(
    gatewayProfileId: string,
    signal?: AbortSignal,
  ): Promise<GatewayProfileRecord>;
  listGatewayProfiles(
    request: GatewayProfileListRequest,
    signal?: AbortSignal,
  ): Promise<GatewayProfilePage<GatewayProfileRecord>>;
  removeGatewayProfile(
    gatewayProfileId: string,
    signal?: AbortSignal,
  ): Promise<void>;
}
