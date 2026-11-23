import {
  QueryClient,
  QueryClientProvider,
  type QueryClient as QueryClientType,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  PlatformPolicyOperationError,
  type PlatformPolicy,
  type PlatformPolicyOperations,
} from "../../application/platform-policies/platform-policy-types";
import {
  editorFormValues,
  validCedar,
  validEvaluation,
} from "./fixtures/platform-policy-editor-fixtures";
import {
  toPlatformPolicySaveFailure,
  usePlatformPolicyEditor,
} from "./use-platform-policy-editor";

const savedPolicy: PlatformPolicy = {
  builtIn: false,
  cedar: validCedar,
  effectiveMode: "audit",
  enforcementMode: "audit",
  findingsLast24h: { wouldAddDefaults: 0, wouldReject: 0 },
  gateways: [],
  id: "policy-1",
  name: "sandbox-resource-baseline",
  rendered: [],
  rollout: { applied: 0, pending: 0, unsupported: 0 },
  target: { all: true, exclude: ["ml-training-gateway"], gatewayIds: [] },
  userMessage: "message",
};

function operations(
  overrides: Partial<PlatformPolicyOperations> = {},
): PlatformPolicyOperations {
  return {
    checkAccess: vi.fn(),
    confirmModeChange: vi.fn(),
    createPolicy: vi.fn().mockResolvedValue(savedPolicy),
    deletePolicy: vi.fn(),
    evaluatePolicy: vi.fn().mockResolvedValue(validEvaluation),
    getPolicy: vi.fn().mockResolvedValue(savedPolicy),
    getSettings: vi.fn().mockResolvedValue({
      defaults: {},
      mode: "audit",
      schemaVersion: "v1",
    }),
    listFindings: vi.fn(),
    listPolicies: vi.fn(),
    setGlobalMode: vi.fn(),
    updatePolicy: vi.fn().mockResolvedValue(savedPolicy),
    ...overrides,
  };
}

function wrapperFor(client: QueryClientType) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  };
}

function newClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: 3 }, queries: { retry: 3 } },
  });
}

describe("usePlatformPolicyEditor", () => {
  // Verifies: WEB-PADM-05, WEB-PADM-08
  it("validates the draft through the dry run after a pause", async () => {
    const evaluatePolicy = vi.fn().mockResolvedValue(validEvaluation);
    const ops = operations({ evaluatePolicy });
    const { result } = renderHook(
      () => usePlatformPolicyEditor({ onSaved: vi.fn(), operations: ops }),
      { wrapper: wrapperFor(newClient()) },
    );
    expect(result.current.view.evaluation).toBeUndefined();
    act(() => {
      result.current.view.onValuesChange(editorFormValues(validCedar));
      result.current.view.onTestValuesChange({
        ...result.current.view.testValues,
        cpuRequest: "100m",
      });
    });
    expect(result.current.view.isEvaluating).toBe(true);
    await waitFor(() => {
      expect(result.current.view.evaluation).toEqual(validEvaluation);
    });
    expect(evaluatePolicy).toHaveBeenCalledWith(
      expect.objectContaining({
        cedar: validCedar,
        name: "sandbox-resource-baseline",
        request: { cpuRequest: "100m" },
      }),
      expect.anything(),
    );
    expect(result.current.view.schemaVersion).toBe("v1");
  });

  // Verifies: WEB-PADM-11
  it("invalidates policy and effective-policy queries after a save", async () => {
    const client = newClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const onSaved = vi.fn();
    const createPolicy = vi.fn().mockResolvedValue(savedPolicy);
    const ops = operations({ createPolicy });
    const { result } = renderHook(
      () => usePlatformPolicyEditor({ onSaved, operations: ops }),
      { wrapper: wrapperFor(client) },
    );
    act(() => {
      result.current.view.onValuesChange(editorFormValues(validCedar));
    });
    act(() => {
      result.current.view.onSave();
    });
    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledWith(savedPolicy);
    });
    expect(createPolicy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "sandbox-resource-baseline",
        target: { all: true, exclude: ["ml-training-gateway"], gatewayIds: [] },
      }),
    );
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["platform-policies"],
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["gateways", "effective-policies"],
    });
  });

  // Verifies: WEB-PADM-01.02, WEB-PADM-10
  it("surfaces a denied write once without retrying it", async () => {
    const createPolicy = vi
      .fn()
      .mockRejectedValue(new PlatformPolicyOperationError("denied"));
    const { result } = renderHook(
      () =>
        usePlatformPolicyEditor({
          onSaved: vi.fn(),
          operations: operations({ createPolicy }),
        }),
      { wrapper: wrapperFor(newClient()) },
    );
    act(() => {
      result.current.view.onValuesChange(editorFormValues(validCedar));
    });
    act(() => {
      result.current.view.onSave();
    });
    await waitFor(() => {
      expect(result.current.view.saveFailure).toEqual({ kind: "denied" });
    });
    expect(createPolicy).toHaveBeenCalledTimes(1);
    act(() => {
      result.current.view.onValuesChange(editorFormValues(""));
    });
    await waitFor(() => {
      expect(result.current.view.saveFailure).toBeUndefined();
    });
  });

  // Verifies: WEB-PADM-10
  it("maps a conflicted update and loads the policy being edited", async () => {
    const updatePolicy = vi
      .fn()
      .mockRejectedValue(new PlatformPolicyOperationError("conflicted"));
    const { result } = renderHook(
      () =>
        usePlatformPolicyEditor({
          onSaved: vi.fn(),
          operations: operations({ updatePolicy }),
          policyId: "policy-1",
        }),
      { wrapper: wrapperFor(newClient()) },
    );
    expect(result.current.load?.isPending).toBe(true);
    await waitFor(() => {
      expect(result.current.view.values.targetScope).toBe("all-except");
    });
    expect(result.current.view.isEditing).toBe(true);
    act(() => {
      result.current.view.onSave();
    });
    await waitFor(() => {
      expect(result.current.view.saveFailure).toEqual({ kind: "conflicted" });
    });
    expect(updatePolicy).toHaveBeenCalledWith(
      "policy-1",
      expect.not.objectContaining({ name: expect.anything() as unknown }),
    );
  });

  it("maps unknown and invalid failures", () => {
    expect(toPlatformPolicySaveFailure(new Error("x"))).toEqual({
      kind: "unknown",
    });
    expect(
      toPlatformPolicySaveFailure(
        new PlatformPolicyOperationError("invalid", { reason: "bad" }),
      ),
    ).toEqual({ kind: "invalid", reason: "bad" });
  });
});
