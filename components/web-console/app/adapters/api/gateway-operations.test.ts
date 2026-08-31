import { GatewayOperationError } from "@openshift-online/hypershell-gateway-management-ui";
import {
  SDKAPIError,
  type Gateway,
  type GatewayList,
  type ManagedCluster,
  type ManagedClusterList,
  type OpenShellGatewayServiceAccountCreateResponse,
  type OpenShellGatewayServiceAccountList,
  type OpenShellGatewayServiceAccountListItem,
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
const serviceAccountApi = {
  create: vi.fn(),
  delete: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  revoke: vi.fn(),
};
const gatewayApiFactory = vi.fn(() => ({
  gateways: gatewayApi,
  managedClusters: managedClusterApi,
  openShellGatewayServiceAccounts: serviceAccountApi,
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
    active_sandbox_count: 0,
    cluster_id: "",
    console_address: "",
    created_at: null,
    created_by: "",
    credential_driver: "",
    database_id: "database-1",
    external_dns: "gateway.example.com",
    fleet_id: "",
    gateway_version: "",
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

function serviceAccount(
  overrides: Partial<OpenShellGatewayServiceAccountListItem> = {},
): OpenShellGatewayServiceAccountListItem {
  return {
    client_id: "service-client",
    created_at: "2026-08-21T12:00:00Z",
    created_by_user_id: "user-1",
    credential_type: "client_secret",
    description: "Deploy bot",
    expires_at: "2026-11-19T12:00:00Z",
    gateway_id: "gateway-1",
    id: "account-1",
    last_error: null,
    name: "deploy-bot",
    revoked_at: null,
    role: "openshell-user",
    status: "ready",
    subject: "service-subject",
    updated_at: "2026-08-21T12:00:00Z",
    ...overrides,
  };
}

function serviceAccountList(
  items: OpenShellGatewayServiceAccountListItem[],
): OpenShellGatewayServiceAccountList {
  return {
    capabilities: {
      allowed_roles: ["openshell-user"],
      can_create: true,
      can_manage_all: false,
      expiration_policy: {
        default_seconds: 7_776_000,
        maximum_seconds: 31_536_000,
        minimum_seconds: 3_600,
      },
    },
    items,
    page: 1,
    size: 20,
    total: items.length,
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

  it("maps one gateway-scoped service-account page and preserves literal search", async () => {
    serviceAccountApi.list.mockResolvedValue(
      serviceAccountList([serviceAccount()]),
    );

    await expect(
      controlPlane.listOpenShellGatewayServiceAccounts(
        "gateway-1",
        {
          order: "desc",
          page: 1,
          search: "bot%_'\\",
          size: 20,
          sort: "created_at",
          status: "ready",
        },
        context,
      ),
    ).resolves.toMatchObject({
      capabilities: {
        allowedRoles: ["openshell-user"],
        canCreate: true,
        canManageAll: false,
      },
      items: [
        {
          clientId: "service-client",
          id: "account-1",
          subject: "service-subject",
        },
      ],
      total: 1,
    });
    expect(serviceAccountApi.list).toHaveBeenCalledWith(
      "gateway-1",
      {
        order: "desc",
        page: 1,
        search: "bot%_'\\",
        size: 20,
        sort: "created_at",
        status: "ready",
      },
      { signal: undefined },
    );
  });

  it("maps one-time create credentials and repeatable lifecycle operations", async () => {
    const credential = {
      access_token_lifetime_seconds: 300,
      audience: "gateway-audience",
      client_id: "service-client",
      client_secret: "one-time-secret",
      gateway_endpoint: "https://gateway.example.test:443",
      gateway_name: "team-gateway",
      grant_type: "client_credentials" as const,
      issuer: "https://issuer.example.test/realms/openshell",
      token_endpoint:
        "https://issuer.example.test/realms/openshell/protocol/openid-connect/token",
    };
    const created: OpenShellGatewayServiceAccountCreateResponse = {
      ...serviceAccount(),
      credential,
    };
    serviceAccountApi.create.mockResolvedValue(created);
    serviceAccountApi.get.mockResolvedValue({
      ...serviceAccount(),
      connection: credential,
    });
    serviceAccountApi.revoke.mockResolvedValue(
      serviceAccount({ status: "revoking" }),
    );
    serviceAccountApi.delete.mockResolvedValue(undefined);

    await expect(
      controlPlane.createOpenShellGatewayServiceAccount(
        "gateway-1",
        {
          expiresAt: "2026-11-19T12:00:00Z",
          name: "deploy-bot",
          role: "openshell-user",
        },
        context,
      ),
    ).resolves.toMatchObject({
      credential: { clientSecret: "one-time-secret" },
      serviceAccount: { id: "account-1" },
    });
    await expect(
      controlPlane.getOpenShellGatewayServiceAccount(
        "gateway-1",
        "account-1",
        context,
      ),
    ).resolves.toMatchObject({ connection: { clientId: "service-client" } });
    await expect(
      controlPlane.revokeOpenShellGatewayServiceAccount(
        "gateway-1",
        "account-1",
        context,
      ),
    ).resolves.toMatchObject({ status: "revoking" });
    await expect(
      controlPlane.deleteOpenShellGatewayServiceAccount(
        "gateway-1",
        "account-1",
        context,
      ),
    ).resolves.toBeUndefined();
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
          "name ilike '%my\\_cluster\\%\\\\west''s%' or cluster_id ilike '%my\\_cluster\\%\\\\west''s%' or status ilike '%my\\_cluster\\%\\\\west''s%' or route_address ilike '%my\\_cluster\\%\\\\west''s%' or external_dns ilike '%my\\_cluster\\%\\\\west''s%'",
      }),
      { signal: undefined },
    );
  });

  it("maps exactly one authoritative gateway page with search and sort", async () => {
    const abortController = new AbortController();
    gatewayApi.list.mockResolvedValueOnce(
      gatewayList([gateway({ created_at: "2026-08-10T14:30:00Z" })], 21, 2),
    );

    await expect(
      controlPlane.listGateways(listRequest, {
        ...context,
        signal: abortController.signal,
      }),
    ).resolves.toMatchObject({
      items: [
        {
          createdAt: "2026-08-10T14:30:00Z",
          id: "gateway-1",
          name: "Team gateway",
        },
      ],
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
          "name ilike '%team''s gateway%' or cluster_id ilike '%team''s gateway%' or status ilike '%team''s gateway%' or route_address ilike '%team''s gateway%' or external_dns ilike '%team''s gateway%'",
        size: 20,
      },
      { signal: abortController.signal },
    );
  });

  it("sorts displayed route endpoints by their authoritative field", async () => {
    gatewayApi.list.mockResolvedValue(gatewayList([], 0, 1));

    await controlPlane.listGateways(
      {
        ...listRequest,
        page: 1,
        search: "",
        sortDirection: "asc",
        sortField: "endpoint",
      },
      context,
    );

    expect(gatewayApi.list).toHaveBeenCalledWith(
      expect.objectContaining({ orderBy: "route_address asc" }),
      { signal: undefined },
    );
  });

  it("sorts gateway creators by the API created-by field", async () => {
    gatewayApi.list.mockResolvedValue(gatewayList([], 0, 1));

    await controlPlane.listGateways(
      {
        ...listRequest,
        page: 1,
        search: "",
        sortDirection: "desc",
        sortField: "owner",
      },
      context,
    );

    expect(gatewayApi.list).toHaveBeenCalledWith(
      expect.objectContaining({ orderBy: "created_by desc" }),
      { signal: undefined },
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

  it("derives the endpoint from route_address when external_dns is absent", async () => {
    gatewayApi.get.mockResolvedValue(
      gateway({
        external_dns: "",
        route_address: "grpcs://openshell-gw-test.apps.example.com:443",
      }),
    );

    await expect(
      controlPlane.getGateway("gateway-1", context),
    ).resolves.toMatchObject({
      externalDns: "openshell-gw-test.apps.example.com:443",
    });
  });

  it("maps console_address to the console URL so the open-console action renders", async () => {
    gatewayApi.get.mockResolvedValue(
      gateway({
        console_address: "https://console-openshell-abc123.gw.localhost",
      }),
    );

    await expect(
      controlPlane.getGateway("gateway-1", context),
    ).resolves.toMatchObject({
      consoleUrl: "https://console-openshell-abc123.gw.localhost",
    });
  });

  it("maps gateway_version to the reconciled gateway version", async () => {
    gatewayApi.get.mockResolvedValue(
      gateway({ gateway_version: " v0.0.109-rh9a8f8 " }),
    );

    await expect(
      controlPlane.getGateway("gateway-1", context),
    ).resolves.toMatchObject({ gatewayVersion: "v0.0.109-rh9a8f8" });
  });

  it("leaves the console URL unavailable when console_address is absent", async () => {
    gatewayApi.get.mockResolvedValue(gateway({ console_address: "" }));

    const result = await controlPlane.getGateway("gateway-1", context);

    expect(result.consoleUrl).toBeUndefined();
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

  it("sorts gateways by the API creation timestamp", async () => {
    gatewayApi.list.mockResolvedValue(gatewayList([], 0, 1));

    await controlPlane.listGateways(
      { ...listRequest, page: 1, search: "", sortField: "created" },
      context,
    );

    expect(gatewayApi.list).toHaveBeenCalledWith(
      { orderBy: "created_at desc", page: 1, size: 20 },
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
        route: '{"enabled":true}',
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
    serviceAccountApi.create.mockRejectedValue(
      new SDKAPIError({
        code: "service_account_name_exists",
        href: "",
        id: "",
        kind: "Error",
        operation_id: "operation-1",
        reason: "raw provider detail",
        status_code: 409,
      }),
    );

    const failure = await controlPlane
      .createOpenShellGatewayServiceAccount(
        "gateway-1",
        {
          expiresAt: "2026-11-19T12:00:00Z",
          name: "deploy-bot",
          role: "openshell-user",
        },
        context,
      )
      .catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(GatewayOperationError);
    expect(failure).toMatchObject({
      code: "service-account-name-exists",
      kind: "conflict",
      operationId: "operation-1",
    });
    expect((failure as Error).message).not.toContain("raw provider detail");
  });

  it("does not expose unrecognized API error codes", async () => {
    gatewayApi.update.mockRejectedValue(
      new SDKAPIError({
        code: "provider_specific_conflict",
        href: "",
        id: "",
        kind: "Error",
        operation_id: "operation-2",
        reason: "raw provider detail",
        status_code: 409,
      }),
    );

    const failure = await controlPlane
      .renameGateway("gateway-1", "Existing gateway", context)
      .catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(GatewayOperationError);
    expect(failure).toMatchObject({
      code: undefined,
      kind: "conflict",
      operationId: "operation-2",
    });
    expect((failure as Error).message).not.toContain("raw provider detail");
  });
});
