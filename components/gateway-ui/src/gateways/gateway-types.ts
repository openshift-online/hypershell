export interface GatewayRecord {
  clusterId: string;
  databaseId: string;
  externalDns?: string;
  id: string;
  name: string;
  namespace: string;
  phase?: string;
  releaseId: string;
  status?: string;
}

export interface GatewayProvisionInput {
  name: string;
  namespace: string;
}

/**
 * Host entry port for the gateway tasks rendered by this package.
 * Implementations translate between these stable task values and transport DTOs.
 */
export interface GatewayOperations {
  getGateway(gatewayId: string, signal?: AbortSignal): Promise<GatewayRecord>;
  listGateways(signal?: AbortSignal): Promise<readonly GatewayRecord[]>;
  provisionGateway(input: GatewayProvisionInput): Promise<GatewayRecord>;
  removeGateway(gatewayId: string): Promise<void>;
  renameGateway(gatewayId: string, name: string): Promise<GatewayRecord>;
}
