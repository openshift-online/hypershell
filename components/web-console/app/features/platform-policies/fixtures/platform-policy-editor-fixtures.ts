import type { PolicyFleetDefaults } from "@openshift-online/hypershell-gateway-management-ui";

import type { PlatformPolicyEvaluation } from "../../../application/platform-policies/platform-policy-types";
import type {
  PlatformPolicyFormValues,
  PlatformPolicyTestValues,
} from "../platform-policy-form-model";

/*
 * Deterministic Storybook and test fixtures for the platform policy editor.
 * They mirror specs/web-console/mockups/admin/platform-policy-form.tsx, with
 * every statement scoped to CreateSandbox (the only action the v1 schema
 * models).
 */

export const editorFleetDefaults: PolicyFleetDefaults = {
  cpuLimit: "2",
  cpuRequest: "500m",
  memoryLimit: "4Gi",
  memoryRequest: "1Gi",
};

export const validCedar = `// CPU and memory requests must stay within the shared-cluster plan.
forbid (
  principal,
  action == OpenShell::Action::"CreateSandbox",
  resource
)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };

forbid (
  principal,
  action == OpenShell::Action::"CreateSandbox",
  resource
)
when { context.memoryRequestBytes > 17179869184 };

// Only members of gpu-users can request more than 1 GPU.
forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.gpuCount > 1 }
unless { principal.roles.contains("gpu-users") };`;

export const invalidCedar = `// CPU and memory requests must stay within the shared-cluster plan.
forbid (
  principal,
  action == OpenShell::Action::"CreateSandbox",
  resource
)
when { context.cpuRequest > 4 };

permit (principal, action, resource)
when { principal.roles.contains("platform-admins") };`;

export function editorFormValues(cedar: string): PlatformPolicyFormValues {
  return {
    cedar,
    description: "Shared-cluster capacity plan for sandboxes.",
    documentationUrl: "https://docs.example.com/hypershell/sandbox-resources",
    name: "sandbox-resource-baseline",
    targetGateways: ["ml-training-gateway"],
    targetScope: "all-except",
    userMessage:
      "Sandboxes on shared clusters must set CPU and memory within platform limits. Ask #hypershell-help for an exception.",
  };
}

export const editorTestValues: PlatformPolicyTestValues = {
  cpuLimit: "",
  cpuRequest: "100m",
  gpuCount: "2",
  image: "",
  memoryLimit: "",
  memoryRequest: "",
  roles: "developers",
};

const cpuRequestRule = {
  bounds: [
    {
      display: "250m",
      field: "cpuRequestMillis",
      label: "CPU request",
      op: "<",
      text: "CPU request: at least 250m",
      value: 250,
    },
    {
      display: "4",
      field: "cpuRequestMillis",
      label: "CPU request",
      op: ">",
      text: "CPU request: at most 4",
      value: 4000,
    },
  ],
  cedar:
    'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)\nwhen { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };',
  recognized: true,
  summary: ["CPU request: at least 250m", "CPU request: at most 4"],
};

const memoryRequestRule = {
  bounds: [
    {
      display: "16Gi",
      field: "memoryRequestBytes",
      label: "Memory request",
      op: ">",
      text: "Memory request: at most 16Gi",
      value: 17179869184,
    },
  ],
  cedar:
    'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)\nwhen { context.memoryRequestBytes > 17179869184 };',
  recognized: true,
  summary: ["Memory request: at most 16Gi"],
};

const gpuRoleRule = {
  bounds: [],
  cedar:
    'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)\nwhen { context.gpuCount > 1 }\nunless { principal.roles.contains("gpu-users") };',
  recognized: false,
  summary: [],
};

export const rejectedDenialText = [
  "Rejected by 1 policy:",
  "- sandbox-resource-baseline: CPU request 100m is below the minimum 250m.",
  "  Another rule in this policy also rejects the request.",
  "  Sandboxes on shared clusters must set CPU and memory within",
  "  platform limits. Ask #hypershell-help for an exception.",
  "Details: https://docs.example.com/hypershell/sandbox-resources",
].join("\n");

export const validEvaluation: PlatformPolicyEvaluation = {
  defaults: editorFleetDefaults,
  denialText: rejectedDenialText,
  diagnostics: [],
  forbidsDefaults: false,
  rejectingStatements: [0, 2],
  rendered: [cpuRequestRule, memoryRequestRule, gpuRoleRule],
  requestAllowed: false,
  valid: true,
};

export const allowedEvaluation: PlatformPolicyEvaluation = {
  ...validEvaluation,
  denialText: "",
  rejectingStatements: [],
  requestAllowed: true,
};

export const invalidEvaluation: PlatformPolicyEvaluation = {
  defaults: editorFleetDefaults,
  denialText: "",
  diagnostics: [
    {
      line: 7,
      message: "context.cpuRequest is not available",
      statement: 0,
      suggestion:
        "use context.cpuRequestMillis (Long, millicores); 4 CPU is 4000",
    },
    {
      line: 9,
      message: "a permit is not allowed; platform policies can only use forbid",
      statement: 1,
      suggestion: "replace permit with forbid and describe what to reject",
    },
  ],
  forbidsDefaults: false,
  rejectingStatements: [],
  rendered: [],
  requestAllowed: false,
  valid: false,
};

export const defaultsForbiddingEvaluation: PlatformPolicyEvaluation = {
  ...validEvaluation,
  diagnostics: [
    {
      line: 2,
      message:
        "the policy forbids the fleet default request (CPU request 500m)",
      statement: 0,
    },
  ],
  forbidsDefaults: true,
  valid: false,
};
