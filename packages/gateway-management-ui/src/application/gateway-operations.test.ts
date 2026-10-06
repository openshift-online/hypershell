import { describe, expect, it, vi } from "vitest";

import type { GatewayProbe } from "./gateway-probes";
import { gatewayProbeCatalog } from "./gateway-probes";
import { createGatewayOperations } from "./gateway-operations";
import {
  GatewayOperationError,
  type GatewayControlPlane,
  type GatewayRecord,
} from "./gateway-types";

const gateway: GatewayRecord = {
  clusterId: "",
  id: "gateway-1",
  name: "Team gateway",
  namespace: "openshell",
  releaseId: "",
};
const listRequest = {
  page: 1,
  search: "",
  size: 20,
  sortDirection: "asc",
  sortField: "name",
} as const;

function setup() {
  const received: GatewayProbe[] = [];
  let correlation = 0;
  const listGateways = vi.fn().mockResolvedValue({
    items: [gateway],
    page: 1,
    size: 20,
    total: 1,
  });
  const renameGateway = vi.fn().mockResolvedValue(gateway);
  const grant = {
    grantedAt: "2026-08-06T18:00:00.000Z",
    isCreator: false,
    role: "user" as const,
    roleBindingId: "rb-1",
    userId: "user-1",
    username: "dana",
  };
  const changeGatewayAccessRole = vi.fn().mockResolvedValue(grant);
  const grantGatewayAccess = vi.fn().mockResolvedValue(grant);
  const controlPlane: GatewayControlPlane = {
    changeGatewayAccessRole,
    grantGatewayAccess,
    createOpenShellGatewayServiceAccount: vi.fn(),
    deleteOpenShellGatewayServiceAccount: vi.fn(),
    findGatewayPlacements: vi.fn().mockResolvedValue({
      hasMore: false,
      items: [],
    }),
    getGateway: vi.fn().mockResolvedValue(gateway),
    getGatewayPlacement: vi.fn().mockResolvedValue({
      id: "cluster-east",
      name: "Cluster East",
      provider: "AWS",
    }),
    getGatewayPlacements: vi.fn().mockResolvedValue([
      {
        id: "cluster-east",
        name: "Cluster East",
        provider: "AWS",
      },
    ]),
    getOpenShellGatewayServiceAccount: vi.fn(),
    listGatewayAccess: vi.fn().mockResolvedValue({
      capabilities: { canManageAccess: true, canManageOwners: true },
      items: [],
      page: 1,
      size: 20,
      total: 0,
    }),
    listGateways,
    listOpenShellGatewayServiceAccounts: vi.fn(),
    provisionGateway: vi.fn().mockResolvedValue(gateway),
    removeGateway: vi.fn().mockResolvedValue(undefined),
    renameGateway,
    revokeGatewayAccess: vi.fn().mockResolvedValue(undefined),
    revokeOpenShellGatewayServiceAccount: vi.fn(),
    searchGatewayDirectory: vi.fn().mockResolvedValue([]),
  };
  const operations = createGatewayOperations({
    controlPlane,
    probes: {
      publish(value) {
        received.push(value);
      },
    },
    runtime: {
      createCorrelationId() {
        correlation += 1;
        return `correlation-${String(correlation)}`;
      },
      createTraceId() {
        return `trace-${String(correlation)}`;
      },
      now() {
        return "2026-08-06T18:00:00.000Z";
      },
    },
  });

  return {
    changeGatewayAccessRole,
    controlPlane,
    grantGatewayAccess,
    listGateways,
    operations,
    received,
    renameGateway,
  };
}

