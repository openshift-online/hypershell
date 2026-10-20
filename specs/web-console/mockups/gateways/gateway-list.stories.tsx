import type { Meta, StoryObj } from "@storybook/react-vite";

import { GatewayListMockup } from "./gateway-list";

const meta = {
  title: "Mockups/Gateways/Gateway list",
  component: GatewayListMockup,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof GatewayListMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loaded: Story = {
  args: { state: "loaded" },
};

export const Empty: Story = {
  args: { state: "empty" },
};

export const Error: Story = {
  args: { state: "error" },
};
