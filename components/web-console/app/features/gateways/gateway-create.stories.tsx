import type {
  GatewayOperations,
  GatewayPlacementAvailability,
  GatewayRecord,
  GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewayCreatePage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

const placementAvailability: GatewayPlacementAvailability = {
  awsPublic: true,
  awsVpn: true,
  ibmPublic: true,
  ibmVpn: false,
  localKind: true,
} as const;

const localDevelopmentAvailability: GatewayPlacementAvailability = {
  awsPublic: false,
  awsVpn: false,
  ibmPublic: false,
  ibmVpn: false,
  localKind: true,
} as const;

const previewGateway: GatewayRecord = {
  clusterId: "preview-cluster",
  id: "preview-gateway",
  name: "preview-gateway",
  namespace: "openshell",
  releaseId: "preview-release",
  status: "Provisioning",
};

const gatewayOperations = {
  getGatewayPlacementAvailability: () => Promise.resolve(placementAvailability),
  provisionGateway: () => Promise.resolve(previewGateway),
} as unknown as GatewayOperations;

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

function GatewayCreatePreview({
  availability = placementAvailability,
}: {
  availability?: GatewayPlacementAvailability;
}) {
  const operationsForStory = {
    ...gatewayOperations,
    getGatewayPlacementAvailability: () => Promise.resolve(availability),
  } as GatewayOperations;
  return (
    <GatewayUiProvider gateways={operationsForStory} navigation={navigation}>
      <GatewayCreatePage />
    </GatewayUiProvider>
  );
}

const meta = {
  title: "HyperShell/Gateways/Create gateway",
  component: GatewayCreatePage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <GatewayCreatePreview />,
} satisfies Meta<typeof GatewayCreatePage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ManagedPlacementsAvailable: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.queryByRole("radio", { name: "Use local-kind" }),
    ).toBeNull();
    await expect(canvas.getByRole("radio", { name: "Public" })).toBeVisible();
    await expect(canvas.getByRole("radio", { name: "VPN" })).toBeVisible();
    await expect(
      canvas.queryByRole("radio", { name: "Amazon Web Services" }),
    ).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "IBM Cloud" })).toBeNull();

    await userEvent.click(canvas.getByRole("radio", { name: "Public" }));
    await expect(
      canvas.getByRole("radio", { name: "Amazon Web Services" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("radio", { name: "IBM Cloud" }),
    ).toBeVisible();
  },
};

export const LocalDevelopmentOnly: Story = {
  render: () => (
    <GatewayCreatePreview availability={localDevelopmentAvailability} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("radio", { name: "Use local-kind" }),
    ).toBeVisible();
    await expect(canvas.queryByRole("radio", { name: "Public" })).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "VPN" })).toBeNull();
    await expect(
      canvas.queryByRole("radio", { name: "Amazon Web Services" }),
    ).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "IBM Cloud" })).toBeNull();
  },
};
