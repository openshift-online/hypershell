import type { GatewayRecord } from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { DocsHomeSurface } from "./docs-home-page";
import { InstallHsctlSurface } from "./install-hsctl-page";
import { ResourceLimitsSurface } from "./resource-limits-page";
import {
  RunbookGatewayStepView,
  type RunbookGatewayStepViewProps,
} from "./runbook-gateway-step";
import { RunbookPageSurface } from "./runbook-page";

/*
 * Production docs page states (WEB-UDOC-13). No approved mockup exists yet, so
 * these stories carry the accessibility and visual checks without parity
 * (UIS-VER-10 applies once mockups are added under
 * specs/web-console/mockups/docs/).
 */

const meta = {
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  parameters: { layout: "fullscreen" },
  title: "Production/Docs",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const runningGateway: GatewayRecord = {
  clusterId: "cluster-1",
  externalDns: "gw-calm-river.apps.example.com",
  gatewayVersion: "v0.0.120",
  id: "6a1f0c2e",
  name: "calm-river-1a2b",
  namespace: "openshell",
  oidcAudience: "openshell-api",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://sso.example.com/realms/openshell",
  phase: "Running",
  status: "Ready",
};

const ibmPublic = {
  availabilitySelector: ["network/public", "provider/ibm"],
  id: "ibm-public" as const,
  placement: {
    excludedSelector: ["runtime/kata"],
    selector: ["network/public", "provider/ibm"],
  },
};

const createCommand = [
  "hsctl create gateway",
  "--name calm-river-1a2b",
  "--placement-selector network/public",
  "--placement-selector provider/ibm",
  "--placement-exclude-selector runtime/kata",
].join(" \\\n  ");

const idleStep: RunbookGatewayStepViewProps = {
  availability: {
    decision: { candidate: ibmPublic, kind: "available" },
    state: "ready",
  },
  choice: "__new__",
  command: createCommand,
  existingGateways: { gateways: [runningGateway], state: "ready" },
  hasTerminalEntry: false,
  name: "calm-river-1a2b",
  onChoiceChange: () => undefined,
  onNameChange: () => undefined,
  onOpenInTerminal: () => undefined,
  onRecheck: () => undefined,
  onRun: () => undefined,
  output: { failed: false, lines: [] },
  phase: "idle",
  runbook: "github-jira",
  session: "signed-in",
  sessionExpired: false,
  signInHref: "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira",
};

function runbookStory(
  step: Partial<RunbookGatewayStepViewProps>,
  gateway?: GatewayRecord,
): Story {
  const props = { ...idleStep, ...step };
  return {
    render: () => (
      <RunbookPageSurface
        gateway={gateway}
        runbook={props.runbook}
        step1={<RunbookGatewayStepView {...props} />}
      />
    ),
  };
}

export const HomeSignedOut: Story = { render: () => <DocsHomeSurface /> };

export const HomeSignedIn: Story = { render: () => <DocsHomeSurface /> };

export const RunbookSignedOut = runbookStory({ session: "signed-out" });

export const RunbookIdle = runbookStory({});

export const RunbookRunning = runbookStory({
  hasTerminalEntry: true,
  output: {
    failed: false,
    lines: [
      `$ ${createCommand}`,
      "Creating gateway calm-river-1a2b...",
      "Gateway calm-river-1a2b created (ID 6a1f0c2e).",
      "Status: Pending",
      "Status: Provisioning",
    ],
  },
  phase: "running",
  stage: "provisioning",
});

export const RunbookGatewayRunning = runbookStory(
  {
    choice: runningGateway.id,
    command: `hsctl get gateway ${runningGateway.id}`,
    gatewayName: runningGateway.name,
    hasTerminalEntry: true,
    output: {
      failed: false,
      lines: [
        `$ ${createCommand}`,
        "Creating gateway calm-river-1a2b...",
        "Gateway calm-river-1a2b created (ID 6a1f0c2e).",
        "Status: Pending",
        "Status: Provisioning",
        "Status: Running",
      ],
    },
    phase: "done",
    stage: "running",
  },
  runningGateway,
);

export const RunbookPlacementUnavailable = runbookStory({
  availability: {
    decision: { kind: "unavailable", reason: "no-eligible-cluster" },
    state: "ready",
  },
});

export const RunbookVpnPlacementUnavailable = runbookStory({
  availability: {
    decision: { kind: "unavailable", reason: "no-eligible-cluster" },
    state: "ready",
  },
  existingGateways: { gateways: [], state: "ready" },
  runbook: "gitlab-jira",
});

export const RunbookFailed = runbookStory({
  hasTerminalEntry: true,
  output: {
    failed: true,
    lines: [
      `$ ${createCommand}`,
      "Creating gateway calm-river-1a2b...",
      "Failed: a gateway with this name already exists. Choose another name.",
    ],
  },
  phase: "failed",
});

export const RunbookError = runbookStory({
  availability: { state: "error" },
  existingGateways: { state: "error" },
});

export const InstallWithoutDownloadLocation: Story = {
  render: () => (
    <InstallHsctlSurface
      onOpenTerminal={() => undefined}
      onSelectOs={() => undefined}
      selectedOs="linux"
    />
  ),
};

export const InstallWithDownloadLocation: Story = {
  render: () => (
    <InstallHsctlSurface
      downloadBaseUrl="https://github.com/openshift-online/hypershell/releases/download/release-bundle-20261124T203545000000Z-aa9866485a32d393"
      onOpenTerminal={() => undefined}
      onSelectOs={() => undefined}
      selectedOs="macos"
    />
  ),
};

export const RefusalWithContext: Story = {
  render: () => (
    <ResourceLimitsSurface
      context={{ gateway: "team-a", policy: "sandbox-cpu-limit" }}
    />
  ),
};

export const RefusalWithoutContext: Story = {
  render: () => <ResourceLimitsSurface context={{}} />,
};
