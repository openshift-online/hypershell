import type { Meta, StoryObj } from "@storybook/react-vite";

import { ProvisionGatewayOptionA } from "./provision-gateway-option-a";
import { ProvisionGatewayOptionB } from "./provision-gateway-option-b";
import { ProvisionGatewayOptionC } from "./provision-gateway-option-c";

const meta = {
  title: "Mockups/Gateways/Provision gateway options",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const SelectASet: Story = { render: () => <ProvisionGatewayOptionA /> };
export const Wizard: Story = { render: () => <ProvisionGatewayOptionB /> };
export const ChunkyButtons: Story = { render: () => <ProvisionGatewayOptionC /> };
