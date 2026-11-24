import { SDKAPIError } from "@openshift-online/hypershell-sdk";
import { describe, expect, it, vi } from "vitest";

import {
  createPlatformPolicyControlPlaneAdapter,
  platformPolicyFailureKindForStatus,
  toPlatformPolicy,
} from "./platform-policies";
import type { PolicyTransport } from "./policy-transport";

const context = { correlationId: "correlation-1" };

function apiError(status: number, reason = "failed") {
  return new SDKAPIError({
    code: "error",
    href: "",
    id: "",
    kind: "Error",
    operation_id: "op-1",
    reason,
    status_code: status,
  });
}

function transport(responses: unknown[]): {
  calls: Parameters<PolicyTransport["request"]>[];
  factory: (correlationId: string) => PolicyTransport;
} {
  const calls: Parameters<PolicyTransport["request"]>[] = [];
  return {
    calls,
    factory: () => ({
      request: (...args) => {
        calls.push(args);
        const next = responses.shift();
        return next instanceof Error
          ? Promise.reject(next)
          : Promise.resolve(next);
      },
    }),
  };
}

const apiPolicy = {
  built_in: false,
  cedar: "forbid (principal, action, resource) when { context.gpuCount > 1 };",
  effective_mode: "audit",
  enforcement_mode: "audit",
  id: "policy-1",
  name: "gpu",
  rendered: [
    {
      bounds: [
        {
          display: "1",
          field: "gpuCount",
          label: "GPU count",
          op: ">",
          text: "GPU count: at most 1",
          value: 1,
        },
      ],
      cedar: "forbid ...",
      recognized: true,
      summary: ["GPU count: at most 1"],
    },
  ],
  status: {
    findings_last_24h: { would_add_defaults: 4, would_reject: 3 },
    gateways: [
      {
        condition: "Unsupported",
        effective_mode: "audit",
        gateway_id: "gw-1",
        message: "VM driver",
        would_reject_24h: 3,
      },
    ],
    rollout: { applied: 0, pending: 0, unsupported: 1 },
  },
  target: { all: true, exclude: ["gw-2"] },
  user_message: "Use one GPU.",
};

