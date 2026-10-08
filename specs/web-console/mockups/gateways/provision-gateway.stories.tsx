import type { Meta, StoryObj } from "@storybook/react-vite";

import { ProvisionGatewayMockup } from "./provision-gateway";

const meta = {
  title: "Mockups/Gateways/Provision gateway",
  component: ProvisionGatewayMockup,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof ProvisionGatewayMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const ValidationErrors: Story = {
  args: { showValidationErrors: true },
};

export const LocalDevelopment: Story = {
  args: { showLocalDevelopment: true },
};
