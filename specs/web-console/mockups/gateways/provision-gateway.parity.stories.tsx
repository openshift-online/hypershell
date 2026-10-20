import type { Meta, StoryObj } from "@storybook/react-vite";

import { ProvisionGatewayPageSurface } from "./provision-gateway-page-surface";
import { MockupParityFrame } from "../shell/mockup-template";

const meta = {
  title: "Parity/Gateways/Provision gateway",
  component: ProvisionGatewayPageSurface,
  render: () => (
    <MockupParityFrame>
      <ProvisionGatewayPageSurface />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ProvisionGatewayPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const LocalDevelopmentOnly: Story = {
  render: () => (
    <MockupParityFrame>
      <ProvisionGatewayPageSurface localDevelopmentOnly />
    </MockupParityFrame>
  ),
};
