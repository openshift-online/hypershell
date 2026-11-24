import { describe, expect, it } from "vitest";

import {
  toFinding,
  toGatewayEffectivePolicies,
  toPolicyModeReason,
  toRenderedRules,
  toRolloutCondition,
} from "./policy-mapping";

describe("policy transport mapping", () => {
  // Verifies: WEB-GLIS-26
  it("maps effective policies and never carries targets", () => {
    const result = toGatewayEffectivePolicies({
      defaults: {
        cpu_limit: "2",
        cpu_request: "500m",
        memory_limit: "4Gi",
        memory_request: "1Gi",
      },
      findings: [
        {
          bundle_revision: "r7",
          gateway_id: "g1",
          id: "f1",
          occurred_at: "2026-11-20T09:12:00Z",
          operation: "CreateSandbox",
          outcome: "would_add_defaults",
          policy_names: ["baseline"],
          principal_subject: "ada",
          request_summary: { cpuRequestMillis: 100, note: "x" },
        },
      ],
      findings_last_24h: { would_add_defaults: 3, would_reject: 2 },
      gateway_id: "g1",
      global_mode: "audit",
      policies: [
        {
          built_in: true,
          cedar: "forbid (principal, action, resource);",
          condition: "Unsupported",
          documentation_url: "https://docs.example.com",
          effective_mode: "audit",
          effective_mode_reason: "audit",
          enforcement_mode: "audit",
          id: "p1",
          name: "baseline",
          rendered: [
            {
              bounds: [
                {
                  display: "4",
                  field: "cpuRequestMillis",
                  label: "CPU request",
                  op: ">",
                  text: "CPU request: at most 4",
                  value: 4000,
                },
              ],
              cedar: "forbid",
              recognized: true,
              summary: ["CPU request: at most 4"],
            },
          ],
          revision: "r7",
          target: { all: false, gateway_ids: ["g2"] },
          updated_at: "2026-11-18T00:00:00Z",
          user_message: "Stay in limits.",
        },
      ],
      schema_version: "v1",
    });

    expect(result.defaults).toEqual({
      cpuLimit: "2",
      cpuRequest: "500m",
      memoryLimit: "4Gi",
      memoryRequest: "1Gi",
    });
    expect(result.findingsLast24h).toEqual({
      wouldAddDefaults: 3,
      wouldReject: 2,
    });
    expect(result.findings[0]).toEqual({
      bundleRevision: "r7",
      gatewayId: "g1",
      id: "f1",
      occurredAt: "2026-11-20T09:12:00Z",
      operation: "CreateSandbox",
      outcome: "would-add-defaults",
      policyNames: ["baseline"],
      principalSubject: "ada",
      requestSummary: { cpuRequestMillis: 100 },
    });
    expect(result.policies[0]).toMatchObject({
      builtIn: true,
      condition: "unsupported",
      documentationUrl: "https://docs.example.com",
      effectiveModeReason: "audit",
      revision: "r7",
      updatedAt: "2026-11-18T00:00:00Z",
      userMessage: "Stay in limits.",
    });
    expect(result.policies[0]?.rendered[0]?.bounds[0]?.value).toBe(4000);
    expect(JSON.stringify(result)).not.toContain("g2");
  });

  it("degrades malformed records to explicit empty values", () => {
    const result = toGatewayEffectivePolicies("not an object");
    expect(result).toEqual({
      defaults: {},
      findings: [],
      findingsLast24h: { wouldAddDefaults: 0, wouldReject: 0 },
      gatewayId: "",
      globalMode: "audit",
      policies: [],
      schemaVersion: "",
    });
    expect(toFinding(null).outcome).toBe("would-reject");
    expect(toRenderedRules([null])).toEqual([
      { bounds: [], cedar: "", recognized: false, summary: [] },
    ]);
    expect(toPolicyModeReason("policy_enforced")).toBe("policy-enforced");
    expect(toRolloutCondition("Applied")).toBe("applied");
    expect(toRolloutCondition(undefined)).toBe("pending");
  });
});