describe("gateway application operations", () => {
  it.each([
    [
      "create-service-account",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.createOpenShellGatewayServiceAccount("gateway-1", {
          expiresAt: "2026-11-19T12:00:00Z",
          name: "deploy-bot",
          role: "openshell-user",
        }),
    ],
    [
      "delete-service-account",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.deleteOpenShellGatewayServiceAccount(
          "gateway-1",
          "account-1",
        ),
    ],
    [
      "find-placements",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.findGatewayPlacements(" east "),
    ],
    [
      "get",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.getGateway("gateway-1"),
    ],
    [
      "get-placement",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.getGatewayPlacement("cluster-east"),
    ],
    [
      "get-service-account",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.getOpenShellGatewayServiceAccount("gateway-1", "account-1"),
    ],
    [
      "get-placements",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.getGatewayPlacements(["cluster-east"]),
    ],
    [
      "list",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.listGateways(listRequest),
    ],
    [
      "list-service-accounts",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.listOpenShellGatewayServiceAccounts("gateway-1", {
          order: "desc",
          page: 1,
          search: "",
          size: 20,
          sort: "created_at",
        }),
    ],
    [
      "provision",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.provisionGateway({
          clusterId: "",
          name: "Team gateway",
        }),
    ],
    [
      "remove",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.removeGateway("gateway-1"),
    ],
    [
      "rename",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.renameGateway("gateway-1", "Renamed gateway"),
    ],
    [
      "revoke-service-account",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.revokeOpenShellGatewayServiceAccount(
          "gateway-1",
          "account-1",
        ),
    ],
    [
      "list-access",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.listGatewayAccess("gateway-1", {
          order: "desc",
          page: 1,
          search: "",
          size: 20,
          sort: "granted_at",
        }),
    ],
    [
      "grant-access",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.grantGatewayAccess("gateway-1", {
          role: "user",
          username: "dana",
        }),
    ],
    [
      "change-access-role",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.changeGatewayAccessRole("gateway-1", "user-1", "admin"),
    ],
    [
      "revoke-access",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.revokeGatewayAccess("gateway-1", "user-1"),
    ],
    [
      "search-directory",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.searchGatewayDirectory("gateway-1", "da"),
    ],
  ] as const)(
    "publishes one successful %s workflow and dependency outcome",
    async (action, invoke) => {
      const { operations, received } = setup();

      await invoke(operations);

      expect(received.map(({ name }) => name)).toEqual([
        "gateway.workflow.started",
        "gateway.dependency.attempted",
        "gateway.dependency.completed",
        "gateway.workflow.completed",
      ]);
      expect(received.map(({ fields }) => fields)).toEqual([
        { action, failureKind: null, outcome: "started" },
        { action, failureKind: null, outcome: "started" },
        { action, failureKind: null, outcome: "succeeded" },
        { action, failureKind: null, outcome: "succeeded" },
      ]);
      expect(received.every(Object.isFrozen)).toBe(true);
    },
  );

  it("passes one correlation identifier through the driven port", async () => {
    const { listGateways, operations, received } = setup();
    const abortController = new AbortController();

    await operations.listGateways(listRequest, abortController.signal);

    expect(listGateways).toHaveBeenCalledWith(listRequest, {
      correlationId: "correlation-1",
      signal: abortController.signal,
    });
    expect(
      received.every(
        ({ context }) => context.correlationId === "correlation-1",
      ),
    ).toBe(true);
    expect(received.every(({ context }) => context.traceId === "trace-1")).toBe(
      true,
    );
  });

  it("publishes a conflicted terminal outcome and preserves the typed failure", async () => {
    const { operations, received, renameGateway } = setup();
    const failure = new GatewayOperationError("conflict", {
      operationId: "operation-1",
    });
    renameGateway.mockRejectedValue(failure);

    await expect(
      operations.renameGateway("gateway-1", "Existing gateway"),
    ).rejects.toBe(failure);

    expect(received.slice(-2).map(({ fields }) => fields)).toEqual([
      { action: "rename", failureKind: "conflict", outcome: "conflicted" },
      { action: "rename", failureKind: "conflict", outcome: "conflicted" },
    ]);
    expect(
      received.slice(-2).map(({ context }) => context.operationId),
    ).toEqual(["operation-1", "operation-1"]);
  });

  it("publishes a conflicted outcome when a last-owner change is rejected", async () => {
    const { changeGatewayAccessRole, operations, received } = setup();
    const failure = new GatewayOperationError("conflict", {
      operationId: "operation-9",
    });
    changeGatewayAccessRole.mockRejectedValue(failure);

    await expect(
      operations.changeGatewayAccessRole("gateway-1", "user-1", "user"),
    ).rejects.toBe(failure);

    expect(received.slice(-2).map(({ fields }) => fields)).toEqual([
      {
        action: "change-access-role",
        failureKind: "conflict",
        outcome: "conflicted",
      },
      {
        action: "change-access-role",
        failureKind: "conflict",
        outcome: "conflicted",
      },
    ]);
  });

  it("publishes a denied outcome when a grant is forbidden", async () => {
    const { grantGatewayAccess, operations, received } = setup();
    const failure = new GatewayOperationError("denied");
    grantGatewayAccess.mockRejectedValue(failure);

    await expect(
      operations.grantGatewayAccess("gateway-1", {
        role: "owner",
        username: "erin",
      }),
    ).rejects.toBe(failure);

    expect(received.slice(-1).map(({ fields }) => fields)).toEqual([
      { action: "grant-access", failureKind: "denied", outcome: "denied" },
    ]);
  });

  it("publishes cancellation without turning it into an application error", async () => {
    const { listGateways, operations, received } = setup();
    const cancellation = Object.assign(new Error("cancelled"), {
      name: "AbortError",
    });
    listGateways.mockRejectedValue(cancellation);

    await expect(operations.listGateways(listRequest)).rejects.toBe(
      cancellation,
    );

    expect(received.slice(-2).map(({ fields }) => fields)).toEqual([
      { action: "list", failureKind: "cancelled", outcome: "cancelled" },
      { action: "list", failureKind: "cancelled", outcome: "cancelled" },
    ]);
  });

  it("keeps the probe catalog synchronized with the closed schema names", () => {
    expect(gatewayProbeCatalog.map(({ name }) => name)).toEqual([
      "gateway.workflow.started",
      "gateway.workflow.completed",
      "gateway.dependency.attempted",
      "gateway.dependency.completed",
    ]);
  });

  it("declares trace as an allowed consumer for every gateway probe", () => {
    expect(
      gatewayProbeCatalog.every(({ allowedConsumers }) =>
        allowedConsumers.includes("trace"),
      ),
    ).toBe(true);
  });
});
