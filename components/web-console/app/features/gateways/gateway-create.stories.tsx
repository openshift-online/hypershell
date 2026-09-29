import type {
  GatewayOperations,
  GatewayRecord,
  GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewayCreatePage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

const placementAvailability = {
  awsPublic: true,
  awsVpn: true,
  ibmPublic: true,
  ibmVpn: false,
  localKind: true,
} as const;

const previewGateway: GatewayRecord = {
  clusterId: "preview-cluster",
  id: "preview-gateway",
  name: "preview-gateway",
  namespace: "openshell",
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

function GatewayCreatePreview() {
  return (
    <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
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
    await expect(canvas.queryByRole("radio", { name: "AWS" })).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "IBM Cloud" })).toBeNull();

    await userEvent.click(canvas.getByRole("radio", { name: "Public" }));
    await expect(canvas.getByRole("radio", { name: "AWS" })).toBeVisible();
    await expect(
      canvas.getByRole("radio", { name: "IBM Cloud" }),
    ).toBeVisible();
  },
};
