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

// "Show only my gateways" off: the switch is visible and unchecked, showing the
// full authorized collection (kim's and another owner's gateways).
export const Loaded: Story = {
  args: { state: "loaded", ownershipFilterOn: false },
};

// "Show only my gateways" on: the switch is checked and the list shows only the
// signed-in user's gateways. The switch stays visible so the filter can be
// turned off (WEB-UIEX-07).
export const OwnershipFilterOn: Story = {
  args: { state: "loaded", ownershipFilterOn: true },
};

export const Empty: Story = {
  args: { state: "empty" },
};

export const Error: Story = {
  args: { state: "error" },
};
