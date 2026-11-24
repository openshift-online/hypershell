import type {
  PolicyFinding,
  PolicyMode,
} from "@openshift-online/hypershell-gateway-management-ui";

import type {
  PlatformPolicy,
  PlatformPolicyGatewayStatus,
  PlatformPolicySettings,
} from "../../../application/platform-policies/platform-policy-types";

/*
 * Deterministic Storybook and test fixtures mirroring the approved mockup data
 * (specs/web-console/mockups/common/policy-rules.tsx). Never imported by
 * production modules.
 */

const action = 'action == OpenShell::Action::"CreateSandbox"';

export const baselineCedar = [
  `forbid (principal, ${action}, resource)`,
  "when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };",
  "",
  `forbid (principal, ${action}, resource)`,
  "when { context.cpuLimitMillis > 8000 };",
  "",
  `forbid (principal, ${action}, resource)`,
  "when { context.memoryRequestBytes < 536870912 || context.memoryRequestBytes > 17179869184 };",
  "",
  `forbid (principal, ${action}, resource)`,
  "when { context.memoryLimitBytes > 34359738368 };",
].join("\n");

export const gpuCedar = [
  `forbid (principal, ${action}, resource)`,
  "when { context.gpuCount > 1 }",
  'unless { principal.roles.contains("gpu-users") };',
].join("\n");

function bound(
  field: string,
  label: string,
  op: string,
  value: number,
  display: string,
) {
  const verb = op === ">" ? "at most" : "at least";
  return {
    display,
    field,
    label,
    op,
    text: `${label}: ${verb} ${display}`,
    value,
  };
}

function gateway(
  gatewayId: string,
  values: Partial<PlatformPolicyGatewayStatus>,
): PlatformPolicyGatewayStatus {
  return {
    condition: "applied",
    effectiveMode: "audit",
    gatewayId,
    gatewayName: gatewayId,
    wouldReject24h: 0,
    ...values,
  };
}

const extraGateways = [
  "analytics-gateway",
  "billing-gateway",
  "checkout-gateway",
  "data-platform-gateway",
  "design-gateway",
  "finance-gateway",
  "identity-gateway",
  "marketing-gateway",
  "mobile-gateway",
  "support-gateway",
];

export function baselinePolicy(
  globalMode: PolicyMode = "audit",
): PlatformPolicy {
  const effectiveMode = globalMode;
  return {
    builtIn: false,
    cedar: baselineCedar,
    description: "Shared-cluster capacity plan for sandboxes.",
    documentationUrl: "https://docs.example.com/hypershell/sandbox-resources",
    effectiveMode,
    enforcementMode: "audit",
    findingsLast24h: { wouldAddDefaults: 41, wouldReject: 12 },
    gateways: [
      gateway("payments-gateway", {
        effectiveMode,
        revision: "r7",
        wouldReject24h: 2,
      }),
      gateway("research-gateway", { effectiveMode, revision: "r7" }),
      gateway("ml-training-gateway", {
        condition: "pending",
        effectiveMode,
        message: "Waiting for the gateway to confirm revision r7.",
        revision: "r6",
        wouldReject24h: 10,
      }),
      gateway("edge-lab-gateway", {
        condition: "unsupported",
        effectiveMode,
        message: "The VM compute driver ignores per-sandbox CPU and memory.",
      }),
    ],
    id: "policy-baseline",
    name: "sandbox-resource-baseline",
    rendered: [
      {
        bounds: [
          bound("cpuRequestMillis", "CPU request", "<", 250, "250m"),
          bound("cpuRequestMillis", "CPU request", ">", 4000, "4"),
        ],
        cedar: baselineCedar.split("\n\n")[0] ?? "",
        recognized: true,
        summary: ["CPU request: at least 250m", "CPU request: at most 4"],
      },
      {
        bounds: [bound("cpuLimitMillis", "CPU limit", ">", 8000, "8")],
        cedar: baselineCedar.split("\n\n")[1] ?? "",
        recognized: true,
        summary: ["CPU limit: at most 8"],
      },
      {
        bounds: [
          bound(
            "memoryRequestBytes",
            "Memory request",
            "<",
            536870912,
            "512Mi",
          ),
          bound(
            "memoryRequestBytes",
            "Memory request",
            ">",
            17179869184,
            "16Gi",
          ),
        ],
        cedar: baselineCedar.split("\n\n")[2] ?? "",
        recognized: true,
        summary: [
          "Memory request: at least 512Mi",
          "Memory request: at most 16Gi",
        ],
      },
      {
        bounds: [
          bound("memoryLimitBytes", "Memory limit", ">", 34359738368, "32Gi"),
        ],
        cedar: baselineCedar.split("\n\n")[3] ?? "",
        recognized: true,
        summary: ["Memory limit: at most 32Gi"],
      },
    ],
    rollout: { applied: 2, pending: 1, unsupported: 1 },
    target: { all: true, exclude: [], gatewayIds: [] },
    updatedAt: "2026-11-18T15:00:00Z",
    userMessage:
      "Sandboxes on shared clusters must set CPU and memory within platform limits. Ask #hypershell-help for an exception.",
  };
}

