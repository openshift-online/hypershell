import { GatewayProfileOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type GatewayProfile,
  type GatewayProfileList,
} from "@openshift-online/hypershell-sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createGatewayProfileControlPlaneAdapter } from "./gateway-profile-operations";

const gatewayProfileApi = {
  create: vi.fn(),
  delete: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
};
const gatewayProfileApiFactory = vi.fn(() => ({
  gatewayProfiles: gatewayProfileApi,
}));
const controlPlane = createGatewayProfileControlPlaneAdapter(
  gatewayProfileApiFactory,
);
const context = {
  correlationId: "11111111-1111-4111-8111-111111111111",
};
const listRequest = {
  page: 2,
  search: "small's profile",
  size: 20,
  sortDirection: "desc",
  sortField: "created",
} as const;

function gatewayProfile(
  overrides: Partial<GatewayProfile> = {},
): GatewayProfile {
  return {
    container_cpu_limit_max: "500m",
    container_cpu_request_default: "100m",
    container_memory_limit_max: "512Mi",
    container_memory_request_default: "128Mi",
    cpu_limit_total: "8",
    cpu_request_total: "4",
    created_at: "2026-08-10T14:30:00Z",
    description: "Small resource quota",
    ephemeral_storage_total: "10Gi",
    href: "/api/hypershell/v1/gateway_profiles/profile-small",
    id: "profile-small",
    kind: "GatewayProfile",
    memory_limit_total: "16Gi",
    memory_request_total: "8Gi",
    name: "Small profile",
    pod_count: 10,
    pvc_count: 5,
    updated_at: null,
    ...overrides,
  };
}

function gatewayProfileList(
  items: GatewayProfile[],
  total = items.length,
  page = 1,
): GatewayProfileList {
  return {
    items,
    kind: "GatewayProfileList",
    page,
    size: items.length,
    total,
  };
}

function apiError(statusCode: number, code = "conflict"): SDKAPIError {
  return new SDKAPIError({
    code,
    href: "",
    id: "",
    kind: "Error",
    operation_id: "operation-1",
    reason: "boom",
    status_code: statusCode,
  });
}

describe("gateway profile control plane adapter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("threads the correlation identifier through the API factory", async () => {
    gatewayProfileApi.get.mockResolvedValue(gatewayProfile());

    await controlPlane.getGatewayProfile("profile-small", context);

    expect(gatewayProfileApiFactory).toHaveBeenCalledWith(
      "11111111-1111-4111-8111-111111111111",
    );
    expect(gatewayProfileApi.get).toHaveBeenCalledWith("profile-small", {
      signal: undefined,
    });
  });

  it("maps a gateway profile record from the API model", async () => {
    gatewayProfileApi.get.mockResolvedValue(gatewayProfile());

    const record = await controlPlane.getGatewayProfile(
      "profile-small",
      context,
    );

    expect(record).toEqual({
      containerCpuLimitMax: "500m",
      containerCpuRequestDefault: "100m",
      containerMemoryLimitMax: "512Mi",
      containerMemoryRequestDefault: "128Mi",
      cpuLimitTotal: "8",
      cpuRequestTotal: "4",
      createdAt: "2026-08-10T14:30:00Z",
      description: "Small resource quota",
      ephemeralStorageTotal: "10Gi",
      id: "profile-small",
      memoryLimitTotal: "16Gi",
      memoryRequestTotal: "8Gi",
      name: "Small profile",
      podCount: 10,
      pvcCount: 5,
    });
  });

  it("omits empty optional quota fields", async () => {
    gatewayProfileApi.get.mockResolvedValue(
      gatewayProfile({
        container_cpu_limit_max: "",
        cpu_request_total: "",
        description: "",
      }),
    );

    const record = await controlPlane.getGatewayProfile(
      "profile-small",
      context,
    );

    expect(record.containerCpuLimitMax).toBeUndefined();
    expect(record.cpuRequestTotal).toBeUndefined();
    expect(record.description).toBeUndefined();
  });

  it("creates a gateway profile from defined fields only", async () => {
    gatewayProfileApi.create.mockResolvedValue(gatewayProfile());

    await controlPlane.createGatewayProfile(
      { cpuRequestTotal: "4", name: "Small profile", podCount: 10 },
      context,
    );

    expect(gatewayProfileApi.create).toHaveBeenCalledWith(
      { cpu_request_total: "4", name: "Small profile", pod_count: 10 },
      { signal: undefined },
    );
  });

  it("lists gateway profiles with an escaped search and sort", async () => {
    gatewayProfileApi.list.mockResolvedValue(
      gatewayProfileList([gatewayProfile()], 21, 2),
    );

    const result = await controlPlane.listGatewayProfiles(listRequest, context);

    expect(gatewayProfileApi.list).toHaveBeenCalledWith(
      {
        orderBy: "created_at desc",
        page: 2,
        search: "name ilike '%small''s profile%'",
        size: 20,
      },
      { signal: undefined },
    );
    expect(result).toEqual({
      items: [expect.objectContaining({ id: "profile-small" })],
      page: 2,
      size: 20,
      total: 21,
    });
  });

  it("omits the search filter when the query is blank", async () => {
    gatewayProfileApi.list.mockResolvedValue(gatewayProfileList([], 0));

    await controlPlane.listGatewayProfiles(
      { ...listRequest, page: 1, search: "  " },
      context,
    );

    expect(gatewayProfileApi.list).toHaveBeenCalledWith(
      { orderBy: "created_at desc", page: 1, size: 20 },
      { signal: undefined },
    );
  });

  it("rejects an inconsistent list page", async () => {
    gatewayProfileApi.list.mockResolvedValue(
      gatewayProfileList([gatewayProfile()], 0, 2),
    );

    await expect(
      controlPlane.listGatewayProfiles(listRequest, context),
    ).rejects.toThrow(GatewayProfileOperationError);
  });

  it("deletes a gateway profile", async () => {
    gatewayProfileApi.delete.mockResolvedValue(undefined);

    await controlPlane.removeGatewayProfile("profile-small", context);

    expect(gatewayProfileApi.delete).toHaveBeenCalledWith("profile-small", {
      signal: undefined,
    });
  });

  it.each([
    [403, "denied"],
    [404, "not-found"],
    [409, "conflict"],
    [503, "unavailable"],
    [418, "unknown"],
  ] as const)("maps HTTP %s to the %s failure kind", async (status, kind) => {
    gatewayProfileApi.delete.mockRejectedValue(apiError(status));

    const error = await controlPlane
      .removeGatewayProfile("profile-small", context)
      .catch((thrown: unknown) => thrown);

    expect(error).toBeInstanceOf(GatewayProfileOperationError);
    expect((error as GatewayProfileOperationError).kind).toBe(kind);
    expect((error as GatewayProfileOperationError).operationId).toBe(
      "operation-1",
    );
  });

  it("propagates non-API errors unchanged", async () => {
    const failure = new Error("network down");
    gatewayProfileApi.get.mockRejectedValue(failure);

    await expect(
      controlPlane.getGatewayProfile("profile-small", context),
    ).rejects.toBe(failure);
  });
});
