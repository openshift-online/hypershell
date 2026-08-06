import type {
  GatewayOperations,
  GatewayRecord,
} from "@openshift-online/hypershell-gateway-ui";
import type { Gateway, SDKClient } from "@openshift-online/hypershell-sdk";

import { apiClient } from "./api.client";

const gatewayPageSize = 100;
type GatewayApi = Pick<
  SDKClient["gateways"],
  "create" | "delete" | "get" | "list" | "update"
>;

function toGatewayRecord(gateway: Gateway): GatewayRecord {
  return {
    clusterId: gateway.cluster_id,
    databaseId: gateway.database_id,
    externalDns: gateway.external_dns,
    id: gateway.id,
    name: gateway.name,
    namespace: gateway.namespace,
    phase: gateway.phase,
    releaseId: gateway.release_id,
    status: gateway.status,
  };
}

export function createGatewayOperations(client: {
  gateways: GatewayApi;
}): GatewayOperations {
  return {
    async getGateway(gatewayId, signal) {
      return toGatewayRecord(await client.gateways.get(gatewayId, { signal }));
    },
    async listGateways(signal) {
      const gateways: GatewayRecord[] = [];
      let page = 1;
      let total = 0;

      do {
        const result = await client.gateways.list(
          { page, size: gatewayPageSize },
          { signal },
        );
        gateways.push(...result.items.map(toGatewayRecord));
        total = result.total;
        if (result.items.length === 0) {
          break;
        }
        page += 1;
      } while (gateways.length < total);

      return gateways;
    },
    async provisionGateway(input) {
      return toGatewayRecord(
        await client.gateways.create({
          ...input,
          cluster_id: "",
          database_id: "",
          fleet_id: "",
          release_id: "",
        }),
      );
    },
    async removeGateway(gatewayId) {
      await client.gateways.delete(gatewayId);
    },
    async renameGateway(gatewayId, name) {
      return toGatewayRecord(await client.gateways.update(gatewayId, { name }));
    },
  };
}

export const gatewayOperations = createGatewayOperations(apiClient);