export function gpuPolicy(): PlatformPolicy {
  return {
    builtIn: false,
    cedar: gpuCedar,
    effectiveMode: "enforce",
    enforcementMode: "enforce",
    findingsLast24h: { wouldAddDefaults: 0, wouldReject: 0 },
    gateways: ["payments-gateway", "research-gateway", ...extraGateways].map(
      (gatewayId) =>
        gateway(gatewayId, { effectiveMode: "enforce", revision: "r3" }),
    ),
    id: "policy-gpu",
    name: "gpu-sandbox-cap",
    rendered: [{ bounds: [], cedar: gpuCedar, recognized: false, summary: [] }],
    rollout: { applied: 12, pending: 0, unsupported: 0 },
    target: {
      all: true,
      exclude: ["ml-training-gateway", "edge-lab-gateway"],
      gatewayIds: [],
    },
    updatedAt: "2026-11-02T10:30:00Z",
    userMessage:
      "Sandboxes can use 1 GPU unless you are in gpu-users. Use an ML training gateway for larger jobs.",
  };
}

export function platformPolicies(
  globalMode: PolicyMode = "audit",
): readonly PlatformPolicy[] {
  return [baselinePolicy(globalMode), gpuPolicy()];
}

export function platformPolicySettings(
  mode: PolicyMode = "audit",
): PlatformPolicySettings {
  return {
    defaults: {
      cpuLimit: "2",
      cpuRequest: "500m",
      memoryLimit: "4Gi",
      memoryRequest: "1Gi",
    },
    mode,
    schemaVersion: "v1",
    updatedAt: "2026-11-10T09:00:00Z",
    updatedBy: "grace.hopper",
  };
}

export const baselineFindings: readonly PolicyFinding[] = [
  {
    gatewayId: "payments-gateway",
    id: "finding-1",
    occurredAt: "2026-11-20T15:12:00Z",
    operation: "CreateSandbox",
    outcome: "would-reject",
    policyNames: ["sandbox-resource-baseline"],
    principalSubject: "lin.chen",
    requestSummary: { cpuRequestMillis: 100 },
  },
  {
    gatewayId: "payments-gateway",
    id: "finding-2",
    occurredAt: "2026-11-20T14:47:00Z",
    operation: "CreateSandbox",
    outcome: "would-add-defaults",
    policyNames: ["sandbox-resource-baseline"],
    principalSubject: "ada.lovelace",
    requestSummary: { cpuLimitMillis: 2000, memoryLimitBytes: 4294967296 },
  },
  {
    gatewayId: "payments-gateway",
    id: "finding-3",
    occurredAt: "2026-11-19T22:03:00Z",
    operation: "CreateSandbox",
    outcome: "would-reject",
    policyNames: ["sandbox-resource-baseline"],
    principalSubject: "sam.okafor",
    requestSummary: { memoryRequestBytes: 268435456 },
  },
];
