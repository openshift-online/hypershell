import type { Gateway, GatewayList } from "@openshift-online/hypershell-sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  deleteGateway,
  getGateway,
  listGateways,
  renameGateway,
  toGatewayConnection,
} from "./gateway-data";

const {
  deleteGatewayMock,
  getGatewayMock,
  listGatewaysMock,
  updateGatewayMock,
} = vi.hoisted(() => ({
  deleteGatewayMock: vi.fn(),
  getGatewayMock: vi.fn(),
  listGatewaysMock: vi.fn(),
  updateGatewayMock: vi.fn(),
}));

vi.mock("../../lib/api.client", () => ({
  apiClient: {
    gateways: {
      delete: deleteGatewayMock,
      get: getGatewayMock,
      list: listGatewaysMock,
      update: updateGatewayMock,
    },
  },
}));

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

describe("gateway API data", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("maps API gateway fields into the connection view", () => {
    expect(toGatewayConnection(gateway())).toMatchObject({
      clusterName: "Local cluster",
      endpoint: "https://gateway.example.com:443",
      id: "gateway-1",
      name: "Team gateway",
      status: "Ready",
    });
  });

  it("uses a returned cluster identifier when one is present", () => {
    expect(
      toGatewayConnection(gateway({ cluster_id: "cluster-east" })).clusterName,
    ).toBe("cluster-east");
  });

  it("uses the development endpoint when optional DNS is absent", () => {
    const connection = toGatewayConnection({
      ...gateway(),
      external_dns: undefined,
      status: undefined,
    });

    expect(connection.endpoint).toBe(
      "https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443",
    );
    expect(connection.status).toBe("Unknown");
  });

  it("loads every page of gateways", async () => {
    listGatewaysMock
      .mockResolvedValueOnce(gatewayList([gateway()], 2))
      .mockResolvedValueOnce(
        gatewayList([gateway({ id: "gateway-2", name: "Other gateway" })], 2),
      );

    await expect(listGateways()).resolves.toHaveLength(2);
    expect(listGatewaysMock).toHaveBeenNthCalledWith(
      1,
      { page: 1, size: 100 },
      { signal: undefined },
    );
    expect(listGatewaysMock).toHaveBeenNthCalledWith(
      2,
      { page: 2, size: 100 },
      { signal: undefined },
    );
  });

  it("loads a gateway detail by ID", async () => {
    getGatewayMock.mockResolvedValue(gateway());

    await expect(getGateway("gateway-1")).resolves.toMatchObject({
      id: "gateway-1",
      name: "Team gateway",
    });
    expect(getGatewayMock).toHaveBeenCalledWith("gateway-1", {
      signal: undefined,
    });
  });

  it("deletes a gateway by ID", async () => {
    deleteGatewayMock.mockResolvedValue(undefined);

    await expect(deleteGateway("gateway-1")).resolves.toBeUndefined();
    expect(deleteGatewayMock).toHaveBeenCalledWith("gateway-1");
  });

  it("renames a gateway by ID", async () => {
    updateGatewayMock.mockResolvedValue(gateway({ name: "Renamed gateway" }));

    await expect(
      renameGateway("gateway-1", "Renamed gateway"),
    ).resolves.toMatchObject({ name: "Renamed gateway" });
    expect(updateGatewayMock).toHaveBeenCalledWith("gateway-1", {
      name: "Renamed gateway",
    });
  });
});
