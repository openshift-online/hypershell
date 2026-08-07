import { GatewayOperationError } from "@openshift-online/hypershell-gateway-ui";
import {
  SDKAPIError,
  type Gateway,
  type GatewayList,
} from "@openshift-online/hypershell-sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createGatewayControlPlaneAdapter } from "./gateway-operations";

const gatewayApi = {
  create: vi.fn(),
  delete: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  update: vi.fn(),
};
const gatewayApiFactory = vi.fn(() => gatewayApi);
const controlPlane = createGatewayControlPlaneAdapter(gatewayApiFactory);
const context = {
  correlationId: "11111111-1111-4111-8111-111111111111",
};
const listRequest = {
  page: 2,
  search: "team's gateway",
  size: 20,
  sortDirection: "desc",
  sortField: "status",
} as const;

function gateway(overrides: Partial<Gateway> = {}): Gateway {
  return {
    cluster_id: "",
    created_at: null,
    database_config: "",
    database_id: "database-1",
    external_dns: "gateway.example.com",
    fleet_id: "",
    href: "/api/hypershell/v1/gateways/gateway-1",
    id: "gateway-1",
    image: "",
    kind: "Gateway",
    name: "Team gateway",
    namespace: "openshell",
    oidc: "",
    phase: "",
    release_id: "release-1",
    route: "",
    route_address: "",
    server_dns_names: "",
    service_type: "",
    status: "Ready",
    tls_mode: "",
    updated_at: null,
    ...overrides,
  };
}

function gatewayList(
  items: Gateway[],
  total = items.length,
  page = 1,
): GatewayList {
  return {
    items,
    kind: "GatewayList",
    page,
    size: items.length,
    total,
  };
}

describe("gateway API operations adapter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("maps exactly one authoritative gateway page with search and sort", async () => {
    const abortController = new AbortController();
    gatewayApi.list.mockResolvedValueOnce(gatewayList([gateway()], 21, 2));

    await expect(
      controlPlane.listGateways(listRequest, {
        ...context,
        signal: abortController.signal,
      }),
    ).resolves.toMatchObject({
      items: [{ id: "gateway-1", name: "Team gateway" }],
      page: 2,
      size: 20,
      total: 21,
    });
    expect(gatewayApiFactory).toHaveBeenCalledWith(context.correlationId);
    expect(gatewayApi.list).toHaveBeenCalledOnce();
    expect(gatewayApi.list).toHaveBeenCalledWith(
      {
        orderBy: "status desc",
        page: 2,
        search:
          "name ilike '%team''s gateway%' or cluster_id ilike '%team''s gateway%' or status ilike '%team''s gateway%' or external_dns ilike '%team''s gateway%'",
        size: 20,
      },
      { signal: abortController.signal },
    );
  });

  it("rejects an incomplete upstream page instead of returning a partial list", async () => {
    gatewayApi.list.mockResolvedValue(gatewayList([gateway()], 41, 2));

    await expect(
      controlPlane.listGateways(listRequest, context),
    ).rejects.toMatchObject({ kind: "unavailable" });
    expect(gatewayApi.list).toHaveBeenCalledOnce();
  });

  it("omits a search expression when the filter is blank", async () => {
    gatewayApi.list.mockResolvedValue(gatewayList([], 0, 1));

    await controlPlane.listGateways(
      { ...listRequest, page: 1, search: "   " },
      context,
    );

    expect(gatewayApi.list).toHaveBeenCalledWith(
      { orderBy: "status desc", page: 1, size: 20 },
      { signal: undefined },
    );
  });

  it("provisions with reconciler-owned identifiers omitted by the form", async () => {
    gatewayApi.create.mockResolvedValue(
      gateway({ database_id: "", release_id: "" }),
    );

    await controlPlane.provisionGateway(
      {
        name: "team-gateway",
        namespace: "openshell",
      },
      context,
    );

    expect(gatewayApi.create).toHaveBeenCalledWith(
      {
        cluster_id: "",
        database_id: "",
        fleet_id: "",
        name: "team-gateway",
        namespace: "openshell",
        release_id: "",
      },
      { signal: undefined },
    );
  });

  it("maps detail, rename, and deletion operations", async () => {
    gatewayApi.get.mockResolvedValue(gateway());
    gatewayApi.update.mockResolvedValue(gateway({ name: "Renamed gateway" }));
    gatewayApi.delete.mockResolvedValue(undefined);

    await expect(
      controlPlane.getGateway("gateway-1", context),
    ).resolves.toMatchObject({ id: "gateway-1", name: "Team gateway" });
    await expect(
      controlPlane.renameGateway("gateway-1", "Renamed gateway", context),
    ).resolves.toMatchObject({ name: "Renamed gateway" });
    await expect(
      controlPlane.removeGateway("gateway-1", context),
    ).resolves.toBeUndefined();

    expect(gatewayApi.get).toHaveBeenCalledWith("gateway-1", {
      signal: undefined,
    });
    expect(gatewayApi.update).toHaveBeenCalledWith(
      "gateway-1",
      { name: "Renamed gateway" },
      { signal: undefined },
    );
    expect(gatewayApi.delete).toHaveBeenCalledWith("gateway-1", {
      signal: undefined,
    });
  });

  it("maps SDK failures into stable application failures", async () => {
    gatewayApi.update.mockRejectedValue(
      new SDKAPIError({
        code: "conflict",
        href: "",
        id: "",
        kind: "Error",
        operation_id: "operation-1",
        reason: "raw provider detail",
        status_code: 409,
      }),
    );

    const failure = await controlPlane
      .renameGateway("gateway-1", "Existing gateway", context)
      .catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(GatewayOperationError);
    expect(failure).toMatchObject({
      kind: "conflict",
      operationId: "operation-1",
    });
    expect((failure as Error).message).not.toContain("raw provider detail");
  });
});
