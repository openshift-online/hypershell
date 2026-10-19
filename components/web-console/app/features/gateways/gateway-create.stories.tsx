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
  localKind: true,
  placements: [
    { available: true, selector: ["network/public", "provider/aws"] },
    { available: true, selector: ["network/vpn", "provider/aws"] },
    { available: true, selector: ["network/public", "provider/ibm"] },
    {
      available: false,
      reason: "no-eligible-cluster",
      selector: ["network/vpn", "provider/ibm"],
    },
  ],
} as const;

const localDevelopmentAvailability: GatewayPlacementAvailability = {
  localKind: true,
  placements: placementAvailability.placements.map(({ selector }) => ({
    available: false,
    reason: "no-eligible-cluster",
    selector,
  })),
} as const;

const vpnUnavailableAvailability: GatewayPlacementAvailability = {
  localKind: false,
  placements: [
    {
      available: true,
      selector: ["network/public", "provider/aws"],
    },
    {
      available: false,
      reason: "no-eligible-cluster",
      selector: ["network/vpn", "provider/aws"],
    },
    {
      available: true,
      selector: ["network/public", "provider/ibm"],
    },
    {
      available: false,
      reason: "no-eligible-cluster",
      selector: ["network/vpn", "provider/ibm"],
    },
  ],
} as const;

const previewGateway: GatewayRecord = {
  clusterId: "preview-cluster",
  id: "preview-gateway",
  name: "preview-gateway",
  namespace: "openshell",
  status: "Provisioning",
};

const gatewayOperations = {
  getGatewayPlacementAvailability: (
    selectors: readonly (readonly string[])[],
  ) =>
    Promise.resolve({
      localKind: true,
      placements: selectors.map((selector) => ({
        available:
          !selector.includes("runtime/kata") &&
          selector.includes("network/public") &&
          selector.includes("provider/ibm"),
        selector: [...selector],
      })),
    }),
  getGatewayProvisioningVersions: () => Promise.resolve(["1.2.3"]),
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
    getGatewayPlacementAvailability:
      availability === placementAvailability
        ? (...args) => gatewayOperations.getGatewayPlacementAvailability(...args)
        : () => Promise.resolve(availability),
  } as GatewayOperations;
  return (
    <GatewayUiProvider gateways={operationsForStory} navigation={navigation}>
      <GatewayCreatePage />
    </GatewayUiProvider>
  );
}

const meta = {
  title: "Production/Gateways/Create gateway",
  component: GatewayCreatePage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <GatewayCreatePreview />,
} satisfies Meta<typeof GatewayCreatePage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.queryByRole("radio", { name: "Use local-kind" }),
    ).toBeNull();
    await expect(canvas.getByRole("radio", { name: "No" })).toBeVisible();
    await expect(canvas.getByRole("radio", { name: "Yes" })).toBeVisible();
    await expect(
      canvas.queryByRole("radio", { name: "Amazon Web Services" }),
    ).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "IBM Cloud" })).toBeNull();

    await userEvent.click(canvas.getByRole("radio", { name: "No" }));
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
    await expect(canvas.queryByRole("radio", { name: "No" })).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "Yes" })).toBeNull();
    await expect(
      canvas.queryByRole("radio", { name: "Amazon Web Services" }),
    ).toBeNull();
    await expect(canvas.queryByRole("radio", { name: "IBM Cloud" })).toBeNull();
  },
};

export const VpnUnavailable: Story = {
  render: () => (
    <GatewayCreatePreview availability={vpnUnavailableAvailability} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const publicNetwork = canvas.getByRole("radio", { name: "No" });
    const vpnNetwork = canvas.getByRole("radio", { name: "Yes" });

    await expect(publicNetwork).toBeEnabled();
    await expect(vpnNetwork).toBeDisabled();
    await expect(
      canvas.getByText(
        "VPN is unavailable: no eligible managed cluster is connected.",
      ),
    ).toBeVisible();
  },
};