describe("platform policy API adapter", () => {
  // Verifies: WEB-PADM-10
  it("maps transport failures to typed operation errors", () => {
    expect(platformPolicyFailureKindForStatus(403)).toBe("denied");
    expect(platformPolicyFailureKindForStatus(401)).toBe("denied");
    expect(platformPolicyFailureKindForStatus(409)).toBe("conflicted");
    expect(platformPolicyFailureKindForStatus(400)).toBe("invalid");
    expect(platformPolicyFailureKindForStatus(404)).toBe("not-found");
    expect(platformPolicyFailureKindForStatus(503)).toBe("unavailable");
    expect(platformPolicyFailureKindForStatus(418)).toBe("unknown");
  });

  // Verifies: WEB-PADM-10
  it("raises a conflicted error for a 409 save and a denied error for a 403", async () => {
    const { factory } = transport([apiError(409), apiError(403)]);
    const adapter = createPlatformPolicyControlPlaneAdapter(factory);

    await expect(
      adapter.updatePolicy("policy-1", { enforcementMode: "enforce" }, context),
    ).rejects.toMatchObject({ kind: "conflicted", operationId: "op-1" });
    await expect(
      adapter.updateSettings("enforce", context),
    ).rejects.toMatchObject({ kind: "denied" });
  });

  it("keeps the validation reason only for invalid writes", async () => {
    const { factory } = transport([apiError(400, "permit is not allowed")]);
    const adapter = createPlatformPolicyControlPlaneAdapter(factory);
    await expect(
      adapter.createPolicy(
        {
          cedar: "permit (principal, action, resource);",
          description: "",
          documentationUrl: "",
          name: "p",
          target: { all: false, exclude: [], gatewayIds: ["gw-1"] },
          userMessage: "m",
        },
        context,
      ),
    ).rejects.toMatchObject({
      kind: "invalid",
      reason: "permit is not allowed",
    });
  });

  it("maps a policy with rollout, findings, and target", () => {
    const policy = toPlatformPolicy(apiPolicy);
    expect(policy.target).toEqual({
      all: true,
      exclude: ["gw-2"],
      gatewayIds: [],
    });
    expect(policy.gateways[0]).toMatchObject({
      condition: "unsupported",
      gatewayId: "gw-1",
      message: "VM driver",
      wouldReject24h: 3,
    });
    expect(policy.findingsLast24h).toEqual({
      wouldAddDefaults: 4,
      wouldReject: 3,
    });
    expect(policy.rendered[0]?.bounds[0]?.field).toBe("gpuCount");
  });

  it("pages through the policy list and resolves gateway names", async () => {
    const { calls, factory } = transport([{ items: [apiPolicy], total: 1 }]);
    const list = vi.fn(() =>
      Promise.resolve({ items: [{ id: "gw-1", name: "edge-lab" }] }),
    );
    const adapter = createPlatformPolicyControlPlaneAdapter(factory, () => ({
      gateways: { list } as never,
    }));

    const policies = await adapter.listPolicies(context);

    expect(calls[0]?.[0]).toBe("GET");
    expect(calls[0]?.[1]).toContain("page=1");
    expect(policies[0]?.gateways[0]?.gatewayName).toBe("edge-lab");
    expect(list).toHaveBeenCalledWith(
      expect.objectContaining({ search: "id in ('gw-1')" }),
      expect.anything(),
    );
  });

  it("falls back to gateway ids when the caller cannot read gateways", async () => {
    const { factory } = transport([apiPolicy]);
    const adapter = createPlatformPolicyControlPlaneAdapter(factory, () => ({
      gateways: { list: () => Promise.reject(apiError(403)) },
    }));

    const policy = await adapter.getPolicy("policy-1", context);

    expect(policy.gateways[0]?.gatewayName).toBeUndefined();
  });

  it("sends settings, findings, delete, and evaluate requests", async () => {
    const { calls, factory } = transport([
      {
        defaults: { cpu_request: "500m" },
        mode: "enforce",
        schema_version: "v1",
      },
      { items: [{ id: "f1", outcome: "would_add_defaults" }], total: 1 },
      undefined,
      {
        denial_text: "Rejected",
        diagnostics: [
          { line: 2, message: "bad", statement: 1, suggestion: "fix" },
        ],
        forbids_defaults: true,
        rejecting_statements: [1],
        rendered: [],
        sample_allowed: false,
        valid: true,
      },
    ]);
    const adapter = createPlatformPolicyControlPlaneAdapter(factory);

    const settings = await adapter.getSettings(context);
    const findings = await adapter.listFindings(
      "policy-1",
      { gatewayId: "gw-1", outcome: "would-reject", page: 2, size: 10 },
      context,
    );
    await adapter.deletePolicy("policy-1", context);
    const evaluation = await adapter.evaluatePolicy(
      {
        cedar: "forbid",
        name: "p",
        request: { cpuRequest: " 8 ", gpuCount: 2, roles: ["dev"] },
      },
      context,
    );

    expect(settings).toMatchObject({
      defaults: { cpuRequest: "500m" },
      mode: "enforce",
    });
    expect(calls[1]?.[1]).toContain("gateway_id=gw-1");
    expect(calls[1]?.[1]).toContain("outcome=would_reject");
    expect(findings.items[0]?.outcome).toBe("would-add-defaults");
    expect(calls[2]?.[0]).toBe("DELETE");
    expect(calls[3]?.[2]?.body).toMatchObject({
      sample: { cpu_request: "8", gpu_count: 2, roles: ["dev"] },
    });
    expect(evaluation).toMatchObject({
      forbidsDefaults: true,
      requestAllowed: false,
      valid: true,
    });
    expect(evaluation.diagnostics[0]?.suggestion).toBe("fix");
  });
});
