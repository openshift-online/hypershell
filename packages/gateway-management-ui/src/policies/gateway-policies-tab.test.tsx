import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import type { GatewayOperations } from "../application/gateway-types";
import type {
  EffectivePolicy,
  GatewayEffectivePolicies,
} from "../application/policy-types";
import {
  GatewayUiProvider,
  type GatewayUiCurrentUser,
} from "../gateway-ui-provider";
import {
  buildFittingSandboxCommand,
  GatewayPoliciesTab,
  sandboxResourcesFromDefaults,
} from "./gateway-policies-tab";

const baseline: EffectivePolicy = {
  builtIn: false,
  cedar:
    'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)\nwhen { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };',
  condition: "applied",
  documentationUrl: "https://docs.example.com/hypershell/sandbox-resources",
  effectiveMode: "audit",
  effectiveModeReason: "audit",
  enforcementMode: "audit",
  id: "policy-baseline",
  name: "sandbox-resource-baseline",
  rendered: [
    {
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
      cedar: "forbid ... cpuRequestMillis",
      recognized: true,
      summary: ["CPU request: at least 250m", "CPU request: at most 4"],
    },
  ],
  revision: "r7",
  updatedAt: "2026-11-18T10:00:00Z",
  userMessage: "Sandboxes on shared clusters must stay within platform limits.",
};

const gpuCedar =
  'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)\nwhen { context.gpuCount > 1 }\nunless { principal.roles.contains("gpu-users") };';

const gpu: EffectivePolicy = {
  builtIn: false,
  cedar: gpuCedar,
  condition: "applied",
  effectiveMode: "enforce",
  effectiveModeReason: "policy-enforced",
  enforcementMode: "enforce",
  id: "policy-gpu",
  name: "gpu-sandbox-cap",
  rendered: [{ bounds: [], cedar: gpuCedar, recognized: false, summary: [] }],
  userMessage: "Sandboxes can use 1 GPU unless you are in gpu-users.",
};

function effectivePolicies(
  overrides: Partial<GatewayEffectivePolicies> = {},
): GatewayEffectivePolicies {
  return {
    defaults: {
      cpuLimit: "2",
      cpuRequest: "500m",
      memoryLimit: "4Gi",
      memoryRequest: "1Gi",
    },
    findings: [
      {
        gatewayId: "payments-gateway",
        id: "finding-1",
        occurredAt: "2026-11-20T09:12:00Z",
        operation: "CreateSandbox",
        outcome: "would-reject",
        policyNames: ["sandbox-resource-baseline"],
        principalSubject: "lin.chen",
        requestSummary: { cpuRequestMillis: 100 },
      },
      {
        gatewayId: "payments-gateway",
        id: "finding-2",
        occurredAt: "2026-11-20T08:47:00Z",
        operation: "CreateSandbox",
        outcome: "would-add-defaults",
        policyNames: ["sandbox-resource-baseline"],
        principalSubject: "ada.lovelace",
        requestSummary: { memoryLimitBytes: 4294967296 },
      },
    ],
    findingsLast24h: { wouldAddDefaults: 1, wouldReject: 1 },
    gatewayId: "payments-gateway",
    globalMode: "audit",
    policies: [baseline, gpu],
    schemaVersion: "v1",
    ...overrides,
  };
}

