import type { Gateway, GatewayList } from "@openshift-online/hypershell-sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createGatewayOperations } from "./gateway-operations";

const gatewayApi = {
  create: vi.fn(),
  delete: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  update: vi.fn(),
};
const operations = createGatewayOperations({ gateways: gatewayApi });

function gateway(overrides: Partial<Gateway> = {}): Gateway {
  return {
    cluster_id: "",
    created_at: null,
    database_id: "database-1",
    external_dns: "gateway.example.com",
    fleet_id: "",
    href: "/api/hypershell/v1/gateways/gateway-1",
    id: "gateway-1",
    kind: "Gateway",
    name: "Team gateway",
    namespace: "openshell",
    phase: "",
    release_id: "release-1",
    service_type: "",
    status: "Ready",
    tls_mode: "",
    updated_at: null,
    ...overrides,
  };
}

function gatewayList(items: Gateway[], total = items.length): GatewayList {
  return {
    items,
    kind: "GatewayList",
    page: 1,
    size: 100,
    total,
  };
}

describe("gateway API operations adapter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("maps and loads every gateway page while preserving cancellation", async () => {
    const abortController = new AbortController();
    gatewayApi.list
      .mockResolvedValueOnce(gatewayList([gateway()], 2))
      .mockResolvedValueOnce(
        gatewayList([gateway({ id: "gateway-2", name: "Other gateway" })], 2),
      );

    await expect(
      operations.listGateways(abortController.signal),
    ).resolves.toMatchObject([
      { id: "gateway-1", name: "Team gateway" },
      { id: "gateway-2", name: "Other gateway" },
    ]);
    expect(gatewayApi.list).toHaveBeenNthCalledWith(
      1,
      { page: 1, size: 100 },
      { signal: abortController.signal },
    );
    expect(gatewayApi.list).toHaveBeenNthCalledWith(
      2,
      { page: 2, size: 100 },
      { signal: abortController.signal },
    );
  });

  it("provisions with reconciler-owned identifiers omitted by the form", async () => {
    gatewayApi.create.mockResolvedValue(
      gateway({ database_id: "", release_id: "" }),
    );

    await operations.provisionGateway({
      name: "team-gateway",
      namespace: "openshell",
    });

    expect(gatewayApi.create).toHaveBeenCalledWith({
      cluster_id: "",
      database_id: "",
      fleet_id: "",
      name: "team-gateway",
      namespace: "openshell",
      release_id: "",
    });
  });

  it("maps detail, rename, and deletion operations", async () => {
    gatewayApi.get.mockResolvedValue(gateway());
    gatewayApi.update.mockResolvedValue(gateway({ name: "Renamed gateway" }));
    gatewayApi.delete.mockResolvedValue(undefined);

    await expect(operations.getGateway("gateway-1")).resolves.toMatchObject({
      id: "gateway-1",
      name: "Team gateway",
    });
    await expect(
      operations.renameGateway("gateway-1", "Renamed gateway"),
    ).resolves.toMatchObject({ name: "Renamed gateway" });
    await expect(
      operations.removeGateway("gateway-1"),
    ).resolves.toBeUndefined();

    expect(gatewayApi.get).toHaveBeenCalledWith("gateway-1", {
      signal: undefined,
    });
    expect(gatewayApi.update).toHaveBeenCalledWith("gateway-1", {
      name: "Renamed gateway",
    });
    expect(gatewayApi.delete).toHaveBeenCalledWith("gateway-1");
  });
});
