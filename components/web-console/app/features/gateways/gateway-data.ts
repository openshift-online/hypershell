import type { Gateway } from "@openshift-online/hypershell-sdk";

import { apiClient } from "../../lib/api.client";
import { previewGateway, type GatewayConnection } from "./gateway-connections";

const gatewayPageSize = 100;

type GatewayApiPayload = Omit<
  Gateway,
  "external_dns" | "phase" | "service_type" | "status" | "tls_mode"
> &
  Partial<
    Pick<
      Gateway,
      "external_dns" | "phase" | "service_type" | "status" | "tls_mode"
    >
  >;

function gatewayEndpoint(gateway: GatewayApiPayload): string {
  const externalDns = gateway.external_dns?.trim() ?? "";
  if (!externalDns) {
    return previewGateway.endpoint;
  }
  if (/^https?:\/\//iu.test(externalDns)) {
    return externalDns;
  }

  return `https://${externalDns}${externalDns.includes(":") ? "" : ":443"}`;
}

export function toGatewayConnection(
  gateway: GatewayApiPayload,
): GatewayConnection {
  const status = [gateway.status, gateway.phase].find(
    (value) => value !== undefined && value.trim().length > 0,
  );

  return {
    clusterName: gateway.cluster_id.trim() || "Local cluster",
    consoleUrl: previewGateway.consoleUrl,
    endpoint: gatewayEndpoint(gateway),
    id: gateway.id,
    name: gateway.name,
    oidcAudience: previewGateway.oidcAudience,
    oidcClientId: previewGateway.oidcClientId,
    oidcIssuer: previewGateway.oidcIssuer,
    status: status ?? "Unknown",
  };
}

export async function listGateways(signal?: AbortSignal): Promise<Gateway[]> {
  const gateways: Gateway[] = [];
  let page = 1;
  let total = 0;

  do {
    const result = await apiClient.gateways.list(
      { page, size: gatewayPageSize },
      { signal },
    );
    gateways.push(...result.items);
    total = result.total;
    if (result.items.length === 0) {
      break;
    }
    page += 1;
  } while (gateways.length < total);

  return gateways;
}

export async function listGatewayConnections(
  signal?: AbortSignal,
): Promise<GatewayConnection[]> {
  return (await listGateways(signal)).map(toGatewayConnection);
}

export async function getGatewayConnection(
  gatewayId: string,
  signal?: AbortSignal,
): Promise<GatewayConnection> {
  return toGatewayConnection(
    await apiClient.gateways.get(gatewayId, { signal }),
  );
}

export async function getGateway(
  gatewayId: string,
  signal?: AbortSignal,
): Promise<Gateway> {
  return apiClient.gateways.get(gatewayId, { signal });
}
