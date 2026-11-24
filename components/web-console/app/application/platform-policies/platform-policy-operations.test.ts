import { describe, expect, it, vi } from "vitest";

import { createPlatformPolicyOperations } from "./platform-policy-operations";
import type { PlatformPolicyProbe } from "./platform-policy-probes";
import {
  PlatformPolicyOperationError,
  type PlatformPolicyControlPlane,
  type PlatformPolicySettings,
} from "./platform-policy-types";

const settings: PlatformPolicySettings = {
  defaults: { cpuRequest: "500m" },
  mode: "audit",
  schemaVersion: "v1",
};

function controlPlane(
  overrides: Partial<PlatformPolicyControlPlane> = {},
): PlatformPolicyControlPlane {
  return {
    createPolicy: vi.fn(),
    deletePolicy: vi.fn(() => Promise.resolve()),
    evaluatePolicy: vi.fn(() =>
      Promise.resolve({
        defaults: {},
        denialText: "Rejected by 1 policy",
        diagnostics: [{ line: 3, message: "permit", statement: 1 }],
        forbidsDefaults: false,
        rejectingStatements: [1],
        rendered: [],
        requestAllowed: false,
        valid: false,
      }),
    ),
    getPolicy: vi.fn(),
    getSettings: vi.fn(() => Promise.resolve(settings)),
    listFindings: vi.fn(() =>
      Promise.resolve({ items: [], page: 1, size: 20, total: 0 }),
    ),
    listPolicies: vi.fn(() => Promise.resolve([])),
    updatePolicy: vi.fn(),
    updateSettings: vi.fn(() =>
      Promise.resolve({ ...settings, mode: "enforce" as const }),
    ),
    ...overrides,
  };
}

function setup(overrides: Partial<PlatformPolicyControlPlane> = {}) {
  const published: PlatformPolicyProbe[] = [];
  const plane = controlPlane(overrides);
  const operations = createPlatformPolicyOperations({
    controlPlane: plane,
    probes: { publish: (probe) => published.push(probe) },
    runtime: {
      createCorrelationId: () => "correlation-1",
      createTraceId: () => "0123456789abcdef0123456789abcdef",
      now: () => "2026-11-23T00:00:00.000Z",
    },
  });
  const spies = plane as unknown as Readonly<
    Record<keyof PlatformPolicyControlPlane, ReturnType<typeof vi.fn>>
  >;
  return { operations, plane: spies, published };
}

describe("createPlatformPolicyOperations", () => {
  it("publishes one started and one terminal probe with the loaded fact", async () => {
    const { operations, plane, published } = setup();

    await operations.listPolicies();

    expect(plane.listPolicies).toHaveBeenCalledWith({
      correlationId: "correlation-1",
    });
    expect(published.map(({ name }) => name)).toEqual([
      "platform-policy.workflow.started",
      "platform-policy.dependency.attempted",
      "platform-policy.dependency.completed",
      "platform-policy.loaded",
      "platform-policy.workflow.completed",
    ]);
    expect(Object.isFrozen(published[0])).toBe(true);
  });

  // Verifies: WEB-PADM-10
  it("publishes the save-denied fact for a 409 without any secret payload", async () => {
    const secret = "do-not-log-this-cedar-or-message";
    const { operations, published } = setup({
      updatePolicy: vi.fn(() =>
        Promise.reject(
          new PlatformPolicyOperationError("conflicted", {
            operationId: "op-9",
          }),
        ),
      ),
    });

    await expect(
      operations.updatePolicy("policy-1", {
        cedar: secret,
        userMessage: secret,
      }),
    ).rejects.toMatchObject({ kind: "conflicted" });

    const denied = published.find(
      ({ name }) => name === "platform-policy.save.denied",
    );
    expect(denied?.fields).toEqual({
      action: "update",
      failureKind: "conflicted",
    });
    const terminal = published.at(-1);
    expect(terminal?.name).toBe("platform-policy.workflow.completed");
    expect(terminal?.fields).toMatchObject({ outcome: "conflicted" });
    expect(terminal?.context.operationId).toBe("op-9");
    expect(JSON.stringify(published)).not.toContain(secret);
  });

  it("publishes the saved fact and the mode-change confirmation", async () => {
    const { operations, published } = setup();

    operations.confirmModeChange("audit", "enforce");
    await operations.setGlobalMode("enforce");

    expect(published[0]?.name).toBe("platform-policy.mode-change.confirmed");
    expect(published[0]?.fields).toEqual({ from: "audit", to: "enforce" });
    expect(
      published.find(({ name }) => name === "platform-policy.saved")?.fields,
    ).toEqual({ action: "set-global-mode" });
  });

  it("publishes the validation outcome of the editor dry run", async () => {
    const { operations, published } = setup();

    await operations.evaluatePolicy({
      cedar: "permit (principal, action, resource);",
    });

    expect(
      published.find(({ name }) => name === "platform-policy.validated")
        ?.fields,
    ).toEqual({
      diagnosticCount: 1,
      forbidsDefaults: false,
      requestAllowed: false,
      valid: false,
    });
  });

  // Verifies: WEB-PADM-01
  it("resolves access from the API: 200 authorized, 403 denied", async () => {
    await expect(setup().operations.checkAccess()).resolves.toBe("authorized");
    const denied = setup({
      getSettings: vi.fn(() =>
        Promise.reject(new PlatformPolicyOperationError("denied")),
      ),
    });
    await expect(denied.operations.checkAccess()).resolves.toBe("denied");
    expect(denied.plane.getSettings).toHaveBeenCalledTimes(1);
  });

  it("propagates access-check failures other than denial", async () => {
    const { operations } = setup({
      getSettings: vi.fn(() =>
        Promise.reject(new PlatformPolicyOperationError("unavailable")),
      ),
    });
    await expect(operations.checkAccess()).rejects.toMatchObject({
      kind: "unavailable",
    });
  });

  it("reports cancellation as a cancelled outcome", async () => {
    const abort = Object.assign(new Error("aborted"), { name: "AbortError" });
    const { operations, published } = setup({
      deletePolicy: vi.fn(() => Promise.reject(abort)),
    });

    await expect(operations.deletePolicy("policy-1")).rejects.toBe(abort);

    expect(published.at(-1)?.fields).toMatchObject({
      failureKind: "cancelled",
      outcome: "cancelled",
    });
    expect(
      published.some(({ name }) => name === "platform-policy.save.denied"),
    ).toBe(false);
  });

  it("covers the remaining entry ports", async () => {
    const policy = { id: "policy-1" };
    const { operations, plane } = setup({
      createPolicy: vi.fn(() => Promise.resolve(policy as never)),
      getPolicy: vi.fn(() => Promise.resolve(policy as never)),
    });
    await operations.getPolicy("policy-1");
    await operations.getSettings();
    await operations.listFindings("policy-1", { page: 1, size: 20 });
    await operations.createPolicy({
      cedar: "",
      description: "",
      documentationUrl: "",
      name: "p",
      target: { all: true, exclude: [], gatewayIds: [] },
      userMessage: "",
    });
    expect(plane.listFindings).toHaveBeenCalledWith(
      "policy-1",
      { page: 1, size: 20 },
      { correlationId: "correlation-1" },
    );
  });
});
