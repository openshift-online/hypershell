import type {
  GatewayConnection,
  GatewayListRequest,
  GatewayOperations,
  GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewaysPage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

const gateways: GatewayConnection[] = [
  {
    activeSandboxCount: 3,
    canDelete: true,
    canEdit: true,
    clusterId: "cluster-aws-us-east-1",
    clusterName: "aws-us-east-1-production",
    consoleUrl: "https://payments-gateway.example.com/console",
    createdAt: "2026-10-01T09:30:00Z",
    createdBy: "kim-developer",
    endpoint: "payments-gateway.example.com",
    gatewayVersion: "1.8.0",
    id: "gateway-payments",
    name: "payments-gateway",
    oidcAudience: "openshell-payments",
    oidcClientId: "payments-client",
    oidcIssuer: "https://sso.example.com/realms/openshell",
    phase: "Running",
    status: "Ready",
  },
  {
    activeSandboxCount: 1,
    canDelete: true,
    canEdit: true,
    clusterId: "cluster-ibm-us-south",
    clusterName: "ibm-us-south-production",
    createdAt: "2026-10-12T11:15:00Z",
    createdBy: "alex-operator",
    gatewayVersion: "1.7.2",
    id: "gateway-research",
    name: "research-gateway",
    phase: "Running",
    status: "Unavailable",
  },
  {
    canDelete: true,
    canEdit: true,
    clusterId: "cluster-aws-eu-west-1",
    clusterName: "aws-eu-west-1-production",
    createdAt: "2026-10-29T13:00:00Z",
    createdBy: "jordan-automation",
    id: "gateway-staging",
    name: "staging-gateway",
    phase: "Provisioning",
    status: "Provisioning",
  },
];

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

const gatewayOperations = {
  getGatewayPlacements: () => Promise.resolve([]),
  listGateways: () =>
    Promise.resolve({ items: [], page: 1, size: 20, total: 0 }),
  removeGateway: () => Promise.resolve(),
  renameGateway: (_gatewayId: string, name: string) =>
    Promise.resolve({ id: _gatewayId, name }),
} as unknown as GatewayOperations;

function GatewayListPreview({
  collectionState,
  deletedGatewayName,
  gateways: gatewaysForStory = gateways,
  listGateways,
}: {
  collectionState?: GatewayListRequest;
  deletedGatewayName?: string;
  gateways?: GatewayConnection[];
  listGateways?: GatewayOperations["listGateways"];
}) {
  return (
    <GatewayUiProvider
      gateways={{ ...gatewayOperations, listGateways }}
      navigation={navigation}
    >
      <GatewaysPage
        collectionState={collectionState}
        deletedGatewayName={deletedGatewayName}
        gateways={gatewaysForStory}
      />
    </GatewayUiProvider>
  );
}

const meta = {
  title: "Production/Gateways/Gateway list",
  component: GatewaysPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <GatewayListPreview />,
} satisfies Meta<typeof GatewaysPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "Gateways" }),
    ).toBeVisible();
    await expect(canvas.getByText("payments-gateway")).toBeVisible();
    await expect(canvas.getByText("research-gateway")).toBeVisible();
    await expect(canvas.getByText("staging-gateway")).toBeVisible();
  },
};

export const Filtered: Story = {
  render: () => (
    <GatewayListPreview
      collectionState={{
        page: 1,
        search: "research",
        size: 20,
        sortDirection: "asc",
        sortField: "name",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("research-gateway")).toBeVisible();
    await expect(canvas.queryByText("payments-gateway")).toBeNull();
  },
};

export const Empty: Story = {
  render: () => <GatewayListPreview gateways={[]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No gateways")).toBeVisible();
  },
};

export const LoadError: Story = {
  render: () => (
    <GatewayListPreview
      gateways={undefined}
      listGateways={() => Promise.reject(new Error("Mock gateway API error"))}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Unable to load gateways")).toBeVisible();
  },
};

export const DeletedGateway: Story = {
  render: () => <GatewayListPreview deletedGatewayName="retired-gateway" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("retired-gateway deleted")).toBeVisible();
  },
};

export const RenameGatewayModal: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Actions for payments-gateway",
      }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Rename gateway" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Rename gateway" }),
    ).toBeVisible();
  },
};

export const DeleteGatewayModal: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Actions for payments-gateway",
      }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Delete gateway" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Delete payments-gateway?" }),
    ).toBeVisible();
    await expect(canvas.getByText("3 active sandboxes")).toBeVisible();
  },
};
