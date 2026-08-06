import { apiClient } from "../../adapters/api/api.client";
import { previewGateway, type GatewayConnection } from "./gateway-connections";

const gatewayPageSize = 100;

export function gatewayQueryKey(gatewayId: string) {
  return ["gateways", "detail", gatewayId] as const;
}

export interface GatewayRecord {
  cluster_id: string;
  created_at: string | null;
  database_id: string;
  external_dns: string;
  fleet_id: string;
  href: string;
  id: string;
  kind: string;
  name: string;
  namespace: string;
  phase: string;
  release_id: string;
  service_type: string;
  status: string;
  tls_mode: string;
  updated_at: string | null;
}

type GatewayApiPayload = Omit<
  GatewayRecord,
  "external_dns" | "phase" | "service_type" | "status" | "tls_mode"
> &
  Partial<
    Pick<
      GatewayRecord,
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

export async function listGateways(
  signal?: AbortSignal,
): Promise<GatewayRecord[]> {
  const gateways: GatewayRecord[] = [];
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

export async function getGateway(
  gatewayId: string,
  signal?: AbortSignal,
): Promise<GatewayRecord> {
  return apiClient.gateways.get(gatewayId, { signal });
}

export async function deleteGateway(gatewayId: string): Promise<void> {
  await apiClient.gateways.delete(gatewayId);
}

export async function renameGateway(
  gatewayId: string,
  name: string,
): Promise<GatewayRecord> {
  return apiClient.gateways.update(gatewayId, { name });
}
