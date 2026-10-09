import type { Meta, StoryObj } from "@storybook/react-vite";

import { ProvisionGatewayParityMockup } from "./provision-gateway";

const meta = {
  title: "Mockups/Gateways/Provision gateway",
  component: ProvisionGatewayParityMockup,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof ProvisionGatewayParityMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const ValidationErrors: Story = {
  args: { showValidationErrors: true },
};
