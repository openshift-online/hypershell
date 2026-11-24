import { describe, expect, it } from "vitest";

import type { PlatformPolicy } from "../../application/platform-policies/platform-policy-types";
import {
  validCedar,
  validEvaluation,
} from "./fixtures/platform-policy-editor-fixtures";
import {
  cedarStatementHeadline,
  emptyPlatformPolicyFormValues,
  emptyPlatformPolicyTestValues,
  platformPolicyFormValues,
  platformPolicyPreviewRows,
  platformPolicyRejections,
  toPlatformPolicyDraft,
  toPlatformPolicyTestRequest,
} from "./platform-policy-form-model";

const policy: PlatformPolicy = {
  builtIn: false,
  cedar: "forbid (principal, action, resource);",
  effectiveMode: "audit",
  enforcementMode: "audit",
  findingsLast24h: { wouldAddDefaults: 0, wouldReject: 0 },
  gateways: [],
  id: "p1",
  name: "baseline",
  rendered: [],
  rollout: { applied: 0, pending: 0, unsupported: 0 },
  target: { all: true, exclude: [], gatewayIds: [] },
  userMessage: "message",
};

describe("platform policy form model", () => {
  it("maps targets between the form and the draft", () => {
    const base = {
      ...emptyPlatformPolicyFormValues,
      name: " n ",
      userMessage: " m ",
    };
    expect(toPlatformPolicyDraft(base)).toMatchObject({
      name: "n",
      target: { all: true, exclude: [], gatewayIds: [] },
      userMessage: "m",
    });
    expect(
      toPlatformPolicyDraft({
        ...base,
        targetGateways: ["g1"],
        targetScope: "all-except",
      }).target,
    ).toEqual({ all: true, exclude: ["g1"], gatewayIds: [] });
    expect(
      toPlatformPolicyDraft({
        ...base,
        targetGateways: ["g2"],
        targetScope: "selected",
      }).target,
    ).toEqual({ all: false, exclude: [], gatewayIds: ["g2"] });
    expect(platformPolicyFormValues(policy).targetScope).toBe("all");
    expect(
      platformPolicyFormValues({
        ...policy,
        description: "d",
        documentationUrl: "u",
        target: { all: true, exclude: ["g"], gatewayIds: [] },
      }),
    ).toMatchObject({
      description: "d",
      documentationUrl: "u",
      targetGateways: ["g"],
      targetScope: "all-except",
    });
    expect(
      platformPolicyFormValues({
        ...policy,
        target: { all: false, exclude: [], gatewayIds: ["x"] },
      }),
    ).toMatchObject({ targetGateways: ["x"], targetScope: "selected" });
  });

  // Verifies: WEB-PADM-08
  it("omits empty test fields so the fleet defaults apply", () => {
    expect(toPlatformPolicyTestRequest(emptyPlatformPolicyTestValues)).toEqual({
      errors: {},
      request: {},
    });
    expect(
      toPlatformPolicyTestRequest({
        cpuLimit: "bad",
        cpuRequest: "100m",
        gpuCount: "2",
        image: " quay.io/a/b ",
        memoryLimit: "4Gi",
        memoryRequest: "lots",
        roles: "developers, gpu-users",
      }),
    ).toEqual({
      errors: { cpuLimit: "invalid-cpu", memoryRequest: "invalid-memory" },
      request: {
        cpuRequest: "100m",
        gpuCount: 2,
        image: "quay.io/a/b",
        memoryLimit: "4Gi",
        roles: ["developers", "gpu-users"],
      },
    });
    expect(
      toPlatformPolicyTestRequest({
        ...emptyPlatformPolicyTestValues,
        gpuCount: "x",
      }).errors,
    ).toEqual({ gpuCount: "invalid-count" });
  });

  // Verifies: WEB-PADM-09
  it("projects rendered rules into the preview and the test rejections", () => {
    expect(
      platformPolicyPreviewRows(validEvaluation.rendered, validCedar),
    ).toEqual([
      {
        kind: "resource",
        maximum: "4",
        minimum: "250m",
        resource: "cpuRequest",
      },
      { kind: "resource", maximum: "16Gi", resource: "memoryRequest" },
      { kind: "additional", line: 17, statement: 2 },
    ]);
    const rejections = platformPolicyRejections(validEvaluation, validCedar);
    expect(rejections.map(({ line }) => line)).toEqual([2, 17]);
    expect(
      platformPolicyRejections(
        { ...validEvaluation, rejectingStatements: [9] },
        "",
      ),
    ).toEqual([{ statement: 9 }]);
    expect(
      platformPolicyPreviewRows(
        [{ bounds: [], cedar: "x", recognized: false, summary: [] }],
        "",
      ),
    ).toEqual([{ kind: "additional", statement: 0 }]);
  });

  it("names a statement by its first code line", () => {
    expect(
      cedarStatementHeadline({
        bounds: [],
        cedar: '// note\n@id("x")\nforbid (principal, action, resource)',
        recognized: false,
        summary: [],
      }),
    ).toBe("forbid (principal, action, resource)");
  });
});
