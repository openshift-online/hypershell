import type {
  EffectivePolicy,
  GatewayEffectivePolicies,
  GatewayOperations,
  GatewayRecord,
  GatewayUiNavigation,
  PolicyMode,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewayPage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { IntlProvider } from "react-intl";

const gateway: GatewayRecord = {
  activeSandboxCount: 3,
  canDelete: true,
  canEdit: true,
  clusterId: "cluster-ibm-public",
  clusterType: "IBM Public",
  consoleUrl: "https://payments-gateway.example.com/console",
  createdAt: "2026-10-29T09:41:00-05:00",
  createdBy: "kim-developer",
  externalDns: "https://payments-gateway.example.com",
  gatewayVersion: "1.8.0",
  id: "payments-gateway",
  name: "payments-gateway",
  namespace: "openshell-payments",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://sso.example.com/realms/openshell",
  phase: "Running",
  provisioningConditions: [
    {
      conditionStatus: "Complete",
      message: "",
      type: "EnvironmentReady",
    },
    {
      conditionStatus: "Complete",
      message: "",
      type: "GatewayHealthy",
    },
  ],
  status: "Healthy",
};

const gatewayOperations = {
  getGatewayPlacement: () =>
    Promise.resolve({
      id: "cluster-ibm-public",
      name: "Cluster East",
      status: "Ready",
    }),
  listGatewayAccess: () =>
    Promise.resolve({
      capabilities: {
        canManageAccess: true,
        canManageOwners: true,
      },
      items: [],
      page: 1,
      size: 20,
      total: 0,
    }),
  listOpenShellGatewayServiceAccounts: () =>
    Promise.resolve({
      capabilities: {
        allowedRoles: ["openshell-admin", "openshell-user"],
        canCreate: true,
        canManageAll: true,
        expirationPolicy: {
          defaultSeconds: 7776000,
          maximumSeconds: 31536000,
          minimumSeconds: 3600,
        },
      },
      items: [],
      page: 1,
      size: 20,
      total: 0,
    }),
} as unknown as GatewayOperations;

const gpuCedar = `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.gpuCount > 1 }
unless { principal.roles.contains("gpu-users") };`;

const baselineCedar = `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuLimitMillis > 8000 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.memoryRequestBytes < 536870912 || context.memoryRequestBytes > 17179869184 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.memoryLimitBytes > 34359738368 };`;

function bound(
  field: string,
  label: string,
  op: string,
  value: number,
  display: string,
) {
  const word = op === "<" ? "at least" : "at most";
  return {
    display,
    field,
    label,
    op,
    text: `${label}: ${word} ${display}`,
    value,
  };
}

function recognizedRule(bounds: ReturnType<typeof bound>[]) {
  return {
    bounds,
    cedar: "",
    recognized: true,
    summary: bounds.map(({ text }) => text),
  };
}

function effectivePolicies(globalMode: PolicyMode): GatewayEffectivePolicies {
  const platformEnforced = globalMode === "enforce";
  const baseline: EffectivePolicy = {
    builtIn: false,
    cedar: baselineCedar,
    condition: "applied",
    documentationUrl: "https://docs.example.com/hypershell/sandbox-resources",
    effectiveMode: globalMode,
    effectiveModeReason: platformEnforced ? "platform-enforced" : "audit",
    enforcementMode: "audit",
    id: "policy-sandbox-resource-baseline",
    name: "sandbox-resource-baseline",
    rendered: [
      recognizedRule([
        bound("cpuRequestMillis", "CPU request", "<", 250, "250m"),
        bound("cpuRequestMillis", "CPU request", ">", 4000, "4"),
      ]),
      recognizedRule([bound("cpuLimitMillis", "CPU limit", ">", 8000, "8")]),
      recognizedRule([
        bound("memoryRequestBytes", "Memory request", "<", 536870912, "512Mi"),
        bound("memoryRequestBytes", "Memory request", ">", 17179869184, "16Gi"),
      ]),
      recognizedRule([
        bound("memoryLimitBytes", "Memory limit", ">", 34359738368, "32Gi"),
      ]),
    ],
    revision: "r7",
    updatedAt: "2026-11-18T15:00:00Z",
    userMessage:
      "Sandboxes on shared clusters must set CPU and memory within platform limits. Ask #hypershell-help for an exception.",
  };
  const gpu: EffectivePolicy = {
    builtIn: false,
    cedar: gpuCedar,
    condition: "applied",
    effectiveMode: "enforce",
    effectiveModeReason: platformEnforced
      ? "platform-enforced"
      : "policy-enforced",
    enforcementMode: "enforce",
    id: "policy-gpu-sandbox-cap",
    name: "gpu-sandbox-cap",
    rendered: [{ bounds: [], cedar: gpuCedar, recognized: false, summary: [] }],
    revision: "r3",
    updatedAt: "2026-11-02T15:00:00Z",
    userMessage:
      "Sandboxes can use 1 GPU unless you are in gpu-users. Use an ML training gateway for larger jobs.",
  };
  return {
    defaults: {
      cpuLimit: "2",
      cpuRequest: "500m",
      memoryLimit: "4Gi",
      memoryRequest: "1Gi",
    },
    findings: [
      {
        gatewayId: gateway.id,
        id: "finding-1",
        occurredAt: "2026-11-20T15:12:00Z",
        operation: "CreateSandbox",
        outcome: "would-reject",
        policyNames: ["sandbox-resource-baseline"],
        principalSubject: "lin.chen",
        requestSummary: { cpuRequestMillis: 100 },
      },
      {
        gatewayId: gateway.id,
        id: "finding-2",
        occurredAt: "2026-11-20T14:47:00Z",
        operation: "CreateSandbox",
        outcome: "would-add-defaults",
        policyNames: ["sandbox-resource-baseline"],
        principalSubject: "ada.lovelace",
        requestSummary: { cpuLimitMillis: 2000, memoryLimitBytes: 4294967296 },
      },
      {
        gatewayId: gateway.id,
        id: "finding-3",
        occurredAt: "2026-11-19T22:03:00Z",
        operation: "CreateSandbox",
        outcome: "would-reject",
        policyNames: ["sandbox-resource-baseline"],
        principalSubject: "sam.okafor",
        requestSummary: { memoryRequestBytes: 268435456 },
      },
    ],
    findingsLast24h: { wouldAddDefaults: 1, wouldReject: 2 },
    gatewayId: gateway.id,
    globalMode,
    policies: [baseline, gpu],
    schemaVersion: "v1",
  };
}

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

const currentUser = { subject: "ada.lovelace", username: "ada.lovelace" };

function GatewayDetailProductionPreview({
  activeTab,
  globalMode = "audit",
  initialPolicyName,
}: {
  activeTab:
    "details" | "connection" | "user-access" | "service-accounts" | "policies";
  globalMode?: PolicyMode;
  initialPolicyName?: string;
}) {
  // A fresh client per story keeps one story's policy data out of another.
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { refetchOnWindowFocus: false, retry: false },
        },
      }),
  );
  const operations = useMemo(
    () => ({
      ...gatewayOperations,
      getGatewayEffectivePolicies: () =>
        Promise.resolve(effectivePolicies(globalMode)),
    }),
    [globalMode],
  );
  return (
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <GatewayUiProvider
          currentUser={currentUser}
          gateways={operations}
          navigation={navigation}
        >
          <GatewayPage
            activeTab={activeTab}
            gateway={gateway}
            gatewayId={gateway.id}
            initialPolicyName={initialPolicyName}
          />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>
  );
}

const meta = {
  title: "Production/Gateways/Gateway detail",
  component: GatewayDetailProductionPreview,
  parameters: {
    layout: "fullscreen",
  },
  render: (args) => <GatewayDetailProductionPreview {...args} />,
} satisfies Meta<typeof GatewayDetailProductionPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Details: Story = {
  args: { activeTab: "details" },
};

export const Connection: Story = {
  args: { activeTab: "connection" },
};

export const UserAccess: Story = {
  args: { activeTab: "user-access" },
};

export const ServiceAccounts: Story = {
  args: { activeTab: "service-accounts" },
};

export const PoliciesAuditOnly: Story = {
  args: { activeTab: "policies", globalMode: "audit" },
};

export const PoliciesEnforced: Story = {
  args: { activeTab: "policies", globalMode: "enforce" },
};

export const PolicyDetailsDrawer: Story = {
  args: {
    activeTab: "policies",
    globalMode: "audit",
    initialPolicyName: "gpu-sandbox-cap",
  },
};
