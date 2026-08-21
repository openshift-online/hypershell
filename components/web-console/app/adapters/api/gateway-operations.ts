import type {
  GatewayControlPlane,
  GatewayFailureCode,
  GatewayFailureKind,
  GatewayInvocationContext,
  GatewayListRequest,
  GatewayPlacement,
  GatewayRecord,
  OpenShellGatewayServiceAccountCapabilities,
  OpenShellGatewayServiceAccountConnection,
  OpenShellGatewayServiceAccountCredential,
  OpenShellGatewayServiceAccountRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  defaultGatewayListRequest,
  GatewayOperationError,
  normalizeGatewayPlacementClusterIds,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type Gateway,
  type ManagedCluster,
  type OpenShellGatewayServiceAccountCapabilities as ApiServiceAccountCapabilities,
  type OpenShellGatewayServiceAccountConnection as ApiServiceAccountConnection,
  type OpenShellGatewayServiceAccountCreateResponse,
  type OpenShellGatewayServiceAccountCredential as ApiServiceAccountCredential,
  type OpenShellGatewayServiceAccountGetResponse,
  type OpenShellGatewayServiceAccountListItem,
  type SDKClient,
} from "@openshift-online/hypershell-sdk";

type GatewayApi = Pick<
  SDKClient["gateways"],
  "create" | "delete" | "get" | "list" | "update"
>;
type ManagedClusterApi = Pick<SDKClient["managedClusters"], "get" | "list">;
type ServiceAccountApi = Pick<
  SDKClient["openShellGatewayServiceAccounts"],
  "create" | "delete" | "get" | "list" | "revoke"
>;
interface GatewayApiClient {
  gateways: GatewayApi;
  managedClusters: ManagedClusterApi;
  openShellGatewayServiceAccounts: ServiceAccountApi;
}
type GatewayApiFactory = (correlationId: string) => GatewayApiClient;

const placementPageSize = defaultGatewayListRequest.size;

const gatewaySortFields = {
  cluster: "cluster_id",
  created: "created_at",
  endpoint: "route_address",
  name: "name",
  status: "status",
} as const satisfies Record<GatewayListRequest["sortField"], string>;

