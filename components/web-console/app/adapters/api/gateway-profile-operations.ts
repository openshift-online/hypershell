import type {
  GatewayProfileControlPlane,
  GatewayProfileCreateInput,
  GatewayProfileFailureKind,
  GatewayProfileInvocationContext,
  GatewayProfileListRequest,
  GatewayProfileRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { GatewayProfileOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type GatewayProfile,
  type GatewayProfileCreateRequest,
  type SDKClient,
} from "@openshift-online/hypershell-sdk";

type GatewayProfileApi = Pick<
  SDKClient["gatewayProfiles"],
  "create" | "delete" | "get" | "list"
>;
interface GatewayProfileApiClient {
  gatewayProfiles: GatewayProfileApi;
}
type GatewayProfileApiFactory = (
  correlationId: string,
) => GatewayProfileApiClient;

const gatewayProfileSortFields = {
  created: "created_at",
  name: "name",
} as const satisfies Record<GatewayProfileListRequest["sortField"], string>;

function apiClient(
  factory: GatewayProfileApiFactory,
  context: GatewayProfileInvocationContext,
): GatewayProfileApiClient {
  return factory(context.correlationId);
}

function escapeIlikeLiteral(value: string): string {
  // Escape backslashes first so the escapes added for SQL wildcards remain
  // single escapes rather than being escaped again.
  return value
    .replaceAll("'", "''")
    .replaceAll("\\", "\\\\")
    .replaceAll("%", "\\%")
    .replaceAll("_", "\\_");
}

function gatewayProfileSearch(value: string): string | undefined {
  const query = value.trim();
  if (!query) {
    return undefined;
  }
  return `name ilike '%${escapeIlikeLiteral(query)}%'`;
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
}

function toGatewayProfileRecord(profile: GatewayProfile): GatewayProfileRecord {
  const containerCpuLimitMax = optionalString(profile.container_cpu_limit_max);
  const containerCpuRequestDefault = optionalString(
    profile.container_cpu_request_default,
  );
  const containerMemoryLimitMax = optionalString(
    profile.container_memory_limit_max,
  );
  const containerMemoryRequestDefault = optionalString(
    profile.container_memory_request_default,
  );
  const cpuLimitTotal = optionalString(profile.cpu_limit_total);
  const cpuRequestTotal = optionalString(profile.cpu_request_total);
  const description = optionalString(profile.description);
  const ephemeralStorageTotal = optionalString(profile.ephemeral_storage_total);
  const memoryLimitTotal = optionalString(profile.memory_limit_total);
  const memoryRequestTotal = optionalString(profile.memory_request_total);

  return {
    ...(containerCpuLimitMax ? { containerCpuLimitMax } : {}),
    ...(containerCpuRequestDefault ? { containerCpuRequestDefault } : {}),
    ...(containerMemoryLimitMax ? { containerMemoryLimitMax } : {}),
    ...(containerMemoryRequestDefault ? { containerMemoryRequestDefault } : {}),
    ...(cpuLimitTotal ? { cpuLimitTotal } : {}),
    ...(cpuRequestTotal ? { cpuRequestTotal } : {}),
    ...(profile.created_at ? { createdAt: profile.created_at } : {}),
    ...(description ? { description } : {}),
    ...(ephemeralStorageTotal ? { ephemeralStorageTotal } : {}),
    id: profile.id,
    ...(memoryLimitTotal ? { memoryLimitTotal } : {}),
    ...(memoryRequestTotal ? { memoryRequestTotal } : {}),
    name: profile.name,
    podCount: profile.pod_count,
    pvcCount: profile.pvc_count,
  };
}

function toCreateRequest(
  input: GatewayProfileCreateInput,
): GatewayProfileCreateRequest {
  return {
    ...(input.containerCpuLimitMax === undefined
      ? {}
      : { container_cpu_limit_max: input.containerCpuLimitMax }),
    ...(input.containerCpuRequestDefault === undefined
      ? {}
      : { container_cpu_request_default: input.containerCpuRequestDefault }),
    ...(input.containerMemoryLimitMax === undefined
      ? {}
      : { container_memory_limit_max: input.containerMemoryLimitMax }),
    ...(input.containerMemoryRequestDefault === undefined
      ? {}
      : {
          container_memory_request_default: input.containerMemoryRequestDefault,
        }),
    ...(input.cpuLimitTotal === undefined
      ? {}
      : { cpu_limit_total: input.cpuLimitTotal }),
    ...(input.cpuRequestTotal === undefined
      ? {}
      : { cpu_request_total: input.cpuRequestTotal }),
    ...(input.description === undefined
      ? {}
      : { description: input.description }),
    ...(input.ephemeralStorageTotal === undefined
      ? {}
      : { ephemeral_storage_total: input.ephemeralStorageTotal }),
    ...(input.memoryLimitTotal === undefined
      ? {}
      : { memory_limit_total: input.memoryLimitTotal }),
    ...(input.memoryRequestTotal === undefined
      ? {}
      : { memory_request_total: input.memoryRequestTotal }),
    name: input.name,
    ...(input.podCount === undefined ? {} : { pod_count: input.podCount }),
    ...(input.pvcCount === undefined ? {} : { pvc_count: input.pvcCount }),
  };
}

function gatewayProfileFailureKind(
  statusCode: number,
): GatewayProfileFailureKind {
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
      throw new GatewayProfileOperationError(
        gatewayProfileFailureKind(error.statusCode),
        {
          cause: error,
          operationId: error.operationId || undefined,
        },
      );
    }
    throw error;
  }
}

export function createGatewayProfileControlPlaneAdapter(
  apiFactory: GatewayProfileApiFactory,
): GatewayProfileControlPlane {
  return {
    async createGatewayProfile(input, context) {
      return mapFailure(async () =>
        toGatewayProfileRecord(
          await apiClient(apiFactory, context).gatewayProfiles.create(
            toCreateRequest(input),
            { signal: context.signal },
          ),
        ),
      );
    },
    async getGatewayProfile(gatewayProfileId, context) {
      return mapFailure(async () =>
        toGatewayProfileRecord(
          await apiClient(apiFactory, context).gatewayProfiles.get(
            gatewayProfileId,
            { signal: context.signal },
          ),
        ),
      );
    },
    async listGatewayProfiles(request, context) {
      return mapFailure(async () => {
        const search = gatewayProfileSearch(request.search);
        const result = await apiClient(
          apiFactory,
          context,
        ).gatewayProfiles.list(
          {
            orderBy: `${gatewayProfileSortFields[request.sortField]} ${request.sortDirection}`,
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
          throw new GatewayProfileOperationError("unavailable");
        }
        return {
          items: result.items.map(toGatewayProfileRecord),
          page: result.page,
          size: request.size,
          total: result.total,
        };
      });
    },
    async removeGatewayProfile(gatewayProfileId, context) {
      await mapFailure(() =>
        apiClient(apiFactory, context).gatewayProfiles.delete(
          gatewayProfileId,
          {
            signal: context.signal,
          },
        ),
      );
    },
  };
}
