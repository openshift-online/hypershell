import type {
  GatewayControlPlane,
  GatewayFailureKind,
  GatewayInvocationContext,
  GatewayListRequest,
  GatewayPlacement,
  GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { GatewayOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type Gateway,
  type ManagedCluster,
  type SDKClient,
} from "@openshift-online/hypershell-sdk";

type GatewayApi = Pick<
  SDKClient["gateways"],
  "create" | "delete" | "get" | "list" | "update"
>;
type ManagedClusterApi = Pick<SDKClient["managedClusters"], "get" | "list">;
interface GatewayApiClient {
  gateways: GatewayApi;
  managedClusters: ManagedClusterApi;
}
type GatewayApiFactory = (correlationId: string) => GatewayApiClient;

const placementPageSize = 20;

const gatewaySortFields = {
  cluster: "cluster_id",
  endpoint: "external_dns",
  name: "name",
  status: "status",
} as const satisfies Record<GatewayListRequest["sortField"], string>;

function escapeIlikeLiteral(value: string): string {
  return escapeSearchLiteral(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("%", "\\%")
    .replaceAll("_", "\\_");
}

function escapeSearchLiteral(value: string): string {
  return value.replaceAll("'", "''");
}

function normalizeClusterIds(clusterIds: readonly string[]): string[] {
  return [
    ...new Set(clusterIds.map((clusterId) => clusterId.trim()).filter(Boolean)),
  ].sort();
}

function gatewaySearch(value: string): string | undefined {
  const query = value.trim();
  if (!query) {
    return undefined;
  }
  const literal = escapeIlikeLiteral(query);
  return ["name", "cluster_id", "status", "external_dns"]
    .map((field) => `${field} ilike '%${literal}%'`)
    .join(" or ");
}

function apiClient(
  factory: GatewayApiFactory,
  context: GatewayInvocationContext,
): GatewayApiClient {
  return factory(context.correlationId);
}

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

function optionalString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function toGatewayPlacement(cluster: ManagedCluster): GatewayPlacement {
  const region = optionalString(cluster.region);
  const status = optionalString(cluster.status);
  return {
    id: cluster.id,
    name: cluster.name,
    provider: cluster.provider,
    ...(region ? { region } : {}),
    ...(status ? { status } : {}),
  };
}

function gatewayFailureKind(statusCode: number): GatewayFailureKind {
  if (statusCode === 401 || statusCode === 403) {
    return "denied";
  }
  if (statusCode === 404) {
    return "not-found";
  }
  if (statusCode === 409) {
    return "conflict";
  }
  if (statusCode === 408 || statusCode === 429 || statusCode >= 500) {
    return "unavailable";
  }
  return "unknown";
}

async function mapFailure<T>(task: () => Promise<T>): Promise<T> {
  try {
    return await task();
  } catch (error) {
    if (error instanceof SDKAPIError) {
      throw new GatewayOperationError(gatewayFailureKind(error.statusCode), {
        cause: error,
        operationId: error.operationId || undefined,
      });
    }
    throw error;
  }
}

export function createGatewayControlPlaneAdapter(
  apiFactory: GatewayApiFactory,
): GatewayControlPlane {
  return {
    async findGatewayPlacements(search, context) {
      return mapFailure(async () => {
        const normalizedSearch = search.trim();
        const literal = escapeIlikeLiteral(normalizedSearch);
        const result = await apiClient(
          apiFactory,
          context,
        ).managedClusters.list(
          {
            orderBy: "name asc",
            page: 1,
            ...(literal ? { search: `name ilike '%${literal}%'` } : {}),
            size: placementPageSize,
          },
          { signal: context.signal },
        );
        const expectedItemCount = Math.min(placementPageSize, result.total);
        if (
          result.page !== 1 ||
          result.total < 0 ||
          result.items.length !== expectedItemCount
        ) {
          throw new GatewayOperationError("unavailable");
        }
        return {
          hasMore: result.total > result.items.length,
          items: result.items.map(toGatewayPlacement),
        };
      });
    },
    async getGatewayPlacement(clusterId, context) {
      return mapFailure(async () =>
        toGatewayPlacement(
          await apiClient(apiFactory, context).managedClusters.get(clusterId, {
            signal: context.signal,
          }),
        ),
      );
    },
    async getGatewayPlacements(clusterIds, context) {
      return mapFailure(async () => {
        const normalizedClusterIds = normalizeClusterIds(clusterIds);
        if (normalizedClusterIds.length === 0) {
          return [];
        }

        const requestedClusterIds = new Set(normalizedClusterIds);
        const result = await apiClient(
          apiFactory,
          context,
        ).managedClusters.list(
          {
            orderBy: "id asc",
            page: 1,
            search: `id in (${normalizedClusterIds
              .map((clusterId) => `'${escapeSearchLiteral(clusterId)}'`)
              .join(", ")})`,
            size: normalizedClusterIds.length,
          },
          { signal: context.signal },
        );
        const returnedClusterIds = result.items.map(({ id }) => id);
        if (
          result.page !== 1 ||
          result.total < 0 ||
          result.total > normalizedClusterIds.length ||
          result.items.length !== result.total ||
          new Set(returnedClusterIds).size !== returnedClusterIds.length ||
          returnedClusterIds.some(
            (clusterId) => !requestedClusterIds.has(clusterId),
          )
        ) {
          throw new GatewayOperationError("unavailable");
        }
        return result.items.map(toGatewayPlacement);
      });
    },
    async getGateway(gatewayId, context) {
      return mapFailure(async () =>
        toGatewayRecord(
          await apiClient(apiFactory, context).gateways.get(gatewayId, {
            signal: context.signal,
          }),
        ),
      );
    },
    async listGateways(request, context) {
      return mapFailure(async () => {
        const search = gatewaySearch(request.search);
        const result = await apiClient(apiFactory, context).gateways.list(
          {
            orderBy: `${gatewaySortFields[request.sortField]} ${request.sortDirection}`,
            page: request.page,
            ...(search === undefined ? {} : { search }),
            size: request.size,
          },
          { signal: context.signal },
        );
        const pageOffset = (request.page - 1) * request.size;
        const expectedItemCount = Math.max(
          0,
          Math.min(request.size, result.total - pageOffset),
        );
        if (
          result.page !== request.page ||
          result.total < 0 ||
          result.items.length !== expectedItemCount
        ) {
          throw new GatewayOperationError("unavailable");
        }
        return {
          items: result.items.map(toGatewayRecord),
          page: result.page,
          size: request.size,
          total: result.total,
        };
      });
    },
    async provisionGateway(input, context) {
      return mapFailure(async () =>
        toGatewayRecord(
          await apiClient(apiFactory, context).gateways.create(
            {
              cluster_id: input.clusterId,
              database_id: "",
              fleet_id: "",
              name: input.name,
              release_id: "",
            },
            { signal: context.signal },
          ),
        ),
      );
    },
    async removeGateway(gatewayId, context) {
      await mapFailure(() =>
        apiClient(apiFactory, context).gateways.delete(gatewayId, {
          signal: context.signal,
        }),
      );
    },
    async renameGateway(gatewayId, name, context) {
      return mapFailure(async () =>
        toGatewayRecord(
          await apiClient(apiFactory, context).gateways.update(
            gatewayId,
            { name },
            { signal: context.signal },
          ),
        ),
      );
    },
  };
}
