export interface GatewayRecord {
  clusterId: string;
  consoleUrl?: string;
  databaseId: string;
  externalDns?: string;
  id: string;
  name: string;
  namespace: string;
  oidcAudience?: string;
  oidcClientId?: string;
  oidcIssuer?: string;
  phase?: string;
  releaseId: string;
  status?: string;
}

export type GatewaySortDirection = "asc" | "desc";
export type GatewaySortField = "cluster" | "endpoint" | "name" | "status";

export interface GatewayListRequest {
  page: number;
  search: string;
  size: number;
  sortDirection: GatewaySortDirection;
  sortField: GatewaySortField;
}

export const defaultGatewayListRequest: Readonly<GatewayListRequest> =
  Object.freeze({
    page: 1,
    search: "",
    size: 20,
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
  namespace: string;
}

export type GatewayFailureKind =
  "cancelled" | "conflict" | "denied" | "not-found" | "unavailable" | "unknown";

export class GatewayOperationError extends Error {
  readonly kind: GatewayFailureKind;
  readonly operationId?: string;

  constructor(
    kind: GatewayFailureKind,
    options: ErrorOptions & { operationId?: string } = {},
  ) {
    super(`Gateway operation failed: ${kind}`, options);
    this.name = "GatewayOperationError";
    this.kind = kind;
    this.operationId = options.operationId;
  }
}

/** Application-owned driven port for the HyperShell gateway control plane. */
export interface GatewayControlPlane {
  getGateway(
    gatewayId: string,
    context: GatewayInvocationContext,
  ): Promise<GatewayRecord>;
  listGateways(
    request: GatewayListRequest,
    context: GatewayInvocationContext,
  ): Promise<GatewayPage<GatewayRecord>>;
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
}

/** Driving entry port used by the Gateway UI presentation adapters. */
export interface GatewayOperations {
  getGateway(gatewayId: string, signal?: AbortSignal): Promise<GatewayRecord>;
  listGateways(
    request: GatewayListRequest,
    signal?: AbortSignal,
  ): Promise<GatewayPage<GatewayRecord>>;
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
}

/** Application-owned port for nondeterministic workflow context. */
export interface GatewayWorkflowRuntime {
  createCorrelationId(): string;
  now(): string;
}
