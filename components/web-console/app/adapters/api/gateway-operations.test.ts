import { GatewayOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type Gateway,
  type GatewayList,
  type ManagedCluster,
  type ManagedClusterList,
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
const managedClusterApi = {
  get: vi.fn(),
  list: vi.fn(),
};
const gatewayApiFactory = vi.fn(() => ({
  gateways: gatewayApi,
  managedClusters: managedClusterApi,
}));
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
    supervisor_image: "",
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

function managedCluster(
  overrides: Partial<ManagedCluster> = {},
): ManagedCluster {
  return {
    api_server_url: "https://api.east.example.com",
    created_at: null,
    fleet_id: "fleet-1",
    href: "/api/hypershell/v1/managed_clusters/cluster-east",
    id: "cluster-east",
    kind: "ManagedCluster",
    kubeconfig_secret: "cluster-east-kubeconfig",
    name: "Cluster East",
    provider: "AWS",
    region: "us-east-1",
    status: "Ready",
    updated_at: null,
    ...overrides,
  };
}

function managedClusterList(
  items: ManagedCluster[],
  total = items.length,
): ManagedClusterList {
  return {
    items,
    kind: "ManagedClusterList",
    page: 1,
    size: items.length,
    total,
  };
}

describe("gateway API operations adapter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("maps one authoritative managed-cluster search page into placements", async () => {
    const abortController = new AbortController();
    managedClusterApi.list.mockResolvedValue(
      managedClusterList([managedCluster()]),
    );

    await expect(
      controlPlane.findGatewayPlacements(" team's east ", {
        ...context,
        signal: abortController.signal,
      }),
    ).resolves.toEqual({
      hasMore: false,
      items: [
        {
          id: "cluster-east",
          name: "Cluster East",
          provider: "AWS",
          region: "us-east-1",
          status: "Ready",
        },
      ],
    });
    expect(managedClusterApi.list).toHaveBeenCalledOnce();
    expect(managedClusterApi.list).toHaveBeenCalledWith(
      {
        orderBy: "name asc",
        page: 1,
        search: "name ilike '%team''s east%'",
        size: 20,
      },
      { signal: abortController.signal },
    );
  });

  it("resolves a managed cluster into a gateway placement", async () => {
    const abortController = new AbortController();
    managedClusterApi.get.mockResolvedValue(managedCluster());

    await expect(
      controlPlane.getGatewayPlacement("cluster-east", {
        ...context,
        signal: abortController.signal,
      }),
    ).resolves.toMatchObject({
      id: "cluster-east",
      name: "Cluster East",
    });
    expect(managedClusterApi.get).toHaveBeenCalledWith("cluster-east", {
      signal: abortController.signal,
    });
  });

  it("resolves a cold page of distinct cluster identifiers in one batch", async () => {
    const clusterIds = Array.from(
      { length: 20 },
      (_, index) => `cluster-${String(index)}`,
    ).sort();
    managedClusterApi.list.mockResolvedValue(
      managedClusterList(
        clusterIds.map((id) => managedCluster({ id, name: `Name for ${id}` })),
      ),
    );

    await expect(
      controlPlane.getGatewayPlacements(
        [...clusterIds, " cluster-0 ", "", "cluster-1"],
        context,
      ),
    ).resolves.toHaveLength(20);
    expect(managedClusterApi.list).toHaveBeenCalledOnce();
    expect(managedClusterApi.list).toHaveBeenCalledWith(
      {
        orderBy: "id asc",
        page: 1,
        search: `id in (${clusterIds.map((id) => `'${id}'`).join(", ")})`,
        size: 20,
      },
      { signal: undefined },
    );
    expect(managedClusterApi.get).not.toHaveBeenCalled();
  });

  it("rejects an inconsistent cluster batch response", async () => {
    managedClusterApi.list.mockResolvedValue(
      managedClusterList([managedCluster({ id: "cluster-unrequested" })]),
    );

    await expect(
      controlPlane.getGatewayPlacements(["cluster-east"], context),
    ).rejects.toMatchObject({ kind: "unavailable" });
  });

  it("skips the cluster API when a placement batch has no identifiers", async () => {
    await expect(
      controlPlane.getGatewayPlacements(["", "  "], context),
    ).resolves.toEqual([]);
    expect(managedClusterApi.list).not.toHaveBeenCalled();
  });

  it("uses the API search grammar for quoted batch identifiers", async () => {
    managedClusterApi.list.mockResolvedValue(managedClusterList([]));

    await controlPlane.getGatewayPlacements(["team's\\cluster"], context);

    expect(managedClusterApi.list).toHaveBeenCalledWith(
      expect.objectContaining({
        search: "id in ('team''s\\cluster')",
      }),
      { signal: undefined },
    );
  });

  it("reports when a bounded placement search has more results", async () => {
    managedClusterApi.list.mockResolvedValue(
      managedClusterList(
        Array.from({ length: 20 }, (_, index) =>
          managedCluster({
            id: `cluster-${String(index)}`,
            name: `Cluster ${String(index)}`,
          }),
        ),
        21,
      ),
    );

    await expect(
      controlPlane.findGatewayPlacements("", context),
    ).resolves.toMatchObject({ hasMore: true });
    expect(managedClusterApi.list).toHaveBeenCalledOnce();
  });

  it("treats ILIKE wildcard and escape characters as search literals", async () => {
    managedClusterApi.list.mockResolvedValue(managedClusterList([]));
    gatewayApi.list.mockResolvedValue(gatewayList([], 0, 1));

    await controlPlane.findGatewayPlacements("my_cluster%\\west's", context);
    await controlPlane.listGateways(
      {
        ...listRequest,
        page: 1,
        search: "my_cluster%\\west's",
      },
      context,
    );

    expect(managedClusterApi.list).toHaveBeenCalledWith(
      expect.objectContaining({
        search: "name ilike '%my\\_cluster\\%\\\\west''s%'",
      }),
      { signal: undefined },
    );
    expect(gatewayApi.list).toHaveBeenCalledWith(
      expect.objectContaining({
        search:
          "name ilike '%my\\_cluster\\%\\\\west''s%' or cluster_id ilike '%my\\_cluster\\%\\\\west''s%' or status ilike '%my\\_cluster\\%\\\\west''s%' or external_dns ilike '%my\\_cluster\\%\\\\west''s%'",
      }),
      { signal: undefined },
    );
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

  it("maps explicit OIDC connection values from the gateway response", async () => {
    gatewayApi.get.mockResolvedValue(
      gateway({
        oidc: JSON.stringify({
          audience: "openshell-api",
          client_id: "openshell-cli",
          issuer: "https://issuer.example.test/realms/openshell",
        }),
      }),
    );

    await expect(
      controlPlane.getGateway("gateway-1", context),
    ).resolves.toMatchObject({
      oidcAudience: "openshell-api",
      oidcClientId: "openshell-cli",
      oidcIssuer: "https://issuer.example.test/realms/openshell",
    });
  });

  it("keeps malformed OIDC connection values unavailable", async () => {
    gatewayApi.get.mockResolvedValue(gateway({ oidc: "not-json" }));

    const result = await controlPlane.getGateway("gateway-1", context);

    expect(result.oidcAudience).toBeUndefined();
    expect(result.oidcClientId).toBeUndefined();
    expect(result.oidcIssuer).toBeUndefined();
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

  it("provisions on the selected cluster with hidden request defaults", async () => {
    gatewayApi.create.mockResolvedValue(
      gateway({ database_id: "", release_id: "" }),
    );

    await controlPlane.provisionGateway(
      {
        clusterId: "cluster-east",
        name: "team-gateway",
      },
      context,
    );

    expect(gatewayApi.create).toHaveBeenCalledWith(
      {
        cluster_id: "cluster-east",
        database_id: "",
        fleet_id: "",
        name: "team-gateway",
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
