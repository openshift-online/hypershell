import { describe, expect, it, vi } from "vitest";

import type { GatewayProfileProbe } from "./gateway-profile-probes";
import { gatewayProfileProbeCatalog } from "./gateway-profile-probes";
import { createGatewayProfileOperations } from "./gateway-profile-operations";
import {
  GatewayProfileOperationError,
  type GatewayProfileControlPlane,
  type GatewayProfileRecord,
} from "./gateway-profile-types";

const gatewayProfile: GatewayProfileRecord = {
  cpuLimitTotal: "8",
  id: "profile-1",
  name: "Small profile",
  podCount: 10,
};
const listRequest = {
  page: 1,
  search: "",
  size: 20,
  sortDirection: "asc",
  sortField: "name",
} as const;

function setup() {
  const received: GatewayProfileProbe[] = [];
  let correlation = 0;
  const listGatewayProfiles = vi.fn().mockResolvedValue({
    items: [gatewayProfile],
    page: 1,
    size: 20,
    total: 1,
  });
  const removeGatewayProfile = vi.fn().mockResolvedValue(undefined);
  const controlPlane: GatewayProfileControlPlane = {
    createGatewayProfile: vi.fn().mockResolvedValue(gatewayProfile),
    getGatewayProfile: vi.fn().mockResolvedValue(gatewayProfile),
    listGatewayProfiles,
    removeGatewayProfile,
  };
  const operations = createGatewayProfileOperations({
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
    controlPlane,
    listGatewayProfiles,
    operations,
    received,
    removeGatewayProfile,
  };
}

describe("gateway profile application operations", () => {
  it.each([
    [
      "create",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.createGatewayProfile({ name: "Small profile" }),
    ],
    [
      "get",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.getGatewayProfile("profile-1"),
    ],
    [
      "list",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.listGatewayProfiles(listRequest),
    ],
    [
      "remove",
      (operations: ReturnType<typeof setup>["operations"]) =>
        operations.removeGatewayProfile("profile-1"),
    ],
  ] as const)(
    "publishes one successful %s workflow and dependency outcome",
    async (action, invoke) => {
      const { operations, received } = setup();

      await invoke(operations);

      expect(received.map(({ name }) => name)).toEqual([
        "gatewayProfile.workflow.started",
        "gatewayProfile.dependency.attempted",
        "gatewayProfile.dependency.completed",
        "gatewayProfile.workflow.completed",
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
    const { listGatewayProfiles, operations, received } = setup();
    const abortController = new AbortController();

    await operations.listGatewayProfiles(listRequest, abortController.signal);

    expect(listGatewayProfiles).toHaveBeenCalledWith(listRequest, {
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
    const { operations, received, removeGatewayProfile } = setup();
    const failure = new GatewayProfileOperationError("conflict", {
      operationId: "operation-1",
    });
    removeGatewayProfile.mockRejectedValue(failure);

    await expect(operations.removeGatewayProfile("profile-1")).rejects.toBe(
      failure,
    );

    expect(received.slice(-2).map(({ fields }) => fields)).toEqual([
      { action: "remove", failureKind: "conflict", outcome: "conflicted" },
      { action: "remove", failureKind: "conflict", outcome: "conflicted" },
    ]);
    expect(
      received.slice(-2).map(({ context }) => context.operationId),
    ).toEqual(["operation-1", "operation-1"]);
  });

  it("maps a denied failure to a denied outcome", async () => {
    const { operations, received, removeGatewayProfile } = setup();
    removeGatewayProfile.mockRejectedValue(
      new GatewayProfileOperationError("denied"),
    );

    await expect(operations.removeGatewayProfile("profile-1")).rejects.toThrow(
      GatewayProfileOperationError,
    );

    expect(received.slice(-1).map(({ fields }) => fields)).toEqual([
      { action: "remove", failureKind: "denied", outcome: "denied" },
    ]);
  });

  it("maps an unknown failure to a failed outcome", async () => {
    const { operations, received, removeGatewayProfile } = setup();
    removeGatewayProfile.mockRejectedValue(new Error("boom"));

    await expect(operations.removeGatewayProfile("profile-1")).rejects.toThrow(
      "boom",
    );

    expect(received.slice(-1).map(({ fields }) => fields)).toEqual([
      { action: "remove", failureKind: "unknown", outcome: "failed" },
    ]);
  });

  it("publishes cancellation without turning it into an application error", async () => {
    const { listGatewayProfiles, operations, received } = setup();
    const cancellation = Object.assign(new Error("cancelled"), {
      name: "AbortError",
    });
    listGatewayProfiles.mockRejectedValue(cancellation);

    await expect(operations.listGatewayProfiles(listRequest)).rejects.toBe(
      cancellation,
    );

    expect(received.slice(-2).map(({ fields }) => fields)).toEqual([
      { action: "list", failureKind: "cancelled", outcome: "cancelled" },
      { action: "list", failureKind: "cancelled", outcome: "cancelled" },
    ]);
  });

  it("sets a descriptive message on the operation error", () => {
    expect(new GatewayProfileOperationError("not-found").message).toBe(
      "Gateway profile operation failed: not-found",
    );
  });

  it("keeps the probe catalog synchronized with the closed schema names", () => {
    expect(gatewayProfileProbeCatalog.map(({ name }) => name)).toEqual([
      "gatewayProfile.workflow.started",
      "gatewayProfile.workflow.completed",
      "gatewayProfile.dependency.attempted",
      "gatewayProfile.dependency.completed",
    ]);
  });

  it("declares trace as an allowed consumer for every gateway profile probe", () => {
    expect(
      gatewayProfileProbeCatalog.every(({ allowedConsumers }) =>
        allowedConsumers.includes("trace"),
      ),
    ).toBe(true);
  });
});
