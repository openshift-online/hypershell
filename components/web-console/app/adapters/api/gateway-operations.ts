import type {
  GatewayControlPlane,
  GatewayFailureKind,
  GatewayInvocationContext,
  GatewayListRequest,
  GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { GatewayOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type Gateway,
  type SDKClient,
} from "@openshift-online/hypershell-sdk";

type GatewayApi = Pick<
  SDKClient["gateways"],
  "create" | "delete" | "get" | "list" | "update"
>;
type GatewayApiFactory = (correlationId: string) => GatewayApi;

const gatewaySortFields = {
  cluster: "cluster_id",
  endpoint: "external_dns",
  name: "name",
  status: "status",
} as const satisfies Record<GatewayListRequest["sortField"], string>;

function gatewaySearch(value: string): string | undefined {
  const query = value.trim();
  if (!query) {
    return undefined;
  }
  const literal = query.replaceAll("'", "''");
  return ["name", "cluster_id", "status", "external_dns"]
    .map((field) => `${field} ilike '%${literal}%'`)
    .join(" or ");
}

function gatewayApi(
  factory: GatewayApiFactory,
  context: GatewayInvocationContext,
): GatewayApi {
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
    async getGateway(gatewayId, context) {
      return mapFailure(async () =>
        toGatewayRecord(
          await gatewayApi(apiFactory, context).get(gatewayId, {
            signal: context.signal,
          }),
        ),
      );
    },
    async listGateways(request, context) {
      return mapFailure(async () => {
        const search = gatewaySearch(request.search);
        const result = await gatewayApi(apiFactory, context).list(
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
          await gatewayApi(apiFactory, context).create(
            {
              ...input,
              cluster_id: "",
              database_id: "",
              fleet_id: "",
              release_id: "",
            },
            { signal: context.signal },
          ),
        ),
      );
    },
    async removeGateway(gatewayId, context) {
      await mapFailure(() =>
        gatewayApi(apiFactory, context).delete(gatewayId, {
          signal: context.signal,
        }),
      );
    },
    async renameGateway(gatewayId, name, context) {
      return mapFailure(async () =>
        toGatewayRecord(
          await gatewayApi(apiFactory, context).update(
            gatewayId,
            { name },
            { signal: context.signal },
          ),
        ),
      );
    },
  };
}