function escapeIlikeLiteral(value: string): string {
  // Escape backslashes first so the escapes added for SQL wildcards remain
  // single escapes rather than being escaped again.
  return escapeSearchLiteral(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("%", "\\%")
    .replaceAll("_", "\\_");
}

function escapeSearchLiteral(value: string): string {
  return value.replaceAll("'", "''");
}

function gatewaySearch(value: string): string | undefined {
  const query = value.trim();
  if (!query) {
    return undefined;
  }
  const literal = escapeIlikeLiteral(query);
  return ["name", "cluster_id", "status", "route_address", "external_dns"]
    .map((field) => `${field} ilike '%${literal}%'`)
    .join(" or ");
}

function apiClient(
  factory: GatewayApiFactory,
  context: GatewayInvocationContext,
): GatewayApiClient {
  return factory(context.correlationId);
}

function jsonObject(value: string): Record<string, unknown> | undefined {
  try {
    const parsed: unknown = JSON.parse(value);
    return typeof parsed === "object" &&
      parsed !== null &&
      !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}

function endpointFromRouteAddress(routeAddress: string): string | undefined {
  if (!routeAddress) {
    return undefined;
  }
  return routeAddress.replace(/^grpcs?:\/\//u, "");
}

function toGatewayRecord(gateway: Gateway): GatewayRecord {
  const oidc = jsonObject(gateway.oidc);
  const oidcAudience = optionalString(oidc?.audience);
  const oidcClientId = optionalString(oidc?.client_id);
  const oidcIssuer = optionalString(oidc?.issuer);

  return {
    ...(typeof gateway.active_sandbox_count === "number"
      ? { activeSandboxCount: gateway.active_sandbox_count }
      : {}),
    clusterId: gateway.cluster_id,
    ...(gateway.console_address ? { consoleUrl: gateway.console_address } : {}),
    ...(gateway.created_at ? { createdAt: gateway.created_at } : {}),
    databaseId: gateway.database_id,
    externalDns:
      gateway.external_dns || endpointFromRouteAddress(gateway.route_address),
    id: gateway.id,
    name: gateway.name,
    namespace: gateway.namespace,
    ...(oidcAudience ? { oidcAudience } : {}),
    ...(oidcClientId ? { oidcClientId } : {}),
    ...(oidcIssuer ? { oidcIssuer } : {}),
    phase: gateway.phase,
    releaseId: gateway.release_id,
    status: gateway.status,
  };
}

function optionalString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

type ApiServiceAccountRecord =
  | OpenShellGatewayServiceAccountCreateResponse
  | OpenShellGatewayServiceAccountGetResponse
  | OpenShellGatewayServiceAccountListItem;

function toServiceAccountRecord(
  account: ApiServiceAccountRecord,
): OpenShellGatewayServiceAccountRecord {
  const description = optionalString(account.description);
  const lastError = optionalString(account.last_error);
  const revokedAt = optionalString(account.revoked_at);
  return {
    clientId: account.client_id,
    createdAt: account.created_at,
    createdByUserId: account.created_by_user_id,
    ...(description ? { description } : {}),
    expiresAt: account.expires_at,
    gatewayId: account.gateway_id,
    id: account.id,
    ...(lastError ? { lastError } : {}),
    name: account.name,
    ...(revokedAt ? { revokedAt } : {}),
    role: account.role,
    status: account.status,
    subject: account.subject,
    updatedAt: account.updated_at,
  };
}

function toServiceAccountConnection(
  connection: ApiServiceAccountConnection,
): OpenShellGatewayServiceAccountConnection {
  return {
    accessTokenLifetimeSeconds: connection.access_token_lifetime_seconds,
    audience: connection.audience,
    clientId: connection.client_id,
    ...(connection.gateway_endpoint
      ? { gatewayEndpoint: connection.gateway_endpoint }
      : {}),
    gatewayName: connection.gateway_name,
    issuer: connection.issuer,
    tokenEndpoint: connection.token_endpoint,
  };
}

function toServiceAccountCredential(
  credential: ApiServiceAccountCredential,
): OpenShellGatewayServiceAccountCredential {
  return {
    ...toServiceAccountConnection(credential),
    clientSecret: credential.client_secret,
  };
}

function toServiceAccountCapabilities(
  capabilities: ApiServiceAccountCapabilities,
): OpenShellGatewayServiceAccountCapabilities {
  return {
    allowedRoles: capabilities.allowed_roles,
    canCreate: capabilities.can_create,
    canManageAll: capabilities.can_manage_all,
    expirationPolicy: {
      defaultSeconds: capabilities.expiration_policy.default_seconds,
      maximumSeconds: capabilities.expiration_policy.maximum_seconds,
      minimumSeconds: capabilities.expiration_policy.minimum_seconds,
    },
  };
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

function gatewayFailureCode(code: string): GatewayFailureCode | undefined {
  if (code === "service_account_name_exists") {
    return "service-account-name-exists";
  }
  return undefined;
}

async function mapFailure<T>(task: () => Promise<T>): Promise<T> {
  try {
    return await task();
  } catch (error) {
    if (error instanceof SDKAPIError) {
      throw new GatewayOperationError(gatewayFailureKind(error.statusCode), {
        cause: error,
        code: gatewayFailureCode(error.code),
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
    async createOpenShellGatewayServiceAccount(gatewayId, input, context) {
      return mapFailure(async () => {
        const response = await apiClient(
          apiFactory,
          context,
        ).openShellGatewayServiceAccounts.create(
          gatewayId,
          {
            ...(input.description === undefined
              ? {}
              : { description: input.description }),
            expires_at: input.expiresAt,
            name: input.name,
            role: input.role,
          },
          { signal: context.signal },
        );
        return {
          credential: toServiceAccountCredential(response.credential),
          serviceAccount: toServiceAccountRecord(response),
        };
      });
    },
    async deleteOpenShellGatewayServiceAccount(
      gatewayId,
      serviceAccountId,
      context,
    ) {
      await mapFailure(() =>
        apiClient(apiFactory, context).openShellGatewayServiceAccounts.delete(
          gatewayId,
          serviceAccountId,
          {
            signal: context.signal,
          },
        ),
      );
    },
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
        const normalizedClusterIds =
          normalizeGatewayPlacementClusterIds(clusterIds);
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
    async getOpenShellGatewayServiceAccount(
      gatewayId,
      serviceAccountId,
      context,
    ) {
      return mapFailure(async () => {
        const response = await apiClient(
          apiFactory,
          context,
        ).openShellGatewayServiceAccounts.get(gatewayId, serviceAccountId, {
          signal: context.signal,
        });
        return {
          connection: toServiceAccountConnection(response.connection),
          serviceAccount: toServiceAccountRecord(response),
        };
      });
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
    async listOpenShellGatewayServiceAccounts(gatewayId, request, context) {
      return mapFailure(async () => {
        const result = await apiClient(
          apiFactory,
          context,
        ).openShellGatewayServiceAccounts.list(
          gatewayId,
          {
            order: request.order,
            page: request.page,
            search: request.search,
            size: request.size,
            sort: request.sort,
            ...(request.status === undefined ? {} : { status: request.status }),
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
          capabilities: toServiceAccountCapabilities(result.capabilities),
          items: result.items.map(toServiceAccountRecord),
          page: result.page,
          size: result.size,
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
              route: JSON.stringify({ enabled: true }),
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
    async revokeOpenShellGatewayServiceAccount(
      gatewayId,
      serviceAccountId,
      context,
    ) {
      return mapFailure(async () =>
        toServiceAccountRecord(
          await apiClient(
            apiFactory,
            context,
          ).openShellGatewayServiceAccounts.revoke(
            gatewayId,
            serviceAccountId,
            { signal: context.signal },
          ),
        ),
      );
    },
  };
}