function renderTab({
  currentUser = { subject: "ada.lovelace" },
  initialPolicyName,
  read = vi.fn(() => Promise.resolve(effectivePolicies())),
}: {
  currentUser?: GatewayUiCurrentUser;
  initialPolicyName?: string;
  read?: GatewayOperations["getGatewayEffectivePolicies"] | null;
} = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const gateways = {
    ...(read ? { getGatewayEffectivePolicies: read } : {}),
  } as unknown as GatewayOperations;
  render(
    <IntlProvider locale="en">
      <QueryClientProvider client={client}>
        <GatewayUiProvider
          currentUser={currentUser}
          gateways={gateways}
          navigation={{
            collectionHref: "/gateways",
            createHref: "/gateways/new",
            detailHref: (id) => `/gateways/${id}`,
            navigate: () => undefined,
          }}
        >
          <GatewayPoliciesTab
            gatewayId="payments-gateway"
            gatewayName="payments-gateway"
            initialPolicyName={initialPolicyName}
            isActive
          />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
  return { read };
}

describe("GatewayPoliciesTab", () => {
  // Verifies: WEB-GLIS-26
  it("shows fleet defaults and attributed bounds without any target", async () => {
    renderTab();

    const limits = await screen.findByRole("grid", {
      name: "Sandbox limits on this gateway",
    });
    const cpuRow = within(limits).getByRole("row", { name: /^CPU request/u });
    expect(within(cpuRow).getByText("250m")).toBeTruthy();
    expect(within(cpuRow).getByText("500m")).toBeTruthy();
    expect(within(cpuRow).getByText("4")).toBeTruthy();
    expect(
      within(cpuRow).getAllByText("sandbox-resource-baseline"),
    ).toHaveLength(2);
    expect(within(cpuRow).getAllByText("Audit only")).toHaveLength(2);
    expect(within(cpuRow).getByText("Fleet default")).toBeTruthy();
    expect(
      within(limits).getByRole("row", { name: /^Memory limit/u }),
    ).toBeTruthy();
    expect(screen.queryByText(/All gateways/u)).toBeNull();
    expect(screen.queryByText(/except/u)).toBeNull();
  });

  // Verifies: WEB-GLIS-26
  it("shows additional rules with their message and Cedar source", async () => {
    renderTab();

    expect(await screen.findByText("1 additional rule applies")).toBeTruthy();
    expect(
      screen.getAllByText(
        /Sandboxes can use 1 GPU unless you are in gpu-users\./u,
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Shown as Cedar").length).toBeGreaterThan(0);
    expect(
      screen.getByText(
        (_content, element) =>
          element?.textContent === gpuCedar && element.tagName === "CODE",
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-GLIS-26
  it("offers a copyable create command built from the fleet defaults", async () => {
    renderTab();

    expect(
      await screen.findByDisplayValue(
        "openshell sandbox create -g payments-gateway --name my-sandbox --cpu 2 --memory 4Gi",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "OpenShell adds the fleet default requests: 500m CPU and 1Gi memory.",
      ),
    ).toBeTruthy();
    expect(
      buildFittingSandboxCommand("gw", sandboxResourcesFromDefaults({})),
    ).toBe("openshell sandbox create -g gw --name my-sandbox");
  });

  // Verifies: WEB-GLIS-26
  it("lists policies with mode, reason, and audit-only rejections", async () => {
    renderTab();

    const table = await screen.findByRole("grid", {
      name: "Platform policies on this gateway",
    });
    const baselineRow = within(table).getByRole("row", {
      name: /sandbox-resource-baseline/u,
    });
    expect(within(baselineRow).getByText("Audit only")).toBeTruthy();
    expect(within(baselineRow).getByText("Policy setting")).toBeTruthy();
    expect(within(baselineRow).getByText("CPU request")).toBeTruthy();
    expect(within(baselineRow).getByText("1")).toBeTruthy();
    const gpuRow = within(table).getByRole("row", { name: /gpu-sandbox-cap/u });
    expect(within(gpuRow).getByText("Enforced")).toBeTruthy();
    expect(within(gpuRow).getByText("Additional rule")).toBeTruthy();
    expect(within(gpuRow).getByText("-")).toBeTruthy();
    expect(screen.getByText("Platform policies (2)")).toBeTruthy();
  });

  // Verifies: WEB-GLIS-26
  it("filters findings by scope and outcome", async () => {
    const user = userEvent.setup();
    renderTab();

    const findings = await screen.findByRole("grid", {
      name: "Audit findings",
    });
    expect(within(findings).getByText("lin.chen")).toBeTruthy();
    expect(within(findings).getByText("ada.lovelace")).toBeTruthy();
    expect(screen.getByText("1 operation would be rejected")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "My operations" }));
    const mine = screen.getByRole("grid", { name: "Audit findings" });
    expect(within(mine).queryByText("lin.chen")).toBeNull();
    expect(within(mine).getByText("ada.lovelace")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Would reject" }));
    expect(screen.getByText("No findings")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "All operations" }));
    const rejected = screen.getByRole("grid", { name: "Audit findings" });
    expect(within(rejected).getByText("lin.chen")).toBeTruthy();
    expect(
      within(rejected).getByText(
        "CPU request 100m (sandbox-resource-baseline)",
      ),
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Would add defaults" }),
    );
    expect(
      within(screen.getByRole("grid", { name: "Audit findings" })).getByText(
        "Memory limit 4Gi (sandbox-resource-baseline)",
      ),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "All outcomes" }));
    expect(
      within(screen.getByRole("grid", { name: "Audit findings" })).getAllByRole(
        "row",
      ),
    ).toHaveLength(3);
  });

  // Verifies: WEB-GLIS-26
  it("opens the policy drawer with message, rules, source, and mode reason", async () => {
    const user = userEvent.setup();
    renderTab({ initialPolicyName: "gpu-sandbox-cap" });

    expect(await screen.findByText("Why it is enforced")).toBeTruthy();
    expect(screen.getByText("Platform policy")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Show Cedar source" }));
    expect(
      screen.getByRole("button", { name: "Hide Cedar source" }),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "Close policy details" }),
    );
    await waitFor(() => {
      expect(screen.queryByText("Why it is enforced")).toBeNull();
    });

    await user.click(
      within(
        screen.getByRole("grid", { name: "Platform policies on this gateway" }),
      ).getByRole("button", { name: "sandbox-resource-baseline" }),
    );
    expect(await screen.findByText("Why it is audit only")).toBeTruthy();
    expect(
      screen.getByRole("grid", { name: "Rules in sandbox-resource-baseline" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Policy documentation" }),
    ).toBeTruthy();
    expect(screen.getByText("Last updated")).toBeTruthy();
  });

  // Verifies: WEB-GLIS-26
  it("explains enforced platforms and the empty state", async () => {
    renderTab({
      read: vi.fn(() =>
        Promise.resolve(
          effectivePolicies({
            globalMode: "enforce",
            policies: [
              {
                ...baseline,
                effectiveMode: "enforce",
                effectiveModeReason: "platform-enforced",
              },
            ],
          }),
        ),
      ),
    });

    expect(await screen.findByText("No audit-only policies")).toBeTruthy();
    expect(screen.getByText("By platform enforcement")).toBeTruthy();
    expect(screen.queryByText(/additional rule/u)).toBeNull();
  });

  it("renders the no-policies state", async () => {
    renderTab({
      read: vi.fn(() =>
        Promise.resolve(effectivePolicies({ defaults: {}, policies: [] })),
      ),
    });
    expect(await screen.findByText("No platform policies")).toBeTruthy();
    expect(
      screen.getByText("OpenShell adds the fleet default requests."),
    ).toBeTruthy();
  });

  it("retries after a load failure", async () => {
    const user = userEvent.setup();
    const read = vi
      .fn()
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValue(effectivePolicies());
    renderTab({ read });
    expect(
      await screen.findByText("Policies could not be loaded"),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Platform policies (2)")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Refresh policies" }));
    await waitFor(() => {
      expect(read).toHaveBeenCalledTimes(3);
    });
  });

  it("reports an unavailable policies service", () => {
    renderTab({ read: null });
    expect(screen.getByText("Policies are unavailable")).toBeTruthy();
  });
});
