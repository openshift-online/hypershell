import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { previewGateway, previewGateways } from "./gateway-connections";
import { GatewayDetails, GatewayDirectory } from "./gateway-directory";

const meta: Meta<typeof GatewayDirectory> = {
  title: "OpenShell/Gateway directory",
  component: GatewayDirectory,
  decorators: [
    (StoryComponent) => (
      <MemoryRouter>
        <StoryComponent />
      </MemoryRouter>
    ),
  ],
  args: {
    gateways: previewGateways,
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Empty: Story = {
  args: {
    gateways: [],
  },
};

export const LongContent: Story = {
  args: {
    gateways: [
      {
        ...previewGateway,
        endpoint:
          "https://a-very-long-gateway-name-for-validating-responsive-content.example.openshell.invalid:443",
        id: "a-very-long-gateway-name-for-validating-responsive-content",
        name: "a-very-long-gateway-name-for-validating-responsive-content",
      },
    ],
  },
};

export const Details: Story = {
  render: () => <GatewayDetails gateway={previewGateway} />,
};
